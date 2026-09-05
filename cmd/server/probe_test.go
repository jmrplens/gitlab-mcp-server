// probe_test.go covers --probe: how a peer's command line is read, where the
// probe connects for each listener a deployment can choose, and what exit code
// each outcome earns.
package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// TestParseListenerFlags_ReadsEverySpelling covers the flag spellings the flag
// package accepts, interleaved with flags the probe does not care about, which
// must be skipped rather than refused: the command line belongs to a process
// that already parsed it.
func TestParseListenerFlags_ReadsEverySpelling(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want listenerFlags
	}{
		{name: "nothing", args: nil, want: listenerFlags{addr: ":8080"}},
		{name: "the image's command", args: []string{"--transport", "auto", "--http-addr", "0.0.0.0:8080"}, want: listenerFlags{addr: "0.0.0.0:8080", transport: "auto"}},
		{name: "equals spellings", args: []string{"--http-addr=:9090", "--tls-cert=/c.pem", "--transport=http"}, want: listenerFlags{addr: ":9090", tlsCert: "/c.pem", transport: "http"}},
		{name: "single dash", args: []string{"-http", "-http-addr", "/run/mcp.sock"}, want: listenerFlags{addr: "/run/mcp.sock", http: true, httpSet: true}},
		{name: "http=false", args: []string{"--http=false"}, want: listenerFlags{addr: ":8080", httpSet: true}},
		{name: "unknown flags and positionals between", args: []string{"--gitlab-url", "https://gitlab.example.com", "extra", "--read-only", "--http-addr=:1234", "--"}, want: listenerFlags{addr: ":1234"}},
		{name: "a value missing at the end", args: []string{"--http", "--http-addr"}, want: listenerFlags{addr: "", http: true, httpSet: true}},
		{name: "a bare -- ends the scan", args: []string{"--http-addr=:9090", "--", "--http", "--http-addr=:1234"}, want: listenerFlags{addr: ":9090"}},
		{name: "everything after -- is positional", args: []string{"--", "--http"}, want: listenerFlags{addr: ":8080"}},
		{name: "a probe is not a server", args: []string{"--probe"}, want: listenerFlags{addr: ":8080", utility: true}},
		{name: "a shutdown is not a server", args: []string{"-shutdown"}, want: listenerFlags{addr: ":8080", utility: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := parseListenerFlags(tc.args); got != tc.want {
				t.Errorf("parseListenerFlags(%q) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}

// TestPeerServesHTTP_DecidesLikeResolveTransport pins the decision to the one
// the server itself makes, including what --transport auto rests on.
func TestPeerServesHTTP_DecidesLikeResolveTransport(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		flags      listenerFlags
		stdinNull  bool
		stdinErr   error
		wantHTTP   bool
		wantReason string
	}{
		{name: "no flag means stdio", flags: listenerFlags{}, wantHTTP: false, wantReason: "no transport flag"},
		{name: "--http", flags: listenerFlags{http: true, httpSet: true}, wantHTTP: true, wantReason: "--http"},
		{name: "--http=false", flags: listenerFlags{httpSet: true}, wantHTTP: false, wantReason: "--http=false"},
		{name: "--transport=http", flags: listenerFlags{transport: "http"}, wantHTTP: true, wantReason: "--transport=http"},
		{name: "--transport=stdio wins over --http", flags: listenerFlags{transport: "stdio", http: true, httpSet: true}, wantHTTP: false, wantReason: "--transport=stdio"},
		{name: "auto with stdin on the null device", flags: listenerFlags{transport: "auto"}, stdinNull: true, wantHTTP: true, wantReason: os.DevNull},
		{name: "auto with a pipe on stdin", flags: listenerFlags{transport: "auto"}, stdinNull: false, wantHTTP: false, wantReason: "is not " + os.DevNull},
		{name: "auto with an unreadable stdin assumes HTTP", flags: listenerFlags{transport: "auto"}, stdinErr: errors.New("no procfs"), wantHTTP: true, wantReason: "HTTP is assumed"},
		{name: "auto spelled loudly", flags: listenerFlags{transport: " AUTO "}, stdinNull: true, wantHTTP: true, wantReason: os.DevNull},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, why := peerServesHTTP(tc.flags, func() (bool, error) { return tc.stdinNull, tc.stdinErr })
			if got != tc.wantHTTP {
				t.Errorf("peerServesHTTP(%+v) = %v, want %v (%s)", tc.flags, got, tc.wantHTTP, why)
			}
			if !strings.Contains(why, tc.wantReason) {
				t.Errorf("reason %q does not mention %q", why, tc.wantReason)
			}
		})
	}
}

// TestProbeTargetFor_FollowsTheListener covers the three listeners a
// deployment can choose and the loopback substitution for an unspecified host.
func TestProbeTargetFor_FollowsTheListener(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		addr    string
		tlsCert string
		want    string
	}{
		{name: "the default", addr: ":8080", want: "http://127.0.0.1:8080/health"},
		{name: "the image's command", addr: "0.0.0.0:8080", want: "http://127.0.0.1:8080/health"},
		{name: "IPv6 unspecified", addr: "[::]:9090", want: "http://127.0.0.1:9090/health"},
		{name: "another port on a named host", addr: "127.0.0.1:9090", want: "http://127.0.0.1:9090/health"},
		{name: "a loopback IPv6 host", addr: "[::1]:9090", want: "http://[::1]:9090/health"},
		{name: "TLS", addr: ":8443", tlsCert: "/etc/ssl/mcp.crt", want: "https://127.0.0.1:8443/health"},
		{name: "a unix socket", addr: "/run/gitlab-mcp/server.sock", want: "unix:/run/gitlab-mcp/server.sock/health"},
		{name: "a unix socket ignores TLS", addr: "/run/gitlab-mcp/server.sock", tlsCert: "/c.pem", want: "unix:/run/gitlab-mcp/server.sock/health"},
		{name: "an address without a port is passed through", addr: "localhost", want: "http://localhost/health"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := probeTargetFor(tc.addr, tc.tlsCert).String(); got != tc.want {
				t.Errorf("probeTargetFor(%q, %q) = %s, want %s", tc.addr, tc.tlsCert, got, tc.want)
			}
		})
	}
}

