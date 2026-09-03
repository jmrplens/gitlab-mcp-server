// listen_test.go covers the listener the HTTP server binds: a unix socket
// when the address is a path, a TCP port otherwise, plus the TLS pair and the
// socket permission mode that go with them.
package main

import (
	"context"
	"crypto/tls"
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

// socketDir returns a temporary directory short enough to hold a unix socket
// path, and removes it when the test ends.
//
// Not [testing.T.TempDir], which embeds the test's own name: a unix address
// holds at most 103 bytes of path, and on macOS the temporary directory alone
// is already 52 of them, so a name like
// TestListenHTTP_UnixSocketWithoutAMode_TakesTheDefault puts the socket 30
// bytes over the limit before the file name is appended. The server is right to
// refuse such a path and says so precisely; the tests were the ones asking for
// something no client could reach.
func socketDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "s") //nolint:usetesting // see above
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

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

	path := filepath.Join(socketDir(t), "mcp.sock")
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

	path := filepath.Join(socketDir(t), "dead.sock")
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

	path := filepath.Join(socketDir(t), "live.sock")
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

	path := filepath.Join(socketDir(t), "unanswered.sock")
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
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if flagValue[i] != value {
				t.Errorf("entry %d = %q, want %q", i, flagValue[i], value)
			}
		})
	}
	if got := flagValue.String(); got != strings.Join(want, ",") {
		t.Errorf("String() = %q, want %q", got, strings.Join(want, ","))
	}
}

// TestListenUnix_RefusesAPathItCannotOwn covers the failures that stop a unix
// socket from being bound, each of which has to arrive as an error naming
// --http-addr rather than as a partially started server.
//
// The cases are the ones an operator actually produces: a path whose directory
// is not a directory (a typo, or a file where a directory was expected), a path
// already occupied by something that is not a socket, and a path longer than
// the address family allows. Every one of them fails the same way for anybody
// who runs it.
//
// The permission case does not, so it is not in this table. Permission bits
// refuse an ordinary account and never refuse root, which makes the expected
// outcome depend on who is running the suite; it lives in
// TestBindUnixSocket_UnwritableDirectoryIsRefused, which skips when the test
// process is privileged. An earlier version of this comment justified leaving
// it out by asserting that the suite always runs as root in CI. That was
// false, and the CI failure this file's parallelism was rearranged for was a
// permission denial.
func TestListenUnix_RefusesAPathItCannotOwn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	notADirectory := filepath.Join(dir, "file")
	if err := os.WriteFile(notADirectory, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("writing the blocking file: %v", err)
	}
	occupied := filepath.Join(dir, "occupied.sock")
	if err := os.WriteFile(occupied, []byte("not a socket"), 0o600); err != nil {
		t.Fatalf("writing the blocking file: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{
			name:    "its directory does not exist",
			path:    filepath.Join(dir, "absent", "mcp.sock"),
			wantErr: "its directory is not usable",
		},
		{
			name:    "its directory is a file",
			path:    filepath.Join(notADirectory, "mcp.sock"),
			wantErr: "--http-addr",
		},
		{
			name:    "the path is already something else",
			path:    occupied,
			wantErr: "refusing to replace it",
		},
		{
			name: "the path is longer than a unix address",
			path: filepath.Join(dir, strings.Repeat("a", 120)+".sock"),
			// This one used to be the kernel's refusal. It is ours now: the
			// socket is bound under a short staging name and published with
			// link(2), which has no address limit, so an over-long path would
			// otherwise bind cleanly into a listener no client can reach.
			wantErr: "a unix address holds at most",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			listener, err := listenHTTP(t.Context(), tt.path, config.DefaultSocketMode)

			if err == nil {
				_ = listener.Close()
				t.Fatalf("listenHTTP(%q) bound a path it must refuse", tt.path)
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to say %q", err, tt.wantErr)
			}
		})
	}
}

// TestListenHTTP_UnixSocketWithoutAMode_TakesTheDefault covers the zero mode,
// which is what a Config assembled without the flag carries.
//
// The default is the one that matters for a same-host proxy: a socket only the
// proxy's group can open, rather than one every local account can.
func TestListenHTTP_UnixSocketWithoutAMode_TakesTheDefault(t *testing.T) {
	t.Parallel()

	path := filepath.Join(socketDir(t), "default-mode.sock")
	listener, err := listenHTTP(t.Context(), path, 0)
	if err != nil {
		t.Fatalf("listenHTTP: %v", err)
	}
	defer listener.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if got := info.Mode().Perm(); got != config.DefaultSocketMode {
		t.Errorf("socket mode = %#o, want the default %#o", got, config.DefaultSocketMode)
	}
}

// TestClearStaleSocket_AnUnreadablePath_IsRefusedRatherThanRemoved covers the
// lstat failing for a reason other than the path being absent.
//
// Absent is the ordinary case and means "nothing to clear". Anything else means
// the question went unanswered, and removing on an unanswered question is how a
// running deployment loses its socket to a racing restart.
func TestClearStaleSocket_AnUnreadablePath_IsRefusedRatherThanRemoved(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	blocking := filepath.Join(dir, "file")
	if err := os.WriteFile(blocking, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("writing the blocking file: %v", err)
	}

	err := clearStaleSocket(t.Context(), filepath.Join(blocking, "mcp.sock"))

	if err == nil {
		t.Fatal("clearStaleSocket accepted a path it could not examine")
	}
	if !strings.Contains(err.Error(), "--http-addr") {
		t.Errorf("error = %q, want it to name the flag the operator typed", err)
	}
}

// TestRepeatedFlag_TheZeroValueRenders covers String on a flag nobody set,
// which the flag package calls while printing usage.
//
// Usage printing happens on the help path, before any value exists, so a nil
// receiver here would turn --help into a panic.
func TestRepeatedFlag_TheZeroValueRenders(t *testing.T) {
	t.Parallel()

	var absent *repeatedFlag
	if got := absent.String(); got != "" {
		t.Errorf("(*repeatedFlag)(nil).String() = %q, want the empty string", got)
	}
	empty := repeatedFlag{}
	if got := empty.String(); got != "" {
		t.Errorf("repeatedFlag{}.String() = %q, want the empty string", got)
	}
}

// TestValidateTLSFiles_ALoadablePairIsAccepted covers the success path of the
// startup check.
//
// The pair is loaded at startup precisely so a typo becomes an error naming the
// file, instead of a TLS handshake failure on the first request that nobody
// sees until a client reports it. The loader is stubbed because what is under
// test is the decision, not crypto/tls.
func TestValidateTLSFiles_ALoadablePairIsAccepted(t *testing.T) {
	original := loadTLSKeyPair
	t.Cleanup(func() { loadTLSKeyPair = original })
	var loadedCert, loadedKey string
	loadTLSKeyPair = func(cert, key string) (tls.Certificate, error) {
		loadedCert, loadedKey = cert, key
		return tls.Certificate{}, nil
	}

	err := validateTLSFiles(&config.Config{TLSCertFile: "/etc/ssl/mcp.crt", TLSKeyFile: "/etc/ssl/mcp.key"})
	if err != nil {
		t.Fatalf("validateTLSFiles = %v, want nil for a loadable pair", err)
	}
	if loadedCert != "/etc/ssl/mcp.crt" || loadedKey != "/etc/ssl/mcp.key" {
		t.Errorf("loaded (%q, %q), want the configured pair", loadedCert, loadedKey)
	}
}
