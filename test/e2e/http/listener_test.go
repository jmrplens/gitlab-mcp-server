//go:build httpe2e

// listener_test.go drives the real binary over the two listeners a deployment
// can choose instead of a plain TCP port: a unix socket, and TLS terminated by
// the server itself.
//
// Unit tests cover the listener helpers in isolation, but nothing proved the
// FLAGS reach them — that --http-addr with a path actually binds a socket, that
// --http-socket-mode actually lands on it, that --tls-cert/--tls-key actually
// serve HTTPS. A flag wired to nothing passes every unit test in the package.
// These start the binary the way an operator does and speak to what comes up.
package httpe2e

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"io/fs"
	"math/big"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// startServerOnUnixSocket launches the binary listening on a unix socket and
// returns a server whose client dials that path.
func startServerOnUnixSocket(t *testing.T, socketPath string, flags ...string) *server {
	t.Helper()

	args := append([]string{"--http-addr=" + socketPath}, flags...)
	srv := startServerWithClient(t, socketDialingClient(socketPath), "http://unix", args...)
	return srv
}

// socketDialingClient speaks HTTP over a unix socket. The host in the URL is
// ignored by the dialer but still has to be present for the request to parse.
func socketDialingClient(socketPath string) *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
	}
}

// startServerWithClient is startServer for a listener the default client
// cannot reach, so the caller supplies both the client and the base URL.
func startServerWithClient(t *testing.T, client *http.Client, baseURL string, flags ...string) *server {
	t.Helper()

	bin := serverBinary(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, append([]string{"--http"}, withInstancePolicy(flags)...)...)
	cmd.Env = append(os.Environ(),
		"LOG_LEVEL=info",
		"TOOL_SURFACE=dynamic",
	)

	var out bytes.Buffer
	var mu sync.Mutex
	cmd.Stdout = &lockedWriter{mu: &mu, buf: &out}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("starting the server: %v", err)
	}

	srv := &server{
		baseURL: baseURL,
		client:  client,
		logs: func() string {
			mu.Lock()
			defer mu.Unlock()
			return out.String()
		},
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	waitHealthy(t, srv)
	return srv
}

// TestListener_UnixSocket_ServesMCP verifies the whole path an operator
// configures when they put the server behind a same-machine proxy: the flag
// binds a socket, the socket carries real MCP traffic, and the permission mode
// is the one that was asked for.
//
// The mode is the part that decides whether the deployment works at all. Too
// narrow and the proxy cannot connect; too wide and every local account can
// reach an endpoint whose entire point is that it is not exposed.
func TestListener_UnixSocket_ServesMCP(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	// A short path: a unix socket address is capped near 100 bytes by the
	// kernel, and t.TempDir() under a long test name can exceed it.
	dir, err := os.MkdirTemp("", "sock") //nolint:usetesting // see above
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socketPath := filepath.Join(dir, "mcp.sock")

	srv := startServerOnUnixSocket(t, socketPath, "--gitlab-url="+gitlab.url)

	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("the socket was not created: %v", err)
	}
	if info.Mode()&fs.ModeSocket == 0 {
		t.Fatal("--http-addr with a path did not bind a socket")
	}
	// The documented default: owner and group, nobody else.
	if perm := info.Mode().Perm(); perm != 0o660 {
		t.Errorf("socket mode = %#o, want %#o", perm, 0o660)
	}

	// It is a real endpoint, not just an open socket: an unauthenticated MCP
	// call gets the same considered rejection it gets over TCP.
	got := srv.do(t, mcpPOST(nil))
	if got.status != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d over the unix socket: %s", got.status, http.StatusUnauthorized, got.body)
	}
	if challenge := got.header.Get("WWW-Authenticate"); challenge == "" {
		t.Error("the 401 over a unix socket carries no challenge")
	}
	if got.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("the security headers must apply over a unix socket too")
	}
}

// TestListener_UnixSocketMode_IsHonored verifies that --http-socket-mode is
// not decoration: the value an operator passes is the value on the socket.
//
// It is read as octal the way chmod is, so "0600" must not be mistaken for
// decimal 600 — a mode nobody asked for.
func TestListener_UnixSocketMode_IsHonored(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")

	for _, tc := range []struct {
		flag string
		want fs.FileMode
	}{
		{flag: "0600", want: 0o600},
		{flag: "660", want: 0o660}, // no leading zero, still octal
		{flag: "0640", want: 0o640},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "sockmode") //nolint:usetesting // short path, see above
			if err != nil {
				t.Fatalf("temp dir: %v", err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			socketPath := filepath.Join(dir, "m.sock")

			startServerOnUnixSocket(t, socketPath,
				"--gitlab-url="+gitlab.url,
				"--http-socket-mode="+tc.flag,
			)

			info, statErr := os.Stat(socketPath)
			if statErr != nil {
				t.Fatalf("stat socket: %v", statErr)
			}
			if perm := info.Mode().Perm(); perm != tc.want {
				t.Errorf("--http-socket-mode=%s produced %#o, want %#o", tc.flag, perm, tc.want)
			}
		})
	}
}