// TestParseProbeTarget_AcceptsTheDocumentedForms covers the explicit target:
// URLs with and without a path, unix sockets in both spellings, host:port,
// and the refusals.
func TestParseProbeTarget_AcceptsTheDocumentedForms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "http URL", in: "http://127.0.0.1:9090", want: "http://127.0.0.1:9090/health"},
		{name: "https URL with its own path", in: "https://mcp.internal:8443/gitlab/health", want: "https://mcp.internal:8443/gitlab/health"},
		{name: "unix prefix", in: "unix:/run/mcp.sock", want: "unix:/run/mcp.sock/health"},
		{name: "bare socket path", in: "/run/mcp.sock", want: "unix:/run/mcp.sock/health"},
		{name: "host:port", in: "0.0.0.0:9090", want: "http://127.0.0.1:9090/health"},
		{name: "surrounding whitespace", in: "  :9090 ", want: "http://127.0.0.1:9090/health"},
		{name: "empty", in: "", wantErr: true},
		{name: "a URL without a host", in: "http://", wantErr: true},
		{name: "a bare word", in: "localhost", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseProbeTarget(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseProbeTarget(%q) = %s, want an error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseProbeTarget(%q): %v", tc.in, err)
			}
			if got.String() != tc.want {
				t.Errorf("parseProbeTarget(%q) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

// TestStdinIsNullUnder_ReadsTheDescriptorLink covers the procfs read with a
// fake root: a link to the null device, a link elsewhere, and no process.
func TestStdinIsNullUnder_ReadsTheDescriptorLink(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks to the null device are a Unix shape")
	}
	root := t.TempDir()
	mk := func(pid, target string) {
		dir := filepath.Join(root, pid, "fd")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, "0")); err != nil {
			t.Fatal(err)
		}
	}
	mk("10", os.DevNull)
	mk("11", "pipe:[12345]")

	cases := []struct {
		name    string
		pid     int32
		want    bool
		wantErr bool
	}{
		{name: "the null device", pid: 10, want: true},
		{name: "a pipe", pid: 11, want: false},
		{name: "no such process", pid: 12, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := stdinIsNullUnder(root, tc.pid)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("stdinIsNullUnder(%d) = %v, want an error", tc.pid, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("stdinIsNullUnder(%d): %v", tc.pid, err)
			}
			if got != tc.want {
				t.Errorf("stdinIsNullUnder(%d) = %v, want %v", tc.pid, got, tc.want)
			}
		})
	}
}

