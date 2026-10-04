// client_test.go contains unit tests for the gitlab package.
// Tests verify [NewClient] creation, [Client.Ping] connectivity checks,
// and [Client.GL] accessor using httptest to mock the GitLab Version API.
package gitlab

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// Test constants used across client tests.
const (
	testValidToken  = "valid-token"
	fmtNewClientErr = "NewClient() unexpected error: %v"
)

// newTestConfig creates a [config.Config] with the given base URL and token
// for use in tests. TLS verification is disabled by default.
func newTestConfig(baseURL, token string) *config.Config {
	return &config.Config{
		GitLabURL:     baseURL,
		GitLabToken:   token,
		SkipTLSVerify: false,
	}
}

// TestNewClient_ValidConfig verifies that [NewClient] creates a non-nil client
// when given a valid configuration pointing to a running test server.
func TestNewClient_ValidConfig(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil client")
	}
}

// TestPing_Success verifies that [Client.Ping] succeeds when the GitLab
// Version API returns HTTP 200 OK and returns the version string.
func TestPing_Success(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	version, err := client.Ping(context.Background())
	if err != nil {
		t.Errorf("Ping() unexpected error: %v", err)
	}
	if version != "16.0.0" {
		t.Errorf("Ping() version = %q, want %q", version, "16.0.0")
	}
}

// TestPing_Unauthorized verifies that [Client.Ping] returns an error when
// the GitLab Version API responds with HTTP 401 Unauthorized.
func TestPing_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, "bad-token"))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	if _, err = client.Ping(context.Background()); err == nil {
		t.Error("Ping() expected error for 401 response, got nil")
	}
}

// TestPing_ContextCancelled verifies that [Client.Ping] returns an error
// immediately when the provided context is already canceled.
func TestPing_ContextCancelled(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err = client.Ping(ctx); err == nil {
		t.Error("Ping() expected error for canceled context, got nil")
	}
}

// TestPing_ContextDeadline_BoundsTheRequest verifies that the context handed
// to [Client.Ping] bounds the GitLab round trip itself, not merely the check
// made before it starts.
//
// The pool's revalidation loop gives each entry a ten-second budget and walks
// the entries one after another. A deadline that stops at the pre-check leaves
// the request bounded by the transport's response-header timeout instead, so a
// handful of unreachable entries outlast several ticker periods and the loop's
// own arithmetic stops meaning anything.
func TestPing_ContextDeadline_BoundsTheRequest(t *testing.T) {
	// Long enough that a Ping ignoring its deadline is unmistakable, short
	// enough that the test still ends if it does.
	const handlerGuard = 5 * time.Second

	var abandoned atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(handlerGuard):
			abandoned.Store(true)
		}
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL, testValidToken)
	// Retries would answer the deadline with five more attempts against a
	// handler that is already refusing to reply.
	cfg.DisableRetries = true
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err = client.Ping(ctx); err == nil {
		t.Error("Ping() expected an error once its context expired, got nil")
	}
	elapsed := time.Since(start)

	if abandoned.Load() {
		t.Error("Ping() left the request running: GitLab never saw the context canceled")
	}
	if elapsed >= handlerGuard {
		t.Errorf("Ping() returned after %v, want it bounded by its own 200ms deadline", elapsed)
	}
}

// TestPing_EmptyVersion verifies that [Client.Ping] returns an error when the
// GitLab Version API returns an empty version string.
func TestPing_EmptyVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"version":  "",
			"revision": "abc123",
		})
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	if _, err = client.Ping(context.Background()); err == nil {
		t.Error("Ping() expected error for empty version, got nil")
	}
}

// gzipVersionServer serves one gzip-compressed /api/v4/version document whose
// decompressed form carries a version string of n bytes, and reports how many
// compressed bytes went over the wire.
func gzipVersionServer(t *testing.T, n int) (url string, wireBytes int) {
	t.Helper()
	payload := `{"version":"` + strings.Repeat("9", n) + `","enterprise":false}`
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(payload)); err != nil {
		t.Fatalf("gzip write unexpected error: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close unexpected error: %v", err)
	}
	body := buf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		if _, err := w.Write(body); err != nil {
			t.Errorf("writing version body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, len(body)
}

// TestVersionDirect_OversizedResponse_IsBounded verifies that the version
// probe honors the client's response ceiling, the same way every call the SDK
// makes does.
//
// This probe is the first request the process makes in stdio mode and it runs
// again on every SDK call while degraded, so a configured instance that is
// hostile or intercepted gets to answer before a single tool call has been
// served. The pairing matters: the bounded case only means something next to a
// control that raises the ceiling above the payload and still receives the
// whole decompressed document.
func TestVersionDirect_OversizedResponse_IsBounded(t *testing.T) {
	const decompressed = 4 << 20 // 4 MiB of version string from a few KB on the wire

	tests := []struct {
		name        string
		limit       int64
		wantBounded bool
	}{
		{name: "ceiling below the payload abandons it", limit: 64 << 10, wantBounded: true},
		{name: "ceiling above the payload decodes it whole", limit: 8 << 20, wantBounded: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, wireBytes := gzipVersionServer(t, decompressed)
			client, err := NewClient(newTestConfig(url, testValidToken))
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}
			client.SetMaxResponseBytes(tt.limit)

			info, err := client.versionDirect(context.Background())

			if tt.wantBounded {
				if !errors.Is(err, ErrResponseTooLarge) {
					t.Errorf("versionDirect() error = %v, want %v (%d wire bytes expanded to %d)",
						err, ErrResponseTooLarge, wireBytes, decompressed)
				}
				return
			}
			if err != nil {
				t.Fatalf("versionDirect() unexpected error: %v", err)
			}
			if len(info.Version) != decompressed {
				t.Errorf("versionDirect() version length = %d, want %d", len(info.Version), decompressed)
			}
		})
	}
}

// TestGL_ReturnsUnderlyingClient verifies that [Client.GL] returns the
// non-nil underlying [gl.Client] instance.
func TestGL_ReturnsUnderlyingClient(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	if client.GL() == nil {
		t.Error("GL() returned nil, expected underlying gitlab client")
	}
}

// TestNewClient_InvalidBaseURL verifies that [NewClient] returns an error
// when the GitLab URL is malformed.
func TestNewClient_InvalidBaseURL(t *testing.T) {
	_, err := NewClient(newTestConfig(":/not-a-valid-url", "token"))
	if err == nil {
		t.Error("NewClient() expected error for malformed base URL, got nil")
	}
}

// TestNewClient_SkipTLSVerifyBuildsInsecureTransport verifies that [NewClient]
// succeeds with SkipTLSVerify=true and the resulting client can still
// communicate with the test server.
func TestNewClient_SkipTLSVerifyBuildsInsecureTransport(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	cfg := &config.Config{
		GitLabURL:     srv.URL,
		GitLabToken:   testValidToken,
		SkipTLSVerify: true,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil client")
	}
	// Verify the client can still ping (exercises the TLS-skip code path)
	if _, err = client.Ping(context.Background()); err != nil {
		t.Errorf("Ping() unexpected error with SkipTLSVerify=true: %v", err)
	}
}

// TestNewClient_DisableRetries verifies that [NewClient] succeeds with
// DisableRetries=true, exercising the `gl.WithoutRetries()` option branch
// that is not covered by the default tests.
func TestNewClient_DisableRetries(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	cfg := &config.Config{
		GitLabURL:      srv.URL,
		GitLabToken:    testValidToken,
		DisableRetries: true,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil client")
	}
	// Verify the client still works with retries disabled.
	if _, err = client.Ping(context.Background()); err != nil {
		t.Errorf("Ping() unexpected error with DisableRetries=true: %v", err)
	}
}

// stubVersionServer creates an httptest server that responds to /api/v4/version.
func stubVersionServer(t *testing.T, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/version" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"version":  "16.0.0",
				"revision": "abc123",
			})
		}
	}))
}