// TestListener_InvalidSocketMode_StopsStartup verifies the defensive half: a
// mode the server cannot honor stops it rather than leaving a socket at
// whatever the umask decided.
func TestListener_InvalidSocketMode_StopsStartup(t *testing.T) {
	for _, mode := range []string{"rw-rw----", "0899", "0", "7777"} {
		t.Run(mode, func(t *testing.T) {
			out, err := runServerExpectingExit(t, serverBinary(t),
				"--http", "--allow-any-gitlab-url",
				"--http-addr=/tmp/should-never-be-created.sock",
				"--http-socket-mode="+mode,
			)
			if err == nil {
				t.Fatalf("the server started with --http-socket-mode=%s; output:\n%s", mode, out)
			}
			if !strings.Contains(out, "--http-socket-mode") {
				t.Errorf("the refusal must name the flag; output:\n%s", out)
			}
			if _, statErr := os.Lstat("/tmp/should-never-be-created.sock"); statErr == nil {
				t.Error("a rejected mode must not leave a socket behind")
			}
		})
	}
}

// TestListener_TLS_ServesHTTPS verifies that --tls-cert/--tls-key terminate TLS
// on the listener itself, which is what a deployment whose proxy sits on
// another machine depends on.
func TestListener_TLS_ServesHTTPS(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	certFile, keyFile, pool := writeSelfSignedCert(t)
	port := freePort(t)
	addr := "127.0.0.1:" + strconv.Itoa(port)

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		}},
	}
	srv := startServerWithClient(t, client, "https://"+addr,
		"--http-addr="+addr,
		"--gitlab-url="+gitlab.url,
		"--tls-cert="+certFile,
		"--tls-key="+keyFile,
	)

	// The negotiated version, asserted rather than assumed. MinVersion sets a
	// FLOOR, not a ceiling: with no MaxVersion, Go negotiates the highest it
	// supports, so a modern client gets TLS 1.3 and only an old one falls back
	// to 1.2. Pinning it here means a future change that caps the version — or
	// one that lets an obsolete version back in — cannot pass unnoticed.
	assertNegotiatedTLS(t, client, "https://"+addr+"/health")

	// Reaching /health at all proves the handshake completed — waitHealthy
	// already did it — so this asserts the endpoint behaves over TLS.
	got := srv.do(t, mcpPOST(nil))
	if got.status != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d over TLS: %s", got.status, http.StatusUnauthorized, got.body)
	}

	// A plaintext client must not be SERVED by a TLS listener. Go answers it
	// 400 with "an HTTP request was sent to an HTTPS server" rather than
	// dropping the connection, so the assertion is that the endpoint is not
	// reachable in cleartext — not that the dial fails.
	plainReq, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/health", http.NoBody)
	if err != nil {
		t.Fatalf("building the plaintext request: %v", err)
	}
	resp, plainErr := (&http.Client{Timeout: 5 * time.Second}).Do(plainReq)
	if plainErr != nil {
		return // connection refused is also an acceptable refusal
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("/health was served over cleartext by a TLS listener")
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "HTTPS") {
		t.Errorf("a cleartext request got %d without explaining that the listener is TLS: %s", resp.StatusCode, truncate(string(body)))
	}
}

// TestListener_HalfConfiguredTLS_StopsStartup verifies that a certificate
// without its key — a deployment that believes it is encrypting and is not —
// fails at startup rather than at the first handshake nobody is watching.
func TestListener_HalfConfiguredTLS_StopsStartup(t *testing.T) {
	certFile, keyFile, _ := writeSelfSignedCert(t)

	for _, tc := range []struct {
		name  string
		flags []string
		want  string
	}{
		{name: "cert without key", flags: []string{"--tls-cert=" + certFile}, want: "--tls-cert requires --tls-key"},
		{name: "key without cert", flags: []string{"--tls-key=" + keyFile}, want: "--tls-key requires --tls-cert"},
		{name: "cert that does not exist", flags: []string{"--tls-cert=/nonexistent.pem", "--tls-key=" + keyFile}, want: "loading the TLS certificate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--http", "--allow-any-gitlab-url", "--http-addr=127.0.0.1:0"}, tc.flags...)
			out, err := runServerExpectingExit(t, serverBinary(t), args...)
			if err == nil {
				t.Fatalf("the server started with a half-configured TLS pair; output:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("output does not explain the refusal (%q); got:\n%s", tc.want, out)
			}
		})
	}
}