// healthMux serves /health with the given status, which is all a probe reads.
func healthMux(status int) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}

// serveOnUnixSocket serves the handler on a socket under the test's temp dir
// and returns the path.
func serveOnUnixSocket(t *testing.T, handler http.Handler) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets are not exercised on Windows")
	}
	// A short directory: socket paths are limited to about a hundred bytes.
	dir, err := os.MkdirTemp("", "probe") //nolint:usetesting // short path, see above
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "s.sock")
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", path)
	if err != nil {
		t.Fatalf("listening on %s: %v", path, err)
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: probeTimeout}
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() { _ = srv.Close() })
	return path
}

// TestRunProbe_ExplicitTarget covers a target given on the command line
// against each kind of listener, plus the answers a probe must not take for
// healthy and the exit code a malformed target earns.
func TestRunProbe_ExplicitTarget(t *testing.T) {
	t.Parallel()

	healthy := httptest.NewServer(healthMux(http.StatusOK))
	t.Cleanup(healthy.Close)
	unhealthy := httptest.NewServer(healthMux(http.StatusServiceUnavailable))
	t.Cleanup(unhealthy.Close)
	tlsServer := httptest.NewTLSServer(healthMux(http.StatusOK))
	t.Cleanup(tlsServer.Close)
	closed := httptest.NewServer(healthMux(http.StatusOK))
	closedURL := closed.URL
	closed.Close()

	cases := []struct {
		name     string
		target   string
		wantCode int
		wantSaid string
	}{
		{name: "plain HTTP", target: healthy.URL, wantCode: probeHealthy, wantSaid: "answered"},
		{name: "host:port", target: strings.TrimPrefix(healthy.URL, "http://"), wantCode: probeHealthy, wantSaid: "answered"},
		{name: "TLS without a pin gets the standard verification, which a self-signed listener fails", target: tlsServer.URL, wantCode: probeUnhealthy, wantSaid: "x509"},
		{name: "a 503 is not healthy", target: unhealthy.URL, wantCode: probeUnhealthy, wantSaid: "503"},
		{name: "nothing listening", target: closedURL, wantCode: probeUnhealthy, wantSaid: "refused"},
		{name: "a target that does not parse", target: "not a target", wantCode: probeUsage, wantSaid: "not a URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var said bytes.Buffer
			code := runProbe(t.Context(), []string{tc.target}, "", probeDeps{}, &said)
			if code != tc.wantCode {
				t.Errorf("runProbe(%q) = %d, want %d: %s", tc.target, code, tc.wantCode, said.String())
			}
			if !strings.Contains(said.String(), tc.wantSaid) {
				t.Errorf("runProbe(%q) said %q, want it to mention %q", tc.target, said.String(), tc.wantSaid)
			}
		})
	}
}

// writeCertPEM writes a certificate as a PEM file the way --tls-cert expects
// one, and returns the path.
func writeCertPEM(t *testing.T, cert *x509.Certificate) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cert.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// freshSelfSignedCert mints a certificate nobody serves.
func freshSelfSignedCert(t *testing.T) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "somebody else"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