// TestNewClientWithToken_Valid verifies that [NewClientWithToken] creates a
// non-nil client when given valid parameters.
func TestNewClientWithToken_Valid(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClientWithToken(srv.URL, testValidToken, false)
	if err != nil {
		t.Fatalf("NewClientWithToken() unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("NewClientWithToken() returned nil client")
	}
	if client.GL() == nil {
		t.Error("GL() returned nil for NewClientWithToken client")
	}
}

// TestNewClientWithToken_InvalidURL verifies that [NewClientWithToken] returns
// an error when the base URL is malformed.
func TestNewClientWithToken_InvalidURL(t *testing.T) {
	_, err := NewClientWithToken(":/not-valid", "some-token", false)
	if err == nil {
		t.Error("NewClientWithToken() expected error for invalid URL, got nil")
	}
}

// TestNewClientWithToken_SkipTLS verifies that [NewClientWithToken] succeeds
// with skipTLSVerify=true and the client can still communicate.
func TestNewClientWithToken_SkipTLS(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClientWithToken(srv.URL, testValidToken, true)
	if err != nil {
		t.Fatalf("NewClientWithToken() unexpected error: %v", err)
	}
	if _, err = client.Ping(context.Background()); err != nil {
		t.Errorf("Ping() unexpected error with SkipTLS: %v", err)
	}
}

// TestNewOAuthClientWithToken_InvalidURL verifies that
// [NewOAuthClientWithToken] returns an error when the base URL is malformed,
// mirroring TestNewClientWithToken_InvalidURL for the Bearer-auth
// constructor used by the server pool in oauth HTTP mode. Without this test,
// the gl.NewAuthSourceClient failure branch (wrapped as "creating gitlab
// oauth client") was never exercised.
func TestNewOAuthClientWithToken_InvalidURL(t *testing.T) {
	_, err := NewOAuthClientWithToken(":/not-valid", "some-token", false)
	if err == nil {
		t.Fatal("NewOAuthClientWithToken() expected error for invalid URL, got nil")
	}
	if !strings.Contains(err.Error(), "creating gitlab oauth client") {
		t.Errorf("NewOAuthClientWithToken() error = %v, want it to mention %q", err, "creating gitlab oauth client")
	}
}

// TestHTTPTransport_ReturnsRoundTripper verifies the shared HTTP transport helper.
func TestHTTPTransport_ReturnsRoundTripper(t *testing.T) {
	for _, skipTLSVerify := range []bool{false, true} {
		t.Run(fmt.Sprintf("skipTLSVerify_%v", skipTLSVerify), func(t *testing.T) {
			if HTTPTransport(skipTLSVerify) == nil {
				t.Fatal("HTTPTransport() returned nil")
			}
		})
	}
}

// TestIsGitLabDotComURL_HostMatching verifies host-only matching for GitLab.com detection.
func TestIsGitLabDotComURL_HostMatching(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "canonical", raw: "https://gitlab.com", want: true},
		{name: "canonical with path", raw: "https://gitlab.com/api/v4", want: true},
		{name: "canonical with port", raw: "https://gitlab.com:443", want: true},
		{name: "uppercase host", raw: "https://GITLAB.COM", want: true},
		{name: "self managed", raw: "https://gitlab.example.com", want: false},
		{name: "lookalike suffix", raw: "https://gitlab.com.example.com", want: false},
		{name: "missing scheme", raw: "gitlab.com", want: false},
		{name: "invalid", raw: ":/invalid", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsGitLabDotComURL(tt.raw); got != tt.want {
				t.Errorf("IsGitLabDotComURL(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

// TestClient_IsGitLabDotCom verifies clients preserve their configured base URL
// so SaaS-only tools can be conditionally registered.
func TestClient_IsGitLabDotCom(t *testing.T) {
	if (*Client)(nil).IsGitLabDotCom() {
		t.Fatal("nil client should not report GitLab.com")
	}

	client, err := NewClient(newTestConfig("https://gitlab.com", testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if !client.IsGitLabDotCom() {
		t.Fatal("NewClient() client should report GitLab.com")
	}

	selfManaged, err := NewClientWithToken("https://gitlab.example.com", testValidToken, false)
	if err != nil {
		t.Fatalf("NewClientWithToken() unexpected error: %v", err)
	}
	if selfManaged.IsGitLabDotCom() {
		t.Fatal("self-managed client should not report GitLab.com")
	}
}

// TestDotUnescape_Transport verifies that [dotUnescapeTransport] replaces %2E
// with literal dots in URL paths before sending requests, working around the
// gitlab client library's aggressive PathEscape that encodes dots.
func TestDotUnescape_Transport(t *testing.T) {
	tests := []struct {
		name    string
		rawPath string
		want    string
	}{
		{"dots encoded", "/api/v4/projects/42/releases/v1%2E1%2E2", "/api/v4/projects/42/releases/v1.1.2"},
		{"no dots", "/api/v4/projects/42/releases/latest", "/api/v4/projects/42/releases/latest"},
		{"mixed encoding", "/api/v4/projects/42/tags/v1%2E0%2E0-beta%2E1", "/api/v4/projects/42/tags/v1.0.0-beta.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotRawPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotRawPath = r.URL.RawPath
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			transport := &dotUnescapeTransport{base: http.DefaultTransport}
			client := &http.Client{Transport: transport}

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+tt.rawPath, nil)
			if err != nil {
				t.Fatalf("NewRequest error: %v", err)
			}
			req.URL.RawPath = tt.rawPath

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Do error: %v", err)
			}
			resp.Body.Close()

			if gotRawPath != "" && gotRawPath != tt.want {
				t.Errorf("RawPath = %q, want %q", gotRawPath, tt.want)
			}
		})
	}
}

// TestBuildBaseTransport_DefaultAndTLS verifies that [buildBaseTransport]
// returns the shared permissive transport, a clone of http.DefaultTransport,
// when TLS verification is enabled, and a custom transport with
// InsecureSkipVerify when disabled.
func TestBuildBaseTransport_DefaultAndTLS(t *testing.T) {
	defTransport, ok := buildBaseTransport(false).(*http.Transport)
	if !ok {
		t.Fatalf("buildBaseTransport(false) = %T, want *http.Transport", buildBaseTransport(false))
	}
	// It must not BE http.DefaultTransport: this package sets a response
	// header timeout on it, and doing that to the shared default would
	// change the behavior of every other package in the process.
	if defTransport == http.DefaultTransport {
		t.Error("buildBaseTransport(false) returned http.DefaultTransport itself; mutating it would leak into the whole process")
	}
	// It must be the SAME instance every time, because the connection pool
	// lives in the transport and a per-client one would give every pool
	// entry its own set of idle connections.
	if second, _ := buildBaseTransport(false).(*http.Transport); second != defTransport {
		t.Error("buildBaseTransport(false) returned a fresh transport; the connection pool must be shared")
	}
	if defTransport.ResponseHeaderTimeout != responseHeaderTimeout {
		t.Errorf("ResponseHeaderTimeout = %v, want %v", defTransport.ResponseHeaderTimeout, responseHeaderTimeout)
	}

	tlsTransport := buildBaseTransport(true)
	ht, ok := tlsTransport.(*http.Transport)
	if !ok {
		t.Fatalf("buildBaseTransport(true) = %T, want *http.Transport", tlsTransport)
	}
	if !ht.TLSClientConfig.InsecureSkipVerify {
		t.Error("buildBaseTransport(true) should have InsecureSkipVerify=true")
	}
	if ht.ResponseHeaderTimeout != responseHeaderTimeout {
		t.Errorf("ResponseHeaderTimeout = %v, want %v", ht.ResponseHeaderTimeout, responseHeaderTimeout)
	}
}

// destinationPoolTransports holds a pair of pools to what every pair must be,
// whoever it was built for, and returns its two transports.
//
// The two must be different transports, since one transport is one idle pool
// and a pair of the same transport is the defect the pair exists to close.
// Each must be a clone rather than [http.DefaultTransport], carry the response
// header timeout, and dial through the guard.
func destinationPoolTransports(t *testing.T, pools destinationPools) (permissive, strict *http.Transport) {
	t.Helper()
	permissive, ok := pools.permissive.(*http.Transport)
	if !ok {
		t.Fatalf("permissive = %T, want *http.Transport", pools.permissive)
	}
	strict, ok = pools.strict.(*http.Transport)
	if !ok {
		t.Fatalf("strict = %T, want *http.Transport", pools.strict)
	}
	if permissive == strict {
		t.Fatal("the two pools are one transport, so a request the guard refuses can be handed a connection one it permits opened")
	}
	for name, transport := range map[string]*http.Transport{"permissive": permissive, "strict": strict} {
		t.Run(name, func(t *testing.T) {
			if transport == http.DefaultTransport {
				t.Error("this pool is http.DefaultTransport itself; mutating it would leak into the whole process")
			}
			if transport.ResponseHeaderTimeout != responseHeaderTimeout {
				t.Errorf("ResponseHeaderTimeout = %v, want %v", transport.ResponseHeaderTimeout, responseHeaderTimeout)
			}
			conn, err := transport.DialContext(t.Context(), "tcp", "169.254.169.254:80")
			if conn != nil {
				_ = conn.Close()
			}
			if !errors.Is(err, ErrDestinationRefused) {
				t.Errorf("this pool dialed a metadata address: %v", err)
			}
		})
	}
	return permissive, strict
}

// skipsVerification reports whether a transport accepts any certificate.
func skipsVerification(transport *http.Transport) bool {
	return transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify
}

// TestBuildDestinationPools_Verified_OnePairSharedAndLent verifies the pair
// every verifying client routes its requests between.
//
// It must be one pair per process, since the connection pool is why the
// transports are shared at all, and its permissive half must be the transport
// [HTTPTransport] lends, so that an unstamped borrower keeps the pool it
// always had.
func TestBuildDestinationPools_Verified_OnePairSharedAndLent(t *testing.T) {
	permissive, strict := destinationPoolTransports(t, buildDestinationPools(false))

	again := buildDestinationPools(false)
	if again.permissive != permissive || again.strict != strict {
		t.Error("buildDestinationPools(false) built a fresh pair; the connection pools must be shared")
	}
	if HTTPTransport(false) != permissive {
		t.Error("HTTPTransport(false) is not the shared permissive pool, so an unstamped borrower moved pools")
	}
	if skipsVerification(permissive) || skipsVerification(strict) {
		t.Error("a shared pool skips certificate verification")
	}
}

// TestBuildDestinationPools_SkipTLSVerify_APairOfTheClientsOwn verifies the
// pair a client that skips certificate verification gets: its own, as the one
// transport such a client got always was, with both halves skipping
// verification above the TLS 1.2 floor.
func TestBuildDestinationPools_SkipTLSVerify_APairOfTheClientsOwn(t *testing.T) {
	permissive, strict := destinationPoolTransports(t, buildDestinationPools(true))

	if !skipsVerification(permissive) || !skipsVerification(strict) {
		t.Fatal("a pool of a client that skips verification verifies")
	}
	if permissive.TLSClientConfig.MinVersion != tls.VersionTLS12 || strict.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Error("skipping verification lowered the TLS floor below 1.2")
	}
	shared := buildDestinationPools(false)
	if permissive == shared.permissive || strict == shared.strict {
		t.Error("a client that skips verification was given a verified pool")
	}
	other := buildDestinationPools(true)
	if other.permissive == permissive || other.strict == strict {
		t.Error("two clients that skip verification share a pool")
	}
}

// TestBaseDialer_RestatesTheDefaultTimeoutsAndCarriesTheGuard pins the three
// fields the package's own dialer exists for.
//
// Both durations restate what net/http's default dialer carries. That reads
// like redundancy and is not: setting [http.Transport.DialContext] replaces
// the dialer those defaults belong to, so a dialer that omits them is a
// transport with no connect deadline and no keep-alive at all, and a GitLab
// that accepts a connection and never completes the handshake pins a goroutine
// and a socket for as long as the operating system allows.
//
// ControlContext is asserted by what it does rather than by comparing function
// values, which Go does not allow: a cloud metadata address must be refused
// before any packet is sent, which is the whole reason the guard is a dialer
// hook and not a round tripper.
func TestBaseDialer_RestatesTheDefaultTimeoutsAndCarriesTheGuard(t *testing.T) {
	d := baseDialer()

	t.Run("connect timeout", func(t *testing.T) {
		if d.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want %v; replacing DialContext removes net/http's own", d.Timeout, 30*time.Second)
		}
	})
	t.Run("keep alive", func(t *testing.T) {
		if d.KeepAlive != 30*time.Second {
			t.Errorf("KeepAlive = %v, want %v; replacing DialContext removes net/http's own", d.KeepAlive, 30*time.Second)
		}
	})
	t.Run("the destination guard runs as ControlContext", func(t *testing.T) {
		if d.ControlContext == nil {
			t.Fatal("ControlContext = nil, want the destination guard")
		}
		err := d.ControlContext(t.Context(), "tcp", "169.254.169.254:80", nil)
		if !errors.Is(err, ErrDestinationRefused) {
			t.Errorf("ControlContext(metadata address) = %v, want %v", err, ErrDestinationRefused)
		}
	})
	t.Run("the transport dials through it", func(t *testing.T) {
		conn, err := newBaseTransport(nil).DialContext(t.Context(), "tcp", "169.254.169.254:80")
		if conn != nil {
			_ = conn.Close()
		}
		if !errors.Is(err, ErrDestinationRefused) {
			t.Errorf("DialContext(metadata address) = %v, want %v", err, ErrDestinationRefused)
		}
	})
}

// TestInitialize_Success verifies that [Client.Initialize] marks the client
// as initialized and returns the GitLab version when the server responds OK.
func TestInitialize_Success(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	if client.IsInitialized() {
		t.Fatal("client should not be initialized before Initialize()")
	}

	ver, err := client.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize() unexpected error: %v", err)
	}
	if ver != "16.0.0" {
		t.Errorf("Initialize() version = %q, want %q", ver, "16.0.0")
	}
	if !client.IsInitialized() {
		t.Error("client should be initialized after successful Initialize()")
	}
}

