// client_test.go contains unit tests for the gitlab package.
// Tests verify [NewClient] creation, [Client.Ping] connectivity checks,
// and [Client.GL] accessor using httptest to mock the GitLab Version API.

package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
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
// returns http.DefaultTransport when TLS verification is enabled, and a
// custom transport with InsecureSkipVerify when disabled.
func TestBuildBaseTransport_DefaultAndTLS(t *testing.T) {
	defTransport := buildBaseTransport(false)
	if defTransport != http.DefaultTransport {
		t.Errorf("buildBaseTransport(false) = %T, want http.DefaultTransport", defTransport)
	}

	tlsTransport := buildBaseTransport(true)
	ht, ok := tlsTransport.(*http.Transport)
	if !ok {
		t.Fatalf("buildBaseTransport(true) = %T, want *http.Transport", tlsTransport)
	}
	if !ht.TLSClientConfig.InsecureSkipVerify {
		t.Error("buildBaseTransport(true) should have InsecureSkipVerify=true")
	}
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

	_, err = client.pingDirect(context.Background())
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

	_, err = client.pingDirect(context.Background())
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

	_, err = client.pingDirect(context.Background())
	if err == nil {
		t.Error("pingDirect() expected error for malformed JSON, got nil")
	}
}

// TestNewClient_EnterpriseConfig verifies that [NewClient] respects the
// Enterprise flag from configuration.
func TestNewClient_EnterpriseConfig(t *testing.T) {
	srv := stubVersionServer(t, http.StatusOK)
	defer srv.Close()

	cfg := &config.Config{
		GitLabURL:   srv.URL,
		GitLabToken: testValidToken,
		Enterprise:  true,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	if !client.IsEnterprise() {
		t.Error("client should be enterprise when config.Enterprise=true")
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

	//nolint:staticcheck // intentionally passing nil context to trigger error path
	_, pingErr := client.pingDirect(nil) //lint:ignore SA1012 intentionally passing nil context to trigger error path
	if pingErr == nil {
		t.Fatal("expected error for nil context, got nil")
	}
}