// TestRunProbe_TLSPinnedToTheCertificate covers how an https listener is
// verified: against the certificate --tls-cert names as the only trusted
// root, under a name that certificate carries, which is what makes a
// self-signed certificate on a loopback address probeable without trusting
// whatever answers there.
func TestRunProbe_TLSPinnedToTheCertificate(t *testing.T) {
	t.Parallel()

	tlsServer := httptest.NewTLSServer(healthMux(http.StatusOK))
	t.Cleanup(tlsServer.Close)
	pin := writeCertPEM(t, tlsServer.Certificate())
	// Every httptest TLS server presents the same built-in certificate, so a
	// certificate that is genuinely somebody else's has to be minted.
	wrongPin := writeCertPEM(t, freshSelfSignedCert(t))
	noCert := filepath.Join(t.TempDir(), "empty.pem")
	if err := os.WriteFile(noCert, []byte("-----BEGIN PRIVATE KEY-----\nAA==\n-----END PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	garbage := filepath.Join(t.TempDir(), "garbage.pem")
	if err := os.WriteFile(garbage, []byte("-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	addr := strings.TrimPrefix(tlsServer.URL, "https://")

	cases := []struct {
		name     string
		args     []string
		certFile string
		peers    []probePeer
		wantCode int
		wantSaid string
	}{
		{name: "a given target with the listener's certificate", args: []string{tlsServer.URL}, certFile: pin, wantCode: probeHealthy, wantSaid: "answered"},
		{name: "a given target with somebody else's certificate", args: []string{tlsServer.URL}, certFile: wrongPin, wantCode: probeUnhealthy, wantSaid: "x509"},
		{name: "a file holding no certificate", args: []string{tlsServer.URL}, certFile: noCert, wantCode: probeUnhealthy, wantSaid: "no CERTIFICATE"},
		{name: "a file holding something that is not a certificate", args: []string{tlsServer.URL}, certFile: garbage, wantCode: probeUnhealthy, wantSaid: "parsing the certificate"},
		{name: "a file that is not there", args: []string{tlsServer.URL}, certFile: filepath.Join(t.TempDir(), "missing.pem"), wantCode: probeUnhealthy, wantSaid: "reading the certificate"},
		{name: "a plain http target ignores the pin", args: []string{"http://" + addr}, certFile: pin, wantCode: probeUnhealthy, wantSaid: "http://" + addr},
		{
			name:     "a discovered TLS instance is pinned to its own --tls-cert",
			peers:    []probePeer{{pid: 20, args: []string{"gitlab-mcp-server", "--http", "--http-addr=" + addr, "--tls-cert=" + pin, "--tls-key=/k.pem"}}},
			wantCode: probeHealthy, wantSaid: "https://" + addr,
		},
		{
			name:     "a discovered TLS instance whose file does not match is refused",
			peers:    []probePeer{{pid: 20, args: []string{"gitlab-mcp-server", "--http", "--http-addr=" + addr, "--tls-cert=" + wrongPin}}},
			wantCode: probeUnhealthy, wantSaid: "x509",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			deps := probeDeps{
				peers:       func() ([]probePeer, error) { return tc.peers, nil },
				stdinIsNull: func(int32) (bool, error) { return false, errors.New("not in this test") },
			}
			var said bytes.Buffer
			code := runProbe(t.Context(), tc.args, tc.certFile, deps, &said)
			if code != tc.wantCode {
				t.Errorf("runProbe(%q) = %d, want %d: %s", tc.args, code, tc.wantCode, said.String())
			}
			if !strings.Contains(said.String(), tc.wantSaid) {
				t.Errorf("runProbe(%q) said %q, want it to mention %q", tc.args, said.String(), tc.wantSaid)
			}
		})
	}
}

// TestRunProbe_UnixSocketTarget verifies a socket path is dialed as one, in
// both spellings.
func TestRunProbe_UnixSocketTarget(t *testing.T) {
	t.Parallel()
	path := serveOnUnixSocket(t, healthMux(http.StatusOK))
	for _, target := range []string{path, "unix:" + path} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			var said bytes.Buffer
			if code := runProbe(t.Context(), []string{target}, "", probeDeps{}, &said); code != probeHealthy {
				t.Errorf("runProbe(%q) = %d, want %d: %s", target, code, probeHealthy, said.String())
			}
		})
	}
}