// TestInitialize_ServerDown verifies that [Client.Initialize] returns an error
// and leaves the client as not initialized when GitLab is unreachable.
func TestInitialize_ServerDown(t *testing.T) {
	// Create a server and immediately close it so the URL is unreachable.
	srv := stubVersionServer(t, http.StatusOK)
	url := srv.URL
	srv.Close()

	client, err := NewClient(newTestConfig(url, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	if _, err = client.Initialize(context.Background()); err == nil {
		t.Error("Initialize() expected error for unreachable server, got nil")
	}
	if client.IsInitialized() {
		t.Error("client should not be initialized after failed Initialize()")
	}
}

// TestInitialize_ContextCancelled verifies that [Client.Initialize] returns
// immediately when the provided context is already canceled.
func TestInitialize_ContextCancelled(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err = client.Initialize(ctx); err == nil {
		t.Error("Initialize() expected error for canceled context, got nil")
	}
}

// TestEnsureInitialized_FastPath verifies that [Client.EnsureInitialized]
// returns immediately when needsLazyInit is false (normal operation).
func TestEnsureInitialized_FastPath(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	// needsLazyInit is false by default — EnsureInitialized should be a no-op.
	client.EnsureInitialized(context.Background())
	if client.IsInitialized() {
		t.Error("EnsureInitialized should not initialize when needsLazyInit is false")
	}
}

// TestEnsureInitialized_Recovery verifies that [Client.EnsureInitialized]
// recovers the client when GitLab becomes available after being down at startup.
func TestEnsureInitialized_Recovery(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	// Simulate startup failure: EnableLazyInit without Initialize.
	client.EnableLazyInit()
	if client.IsInitialized() {
		t.Fatal("client should not be initialized before recovery")
	}

	// Now the server IS available — EnsureInitialized should recover.
	client.EnsureInitialized(context.Background())
	if !client.IsInitialized() {
		t.Error("client should be initialized after successful recovery")
	}
	// Recovery must also switch lazy re-initialization off. The check runs
	// from the transport on every SDK request, so a client that stays in
	// lazy mode after recovering takes the lock, and once per cooldown a
	// whole version round-trip, for the rest of the process's life.
	if client.needsLazyInit.Load() {
		t.Error("lazy re-initialization is still armed after a successful recovery")
	}
	before := client.lastInitAttempt
	client.EnsureInitialized(context.Background())
	if client.lastInitAttempt != before {
		t.Error("a recovered client attempted initialization again")
	}
}

// TestSetOnRecovered_RunsOnlyWhenALazyInitializationSucceeds verifies the
// recovery hook a stdio start registers: it is not told about an attempt the
// instance did not answer, it is told once when the next attempt recovers the
// client, and a nil hook clears it, so a recovery after that tells nobody.
func TestSetOnRecovered_RunsOnlyWhenALazyInitializationSucceeds(t *testing.T) {
	var up atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !up.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"19.4.1-ee","revision":"abc"}`))
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	var told atomic.Int64
	client.SetOnRecovered(func() { told.Add(1) })
	client.EnableLazyInit()

	client.EnsureInitialized(context.Background())
	if told.Load() != 0 || client.IsInitialized() {
		t.Fatalf("after an attempt the instance did not answer: told %d times, initialized %v; want neither",
			told.Load(), client.IsInitialized())
	}

	up.Store(true)
	client.lastInitAttempt = time.Time{}
	client.EnsureInitialized(context.Background())
	if told.Load() != 1 || !client.IsInitialized() {
		t.Fatalf("after the attempt that recovered: told %d times, initialized %v; want once and initialized",
			told.Load(), client.IsInitialized())
	}

	cleared, clearErr := NewClient(newTestConfig(srv.URL, testValidToken))
	if clearErr != nil {
		t.Fatalf(fmtNewClientErr, clearErr)
	}
	cleared.SetOnRecovered(func() { told.Add(1) })
	cleared.SetOnRecovered(nil)
	cleared.EnableLazyInit()
	cleared.EnsureInitialized(context.Background())
	if told.Load() != 1 || !cleared.IsInitialized() {
		t.Errorf("a cleared hook: told %d times in all, initialized %v; want still once and initialized",
			told.Load(), cleared.IsInitialized())
	}
}

// TestEnsureInitialized_Cooldown verifies that [Client.EnsureInitialized]
// respects the 30-second cooldown between re-initialization attempts.
func TestEnsureInitialized_Cooldown(t *testing.T) {
	// Create a server that always returns 503 to simulate persistent outage.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	client.EnableLazyInit()

	// First call: should attempt initialization.
	client.EnsureInitialized(context.Background())
	firstCount := callCount

	// Second call immediately after: should be skipped due to cooldown.
	client.EnsureInitialized(context.Background())
	if callCount != firstCount {
		t.Errorf("expected cooldown to prevent second attempt, got %d calls (want %d)", callCount, firstCount)
	}
}

// TestEnsureInitialized_Cooldown_EndsExactlyWhenItSays verifies that a lazy
// re-initialization attempt is refused until initCooldown has passed since the
// last one, and made the instant it has.
//
// A bubble's clock moves only when its goroutines sleep, so the time since the
// last attempt is exactly what the test slept, which the wall clock never
// allows: this boundary used to be recorded as one no test could schedule. The
// attempt runs under a cancelled context, so Initialize returns before it
// sends anything and the attempt's only trace is lastInitAttempt moving to the
// bubble's present.
func TestEnsureInitialized_Cooldown_EndsExactlyWhenItSays(t *testing.T) {
	cases := []struct {
		name    string
		elapsed time.Duration
		attempt bool
	}{
		{name: "a nanosecond before the cooldown ends", elapsed: initCooldown - time.Nanosecond, attempt: false},
		{name: "the instant the cooldown ends", elapsed: initCooldown, attempt: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewClient(newTestConfig("http://127.0.0.1:1", testValidToken))
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}
			client.EnableLazyInit()
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				client.lastInitAttempt = time.Now()
				before := client.lastInitAttempt
				time.Sleep(tc.elapsed)
				client.EnsureInitialized(ctx)
				if attempted := !client.lastInitAttempt.Equal(before); attempted != tc.attempt {
					t.Errorf("after %v an attempt was made = %v, want %v", tc.elapsed, attempted, tc.attempt)
				}
			})
		})
	}
}

// TestEnableLazyInit_And_IsInitialized verifies the basic state transitions
// of [Client.EnableLazyInit], [Client.IsInitialized], and [Client.MarkInitialized].
func TestEnableLazyInit_And_IsInitialized(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	if client.IsInitialized() {
		t.Error("new client should not be initialized")
	}

	client.MarkInitialized()
	if !client.IsInitialized() {
		t.Error("client should be initialized after MarkInitialized()")
	}

	client.EnableLazyInit()
	// EnableLazyInit sets needsLazyInit, but does NOT clear initialized.
	if !client.IsInitialized() {
		t.Error("EnableLazyInit should not clear initialized flag")
	}
}

// TestResilienceTransport_PassesThrough verifies that [resilienceTransport]
// delegates requests to the base transport when the client is initialized.
func TestResilienceTransport_PassesThrough(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	client.MarkInitialized()

	// Use the SDK client to make a request — it goes through resilienceTransport.
	ver, err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping() through resilience transport: %v", err)
	}
	if ver != "16.0.0" {
		t.Errorf("version = %q, want %q", ver, "16.0.0")
	}
}

// TestSetEnterprise_And_IsEnterprise verifies that [Client.SetEnterprise]
// and [Client.IsEnterprise] correctly toggle the enterprise flag.
func TestSetEnterprise_And_IsEnterprise(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	if client.IsEnterprise() {
		t.Error("new client should not be enterprise by default")
	}

	client.SetEnterprise(true)
	if !client.IsEnterprise() {
		t.Error("client should be enterprise after SetEnterprise(true)")
	}

	client.SetEnterprise(false)
	if client.IsEnterprise() {
		t.Error("client should not be enterprise after SetEnterprise(false)")
	}
}

// TestInitialize_DetectsEnterpriseFromVersion verifies that Initialize applies
// the optional enterprise field returned by GitLab's version endpoint.
func TestInitialize_DetectsEnterpriseFromVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version":    "17.0.0",
			"revision":   "abc123",
			"enterprise": true,
		})
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	version, err := client.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}
	if version != "17.0.0" {
		t.Fatalf("Initialize() version = %q, want 17.0.0", version)
	}
	if !client.IsEnterprise() {
		t.Fatal("client should be enterprise when version endpoint reports enterprise=true")
	}
}

// TestEnsureInitialized_PinnedTier_SurvivesTheEditionProbe holds a tier the
// operator pinned through the lazy re-initialization of a degraded start. That
// path runs Initialize, whose edition probe used to replace the tier with
// Premium for an enterprise build and Free otherwise, after the catalog had been
// registered for the pinned tier, so the handlers that branch on IsEnterprise
// answered for a tier the surface was not built for (issue 1016). Each case is
// one the overwrite changed: an Ultimate pin read as Premium, a Free pin read as
// Premium on an enterprise build, and a Premium pin read as Free on CE.
func TestEnsureInitialized_PinnedTier_SurvivesTheEditionProbe(t *testing.T) {
	cases := []struct {
		name       string
		pin        edition.Tier
		enterprise bool
	}{
		{name: "ultimate pin on an enterprise build", pin: edition.Ultimate, enterprise: true},
		{name: "free pin on an enterprise build", pin: edition.Free, enterprise: true},
		{name: "premium pin on a CE build", pin: edition.Premium, enterprise: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v4/version" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"version": "19.4.1", "enterprise": tc.enterprise})
			}))
			defer srv.Close()

			client, err := NewClient(newTestConfig(srv.URL, testValidToken))
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}
			client.PinTier(tc.pin)
			client.EnableLazyInit()

			client.EnsureInitialized(context.Background())

			if !client.IsInitialized() {
				t.Fatal("the lazy re-initialization did not run, so it proved nothing about the pin")
			}
			if got := client.Tier(); got != tc.pin {
				t.Errorf("Tier() after re-initialization = %v, want the pinned %v", got, tc.pin)
			}
			if !client.TierPinned() {
				t.Error("TierPinned() = false after PinTier")
			}
		})
	}
}

// TestSetEnterprise_UnpinnedTier_FollowsTheEdition keeps the other half of the
// rule: a tier nobody pinned still takes the edition the probe reports, which
// is what a start that detects its tier relies on.
func TestSetEnterprise_UnpinnedTier_FollowsTheEdition(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	client.SetTier(edition.Ultimate)
	if client.TierPinned() {
		t.Fatal("SetTier pinned the tier; only PinTier may")
	}

	client.SetEnterprise(false)
	if got := client.Tier(); got != edition.Free {
		t.Errorf("Tier() after SetEnterprise(false) on an unpinned client = %v, want %v", got, edition.Free)
	}
	client.SetEnterprise(true)
	if got := client.Tier(); got != edition.Premium {
		t.Errorf("Tier() after SetEnterprise(true) on an unpinned client = %v, want %v", got, edition.Premium)
	}
}

// TestDetectEnterprise_OverridesFallback verifies that explicit edition data
// from GitLab wins over the configured fallback.
func TestDetectEnterprise_OverridesFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version":    "17.0.0",
			"enterprise": false,
		})
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	if client.DetectEnterprise(context.Background(), true) {
		t.Fatal("DetectEnterprise() = true, want false from version endpoint")
	}
	if client.IsEnterprise() {
		t.Fatal("client should not be enterprise after detected enterprise=false")
	}
}

// TestDetectEnterprise_MissingFieldUsesFallback verifies graceful fallback for
// GitLab versions that do not expose the enterprise field.
func TestDetectEnterprise_MissingFieldUsesFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "16.11.0"})
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	if !client.DetectEnterprise(context.Background(), true) {
		t.Fatal("DetectEnterprise() = false, want configured fallback true")
	}
	if !client.IsEnterprise() {
		t.Fatal("client should keep enterprise fallback when endpoint omits edition")
	}
}

// TestDetectEnterprise_ErrorUsesFallback verifies edition detection falls back
// to configured mode when the GitLab version endpoint cannot be read.
func TestDetectEnterprise_ErrorUsesFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/version" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	if !client.DetectEnterprise(context.Background(), true) {
		t.Fatal("DetectEnterprise() = false, want fallback true")
	}
	if !client.IsEnterprise() {
		t.Fatal("client should use enterprise fallback when detection fails")
	}
}

// licenseServer returns an httptest server that serves GET /api/v4/license with
// the given status and plan body (plan ignored when status is non-200).
func licenseServer(t *testing.T, status int, plan string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/version":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "17.0.0"})
		case "/api/v4/license":
			if status != http.StatusOK {
				w.WriteHeader(status)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"plan": plan})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestDetectTier_FromLicensePlan verifies that DetectTier maps the license plan
// (including legacy names) to the expected tier and stores it on the client.
func TestDetectTier_FromLicensePlan(t *testing.T) {
	tests := []struct {
		plan string
		want edition.Tier
	}{
		{plan: "premium", want: edition.Premium},
		{plan: "ultimate", want: edition.Ultimate},
		{plan: "starter", want: edition.Premium},
		{plan: "bronze", want: edition.Premium},
		{plan: "silver", want: edition.Premium},
		{plan: "gold", want: edition.Ultimate},
		{plan: "free", want: edition.Free},
		{plan: "", want: edition.Free},
		{plan: "mystery", want: edition.Free},
	}
	for _, tc := range tests {
		t.Run(tc.plan, func(t *testing.T) {
			srv := licenseServer(t, http.StatusOK, tc.plan)
			client, err := NewClient(newTestConfig(srv.URL, testValidToken))
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}
			got := client.DetectTier(context.Background())
			if got != tc.want {
				t.Errorf("DetectTier() = %v, want %v", got, tc.want)
			}
			if client.Tier() != tc.want {
				t.Errorf("client.Tier() = %v, want %v", client.Tier(), tc.want)
			}
		})
	}
}

// TestDetectTier_ErrorFallsBackToFree verifies that a license API error (e.g.
// non-admin token or CE instance) maps the client to the Free tier.
func TestDetectTier_ErrorFallsBackToFree(t *testing.T) {
	srv := licenseServer(t, http.StatusForbidden, "")
	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if got := client.DetectTier(context.Background()); got != edition.Free {
		t.Errorf("DetectTier() on error = %v, want free", got)
	}
	if client.IsEnterprise() {
		t.Error("client should not be enterprise after license detection error")
	}
}

// TestCurrentUsername_Success verifies that [Client.CurrentUsername] returns
// the username from the /user API endpoint.
func TestCurrentUsername_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/user":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       1,
				"username": "testuser",
				"name":     "Test User",
			})
		case "/api/v4/version":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"version":  "16.0.0",
				"revision": "abc123",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	username, err := client.CurrentUsername(context.Background())
	if err != nil {
		t.Fatalf("CurrentUsername() unexpected error: %v", err)
	}
	if username != "testuser" {
		t.Errorf("CurrentUsername() = %q, want %q", username, "testuser")
	}
}

// TestCurrentUsername_ContextCancelled verifies that [Client.CurrentUsername]
// returns an error when the context is already canceled.
func TestCurrentUsername_ContextCancelled(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = client.CurrentUsername(ctx)
	if err == nil {
		t.Error("CurrentUsername() expected error for canceled context, got nil")
	}
}

// TestCurrentUsername_APIError verifies that [Client.CurrentUsername] returns
// an error when the /user API endpoint responds with an error.
func TestCurrentUsername_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/user" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Still respond OK to version for client creation
		if r.URL.Path == "/api/v4/version" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "16.0.0", "revision": "abc"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	_, err = client.CurrentUsername(context.Background())
	if err == nil {
		t.Error("CurrentUsername() expected error for 401 response, got nil")
	}
}

// TestPingDirect_EmptyVersion verifies that [Client.pingDirect] returns an
// error when the /api/v4/version endpoint returns an empty version string.
func TestPingDirect_EmptyVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"version":  "",
			"revision": "abc123",
		})
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	err = client.pingDirect(context.Background())
	if err == nil {
		t.Error("pingDirect() expected error for empty version, got nil")
	}
}

// TestPingDirect_NonOKStatus verifies that [Client.pingDirect] returns an
// error when the /api/v4/version endpoint returns a non-200 status code.
func TestPingDirect_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("service maintenance"))
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	err = client.pingDirect(context.Background())
	if err == nil {
		t.Error("pingDirect() expected error for 503 response, got nil")
	}
}

// TestPingDirect_MalformedJSON verifies that [Client.pingDirect] returns an
// error when the version endpoint returns invalid JSON.
func TestPingDirect_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	err = client.pingDirect(context.Background())
	if err == nil {
		t.Error("pingDirect() expected error for malformed JSON, got nil")
	}
}

// TestNewClient_TierConfig verifies that [NewClient] adopts the configured
// tier and that IsEnterprise derives correctly from it.
func TestNewClient_TierConfig(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	cfg := &config.Config{
		GitLabURL:    srv.URL,
		GitLabToken:  testValidToken,
		Tier:         edition.Ultimate,
		TierExplicit: true,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if client.Tier() != edition.Ultimate {
		t.Errorf("Tier() = %v, want ultimate", client.Tier())
	}
	if !client.IsEnterprise() {
		t.Error("client should be enterprise when config tier is ultimate")
	}
}

// TestEnsureInitialized_DoubleCheckAfterLock verifies the double-check pattern
// in EnsureInitialized: when two goroutines race with needsLazyInit=true, the
// second goroutine sees initialized=true after acquiring the lock and returns
// early without re-initializing.
func TestEnsureInitialized_DoubleCheckAfterLock(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	client.EnableLazyInit()

	// First call initializes successfully.
	client.EnsureInitialized(context.Background())
	if !client.IsInitialized() {
		t.Fatal("client should be initialized after first EnsureInitialized")
	}

	// needsLazyInit was cleared, but we re-enable it to simulate a second
	// goroutine that already passed the needsLazyInit check.
	client.needsLazyInit.Store(true)

	// Second call enters the lock, finds initialized=true (double-check), returns.
	client.EnsureInitialized(context.Background())
	if !client.IsInitialized() {
		t.Error("client should still be initialized after double-check path")
	}
}

// TestPingDirect_NilContext verifies that pingDirect returns an error when
// called with a nil context, which causes http.NewRequestWithContext to fail.
func TestPingDirect_NilContext(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	//nolint:staticcheck // SA1012: a nil context is the one input that makes http.NewRequestWithContext fail, which is the branch under test
	pingErr := client.pingDirect(nil)
	if pingErr == nil {
		t.Fatal("expected error for nil context, got nil")
	}
}

// TestCheckCredential_FourAnswers_KeepsEachApart verifies that the credential
// probe tells its four answers apart rather than folding one into another.
//
// The refusal is the point of the probe: a 401 or a 403 on /api/v4/user is
// the instance saying this credential is no longer good, and it is what makes
// the pool drop the entry and the gate charge the caller. The fail-open rule
// is the other half: a 404 from a stubbed instance, a 5xx, and an instance
// that does not answer at all are "no verdict", and a refusal read from any of
// them would turn one unreachable GitLab into a mass revocation. The pool's
// confirmation of a 401 that named no cause reads the acceptance, which is only
// honest when GitLab actually answered. And the one 403 whose body is GitLab's
// refusal of a fine-grained permission is none of them: the credential was
// accepted, and read as a refusal it would be charged and told to
// reauthorize a token that works.
func TestCheckCredential_FourAnswers_KeepsEachApart(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		unreliable bool
		want       CredentialVerdict
	}{
		{name: "200 accepts", status: http.StatusOK, want: CredentialAccepted},
		{name: "204 accepts", status: http.StatusNoContent, want: CredentialAccepted},
		{name: "401 refuses", status: http.StatusUnauthorized, want: CredentialRefused},
		{name: "403 refuses", status: http.StatusForbidden, want: CredentialRefused},
		{name: "403 for a classic scope refuses", status: http.StatusForbidden, body: `{"error":"insufficient_scope"}`, want: CredentialRefused},
		{name: "403 for a fine-grained permission accepts the credential", status: http.StatusForbidden, body: granularRefusalBody, want: CredentialAcceptedPermissionMissing},
		{name: "401 carrying the fine-grained code still refuses", status: http.StatusUnauthorized, body: granularRefusalBody, want: CredentialRefused},
		{name: "404 is no verdict", status: http.StatusNotFound, want: CredentialUnanswered},
		{name: "500 is no verdict", status: http.StatusInternalServerError, want: CredentialUnanswered},
		{name: "no answer at all is no verdict", unreliable: true, want: CredentialUnanswered},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			client, err := NewClientWithTokenRetries(srv.URL, testValidToken, false, true)
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}
			if tt.unreliable {
				srv.Close()
			}
			if got := client.CheckCredential(t.Context()); got != tt.want {
				t.Errorf("CheckCredential() = %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("a probe that cannot be built is no verdict", func(t *testing.T) {
		client, err := NewClientWithTokenRetries("http://gitlab.example.com", testValidToken, false, true)
		if err != nil {
			t.Fatalf(fmtNewClientErr, err)
		}
		//nolint:staticcheck // SA1012: a nil context is the one input that makes http.NewRequestWithContext fail, which is the branch under test
		if got := client.CheckCredential(nil); got != CredentialUnanswered {
			t.Errorf("CheckCredential(nil) = %v, want CredentialUnanswered", got)
		}
	})
}

// TestCheckCredentialDetail_EachAnswer_KeepsItsCause verifies that the probe
// keeps what its verdict was read from: the status of every response, and a
// cause for every answer that is no verdict, which is what the pool's
// revalidation logs. A verdict carries no cause, so a warning built from one
// cannot be mistaken for a failure; no verdict always carries one, so a round
// that keeps an entry can always say why, whether GitLab answered with a
// status that decides nothing, never answered, or was never asked.
func TestCheckCredentialDetail_EachAnswer_KeepsItsCause(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		unreachable bool
		wantVerdict CredentialVerdict
		wantStatus  int
		// wantCause is a substring of the cause, empty when there must be none.
		wantCause string
	}{
		{name: "an acceptance has no cause", status: http.StatusOK, wantVerdict: CredentialAccepted, wantStatus: http.StatusOK},
		{name: "a refusal has no cause", status: http.StatusUnauthorized, wantVerdict: CredentialRefused, wantStatus: http.StatusUnauthorized},
		{name: "a 500 names its status", status: http.StatusInternalServerError, wantVerdict: CredentialUnanswered, wantStatus: http.StatusInternalServerError, wantCause: "answered HTTP 500"},
		{name: "a 429 names its status", status: http.StatusTooManyRequests, wantVerdict: CredentialUnanswered, wantStatus: http.StatusTooManyRequests, wantCause: "answered HTTP 429"},
		{name: "no response names the transport error", unreachable: true, wantVerdict: CredentialUnanswered, wantCause: "/api/v4/user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()
			client, err := NewClientWithTokenRetries(srv.URL, testValidToken, false, true)
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}
			if tt.unreachable {
				srv.Close()
			}
			check := client.CheckCredentialDetail(t.Context())
			assertCredentialCheck(t, check, tt.wantVerdict, tt.wantStatus, tt.wantCause)
			if check.Description != "" {
				t.Errorf("CheckCredentialDetail().Description = %q, want none outside a fine-grained refusal", check.Description)
			}
		})
	}
	t.Run("a probe that cannot be built names why", func(t *testing.T) {
		client, err := NewClientWithTokenRetries("http://gitlab.example.com", testValidToken, false, true)
		if err != nil {
			t.Fatalf(fmtNewClientErr, err)
		}
		//nolint:staticcheck // SA1012: a nil context is the one input that makes http.NewRequestWithContext fail, which is the branch under test
		assertCredentialCheck(t, client.CheckCredentialDetail(nil), CredentialUnanswered, 0, "build the credential probe")
	})
}

// TestCheckCredentialDetail_FineGrainedRefusal_CarriesGitLabsSentence verifies
// that the one 403 read as an accepted credential keeps GitLab's sentence as
// sent, with no cause, since it is a verdict, and that a body too long to be
// GitLab's error document is read only as far as the probe's bound and then
// counts as the refusal it was before the code was recognized.
func TestCheckCredentialDetail_FineGrainedRefusal_CarriesGitLabsSentence(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		wantVerdict     CredentialVerdict
		wantDescription string
	}{
		{name: "GitLab's document", body: granularRefusalBody, wantVerdict: CredentialAcceptedPermissionMissing, wantDescription: granularRefusalSentence},
		{
			name:        "a document past the probe's bound",
			body:        `{"error":"insufficient_granular_scope","error_description":"` + strings.Repeat("x", refusalBodyBytes) + `"}`,
			wantVerdict: CredentialRefused,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			client, err := NewClientWithTokenRetries(srv.URL, testValidToken, false, true)
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}
			check := client.CheckCredentialDetail(t.Context())
			assertCredentialCheck(t, check, tt.wantVerdict, http.StatusForbidden, "")
			if check.Description != tt.wantDescription {
				t.Errorf("CheckCredentialDetail().Description = %q, want %q", check.Description, tt.wantDescription)
			}
		})
	}
}

// assertCredentialCheck holds one probe's answer to a verdict, a status and a
// cause, where an empty wantCause means the answer must carry none.
func assertCredentialCheck(t *testing.T, got CredentialCheck, wantVerdict CredentialVerdict, wantStatus int, wantCause string) {
	t.Helper()
	if got.Verdict != wantVerdict || got.Status != wantStatus {
		t.Errorf("CheckCredentialDetail() = verdict %v status %d, want verdict %v status %d", got.Verdict, got.Status, wantVerdict, wantStatus)
	}
	if wantCause == "" {
		if got.Err != nil {
			t.Errorf("CheckCredentialDetail().Err = %v, want none for a verdict", got.Err)
		}
		return
	}
	if got.Err == nil || !strings.Contains(got.Err.Error(), wantCause) {
		t.Errorf("CheckCredentialDetail().Err = %v, want one naming %q", got.Err, wantCause)
	}
}

// TestCredentialVerdictFor_StatusEdges_AreReadExactly pins the edges of the
// status reading, where an off-by-one would move a whole class of answers: the
// first and last 2xx accept, the statuses either side of that range are no
// verdict, and among the 4xx only 401 and 403 refuse.
func TestCredentialVerdictFor_StatusEdges_AreReadExactly(t *testing.T) {
	tests := []struct {
		status int
		body   string
		want   CredentialVerdict
	}{
		{status: 199, want: CredentialUnanswered},
		{status: http.StatusOK, want: CredentialAccepted},
		{status: 299, want: CredentialAccepted},
		{status: http.StatusMultipleChoices, want: CredentialUnanswered},
		{status: http.StatusBadRequest, want: CredentialUnanswered},
		{status: http.StatusUnauthorized, want: CredentialRefused},
		{status: http.StatusPaymentRequired, want: CredentialUnanswered},
		{status: http.StatusForbidden, want: CredentialRefused},
		{status: http.StatusForbidden, body: granularRefusalBody, want: CredentialAcceptedPermissionMissing},
		{status: http.StatusUnauthorized, body: granularRefusalBody, want: CredentialRefused},
		{status: http.StatusOK, body: granularRefusalBody, want: CredentialAccepted},
		{status: http.StatusNotFound, body: granularRefusalBody, want: CredentialUnanswered},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status)+" "+strconv.FormatBool(tt.body != ""), func(t *testing.T) {
			if got := credentialVerdictFor(tt.status, []byte(tt.body)); got != tt.want {
				t.Errorf("credentialVerdictFor(%d, %q) = %v, want %v", tt.status, tt.body, got, tt.want)
			}
		})
	}
}

// TestUnauthorizedHook_SDKCallsReportAndProbesDoNot verifies which of a
// client's requests reach the unauthorized hook, on a real client.
//
// Every call the SDK makes does, which is what lets the server pool notice a
// refused credential on the first call GitLab refuses. The credential probe
// does not, even when GitLab answers it with a 401 naming the credential: the
// pool sends that probe to confirm a 401 naming nothing, and if its own 401
// were reported it would raise the very question it was sent to answer.
func TestUnauthorizedHook_SDKCallsReportAndProbesDoNot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(expiredTokenBody))
	}))
	defer srv.Close()

	client, err := NewClientWithTokenRetries(srv.URL, testValidToken, false, true)
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	var told atomic.Int32
	client.SetOnUnauthorized(func(UnauthorizedAnswer) { told.Add(1) })

	if got := client.CheckCredential(t.Context()); got != CredentialRefused {
		t.Fatalf("CheckCredential() = %v, want CredentialRefused from the stub", got)
	}
	if got := told.Load(); got != 0 {
		t.Errorf("the credential probe's own 401 reached the hook %d times, want none", got)
	}

	if _, _, callErr := client.GL().Users.CurrentUser(); callErr == nil {
		t.Fatal("the SDK call succeeded, so the stub did not refuse it")
	}
	if got := told.Load(); got != 1 {
		t.Errorf("the SDK call's 401 reached the hook %d times, want once", got)
	}
}

// TestPing_NullVersionDocument verifies that a version endpoint answering the
// JSON literal null is reported as a failed ping rather than dereferenced.
//
// client-go decodes into a pointer it leaves nil when the body is null, so
// this is the one 200 response that reaches [Client.Ping] with no error and
// no value. Without the nil half of the guard the next line reads
// v.Version off nothing.
func TestPing_NullVersionDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("null"))
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	version, err := client.Ping(context.Background())
	if err == nil {
		t.Fatalf("Ping() = %q, want an error for a null version document", version)
	}
}

// TestDetectTier_NullLicenseDocument verifies that a license endpoint
// answering the JSON literal null falls back to Free rather than being
// dereferenced, the same shape [TestPing_NullVersionDocument] pins for the
// version endpoint.
func TestDetectTier_NullLicenseDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v4/version" {
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "17.0.0"})
			return
		}
		_, _ = w.Write([]byte("null"))
	}))
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	if got := client.DetectTier(context.Background()); got != edition.Free {
		t.Errorf("DetectTier() on a null license document = %v, want free", got)
	}
}

// TestNewBaseTransport_ForeignDefault_StillBuildsATransport covers the
// fallback taken when http.DefaultTransport is not an *http.Transport.
//
// A process whose default is an instrumented or mocked RoundTripper has
// nothing to clone, and this package must build a plain transport rather
// than dereference what it found. The clone exists to inherit proxy and
// dial settings, not to make requests work, so the fallback is only about
// staying alive; what it must not lose is this package's own response
// header timeout, which is asserted here.
//
// The shared transport is realized before the swap so no other test can
// observe the foreign default through the sync.OnceValue.
func TestNewBaseTransport_ForeignDefault_StillBuildsATransport(t *testing.T) {
	_ = buildBaseTransport(false)

	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = testRoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return original.RoundTrip(r)
	})

	transport := newBaseTransport(nil)
	if transport == nil {
		t.Fatal("newBaseTransport() = nil with a foreign DefaultTransport")
	}
	if transport.ResponseHeaderTimeout != responseHeaderTimeout {
		t.Errorf("ResponseHeaderTimeout = %v, want %v", transport.ResponseHeaderTimeout, responseHeaderTimeout)
	}
	if transport.TLSClientConfig != nil {
		t.Errorf("TLSClientConfig = %+v, want none when no TLS configuration was supplied", transport.TLSClientConfig)
	}
}

// testRoundTripperFunc is a RoundTripper that is deliberately not an
// *http.Transport, which is the condition the fallback above exists for.
type testRoundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip delegates to the wrapped function.
func (f testRoundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestPoolClientConstructors_UseTheirAuthScheme verifies each pool client
// constructor authenticates every request path — the raw credential probe,
// the raw version probe, and SDK API calls — with its own scheme: the
// oauth constructor with "Authorization: Bearer" and never PRIVATE-TOKEN,
// the legacy constructor the other way round. A gloas- OAuth access token
// is only valid as Bearer; the PRIVATE-TOKEN header GitLab rejects for it
// is exactly how oauth mode silently failed for real OAuth tokens before
// the oauth constructor existed.
func TestPoolClientConstructors_UseTheirAuthScheme(t *testing.T) {
	const oauthToken = "gloas-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcd"

	tests := []struct {
		name        string
		newClient   func(string, string, bool) (*Client, error)
		token       string
		wantBearer  bool
		wantPrivate bool
	}{
		{"oauth client sends Bearer on every path", NewOAuthClientWithToken, oauthToken, true, false},
		{"legacy client keeps PRIVATE-TOKEN", NewClientWithToken, testValidToken, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type seen struct{ path, bearer, private string }
			var requests []seen
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, seen{r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("PRIVATE-TOKEN")})
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/version"):
					fmt.Fprint(w, `{"version":"18.0.0","revision":"abc"}`)
				case strings.HasSuffix(r.URL.Path, "/user"):
					fmt.Fprint(w, `{"id":7,"username":"probe-user"}`)
				default:
					fmt.Fprint(w, `{}`)
				}
			}))
			defer srv.Close()

			client, err := tt.newClient(srv.URL, tt.token, false)
			if err != nil {
				t.Fatalf("constructor error: %v", err)
			}

			ctx := context.Background()
			if got := client.CheckCredential(ctx); got != CredentialAccepted {
				t.Errorf("CheckCredential() = %v against a 200 backend, want CredentialAccepted", got)
			}
			if _, verErr := client.versionDirect(ctx); verErr != nil {
				t.Errorf("versionDirect() error: %v", verErr)
			}
			if _, _, sdkErr := client.GL().Users.CurrentUser(); sdkErr != nil {
				t.Errorf("SDK CurrentUser() error: %v", sdkErr)
			}

			if len(requests) == 0 {
				t.Fatal("no requests reached the backend")
			}
			for _, req := range requests {
				assertAuthScheme(t, req.path, req.bearer, req.private, tt.token, tt.wantBearer, tt.wantPrivate)
			}
		})
	}
}

// assertAuthScheme checks one probed request carried exactly the expected
// authentication scheme and nothing else.
func assertAuthScheme(t *testing.T, path, bearer, private, token string, wantBearer, wantPrivate bool) {
	t.Helper()
	if (bearer == "Bearer "+token) != wantBearer {
		t.Errorf("%s: Authorization = %q, want bearer=%v", path, bearer, wantBearer)
	}
	if (private == token) != wantPrivate {
		t.Errorf("%s: PRIVATE-TOKEN = %q, want private=%v", path, private, wantPrivate)
	}
}

// redirectProbeToken is the fake personal access token the redirect tests look
// for on the far side of a hop.
const redirectProbeToken = "glpat-REDIRECT-PROBE"

// redirectProbeOAuthToken is the fake OAuth access token used for the Bearer
// half of the redirect table.
const redirectProbeOAuthToken = "gloas-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcd"

// redirectProbe starts a target server that records the headers it is sent and
// an origin server that answers every request with a 302 to it. It returns the
// origin's base URL as the client should be configured with it, and an
// accessor for the headers the target saw.
//
// When crossHost is true the origin is addressed as "localhost" while both
// servers listen on 127.0.0.1, which is what makes the hop genuinely
// cross-hostname: two httptest servers are the same host to net/http however
// many ports they use, so a test that uses their own URLs proves nothing.
func redirectProbe(t *testing.T, tlsOrigin, crossHost bool) (baseURL string, seen func() http.Header) {
	t.Helper()

	var mu sync.Mutex
	var got http.Header
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"username":"probe-user","version":"18.0.0","path_with_namespace":"group/proj"}`)
	}))
	t.Cleanup(target.Close)

	// The Location header is set by hand rather than through http.Redirect so
	// that the destination, which is this test's own server, is not read as a
	// caller-controlled redirect target.
	redirector := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL+r.URL.EscapedPath())
		w.WriteHeader(http.StatusFound)
	})
	var origin *httptest.Server
	if tlsOrigin {
		origin = httptest.NewTLSServer(redirector)
	} else {
		origin = httptest.NewServer(redirector)
	}
	t.Cleanup(origin.Close)

	baseURL = origin.URL
	if crossHost {
		baseURL = strings.Replace(baseURL, "127.0.0.1", "localhost", 1)
	}
	return baseURL, func() http.Header {
		mu.Lock()
		defer mu.Unlock()
		return got
	}
}

