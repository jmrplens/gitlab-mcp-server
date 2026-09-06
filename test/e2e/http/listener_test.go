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
	"runtime"
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
	prepareForTermination(cmd)
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
	if runtime.GOOS == "windows" {
		// Windows binds AF_UNIX sockets but has no POSIX mode to put on them:
		// access follows the directory's ACL. The server says so at startup,
		// and that warning is what a Windows deployment gets instead of the
		// mode, so it is what this leg asserts.
		if !strings.Contains(srv.logs(), "not enforceable on Windows") {
			t.Errorf("the server did not warn that the socket mode is not enforceable on Windows:\n%s", srv.logs())
		}
	} else {
		if info.Mode()&fs.ModeSocket == 0 {
			t.Fatal("--http-addr with a path did not bind a socket")
		}
		// The documented default: owner and group, nobody else.
		if perm := info.Mode().Perm(); perm != 0o660 {
			t.Errorf("socket mode = %#o, want %#o", perm, 0o660)
		}
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
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits on AF_UNIX sockets; the server warns instead, which TestListener_UnixSocket_ServesMCP asserts there")
	}
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
			// A path of its own rather than one under /tmp: the assertion is
			// that nothing appears there, and a socket left by another run of
			// this test would turn that into a false failure.
			socketPath := filepath.Join(t.TempDir(), "should-never-be-created.sock")
			out, err := runServerExpectingExit(t, serverBinary(t),
				"--http", "--allow-any-gitlab-url",
				"--http-addr="+socketPath,
				"--http-socket-mode="+mode,
			)
			if err == nil {
				t.Fatalf("the server started with --http-socket-mode=%s; output:\n%s", mode, out)
			}
			if !strings.Contains(out, "--http-socket-mode") {
				t.Errorf("the refusal must name the flag; output:\n%s", out)
			}
			if _, statErr := os.Lstat(socketPath); statErr == nil {
				t.Error("a rejected mode must not leave a socket behind")
			}
		})
	}
}

// TestListener_UnixSocket_SharesOneAuthenticationBudget pins the cost of the
// socket, so a deployment serving many people can weigh it before choosing.
//
// The authentication failure budget is keyed on the caller's address, and a
// unix socket has none: every connection reports the same peer, so the ten
// failures a minute that block one address block every caller behind the
// proxy at once. Nor can the proxy hand the real one over, because
// --trusted-proxy-header is believed only from a peer that parses as an
// address and is listed in --trusted-proxies, which no socket peer does.
//
// It is asserted rather than left implicit because the alternative is
// concrete: a loopback TCP listener with --trusted-proxies=127.0.0.1 keys the
// budget on the address the proxy forwards, which is what a shared deployment
// wants. The socket stays the better answer where the callers are few or
// trusted, and this is the number that decides which case a deployment is in.
func TestListener_UnixSocket_SharesOneAuthenticationBudget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the peer of an AF_UNIX connection is reported differently on Windows; the property under test is the POSIX one")
	}
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	dir, err := os.MkdirTemp("", "sockbudget") //nolint:usetesting // a short path, as the socket-mode test explains
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socketPath := filepath.Join(dir, "m.sock")
	srv := startServerOnUnixSocket(t, socketPath,
		"--gitlab-url="+gitlab.url,
		"--trusted-proxy-header=X-Real-IP",
		"--trusted-proxies=127.0.0.1,::1",
	)

	// Each request claims a different client address, which over TCP from a
	// trusted proxy would give each its own budget.
	blocked := 0
	for i := range 15 {
		got := srv.do(t, mcpPOST(map[string]string{"X-Real-IP": "203.0.113." + strconv.Itoa(i+1)}))
		if got.status == http.StatusTooManyRequests {
			blocked = i + 1
			break
		}
		if got.status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401 or 429: %s", i+1, got.status, truncate(got.body))
		}
	}
	if blocked == 0 {
		t.Fatal("fifteen failures from fifteen claimed addresses were never cut off over a unix socket; " +
			"the budget is documented as shared there, and a deployment sizing against that would be sizing against nothing")
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

// TestListener_TLS_RotatesTheCertificateWithoutARestart is the property an
// enterprise deployment that terminates TLS on the listener depends on twice a
// year, and it can only be shown against a real process.
//
// A certificate loaded once at startup makes renewal a restart, and a restart
// on this server is not free: every call in flight is cut, every session ends,
// and the instance comes back with an empty credential pool that has to be
// refilled request by request. Behind a balancer that is a rolling deploy for
// something no configuration changed.
//
// The assertion is in three parts, because any one of them alone would pass
// for the wrong reason. The new certificate is what the listener presents,
// which is the promise. The process did not restart, read off /health's
// started_at, which is what makes it a reload rather than a supervisor being
// quick. And the connection is new, because a reload cannot be observed on a
// connection whose handshake already happened.
func TestListener_TLS_RotatesTheCertificateWithoutARestart(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	dir := t.TempDir()
	stamp := time.Now().Add(-time.Hour)
	certFile, keyFile, firstPool := writeSelfSignedCertInto(t, dir, 11, stamp)
	port := freePort(t)
	addr := "127.0.0.1:" + strconv.Itoa(port)

	srv := startServerWithClient(t, tlsClientTrusting(t, firstPool), "https://"+addr,
		"--http-addr="+addr,
		"--gitlab-url="+gitlab.url,
		"--tls-cert="+certFile,
		"--tls-key="+keyFile,
	)

	startedAt, serial := healthStartAndServedSerial(t, srv.httpClient(), "https://"+addr+"/health")
	if serial != 11 {
		t.Fatalf("the listener presented serial %d before the rotation, want 11", serial)
	}

	// The rotation: the same two paths, a different certificate. The stamp is
	// moved forward explicitly so the change is visible whatever the
	// filesystem's timestamp granularity is.
	_, _, secondPool := writeSelfSignedCertInto(t, dir, 22, stamp.Add(time.Minute))

	// A client of its own, so nothing can be answered from a connection whose
	// handshake predates the rotation.
	rotatedStartedAt, rotatedSerial := healthStartAndServedSerial(t, tlsClientTrusting(t, secondPool), "https://"+addr+"/health")
	if rotatedSerial != 22 {
		t.Errorf("the listener presented serial %d after the rotation, want 22: the new pair on disk was not picked up", rotatedSerial)
	}
	if rotatedStartedAt != startedAt {
		t.Errorf("started_at moved from %q to %q: the certificate came back through a restart, not a reload", startedAt, rotatedStartedAt)
	}
}

// TestListener_TLS_AHalfWrittenRotationKeepsServing covers the window between
// the two writes a rotation is made of.
//
// Writing a certificate and its key is not atomic, and in between the pair on
// disk does not match. A listener that refused the handshake there would turn
// every renewal into an outage as long as one file write. The certificate
// already loaded is still valid, so it is what keeps being served until the
// pair is whole.
func TestListener_TLS_AHalfWrittenRotationKeepsServing(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	dir := t.TempDir()
	stamp := time.Now().Add(-time.Hour)
	certFile, keyFile, firstPool := writeSelfSignedCertInto(t, dir, 33, stamp)
	port := freePort(t)
	addr := "127.0.0.1:" + strconv.Itoa(port)

	srv := startServerWithClient(t, tlsClientTrusting(t, firstPool), "https://"+addr,
		"--http-addr="+addr,
		"--gitlab-url="+gitlab.url,
		"--tls-cert="+certFile,
		"--tls-key="+keyFile,
	)
	_ = srv

	// Only the certificate half of a new generation lands. Its key is still
	// the old one, so the pair does not match.
	orphan, _, _ := writeSelfSignedCertInto(t, t.TempDir(), 44, time.Time{})
	orphanPEM, err := os.ReadFile(orphan)
	if err != nil {
		t.Fatalf("reading the replacement certificate: %v", err)
	}
	// Both paths were written by this test into directories it owns; the
	// taint analysis cannot see that the read path came from t.TempDir.
	if writeErr := os.WriteFile(certFile, orphanPEM, 0o600); writeErr != nil { //#nosec G703 -- both paths are this test's own temp files
		t.Fatalf("writing the mismatched certificate: %v", writeErr)
	}
	if chErr := os.Chtimes(certFile, stamp.Add(time.Minute), stamp.Add(time.Minute)); chErr != nil {
		t.Fatalf("stamping the mismatched certificate: %v", chErr)
	}

	_, serial := healthStartAndServedSerial(t, tlsClientTrusting(t, firstPool), "https://"+addr+"/health")
	if serial != 33 {
		t.Errorf("the listener presented serial %d mid-rotation, want the previous 33", serial)
	}
}

// tlsClientTrusting builds an HTTP client that trusts exactly the given pool
// and shares no connection with any other client.
func tlsClientTrusting(t *testing.T, pool *x509.CertPool) *http.Client {
	t.Helper()
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		}},
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

