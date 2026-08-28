// listen_test.go covers the listener the HTTP server binds: a unix socket
// when the address is a path, a TCP port otherwise, plus the TLS pair and the
// socket permission mode that go with them.
package main

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// TestIsUnixSocketAddr_DistinguishesPathsFromHostPort verifies the rule that
// decides which kind of listener gets bound.
//
// The bare name is the case worth pinning: "mcp.sock" is a valid hostname, so
// treating it as a path would bind something other than what the operator
// wrote, silently.
func TestIsUnixSocketAddr_DistinguishesPathsFromHostPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr string
		want bool
	}{
		{addr: ":8080", want: false},
		{addr: "0.0.0.0:8080", want: false},
		{addr: "127.0.0.1:8080", want: false},
		{addr: "[::1]:8080", want: false},
		{addr: "mcp.sock", want: false},
		{addr: "/run/mcp.sock", want: true},
		{addr: "./mcp.sock", want: true},
		{addr: "/var/run/gitlab-mcp/server.sock", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			t.Parallel()
			if got := isUnixSocketAddr(tt.addr); got != tt.want {
				t.Errorf("isUnixSocketAddr(%q) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

// TestListenHTTP_UnixSocket_ServesAndAppliesTheMode verifies the whole point
// of socket support: a real HTTP conversation over a filesystem path, with
// permissions the deployment chose rather than whatever umask happened to be.
//
// The mode is asserted because it is the difference between a proxy that can
// reach the server and one that cannot — and, in the other direction, between
// a socket only the proxy's group can open and one every local account can.
func TestListenHTTP_UnixSocket_ServesAndAppliesTheMode(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "mcp.sock")
	listener, err := listenHTTP(t.Context(), path, config.DefaultSocketMode)
	if err != nil {
		t.Fatalf("listenHTTP() error = %v", err)
	}
	defer listener.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if info.Mode()&fs.ModeSocket == 0 {
		t.Fatal("the bound path is not a socket")
	}
	if perm := info.Mode().Perm(); perm != config.DefaultSocketMode {
		t.Errorf("socket mode = %#o, want %#o", perm, config.DefaultSocketMode)
	}

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}), ReadHeaderTimeout: baseHTTPReadHeaderTimeout}
	go func() { _ = srv.Serve(listener) }()
	defer srv.Close()

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", path)
		},
	}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://unix/health", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request over the unix socket failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// TestClearStaleSocket_AbsentPathIsFine verifies the ordinary startup case:
// nothing is at the path, so there is nothing to clear and no error to
// report.
func TestClearStaleSocket_AbsentPathIsFine(t *testing.T) {
	t.Parallel()

	if err := clearStaleSocket(t.Context(), filepath.Join(t.TempDir(), "absent.sock")); err != nil {
		t.Errorf("clearStaleSocket() error = %v, want nil", err)
	}
}

// TestClearStaleSocket_DeadSocketIsRemoved covers the case the whole check
// exists to allow: a restart after a process died without unlinking its
// socket, which would otherwise fail to bind forever.
func TestClearStaleSocket_DeadSocketIsRemoved(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "dead.sock")
	listener := listenUnixForTest(t, path)
	// A normal Close unlinks the file, so unlinking is disabled to leave
	// behind exactly what an unclean exit leaves behind.
	if unix, ok := listener.(*net.UnixListener); ok {
		unix.SetUnlinkOnClose(false)
	} else {
		t.Fatalf("listener type = %T, want *net.UnixListener", listener)
	}
	_ = listener.Close()

	if err := clearStaleSocket(t.Context(), path); err != nil {
		t.Fatalf("clearStaleSocket() error = %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stale socket still present: %v", err)
	}
}

// TestClearStaleSocket_LiveSocketIsRefused is the counterweight: an
// unconditional remove would let a second instance hijack the socket a
// running server is still serving, leaving it holding a listener nobody can
// reach. A successful connect is the proof somebody is there.
func TestClearStaleSocket_LiveSocketIsRefused(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "live.sock")
	listener := listenUnixForTest(t, path)
	defer listener.Close()
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	err := clearStaleSocket(t.Context(), path)
	if err == nil {
		t.Fatal("clearStaleSocket() = nil, want a refusal for a live socket")
	}
	if !strings.Contains(err.Error(), "already served") {
		t.Errorf("error = %v, want it to say the socket is already served", err)
	}
	if _, statErr := os.Lstat(path); statErr != nil {
		t.Errorf("a live socket must not be removed: %v", statErr)
	}
}