// TestClient_CrossHostRedirect_DropsCredential verifies that a redirect which
// leaves the configured GitLab instance does not carry this server's
// credential with it, while a redirect that stays on the instance still does.
//
// It matters because net/http's own strip list covers Authorization and five
// other standard headers and cannot cover PRIVATE-TOKEN, which is the only
// credential stdio mode has and HTTP legacy mode's default. GitLab answers
// artifact, trace and package downloads with a 302 to object storage whenever
// object storage is configured, so this is a routine disclosure with no
// adversary present.
//
// The table crosses both pooled constructors with both request paths — the SDK
// client, and the raw health client, which has neither the SDK's transport
// chain nor its base-URL guard — over three hop shapes: cross-host (strip),
// same-host (keep, so the fix does not break GitLab's own redirects), and an
// https-to-http downgrade on an unchanged hostname, which net/http does not
// treat as leaving the host at all. Every case also asserts the redirect was
// still followed and the body delivered, since refusing redirects would
// "pass" this test while breaking six shipped download actions.
func TestClient_CrossHostRedirect_DropsCredential(t *testing.T) {
	constructors := []struct {
		name   string
		build  func(baseURL string, skipTLS bool) (*Client, error)
		header string
		token  string
	}{
		{
			name: "pat client",
			build: func(baseURL string, skipTLS bool) (*Client, error) {
				return NewClientWithTokenRetries(baseURL, redirectProbeToken, skipTLS, true)
			},
			header: "PRIVATE-TOKEN",
			token:  redirectProbeToken,
		},
		{
			name: "oauth client",
			build: func(baseURL string, skipTLS bool) (*Client, error) {
				return NewOAuthClientWithToken(baseURL, redirectProbeOAuthToken, skipTLS)
			},
			header: "Authorization",
			token:  "Bearer " + redirectProbeOAuthToken,
		},
	}

	hops := []struct {
		name           string
		tlsOrigin      bool
		crossHost      bool
		wantCredential bool
	}{
		{name: "cross host", crossHost: true},
		{name: "same host", wantCredential: true},
		{name: "https downgraded to http", tlsOrigin: true},
	}

	calls := []struct {
		name string
		do   func(*Client) error
	}{
		{
			name: "sdk call",
			do: func(c *Client) error {
				_, _, err := c.GL().Projects.GetProject("group/proj", nil)
				return err
			},
		},
		{
			name: "health probe",
			do: func(c *Client) error {
				if verdict := c.CheckCredential(context.Background()); verdict == CredentialRefused {
					return errors.New("CheckCredential() = CredentialRefused against a 200 backend")
				}
				return nil
			},
		},
	}

	for _, ctor := range constructors {
		for _, hop := range hops {
			for _, call := range calls {
				t.Run(ctor.name+"/"+hop.name+"/"+call.name, func(t *testing.T) {
					baseURL, seen := redirectProbe(t, hop.tlsOrigin, hop.crossHost)

					client, err := ctor.build(baseURL, hop.tlsOrigin)
					if err != nil {
						t.Fatalf("constructor error: %v", err)
					}
					if callErr := call.do(client); callErr != nil {
						t.Fatalf("call after redirect failed, so the redirect was not followed: %v", callErr)
					}

					assertRedirectTargetSaw(t, seen(), ctor.header, ctor.token, hop.wantCredential)
				})
			}
		}
	}
}