// TestRunProbe_Discovery covers the bare --probe, with the process list and
// the stdin reads injected: which peer is chosen, what a stdio peer means,
// what an unreachable listener means, and what no peer at all means.
func TestRunProbe_Discovery(t *testing.T) {
	t.Parallel()

	healthy := httptest.NewServer(healthMux(http.StatusOK))
	t.Cleanup(healthy.Close)
	healthyAddr := strings.TrimPrefix(healthy.URL, "http://")
	closed := httptest.NewServer(healthMux(http.StatusOK))
	closedAddr := strings.TrimPrefix(closed.URL, "http://")
	closed.Close()

	server := func(pid int32, args ...string) probePeer {
		return probePeer{pid: pid, args: append([]string{"gitlab-mcp-server"}, args...)}
	}
	cases := []struct {
		name      string
		peers     []probePeer
		peersErr  error
		stdinNull map[int32]bool
		wantCode  int
		wantSaid  string
	}{
		{name: "no instance", wantCode: probeUnhealthy, wantSaid: "no running instance"},
		{name: "the process list cannot be read", peersErr: errors.New("permission denied"), wantCode: probeUnhealthy, wantSaid: "listing processes"},
		{name: "an HTTP instance that answers", peers: []probePeer{server(20, "--http", "--http-addr="+healthyAddr)}, wantCode: probeHealthy, wantSaid: "pid 20"},
		{name: "the image's command with stdin on the null device", peers: []probePeer{server(20, "--transport", "auto", "--http-addr", healthyAddr)}, stdinNull: map[int32]bool{20: true}, wantCode: probeHealthy, wantSaid: "answered"},
		{name: "the image's command run by a client over stdio", peers: []probePeer{server(20, "--transport", "auto", "--http-addr", "0.0.0.0:8080")}, stdinNull: map[int32]bool{20: false}, wantCode: probeHealthy, wantSaid: "serves stdio"},
		{name: "an HTTP instance nobody answers for", peers: []probePeer{server(20, "--http", "--http-addr="+closedAddr)}, wantCode: probeUnhealthy, wantSaid: "pid 20"},
		{name: "the lowest pid is tried first and a later one rescues", peers: []probePeer{server(30, "--http", "--http-addr="+healthyAddr), server(20, "--http", "--http-addr="+closedAddr)}, wantCode: probeHealthy, wantSaid: "pid 30"},
		{name: "other probes and shutdowns are not instances", peers: []probePeer{server(40, "--probe"), server(41, "--shutdown")}, wantCode: probeUnhealthy, wantSaid: "no running instance"},
		{name: "a peer with an empty command line is skipped", peers: []probePeer{{pid: 50}}, wantCode: probeUnhealthy, wantSaid: "no running instance"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			deps := probeDeps{
				peers: func() ([]probePeer, error) { return tc.peers, tc.peersErr },
				stdinIsNull: func(pid int32) (bool, error) {
					isNull, known := tc.stdinNull[pid]
					if !known {
						return false, errors.New("not in this test")
					}
					return isNull, nil
				},
			}
			var said bytes.Buffer
			code := runProbe(t.Context(), nil, "", deps, &said)
			if code != tc.wantCode {
				t.Errorf("runProbe() = %d, want %d: %s", code, tc.wantCode, said.String())
			}
			if !strings.Contains(said.String(), tc.wantSaid) {
				t.Errorf("runProbe() said %q, want it to mention %q", said.String(), tc.wantSaid)
			}
		})
	}
}