// TestClearStaleSocket_UnansweredProbeIsRefused verifies that only a definite
// "nothing is listening" clears the path.
//
// A dial can fail for reasons that say nothing about whether a server is
// there: a permission denial, a timeout, a cancelled context. Treating any
// failure as proof of death is how a running deployment loses its socket to a
// racing restart. Here the probe is cancelled before it can complete, which
// stands in for every unanswered question.
func TestClearStaleSocket_UnansweredProbeIsRefused(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "unanswered.sock")
	listener := listenUnixForTest(t, path)
	defer listener.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // the probe cannot answer

	err := clearStaleSocket(ctx, path)
	if err == nil {
		t.Fatal("clearStaleSocket() = nil; an unanswered probe must not authorize a delete")
	}
	if !strings.Contains(err.Error(), "could not be probed") {
		t.Errorf("error = %v, want it to say the socket could not be probed", err)
	}
	if _, statErr := os.Lstat(path); statErr != nil {
		t.Errorf("the socket was removed on an unanswered probe: %v", statErr)
	}
}

// TestClearStaleSocket_RegularFileIsRefused verifies that a typo in
// --http-addr cannot delete an unrelated file.
func TestClearStaleSocket_RegularFileIsRefused(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "not-a-socket")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	err := clearStaleSocket(t.Context(), path)
	if err == nil || !strings.Contains(err.Error(), "not a socket") {
		t.Fatalf("clearStaleSocket() = %v, want a refusal naming the file kind", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("an unrelated file must never be deleted: %v", statErr)
	}
}

// listenUnixForTest binds a unix socket or fails the test.
func listenUnixForTest(t *testing.T, path string) net.Listener {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", path)
	if err != nil {
		t.Fatalf("listen on %q: %v", path, err)
	}
	return listener
}

// TestParseSocketMode_ReadsOctal verifies that the mode is read the way chmod
// reads it. "660" as decimal would be 0o1224 — a mode nobody asked for, and
// one that silently widens or narrows access.
func TestParseSocketMode_ReadsOctal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    os.FileMode
		wantErr bool
	}{
		{name: "empty uses the default", want: config.DefaultSocketMode},
		{name: "leading zero", value: "0660", want: 0o660},
		{name: "no leading zero", value: "660", want: 0o660},
		{name: "go style prefix", value: "0o600", want: 0o600},
		{name: "owner only", value: "600", want: 0o600},
		{name: "world readable", value: "666", want: 0o666},
		{name: "not a number", value: "rw-rw----", wantErr: true},
		{name: "not octal", value: "0899", wantErr: true},
		{name: "zero is not a usable mode", value: "0", wantErr: true},
		{name: "beyond permission bits", value: "7777", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hcfg := &httpConfig{socketMode: tt.value}
			err := parseSocketMode(hcfg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseSocketMode(%q) = nil, want an error", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSocketMode(%q) error = %v", tt.value, err)
			}
			if hcfg.socketModeParsed != tt.want {
				t.Errorf("mode = %#o, want %#o", hcfg.socketModeParsed, tt.want)
			}
		})
	}
}

// TestValidateTLSFiles_RequiresBothOrNeither verifies that a half-configured
// pair fails at startup rather than at the first handshake.
//
// A cert without a key is a deployment that believes it is encrypting and is
// not, and that belief is the whole reason the flags exist.
func TestValidateTLSFiles_RequiresBothOrNeither(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cert    string
		key     string
		wantErr string
	}{
		{name: "neither is fine"},
		{name: "cert alone", cert: "cert.pem", wantErr: "--tls-cert requires --tls-key"},
		{name: "key alone", key: "key.pem", wantErr: "--tls-key requires --tls-cert"},
		{name: "both but unreadable", cert: "cert.pem", key: "key.pem", wantErr: "loading the TLS certificate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateTLSFiles(&config.Config{TLSCertFile: tt.cert, TLSKeyFile: tt.key})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateTLSFiles() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateTLSFiles() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestRepeatedFlag_AcceptsRepetitionAndCommas verifies that both spellings of
// a list reach the same place, since the flag is repeated on a command line
// but arrives as one string from the environment overlay.
func TestRepeatedFlag_AcceptsRepetitionAndCommas(t *testing.T) {
	t.Parallel()

	var flagValue repeatedFlag
	if err := flagValue.Set("https://gitlab.com"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := flagValue.Set(" https://a.example , ,https://b.example "); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	want := []string{"https://gitlab.com", "https://a.example", "https://b.example"}
	if got := []string(flagValue); len(got) != len(want) {
		t.Fatalf("collected %v, want %v", got, want)
	}
	for i, value := range want {
		if flagValue[i] != value {
			t.Errorf("entry %d = %q, want %q", i, flagValue[i], value)
		}
	}
	if got := flagValue.String(); got != strings.Join(want, ",") {
		t.Errorf("String() = %q, want %q", got, strings.Join(want, ","))
	}
}