// assertRedirectTargetSaw checks what the far side of a redirect received:
// either exactly the expected credential, or none of them at all.
func assertRedirectTargetSaw(t *testing.T, headers http.Header, header, token string, wantCredential bool) {
	t.Helper()

	if headers == nil {
		t.Fatal("the redirect target was never reached")
	}
	if got := headers.Get(header); (got == token) != wantCredential {
		t.Errorf("target saw %s = %q, want present=%v", header, got, wantCredential)
	}
	if wantCredential {
		return
	}
	for _, name := range credentialHeaders {
		if got := headers.Get(name); got != "" {
			t.Errorf("target saw credential header %s = %q, want it stripped", name, got)
		}
	}
}

// TestTimeoutConstants_AreProtectionsRatherThanZero pins the two timeouts this
// package ships, against their own magnitude rather than against themselves.
//
// The transport assertions above compare the field with the constant, which is
// the same value twice: set responseHeaderTimeout to 0 and every one of them
// still passes, while the server it configures waits forever for headers an
// unreachable instance never sends. healthTimeout is unpinned the same way.
// Both were reported by mutation testing as surviving mutants, and that is
// exactly what the survival meant: a protection nothing holds to being one.
//
// The bounds are the reasons, not the numbers. A zero is what the mutation
// produces and is the absence of the protection. A ceiling is asserted too,
// because a timeout long enough to outlast a caller's patience protects
// nobody, and the doc comments already name the order of magnitude each was
// chosen against: GitLab's own worker timeout for the headers, a startup probe
// for the health check.
func TestTimeoutConstants_AreProtectionsRatherThanZero(t *testing.T) {
	for _, tc := range []struct {
		name      string
		value     time.Duration
		atLeast   time.Duration
		atMost    time.Duration
		disabling string
	}{
		{
			name:      "responseHeaderTimeout",
			value:     responseHeaderTimeout,
			atLeast:   10 * time.Second,
			atMost:    5 * time.Minute,
			disabling: "the client waits forever for headers a stalled instance never sends",
		},
		{
			name:      "healthTimeout",
			value:     healthTimeout,
			atLeast:   time.Second,
			atMost:    time.Minute,
			disabling: "the startup health probe never returns and the server never finishes starting",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value <= 0 {
				t.Fatalf("%s = %v; a non-positive value disables the timeout entirely, so %s",
					tc.name, tc.value, tc.disabling)
			}
			if tc.value < tc.atLeast {
				t.Errorf("%s = %v, want at least %v: shorter than the work it is meant to allow",
					tc.name, tc.value, tc.atLeast)
			}
			if tc.value > tc.atMost {
				t.Errorf("%s = %v, want at most %v: a bound nobody waits for is not a bound",
					tc.name, tc.value, tc.atMost)
			}
		})
	}
}