// TestRunProbe_UnansweredPeers_StayInsideTheHealthCheckBudget pins the bound
// on a whole run.
//
// Attempts are sequential and each has its own three-second ceiling, so two
// listeners that accept and never answer would spend six seconds between
// them: past the image's five-second HEALTHCHECK, which kills the probe and
// reports the container unhealthy without any verdict of its own. The run
// carries one deadline, so it answers instead.
func TestRunProbe_UnansweredPeers_StayInsideTheHealthCheckBudget(t *testing.T) {
	t.Parallel()

	blocked := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(blocked.Close)
	addr := strings.TrimPrefix(blocked.URL, "http://")
	peer := func(pid int32) probePeer {
		return probePeer{pid: pid, args: []string{"gitlab-mcp-server", "--http", "--http-addr=" + addr}}
	}
	deps := probeDeps{
		peers:       func() ([]probePeer, error) { return []probePeer{peer(20), peer(21)}, nil },
		stdinIsNull: func(int32) (bool, error) { return false, errors.New("not in this test") },
	}

	var said bytes.Buffer
	start := time.Now()
	code := runProbe(context.Background(), nil, "", deps, &said)
	elapsed := time.Since(start)

	if code != probeUnhealthy {
		t.Errorf("runProbe() = %d, want %d: %s", code, probeUnhealthy, said.String())
	}
	if elapsed >= 5*time.Second {
		t.Errorf("two unanswered peers took %s, which the image's five-second HEALTHCHECK would kill", elapsed)
	}
}