// healthStartAndServedSerial reads /health over TLS and returns the process
// start instant it reports together with the serial number of the certificate
// the listener presented on that connection.
//
// The two travel together because the interesting question is about both at
// once: which certificate is being served, and whether the process serving it
// is the one that was running a moment ago.
func healthStartAndServedSerial(t *testing.T, client *http.Client, url string) (startedAt string, serial int64) {
	t.Helper()

	var resp *http.Response
	deadline := time.Now().Add(30 * time.Second)
	for {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
		if err != nil {
			t.Fatalf("building the health request: %v", err)
		}
		var doErr error
		resp, doErr = client.Do(req)
		if doErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the TLS listener never answered %s: %v", url, doErr)
		}
		time.Sleep(100 * time.Millisecond)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the health body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/health = %d over TLS: %s", resp.StatusCode, truncate(string(body)))
	}
	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		t.Fatal("the response carries no peer certificate")
	}
	var health struct {
		StartedAt string `json:"started_at"`
	}
	if unmarshalErr := json.Unmarshal(body, &health); unmarshalErr != nil {
		t.Fatalf("decoding the health body %q: %v", truncate(string(body)), unmarshalErr)
	}
	return health.StartedAt, resp.TLS.PeerCertificates[0].SerialNumber.Int64()
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
	return writeSelfSignedCertInto(t, t.TempDir(), 1, time.Time{})
}

// writeSelfSignedCertInto writes a certificate pair into a directory the
// caller owns, under the serial number it names.
//
// The directory and the serial are parameters so that a rotation can be
// expressed: the same two paths rewritten with a different certificate is
// exactly what certbot, a Kubernetes secret projection and Vault's agent do.
// When stamp is non-zero both files are given that modification time, which
// keeps a rotation detectable on a filesystem whose timestamps are coarser
// than the gap between two writes in a test.
func writeSelfSignedCertInto(t *testing.T, dir string, serial int64, stamp time.Time) (certFile, keyFile string, pool *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(serial),
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
	if !stamp.IsZero() {
		for _, path := range []string{certFile, keyFile} {
			if chErr := os.Chtimes(path, stamp, stamp); chErr != nil {
				t.Fatalf("stamping %s: %v", path, chErr)
			}
		}
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