// TestServerCard_LegacyModeDeclaresHeaderToken verifies the other branch of the
// card's authentication block.
//
// The oauth branch is covered; this one is what a legacy deployment publishes,
// and it is the branch that was WRONG before this work — the card declared
// header-token whatever the mode was. A test for only the fixed branch would
// not notice the field going back to a constant.
func TestServerCard_LegacyModeDeclaresHeaderToken(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	got := srv.do(t, request{method: http.MethodGet, path: "/server-card"})
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, want %d", got.status, http.StatusOK)
	}

	var card struct {
		Authentication struct {
			Required         bool     `json:"required"`
			Schemes          []string `json:"schemes"`
			ResourceMetadata string   `json:"resourceMetadata"`
		} `json:"authentication"`
	}
	if err := json.Unmarshal([]byte(got.body), &card); err != nil {
		t.Fatalf("card is not JSON: %v", err)
	}
	if len(card.Authentication.Schemes) != 1 || card.Authentication.Schemes[0] != "header-token" {
		t.Errorf("schemes = %v, want [header-token] in legacy mode", card.Authentication.Schemes)
	}
	if !card.Authentication.Required {
		t.Error("required = false; legacy mode still demands a credential")
	}
	// No RFC 9728 document exists in legacy mode, so pointing at one would
	// send a client into a discovery flow that cannot complete.
	if card.Authentication.ResourceMetadata != "" {
		t.Errorf("resourceMetadata = %q, want none in legacy mode", card.Authentication.ResourceMetadata)
	}
}

// assertNegotiatedTLS records which TLS version a modern client actually gets,
// and refuses anything below 1.2.
func assertNegotiatedTLS(t *testing.T, client *http.Client, url string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("building the TLS probe request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("TLS probe failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.TLS == nil {
		t.Fatal("the response carries no TLS state; the listener is not serving TLS")
	}
	switch resp.TLS.Version {
	case tls.VersionTLS13:
		// What a current client should get.
	case tls.VersionTLS12:
		t.Errorf("negotiated TLS 1.2 with a client that offers 1.3; the server is capping the version")
	default:
		t.Errorf("negotiated TLS version %#x, want 1.3 (or at minimum 1.2)", resp.TLS.Version)
	}
}

// TestListener_TLS_RefusesObsoleteVersions verifies the floor: a client that
// will not go above TLS 1.1 is refused rather than served.
//
// MinVersion is the only thing standing between this listener and a downgrade,
// and a deployment that turned TLS on to satisfy an auditor should be able to
// rely on it.
func TestListener_TLS_RefusesObsoleteVersions(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	certFile, keyFile, pool := writeSelfSignedCert(t)
	port := freePort(t)
	addr := "127.0.0.1:" + strconv.Itoa(port)

	modern := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
	}}
	startServerWithClient(t, modern, "https://"+addr,
		"--http-addr="+addr,
		"--gitlab-url="+gitlab.url,
		"--tls-cert="+certFile,
		"--tls-key="+keyFile,
	)

	obsolete := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{ //#nosec G402 -- deliberately obsolete: this client exists to prove the server refuses TLS below 1.2
			RootCAs:    pool,
			MinVersion: tls.VersionTLS10,
			MaxVersion: tls.VersionTLS11,
		},
	}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+addr+"/health", http.NoBody)
	if err != nil {
		t.Fatalf("building the obsolete-client request: %v", err)
	}
	resp, err := obsolete.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Errorf("a TLS 1.1 client was served (status %d); the version floor is not enforced", resp.StatusCode)
	}
}

// writeSelfSignedCert generates a certificate for 127.0.0.1, writes the PEM
// pair into the test's temp dir, and returns their paths plus a pool that
// trusts it.
func writeSelfSignedCert(t *testing.T) (certFile, keyFile string, pool *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "gitlab-mcp-server e2e"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
		DNSNames:              []string{"localhost"},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating the certificate: %v", err)
	}

	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if writeErr := os.WriteFile(certFile, certPEM, 0o600); writeErr != nil {
		t.Fatalf("writing the certificate: %v", writeErr)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshaling the key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if writeErr := os.WriteFile(keyFile, keyPEM, 0o600); writeErr != nil {
		t.Fatalf("writing the key: %v", writeErr)
	}

	pool = x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		t.Fatal("the generated certificate was not accepted into a pool")
	}
	return certFile, keyFile, pool
}

// TestServerCard_MediaTypeMatchesThePath pins what each card location answers
// with.
//
// The extension registers `application/mcp-server-card+json`, and the path it
// recommends serves it. The legacy `.well-known` location keeps
// `application/json`: its own `.json` suffix promises that, and the scanners
// still fetching it were written against the draft that used it.
//
// Both paths used to answer `application/json`, which the documentation
// contradicted. The public deployment looked correct only because a reverse
// proxy in front of it rewrote the header — and this server is meant to be
// correct on its own, without one.
func TestServerCard_MediaTypeMatchesThePath(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	tests := []struct {
		path string
		want string
	}{
		{path: "/server-card", want: "application/mcp-server-card+json"},
		{path: "/.well-known/mcp/server-card.json", want: "application/json"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := srv.do(t, request{method: http.MethodGet, path: tt.path})
			if got.status != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", got.status, http.StatusOK, got.body)
			}
			mediaType, _, err := mime.ParseMediaType(got.header.Get("Content-Type"))
			if err != nil {
				t.Fatalf("Content-Type %q: %v", got.header.Get("Content-Type"), err)
			}
			if mediaType != tt.want {
				t.Errorf("Content-Type = %q, want %q", mediaType, tt.want)
			}
		})
	}
}