// TestRunProbe_CallersDeadlineIsKept verifies a caller that imposed a tighter
// deadline than the run's own budget keeps it.
func TestRunProbe_CallersDeadlineIsKept(t *testing.T) {
	t.Parallel()

	blocked := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(blocked.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	var said bytes.Buffer
	start := time.Now()
	code := runProbe(ctx, []string{blocked.URL}, "", probeDeps{}, &said)
	elapsed := time.Since(start)

	if code != probeUnhealthy {
		t.Errorf("runProbe() = %d, want %d: %s", code, probeUnhealthy, said.String())
	}
	if elapsed >= probeBudget {
		t.Errorf("the run took %s, want it inside the caller's 300ms deadline rather than the %s budget", elapsed, probeBudget)
	}
}

// TestRunProbe_CancelledContext verifies the probe honors its context rather
// than waiting out the timeout.
func TestRunProbe_CancelledContext(t *testing.T) {
	t.Parallel()
	blocked := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(blocked.Close)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var said bytes.Buffer
	if code := runProbe(ctx, []string{blocked.URL}, "", probeDeps{}, &said); code != probeUnhealthy {
		t.Errorf("runProbe() = %d with a cancelled context, want %d: %s", code, probeUnhealthy, said.String())
	}
}

// TestLivePeers_ExcludesThisProcess verifies the process listing the probe
// uses never returns the probe itself, whatever else is running.
func TestLivePeers_ExcludesThisProcess(t *testing.T) {
	t.Parallel()
	peers, err := livePeers()
	if err != nil {
		t.Skipf("the process list is not readable here: %v", err)
	}
	self := int32(os.Getpid()) //nolint:gosec // a pid fits
	for _, p := range peers {
		if p.pid == self {
			t.Errorf("livePeers listed this process (pid %d)", self)
		}
		if len(p.args) == 0 {
			t.Errorf("livePeers returned pid %d with no command line", p.pid)
		}
	}
}

// TestLivePeers_ListsRunningPeersAndSkipsOnesWithoutACommandLine covers what
// the probe reads off the process table: a running instance with its command
// line, and nothing for a process that has no command line to read.
//
// The second is a real shape rather than a defensive one. A process that has
// exited but not yet been reaped is still listed under its name, so
// discovery finds it, and its command line is empty, so there is no listener
// to derive from it. Listing it would send the probe to the default port on
// behalf of a process that serves nothing.
func TestLivePeers_ListsRunningPeersAndSkipsOnesWithoutACommandLine(t *testing.T) {
	binary := buildPeer(t, filepath.Join(t.TempDir(), peerName(t)))
	withArgv0(t, binary)
	running := startPeer(t, binary, nil)

	// Started and killed but deliberately not reaped until the test ends, so
	// it stays in the table as a zombie for the duration of the assertions.
	zombie := exec.CommandContext(t.Context(), binary)
	if err := zombie.Start(); err != nil {
		t.Fatalf("starting the peer that will become a zombie: %v", err)
	}
	t.Cleanup(func() { _ = zombie.Wait() })
	if err := zombie.Process.Kill(); err != nil {
		t.Fatalf("killing the peer: %v", err)
	}

	runningPID := pid32(t, running.cmd.Process.Pid)
	zombiePID := pid32(t, zombie.Process.Pid)
	deadline := time.Now().Add(10 * time.Second)
	for {
		peers, err := livePeers()
		if err != nil {
			t.Fatalf("livePeers: %v", err)
		}
		var sawRunning, sawZombie bool
		for _, p := range peers {
			switch p.pid {
			case runningPID:
				sawRunning = true
				if len(p.args) == 0 || p.args[0] != binary {
					t.Errorf("pid %d listed with args %q, want its command line starting with %q", p.pid, p.args, binary)
				}
			case zombiePID:
				sawZombie = true
			}
		}
		if sawRunning && !sawZombie {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("after 10s: running peer listed=%v, zombie listed=%v; want the running one and not the zombie", sawRunning, sawZombie)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestLivePeers_ProcessListingFails_ReturnsTheError pins that discovery
// reports an unreadable process table rather than answering "no instances",
// which --probe would turn into an unhealthy verdict with the wrong cause.
func TestLivePeers_ProcessListingFails_ReturnsTheError(t *testing.T) {
	original := listProcesses
	t.Cleanup(func() { listProcesses = original })
	listProcesses = func() ([]*process.Process, error) {
		return nil, errors.New("procfs is not mounted")
	}

	peers, err := livePeers()
	if err == nil || !strings.Contains(err.Error(), "procfs is not mounted") {
		t.Fatalf("livePeers() = %v, %v; want the listing's error", peers, err)
	}
}

// TestRunProbe_ATargetThatCannotBecomeARequest_IsUnhealthy covers a host:port
// that splits cleanly and still cannot be asked: net.SplitHostPort accepts any
// host text, and the URL built from it is what refuses the space.
func TestRunProbe_ATargetThatCannotBecomeARequest_IsUnhealthy(t *testing.T) {
	t.Parallel()
	var said bytes.Buffer
	code := runProbe(t.Context(), []string{"a b:80"}, "", probeDeps{}, &said)
	if code != probeUnhealthy {
		t.Errorf("runProbe(%q) = %d, want %d: %s", "a b:80", code, probeUnhealthy, said.String())
	}
	if !strings.Contains(said.String(), "invalid character") {
		t.Errorf("runProbe said %q, want the URL parser's complaint about the host", said.String())
	}
}

// TestCertificateName_PrefersDNSThenIPThenCommonName pins the name the probe
// asks a pinned listener to answer to, in the order the certificate's own
// contents decide it.
func TestCertificateName_PrefersDNSThenIPThenCommonName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cert *x509.Certificate
		want string
	}{
		{name: "a DNS name wins", cert: &x509.Certificate{DNSNames: []string{"mcp.example"}, IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}, Subject: pkix.Name{CommonName: "cn"}}, want: "mcp.example"},
		{name: "an IP address without DNS names", cert: &x509.Certificate{IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}, Subject: pkix.Name{CommonName: "cn"}}, want: "127.0.0.1"},
		{name: "the common name is the last resort", cert: &x509.Certificate{Subject: pkix.Name{CommonName: "cn"}}, want: "cn"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := certificateName(tc.cert); got != tc.want {
				t.Errorf("certificateName() = %q, want %q", got, tc.want)
			}
		})
	}
}