// tierCascadeServer answers the three endpoints the tier cascade asks, so a
// test can describe a whole deployment rather than one response.
//
// licenseStatus is what GET /license answers (403 stands for the non-admin
// token and for GitLab.com), namespaces is the list GET /namespaces returns,
// and enterprise is what GET /version reports, which decides only whether an
// unresolved tier is worth warning about.
func tierCascadeServer(t *testing.T, licenseStatus int, licensePlan string, namespaces []map[string]any, enterprise bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Every endpoint the cascade asks is a read. Answering a write the
		// same way would let a probe that started sending one pass unnoticed,
		// so it is refused here and reported on the test goroutine's behalf.
		if r.Method != http.MethodGet {
			t.Errorf("the tier cascade sent %s %s; every endpoint it asks is a GET", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/api/v4/version":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "19.3.1-ee", "enterprise": enterprise})
		case "/api/v4/license":
			if licenseStatus != http.StatusOK {
				w.WriteHeader(licenseStatus)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"plan": licensePlan})
		case "/api/v4/namespaces":
			if namespaces == nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(namespaces)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestDetectTier_NamespacePlan_RescuesTheNonAdminToken is the case the cascade
// was built for, measured against a real GitLab on 2026-09-22: a caller who is
// refused the instance license still administers namespaces whose plan GitLab
// does report, which is every GitLab.com account.
//
// Before this, all of them resolved Free and were served the Free catalog
// whatever they were paying for.
func TestDetectTier_NamespacePlan_RescuesTheNonAdminToken(t *testing.T) {
	cases := []struct {
		name       string
		namespaces []map[string]any
		want       edition.Tier
	}{
		{
			name:       "a premium namespace",
			namespaces: []map[string]any{{"full_path": "acme", "kind": "group", "plan": "premium"}},
			want:       edition.Premium,
		},
		{
			name:       "an ultimate namespace",
			namespaces: []map[string]any{{"full_path": "acme", "kind": "group", "plan": "ultimate"}},
			want:       edition.Ultimate,
		},
		{
			// The asymmetry ADR-0018 records for scopes: a tier resolved too
			// low silently removes tools, one resolved too high surfaces as
			// GitLab's own refusal on the one call that needed it.
			name: "a free personal namespace beside an ultimate group",
			namespaces: []map[string]any{
				{"full_path": "someone", "kind": "user", "plan": "free"},
				{"full_path": "acme", "kind": "group", "plan": "ultimate"},
			},
			want: edition.Ultimate,
		},
		{
			name: "the highest of several paid plans",
			namespaces: []map[string]any{
				{"full_path": "a", "kind": "group", "plan": "premium"},
				{"full_path": "b", "kind": "group", "plan": "ultimate"},
				{"full_path": "c", "kind": "group", "plan": "premium"},
			},
			want: edition.Ultimate,
		},
		{
			// A namespace the caller does not administer carries no plan key
			// at all, which is what GitLab.com returned for a group I am only
			// a member of.
			name: "a namespace with no plan key is skipped",
			namespaces: []map[string]any{
				{"full_path": "not-mine", "kind": "group"},
				{"full_path": "mine", "kind": "group", "plan": "premium"},
			},
			want: edition.Premium,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := tierCascadeServer(t, http.StatusForbidden, "", tc.namespaces, true)
			client, err := NewClient(newTestConfig(srv.URL, testValidToken))
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}
			if got := client.DetectTier(context.Background()); got != tc.want {
				t.Errorf("DetectTier() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDetectTier_SelfManagedNamespacePlan_IsNotEvidence pins the measurement
// that decided the cascade's shape.
//
// Self-managed reports plan "default" for every namespace whatever the instance
// is licensed for, because a subscription is a GitLab.com concept. So "default"
// must not be read as Free evidence: it means the question was not answered,
// and the caller falls through to the warning rather than to a confident Free.
func TestDetectTier_SelfManagedNamespacePlan_IsNotEvidence(t *testing.T) {
	srv := tierCascadeServer(t, http.StatusForbidden, "",
		[]map[string]any{{"full_path": "root", "kind": "user", "plan": "default"}}, true)
	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if got := client.DetectTier(context.Background()); got != edition.Free {
		t.Errorf("DetectTier() = %v, want Free: \"default\" says nothing about the instance license", got)
	}
}

// pagedNamespaceServer answers GET /namespaces a page at a time, refusing
// /license so the cascade reaches the namespace step, and records how many
// pages were asked for.
//
// pages[i] is what page i+1 returns; X-Next-Page is set while a later page
// exists, which is what the client reads to continue.
func pagedNamespaceServer(t *testing.T, pages [][]map[string]any, asked *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("the tier cascade sent %s %s; every endpoint it asks is a GET", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/api/v4/version":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "19.3.1-ee", "enterprise": true})
		case "/api/v4/license":
			w.WriteHeader(http.StatusForbidden)
		case "/api/v4/namespaces":
			page := 1
			if raw := r.URL.Query().Get("page"); raw != "" {
				parsed, err := strconv.Atoi(raw)
				if err != nil {
					t.Errorf("page parameter %q is not a number", raw)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				page = parsed
			}
			if page < 1 || page > len(pages) {
				t.Errorf("asked for page %d, which this fixture does not have", page)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			atomic.AddInt32(asked, 1)
			if page < len(pages) {
				w.Header().Set("X-Next-Page", strconv.Itoa(page+1))
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(pages[page-1])
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestDetectTier_NamespacePlan_ReadsPastTheFirstPage covers the defect one page
// left: a caller whose only paid namespace sits past the hundredth resolved
// Free, and nothing said so.
func TestDetectTier_NamespacePlan_ReadsPastTheFirstPage(t *testing.T) {
	var asked int32
	srv := pagedNamespaceServer(t, [][]map[string]any{
		{{"full_path": "free-a", "kind": "user", "plan": "free"}},
		{{"full_path": "paid-group", "kind": "group", "plan": "premium"}},
	}, &asked)

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if got := client.DetectTier(context.Background()); got != edition.Premium {
		t.Errorf("DetectTier() = %v, want Premium: the paid namespace is on the second page", got)
	}
	if asked != 2 {
		t.Errorf("pages read = %d, want 2", asked)
	}
}

// TestDetectTier_NamespacePlan_StopsAtUltimate checks the early exit: no later
// namespace can raise the answer past the highest tier there is, so the probe
// must not keep paying for pages once it has one.
func TestDetectTier_NamespacePlan_StopsAtUltimate(t *testing.T) {
	var asked int32
	srv := pagedNamespaceServer(t, [][]map[string]any{
		{{"full_path": "top", "kind": "group", "plan": "ultimate"}},
		{{"full_path": "never-read", "kind": "group", "plan": "premium"}},
	}, &asked)

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if got := client.DetectTier(context.Background()); got != edition.Ultimate {
		t.Errorf("DetectTier() = %v, want Ultimate", got)
	}
	if asked != 1 {
		t.Errorf("pages read = %d, want 1: Ultimate is the highest answer there is", asked)
	}
}

// TestDetectTier_NamespacePlan_KeepsWhatAnEarlierPageAnswered checks the
// failure path mid-walk. A page that fails after an earlier one answered must
// keep that answer: the tier found so far is a fact, and discarding it would
// resolve lower than the caller has already been shown to hold.
func TestDetectTier_NamespacePlan_KeepsWhatAnEarlierPageAnswered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("the tier cascade sent %s %s; every endpoint it asks is a GET", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/api/v4/version":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "19.3.1-ee", "enterprise": true})
		case "/api/v4/license":
			w.WriteHeader(http.StatusForbidden)
		case "/api/v4/namespaces":
			if r.URL.Query().Get("page") == "2" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("X-Next-Page", "2")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"full_path": "paid", "kind": "group", "plan": "premium"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if got := client.DetectTier(context.Background()); got != edition.Premium {
		t.Errorf("DetectTier() = %v, want Premium: the first page answered before the second failed", got)
	}
}

// TestDetectTier_NamespacePlan_StopsAtThePageBound verifies both edges of
// namespacePlanMaxPages against an account that administers more namespaces
// than the bound reads.
//
// The last page the bound allows is read, so a paid plan there is found; the
// page after it is not, so a paid plan there is not, and the probe asks for
// exactly as many pages as the bound names. Both halves matter: a walk that
// stopped one page early would resolve the first case Free, and one that
// never stopped would read every namespace an account holds, which is the cost
// the bound exists to cap.
func TestDetectTier_NamespacePlan_StopsAtThePageBound(t *testing.T) {
	cases := []struct {
		name     string
		paidPage int
		want     edition.Tier
	}{
		{name: "a paid plan on the last page the bound reads", paidPage: namespacePlanMaxPages, want: edition.Premium},
		{name: "a paid plan on the first page past the bound", paidPage: namespacePlanMaxPages + 1, want: edition.Free},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pages := make([][]map[string]any, namespacePlanMaxPages+1)
			for i := range pages {
				pages[i] = []map[string]any{{"full_path": fmt.Sprintf("free-%d", i+1), "kind": "user", "plan": "free"}}
			}
			pages[tc.paidPage-1] = []map[string]any{{"full_path": "paid-group", "kind": "group", "plan": "premium"}}
			var asked int32
			srv := pagedNamespaceServer(t, pages, &asked)

			client, err := NewClient(newTestConfig(srv.URL, testValidToken))
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}
			if got := client.DetectTier(context.Background()); got != tc.want {
				t.Errorf("DetectTier() = %v, want %v", got, tc.want)
			}
			if got := atomic.LoadInt32(&asked); got != namespacePlanMaxPages {
				t.Errorf("pages read = %d, want %d: the bound, and not one page either side of it", got, namespacePlanMaxPages)
			}
		})
	}
}

// TestTierFromNamespaces_EntriesThatAnswerNothing_AreNotEvidence verifies that
// a namespace carrying no usable plan neither decides the tier nor counts as an
// answer.
//
// The found half is the one DetectTier cannot show on its own, since an
// unanswered probe and a Free answer both resolve Free there, and it is what
// decides whether an enterprise build warns that its tier could not be read. A
// null entry is covered because GitLab's list is decoded into pointers, and a
// null in the array arrives as a nil one.
func TestTierFromNamespaces_EntriesThatAnswerNothing_AreNotEvidence(t *testing.T) {
	cases := []struct {
		name       string
		namespaces []map[string]any
		wantTier   edition.Tier
		wantFound  bool
	}{
		{
			name:       "a null entry beside a paid namespace",
			namespaces: []map[string]any{nil, {"full_path": "paid", "kind": "group", "plan": "premium"}},
			wantTier:   edition.Premium,
			wantFound:  true,
		},
		{
			name:       "only a null entry",
			namespaces: []map[string]any{nil},
			wantTier:   edition.Free,
		},
		{
			name: "only plans that answer nothing",
			namespaces: []map[string]any{
				{"full_path": "root", "kind": "user", "plan": "default"},
				{"full_path": "not-mine", "kind": "group"},
			},
			wantTier: edition.Free,
		},
		{
			name:       "a free plan is an answer",
			namespaces: []map[string]any{{"full_path": "someone", "kind": "user", "plan": "free"}},
			wantTier:   edition.Free,
			wantFound:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := tierCascadeServer(t, http.StatusForbidden, "", tc.namespaces, true)
			client, err := NewClient(newTestConfig(srv.URL, testValidToken))
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}
			tier, found := client.tierFromNamespaces(context.Background())
			if tier != tc.wantTier || found != tc.wantFound {
				t.Errorf("tierFromNamespaces() = (%v, %v), want (%v, %v)", tier, found, tc.wantTier, tc.wantFound)
			}
		})
	}
}

// TestNextNamespacePage_NoResponse_EndsTheWalk verifies the one input the SDK
// never produces and the walk must still survive: a missing response reads as
// "no next page" rather than as a nil dereference in the middle of resolving a
// tier.
func TestNextNamespacePage_NoResponse_EndsTheWalk(t *testing.T) {
	cases := []struct {
		name string
		resp *gl.Response
		want int64
	}{
		{name: "no response", resp: nil, want: 0},
		{name: "a response naming the next page", resp: &gl.Response{NextPage: 3}, want: 3},
		{name: "a response on the last page", resp: &gl.Response{}, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextNamespacePage(tc.resp); got != tc.want {
				t.Errorf("nextNamespacePage() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestDetectTier_LicenseWins checks the order: where the license is readable it
// is authoritative, and the namespace plan is not consulted to contradict it.
func TestDetectTier_LicenseWins(t *testing.T) {
	srv := tierCascadeServer(t, http.StatusOK, "ultimate",
		[]map[string]any{{"full_path": "acme", "kind": "group", "plan": "premium"}}, true)
	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if got := client.DetectTier(context.Background()); got != edition.Ultimate {
		t.Errorf("DetectTier() = %v, want Ultimate from the license rather than Premium from a namespace", got)
	}
}

// TestDetectTier_NamespacesUnreadable_FallsBackToFree covers the deployment
// where neither question can be asked, which must still resolve rather than
// fail.
func TestDetectTier_NamespacesUnreadable_FallsBackToFree(t *testing.T) {
	for _, enterprise := range []bool{true, false} {
		t.Run(map[bool]string{true: "enterprise build", false: "CE build"}[enterprise], func(t *testing.T) {
			srv := tierCascadeServer(t, http.StatusForbidden, "", nil, enterprise)
			client, err := NewClient(newTestConfig(srv.URL, testValidToken))
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}
			if got := client.DetectTier(context.Background()); got != edition.Free {
				t.Errorf("DetectTier() = %v, want Free", got)
			}
		})
	}
}

// TestDetectTier_GitLabComFreeIsAnAnswer separates the two plans that both map
// to Free and mean opposite things.
//
// Measured on 2026-09-22: GitLab.com reports "free" for a namespace on the free
// plan, and self-managed reports "default" for every namespace whatever the
// instance is licensed for. Both resolve Free, and only the second is an
// unanswered question, so only the second may warn. Reading them alike warned
// every GitLab.com account on the free plan, at every startup, about a tier
// that was correct: this test is what caught that when the cascade was first
// driven against the real gitlab.com.
func TestDetectTier_GitLabComFreeIsAnAnswer(t *testing.T) {
	cases := []struct {
		name     string
		plan     string
		answered bool
	}{
		{name: "gitlab.com free plan", plan: "free", answered: true},
		{name: "self-managed default", plan: "default", answered: false},
		{name: "no plan key", plan: "", answered: false},
		{name: "a paid plan", plan: "premium", answered: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := namespacePlanAnswers(tc.plan); got != tc.answered {
				t.Errorf("namespacePlanAnswers(%q) = %v, want %v", tc.plan, got, tc.answered)
			}
		})
	}
}

// metadataRefusalBody is GitLab's answer to GET /api/v4/version for a
// fine-grained token not granted Metadata: Read (lib/api/metadata.rb declares
// read_metadata at the instance boundary), as its API guard renders it.
const metadataRefusalBody = `{"error":"insufficient_granular_scope","error_description":"` + missingMetadataSentence + `"}`

// versionRefusingInstance is a stand-in GitLab that refuses the version
// endpoint with metadataRefusalBody while refuse holds, and answers it with
// answer otherwise, and counts every request that reached the endpoint. It
// answers the license and namespace listings as a non-administrator's token on
// a self-managed instance is answered, so a tier probe run against it resolves
// nothing and goes on to ask for the edition.
type versionRefusingInstance struct {
	url      string
	refuse   atomic.Bool
	answer   atomic.Pointer[string]
	versions atomic.Int32
}

func newVersionRefusingInstance(t *testing.T) *versionRefusingInstance {
	t.Helper()
	instance := &versionRefusingInstance{}
	instance.refuse.Store(true)
	answer := `{"version":"19.4.1-ee","enterprise":true}`
	instance.answer.Store(&answer)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v4/version":
			instance.versions.Add(1)
			if instance.refuse.Load() {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(metadataRefusalBody))
				return
			}
			_, _ = w.Write([]byte(*instance.answer.Load()))
		case "/api/v4/license":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
		case "/api/v4/namespaces":
			_, _ = w.Write([]byte(`[{"id":1,"full_path":"someone","plan":"default"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	instance.url = srv.URL
	return instance
}

// captureClientLog routes the default logger to a buffer at debug level for
// the rest of the test, and returns the buffer.
func captureClientLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(original) })
	return &buf
}

// TestInitialize_FineGrainedVersionRefusal_IsReachableWithTheVersionUnknown
// verifies the refusal a fine-grained token without Metadata: Read gets from
// the version endpoint: the instance answered and authenticated the token, so
// the client is initialized, no lazy re-initialization is armed, no version is
// returned or kept, and GitLab's sentence is kept for the caller that has to
// say so. Before, the refusal was a failure: the client re-asked on every SDK
// request once per cooldown for the life of the process, and the start never
// resolved identity or tier.
func TestInitialize_FineGrainedVersionRefusal_IsReachableWithTheVersionUnknown(t *testing.T) {
	instance := newVersionRefusingInstance(t)
	client, err := NewClient(newTestConfig(instance.url, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	version, err := client.Initialize(t.Context())
	if err != nil {
		t.Fatalf("Initialize() error = %v, want the refusal read as a reachable instance", err)
	}
	if version != "" || client.Version() != "" {
		t.Errorf("Initialize() version = %q and Version() = %q, want both empty for a version the token may not read", version, client.Version())
	}
	if !client.IsInitialized() {
		t.Error("the client is not initialized after GitLab authenticated the token")
	}
	if sentence, refused := client.VersionRefusal(); !refused || sentence != missingMetadataSentence {
		t.Errorf("VersionRefusal() = %q, %v, want GitLab's sentence, true", sentence, refused)
	}

	client.EnableLazyInit()
	client.EnsureInitialized(t.Context())
	if got := instance.versions.Load(); got != 1 {
		t.Errorf("the version endpoint was asked %d times, want once: an initialized client never asks again", got)
	}
}

// TestInitialize_OtherRefusalsOfTheVersion_StayFailures is the negative half:
// only a 403 carrying GitLab's fine-grained code is a reachable instance. A
// classic token's missing scope, a plain 403, the same code on a 401 and a
// body past the probe's bound are failures, as they were, and none of them is
// kept as a refusal.
func TestInitialize_OtherRefusalsOfTheVersion_StayFailures(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "a classic token's missing scope", status: http.StatusForbidden, body: `{"error":"insufficient_scope","error_description":"The request requires higher privileges than provided by the access token."}`},
		{name: "a plain 403", status: http.StatusForbidden, body: `{"message":"403 Forbidden"}`},
		{name: "the fine-grained code on a 401", status: http.StatusUnauthorized, body: metadataRefusalBody},
		{name: "a refusal past the probe's bound", status: http.StatusForbidden, body: `{"error":"insufficient_granular_scope","error_description":"` + strings.Repeat("x", refusalBodyBytes) + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(srv.Close)
			client, err := NewClient(newTestConfig(srv.URL, testValidToken))
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}

			if _, err = client.Initialize(t.Context()); err == nil {
				t.Error("Initialize() error = nil, want a failure")
			}
			if client.IsInitialized() {
				t.Error("the client is initialized after a refusal that is not the fine-grained one")
			}
			if _, refused := client.VersionRefusal(); refused {
				t.Error("VersionRefusal() = true for a refusal that is not the fine-grained one")
			}
		})
	}
}

// TestEnsureInitialized_FineGrainedVersionRefusal_EndsLazyInitialization
// verifies the lazy path reads the refusal the way the start does: a client
// that went into lazy re-initialization because GitLab was unreachable at
// start recovers on the refusal and stops asking.
func TestEnsureInitialized_FineGrainedVersionRefusal_EndsLazyInitialization(t *testing.T) {
	instance := newVersionRefusingInstance(t)
	client, err := NewClient(newTestConfig(instance.url, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	client.EnableLazyInit()

	client.EnsureInitialized(t.Context())
	if !client.IsInitialized() || client.needsLazyInit.Load() {
		t.Fatalf("initialized = %v, lazy re-initialization armed = %v; want true and false after the refusal",
			client.IsInitialized(), client.needsLazyInit.Load())
	}
}

// TestDetectEnterprise_VersionRefused_UsesTheFallbackWithoutAsking verifies
// edition detection for a token GitLab refuses the version endpoint: the
// configured fallback, logged at debug and never as a failed detection, and
// the endpoint asked once whichever of the two paths learned the refusal. The
// grant cannot change, so asking again only repeats the refusal.
func TestDetectEnterprise_VersionRefused_UsesTheFallbackWithoutAsking(t *testing.T) {
	tests := []struct {
		name       string
		initialize bool
	}{
		{name: "learned by the start", initialize: true},
		{name: "learned by the detection", initialize: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertEditionFallbackAskedOnce(t, tt.initialize)
		})
	}
}

// assertEditionFallbackAskedOnce runs edition detection twice for a token the
// stand-in refuses the version endpoint, after a start when initialize is
// set, and holds it to the fallback, one request and no warning.
func assertEditionFallbackAskedOnce(t *testing.T, initialize bool) {
	t.Helper()
	instance := newVersionRefusingInstance(t)
	client, err := NewClient(newTestConfig(instance.url, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if initialize {
		if _, err = client.Initialize(t.Context()); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}
	}
	logged := captureClientLog(t)

	for range 2 {
		if !client.DetectEnterprise(t.Context(), true) {
			t.Error("DetectEnterprise() = false, want the fallback true")
		}
	}
	if !client.IsEnterprise() {
		t.Error("the client does not carry the fallback edition")
	}
	if got := instance.versions.Load(); got != 1 {
		t.Errorf("the version endpoint was asked %d times, want once", got)
	}
	if strings.Contains(logged.String(), `"level":"WARN"`) {
		t.Errorf("a refusal the token's grant decides was logged as a warning:\n%s", logged)
	}
	if !strings.Contains(logged.String(), "the token may not read the instance version") {
		t.Errorf("the fallback's reason was not logged:\n%s", logged)
	}
}

// TestDetectTier_VersionRefused_ResolvesFreeWithoutCallingItCE verifies the
// tier a token GitLab refuses the version endpoint resolves when the license
// and the namespace plans answer nothing: Free, as before, with no warning
// (the start already gave one) and without a debug line claiming a CE build,
// since the edition is exactly what the token could not read. The version
// endpoint is asked once, by the start.
func TestDetectTier_VersionRefused_ResolvesFreeWithoutCallingItCE(t *testing.T) {
	instance := newVersionRefusingInstance(t)
	client, err := NewClient(newTestConfig(instance.url, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if _, err = client.Initialize(t.Context()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	logged := captureClientLog(t)

	if got := client.DetectTier(t.Context()); got != edition.Free {
		t.Errorf("DetectTier() = %v, want Free", got)
	}
	if strings.Contains(logged.String(), "on a CE instance") || strings.Contains(logged.String(), `"level":"WARN"`) {
		t.Errorf("the tier fallback called the build CE or warned:\n%s", logged)
	}
	if !strings.Contains(logged.String(), "no edition the token may read") {
		t.Errorf("the tier fallback did not say the edition was unreadable:\n%s", logged)
	}
	if got := instance.versions.Load(); got != 1 {
		t.Errorf("the version endpoint was asked %d times, want once", got)
	}
}

// TestVersionRefusal_ALaterAnswer_ClearsIt verifies that a refusal stands only
// until the version endpoint answers: GitLab's one refusal that can change
// while a process runs is fine-grained tokens not yet enabled for the user,
// and an administrator may enable them. The answer is kept as the version.
func TestVersionRefusal_ALaterAnswer_ClearsIt(t *testing.T) {
	instance := newVersionRefusingInstance(t)
	client, err := NewClient(newTestConfig(instance.url, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if _, err = client.Initialize(t.Context()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	instance.refuse.Store(false)
	version, err := client.Initialize(t.Context())
	if err != nil || version != "19.4.1-ee" {
		t.Fatalf("Initialize() = %q, %v; want the version the instance now answers", version, err)
	}
	if _, refused := client.VersionRefusal(); refused {
		t.Error("VersionRefusal() = true after the version endpoint answered")
	}
	if got := client.Version(); got != "19.4.1-ee" {
		t.Errorf("Version() = %q, want 19.4.1-ee", got)
	}
}

// TestVersion_OnlyAVersionGitLabCouldSend_IsKept verifies what the client
// keeps of the version an instance reports: three numbers and an optional
// suffix of letters, digits and dots, at most 64 bytes. Anything else is kept
// as no version at all, since the string is the instance's and a reader
// prints it and matches it against recorded releases. The 64-byte and 65-byte
// rows pin the bound from both sides.
func TestVersion_OnlyAVersionGitLabCouldSend_IsKept(t *testing.T) {
	atBound := "19.4.1-" + strings.Repeat("e", versionMaxBytes-len("19.4.1-"))
	tests := []struct {
		name     string
		reported string
		want     string
	}{
		{name: "an enterprise release", reported: "19.4.1-ee", want: "19.4.1-ee"},
		{name: "GitLab.com's pre-release", reported: "19.5.0-pre", want: "19.5.0-pre"},
		{name: "a bare release", reported: "17.0.0", want: "17.0.0"},
		{name: "a dotted suffix", reported: "19.4.1-rc1.ee", want: "19.4.1-rc1.ee"},
		{name: "at the bound", reported: atBound, want: atBound},
		{name: "past the bound", reported: atBound + "e", want: ""},
		{name: "two numbers", reported: "19.4", want: ""},
		{name: "a prefix", reported: "v19.4.1", want: ""},
		{name: "an empty suffix", reported: "19.4.1-", want: ""},
		{name: "a build suffix", reported: "19.4.1-ee+abc", want: ""},
		{name: "a trailing line", reported: "19.4.1\n", want: ""},
		{name: "prose", reported: "latest and greatest", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := newVersionRefusingInstance(t)
			reported, err := json.Marshal(map[string]string{"version": tt.reported})
			if err != nil {
				t.Fatalf("encoding the version: %v", err)
			}
			answer := string(reported)
			instance.answer.Store(&answer)
			instance.refuse.Store(false)
			client, err := NewClient(newTestConfig(instance.url, testValidToken))
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}

			if _, err = client.Initialize(t.Context()); err != nil {
				t.Fatalf("Initialize() error = %v", err)
			}
			if got := client.Version(); got != tt.want {
				t.Errorf("Version() after %q = %q, want %q", tt.reported, got, tt.want)
			}
		})
	}
}

// TestVersion_NothingRead_IsEmpty verifies the version of a client whose
// version endpoint has not answered.
func TestVersion_NothingRead_IsEmpty(t *testing.T) {
	client, err := NewClient(newTestConfig("https://gitlab.example.com", testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if got := client.Version(); got != "" {
		t.Errorf("Version() = %q before any read, want empty", got)
	}
}

// TestVersionDirect_UnexpectedAnswer_QuotesItsHeadOnly verifies what the
// version probe's error quotes of an answer it cannot use: the first 512 bytes
// of the body, as it always has, now that it reads further to recognize
// GitLab's fine-grained refusal.
func TestVersionDirect_UnexpectedAnswer_QuotesItsHeadOnly(t *testing.T) {
	head := strings.Repeat("h", versionErrorBodyBytes)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(head + "TAIL"))
	}))
	t.Cleanup(srv.Close)
	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	_, err = client.Initialize(t.Context())
	if err == nil || !strings.Contains(err.Error(), "HTTP 502: "+head) || strings.Contains(err.Error(), "TAIL") {
		t.Errorf("Initialize() error = %v, want the status and the body's first %d bytes and nothing after", err, versionErrorBodyBytes)
	}
}

// TestVersionRefusedError_NamesTheRefusalAndNotGitLabsSentence verifies the
// error the probe hands its callers for the fine-grained refusal: it says what
// happened in this server's words, and leaves GitLab's sentence, the
// instance's text, to [Client.VersionRefusal].
func TestVersionRefusedError_NamesTheRefusalAndNotGitLabsSentence(t *testing.T) {
	err := &versionRefusedError{description: missingMetadataSentence}
	if strings.Contains(err.Error(), "Metadata: Read") || !strings.Contains(err.Error(), "fine-grained permission") {
		t.Errorf("versionRefusedError.Error() = %q, want the refusal named without GitLab's sentence", err.Error())
	}
}
