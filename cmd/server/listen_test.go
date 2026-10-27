// listen_test.go covers the listener the HTTP server binds: a unix socket
// when the address is a path, a TCP port otherwise, plus the TLS pair and the
// socket permission mode that go with them.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
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

	dir, err := os.MkdirTemp("", "s") //nolint:usetesting // t.TempDir embeds the test name, which puts a unix socket path over its 103-byte limit
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

// TestListenHTTP_UnixSocket_Serves verifies the whole point of socket support:
// a real HTTP conversation over a filesystem path.
//
// The permission mode used to be asserted here too and is now in
// listen_unix_test.go, because it is the one part of this that Windows does not
// have. Windows supports AF_UNIX and everything below still holds there; what
// it has no equivalent of is the POSIX bits, which is why bindUnixSocket says
// so out loud on that platform instead of pretending.
func TestListenHTTP_UnixSocket_Serves(t *testing.T) {
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
//
// It does not run in parallel, for the reason deadUnixSocket gives.
func TestClearStaleSocket_DeadSocketIsRemoved(t *testing.T) {
	path := filepath.Join(socketDir(t), "dead.sock")
	deadUnixSocket(t, path)

	if err := clearStaleSocket(t.Context(), path); err != nil {
		t.Fatalf("clearStaleSocket() error = %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stale socket still present: %v", err)
	}
}

// TestClearStaleSocket_ADeadSocketThatCannotBeRemoved_IsReported covers the
// unlink failing after the probe proved the socket dead. The tests hold every
// permission, so the unlink is failed through its seam; what is asserted is
// that the failure names the path and reaches the operator instead of the
// bind failing a moment later on a file the log never mentioned.
//
// It does not run in parallel, for two reasons: it swaps the unlink seam every
// test of the package reads, and it dials a dead socket, which deadUnixSocket
// explains.
func TestClearStaleSocket_ADeadSocketThatCannotBeRemoved_IsReported(t *testing.T) {
	path := filepath.Join(socketDir(t), "stuck.sock")
	deadUnixSocket(t, path)

	original := removeStaleSocket
	t.Cleanup(func() { removeStaleSocket = original })
	removeStaleSocket = func(string) error { return errors.New("operation not permitted") }

	err := clearStaleSocket(t.Context(), path)
	if err == nil {
		t.Fatal("clearStaleSocket() reported success for a socket it could not remove")
	}
	// The path is rendered with %q, which escapes the backslashes of a
	// Windows path, so the expectation is the quoted form rather than the
	// raw one.
	if !strings.Contains(err.Error(), "removing stale socket") || !strings.Contains(err.Error(), strconv.Quote(path)) {
		t.Errorf("error = %q, want the unlink failure naming %s", err, strconv.Quote(path))
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

// deadUnixSocket leaves at path what a process that died without unlinking its
// socket leaves behind: the socket file, with nothing listening on it.
//
// A test that dials such a path and asserts the refusal must not run in
// parallel. Starting a process copies every descriptor this one holds into the
// child, and close-on-exec closes the copy only when the child reaches exec.
// For the moments between a sibling's fork and its exec, the listener closed
// here is therefore still listening in the child, and a connect succeeds into
// its backlog: clearStaleSocket then reports the path as served, which is the
// right answer for a socket somebody holds. Tests of this package start
// processes during the parallel phase, and with only those beside it the
// dead-socket test failed in 242 of 3000 runs (269 under the race detector).
// os/exec's Start returns only once the child's exec has closed those copies,
// since that is how it learns the exec succeeded, and no test of this package
// starts a process from a goroutine that outlives it, so outside the parallel
// phase no copy can exist when the dial is made.
func deadUnixSocket(t *testing.T, path string) {
	t.Helper()
	listener := listenUnixForTest(t, path)
	// A normal Close unlinks the file, so unlinking is disabled to leave
	// behind exactly what an unclean exit leaves behind.
	unix, ok := listener.(*net.UnixListener)
	if !ok {
		t.Fatalf("listener type = %T, want *net.UnixListener", listener)
	}
	unix.SetUnlinkOnClose(false)
	_ = listener.Close()
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
		// The top of the permission bits is a mode like any other; the refusal
		// starts one past it, at the setuid, setgid and sticky bits.
		{name: "every permission bit", value: "777", want: 0o777},
		{name: "the smallest usable mode", value: "1", want: 0o001},
		{name: "one past the permission bits", value: "1000", wantErr: true},
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
		// posixOnly marks a case whose premise the platform has to supply, and
		// which therefore cannot be built everywhere. The refusal each one
		// pins is still a property of this server; what is POSIX-specific is
		// the way the case arranges for it. Kept in the table rather than
		// split into a constrained file, because these four are one question,
		// "which paths must listenUnix refuse", and reading them apart is
		// worse than reading them with a skip.
		posixOnly string
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
			// Windows reports a path under a regular file as ERROR_PATH_NOT_FOUND,
			// which Errno.Is maps to fs.ErrNotExist, so lstat answers "nothing
			// there" rather than the ENOTDIR this leans on. Answering an absent
			// path with "nothing to clear" is correct; it just cannot be told
			// apart from the case being built here.
			posixOnly: "only POSIX distinguishes ENOTDIR from ENOENT, so a path under a file reads as simply absent here",
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
			// And that staging trick is the POSIX implementation's, so the
			// guard compensating for it is too. Windows binds the operator's
			// path directly and the kernel refuses an over-long one itself,
			// with "bind: invalid argument" rather than our sentence. Nothing
			// is bound that should not be; the operator just gets the worse
			// message.
			posixOnly: "the length guard exists to cover a staging-and-link bind that only the POSIX path does",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.posixOnly != "" && runtime.GOOS == "windows" {
				t.Skip(tt.posixOnly)
			}

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

// TestStaleSocketDialTimeout_BoundsTheProbeRatherThanDisablingIt states the one
// thing the constant must not become.
//
// [net.Dialer] reads a zero Timeout as no timeout at all, so a budget that
// collapsed to zero would not make the probe quick, it would remove the bound
// from it: clearStaleSocket runs before the listener is bound, on the startup
// path, and a path whose peer accepts the connection and then never speaks
// would hold the process there with nothing to report. No test can catch that
// by dialing, because a unix connect either completes at once or is refused at
// once, and neither reaches the timeout — which is exactly why the value is
// stated here instead.
//
// The upper bound is the other half of the same reasoning: the probe asks a
// local path a question the kernel answers immediately, so a budget measured
// in seconds would only ever be spent making startup slower.
func TestStaleSocketDialTimeout_BoundsTheProbeRatherThanDisablingIt(t *testing.T) {
	t.Parallel()

	if staleSocketDialTimeout <= 0 {
		t.Errorf("staleSocketDialTimeout = %s: net.Dialer takes a zero Timeout as no timeout, "+
			"so clearStaleSocket would have no bound at all on the startup path", staleSocketDialTimeout)
	}
	if staleSocketDialTimeout > time.Second {
		t.Errorf("staleSocketDialTimeout = %s: a connect to a local path completes or fails at once, "+
			"so a budget this large only delays startup", staleSocketDialTimeout)
	}
}

// TestCheckPlaintextListener_SaysWhatTheListenerExposes verifies what startup
// does about a listener that serves plain HTTP, under each value of the
// setting, and the listeners it says nothing about.
//
// The warning is for a TCP listener another machine can open a connection to,
// with no TLS on it and no word from the operator about it: that is the
// configuration that puts every caller's token on a network segment in the
// clear, and it is also what the default --http-addr and the container
// image's command bind. A loopback address and a unix socket carry nothing
// across a network, and TLS on the listener is the remedy itself, so all three
// are silent whatever the setting. The opt-in at true turns the warning into a
// line at INFO rather than into silence, so a deployment that declared it
// still says what it serves, and at false, the hardened mode, it refuses the
// listener and logs nothing, since the refusal is the error startup returns.
//
// Every case names its addr, because the address is what an operator reading
// the line has to change, and the message is a constant so that a log
// pipeline can match it.
func TestCheckPlaintextListener_SaysWhatTheListenerExposes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		addr    string
		tlsCert string
		setting plaintextHTTP
		// resolve is what the name in addr resolves to; nil leaves a literal
		// unresolved, as listenerIsHostLocal does.
		resolve []string
		level   string
		msg     string
		refused bool
	}{
		{name: "a wildcard bind", addr: "0.0.0.0:8080", level: "WARN", msg: plaintextListenerWarning},
		{name: "a port-only bind, the default address", addr: ":8080", level: "WARN", msg: plaintextListenerWarning},
		{name: "an ipv6 wildcard bind", addr: "[::]:8080", level: "WARN", msg: plaintextListenerWarning},
		{name: "a routable address", addr: "10.0.0.7:8080", level: "WARN", msg: plaintextListenerWarning},
		{name: "a name resolving off the machine", addr: "mcp.internal:8080", resolve: []string{"10.0.0.7"}, level: "WARN", msg: plaintextListenerWarning},
		{name: "a wildcard bind under the opt-in", addr: "0.0.0.0:8080", setting: plaintextHTTPAllowed, level: "INFO", msg: plaintextListenerAllowed},
		{name: "a routable address under the opt-in", addr: "10.0.0.7:8080", setting: plaintextHTTPAllowed, level: "INFO", msg: plaintextListenerAllowed},
		{name: "a wildcard bind in the hardened mode", addr: "0.0.0.0:8080", setting: plaintextHTTPRefused, refused: true},
		{name: "a port-only bind in the hardened mode", addr: ":8080", setting: plaintextHTTPRefused, refused: true},
		{name: "a name resolving off the machine in the hardened mode", addr: "mcp.internal:8080", resolve: []string{"10.0.0.7"}, setting: plaintextHTTPRefused, refused: true},
		{name: "a wildcard bind serving TLS", addr: "0.0.0.0:8443", tlsCert: "/etc/ssl/mcp.crt"},
		{name: "a wildcard bind serving TLS under the opt-in", addr: "0.0.0.0:8443", tlsCert: "/etc/ssl/mcp.crt", setting: plaintextHTTPAllowed},
		{name: "a wildcard bind serving TLS in the hardened mode", addr: "0.0.0.0:8443", tlsCert: "/etc/ssl/mcp.crt", setting: plaintextHTTPRefused},
		{name: "a loopback address", addr: "127.0.0.1:8080"},
		{name: "an ipv6 loopback address", addr: "[::1]:8080"},
		{name: "a name resolving to loopback", addr: "localhost:8080", resolve: []string{"127.0.0.1", "::1"}},
		{name: "a loopback address under the opt-in", addr: "127.0.0.1:8080", setting: plaintextHTTPAllowed},
		{name: "a loopback address in the hardened mode", addr: "127.0.0.1:8080", setting: plaintextHTTPRefused},
		{name: "a name resolving to loopback in the hardened mode", addr: "localhost:8080", resolve: []string{"127.0.0.1", "::1"}, setting: plaintextHTTPRefused},
		{name: "a unix socket", addr: "/run/gitlab-mcp/server.sock"},
		{name: "a unix socket in the hardened mode", addr: "/run/gitlab-mcp/server.sock", setting: plaintextHTTPRefused},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolveListenerHostTo(t, tc.resolve)
			logged := captureJSONLog(t)

			err := checkPlaintextListener(t.Context(), tc.addr, tc.tlsCert, tc.setting)

			records := decodeRecords(t, logged)
			if tc.refused {
				assertPlaintextRefusal(t, tc.addr, err, records)
				return
			}
			if err != nil {
				t.Fatalf("checkPlaintextListener() = %v, want the listener served", err)
			}
			assertPlaintextAnnouncement(t, tc.addr, tc.level, tc.msg, records)
		})
	}
}

// assertPlaintextRefusal holds the hardened mode's answer for addr: the
// refusal, opening with the register's text and the quoted address, and
// nothing logged beside it.
func assertPlaintextRefusal(t *testing.T, addr string, err error, records []map[string]any) {
	t.Helper()
	if err == nil || !strings.HasPrefix(err.Error(), plaintextRefusalPrefix+strconv.Quote(addr)) {
		t.Errorf("checkPlaintextListener() = %v, want the hardened mode's refusal naming %q", err, addr)
	}
	if len(records) != 0 {
		t.Errorf("a refused listener was logged as well: %v", records)
	}
}

// assertPlaintextAnnouncement holds what a served listener at addr logged:
// nothing when level is empty, and otherwise the one line at that level,
// with that message and the address.
func assertPlaintextAnnouncement(t *testing.T, addr, level, msg string, records []map[string]any) {
	t.Helper()
	if level == "" {
		if len(records) != 0 {
			t.Fatalf("a listener that crosses no network in the clear was announced: %v", records)
		}
		return
	}
	if len(records) != 1 {
		t.Fatalf("logged %d records, want the one line about the listener: %v", len(records), records)
	}
	record := records[0]
	if record["level"] != level || record["msg"] != msg {
		t.Errorf("logged %v %q, want %s %q", record["level"], record["msg"], level, msg)
	}
	if record["addr"] != addr {
		t.Errorf("addr = %v, want the address the listener binds, %q", record["addr"], addr)
	}
}

// resolveListenerHostTo makes [lookupHost] answer every name with the
// literals in addresses for the rest of the test, so a case that names a host
// is judged by what the case says it resolves to rather than by the machine's
// resolver. A nil list answers with an error, as a name that does not resolve
// does, which is also what a literal address never reaches.
func resolveListenerHostTo(t *testing.T, addresses []string) {
	t.Helper()
	previous := lookupHost
	lookupHost = func(string) ([]net.IP, error) {
		if addresses == nil {
			return nil, errors.New("the case names no host to resolve")
		}
		resolved := make([]net.IP, 0, len(addresses))
		for _, literal := range addresses {
			resolved = append(resolved, net.ParseIP(literal))
		}
		return resolved, nil
	}
	t.Cleanup(func() { lookupHost = previous })
}

// plaintextRefusalPrefix is how the hardened mode's refusal opens, up to the
// quoted address, which is the stable text register row ADM-015 pins.
const plaintextRefusalPrefix = "--allow-plaintext-http=false, or GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=false, refuses a listener that serves plain HTTP " +
	"on an address other machines can reach, and --http-addr "

// TestPlaintextListenerLines_NameEveryWayOut holds the two lines and the
// refusal to what an operator needs from them.
//
// The warning is read by somebody deciding what to change, so it names each
// remedy by the flag or variable that applies it: TLS on the listener, a
// loopback address or a unix socket for a proxy on the same machine, and the
// opt-in in both spellings, since a container takes the variable without
// replacing the image's command line; and it names the hardened mode, in both
// spellings too, for an operator who would rather the listener did not start.
// The line written under the opt-in names the opt-in, so a reader of a
// deployment's log can tell a declared plaintext listener from one nobody
// looked at. The refusal names the address, the remedies, and the opt-in for
// a hop something in front of the listener encrypts.
func TestPlaintextListenerLines_NameEveryWayOut(t *testing.T) {
	t.Parallel()

	refusal := checkPlaintextListener(t.Context(), "0.0.0.0:8080", "", plaintextHTTPRefused)
	if refusal == nil {
		t.Fatal("the hardened mode served a wildcard bind in plain HTTP")
	}
	for _, tc := range []struct {
		name  string
		line  string
		names []string
	}{
		{
			name: "the warning",
			line: plaintextListenerWarning,
			names: []string{
				"--http-addr", "plain HTTP", "--tls-cert", "--tls-key", "loopback", "unix socket", "--allow-plaintext-http",
				"GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=true", "hardened mode", "--allow-plaintext-http=false",
				"GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=false",
			},
		},
		{
			name:  "the line under the opt-in",
			line:  plaintextListenerAllowed,
			names: []string{"--http-addr", "plain HTTP", "--allow-plaintext-http"},
		},
		{
			name: "the refusal",
			line: refusal.Error(),
			names: []string{
				`"0.0.0.0:8080"`, "plain HTTP", "--tls-cert", "--tls-key", "loopback", "unix socket",
				"GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=true", "GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=false",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, name := range tc.names {
				if !strings.Contains(tc.line, name) {
					t.Errorf("%s does not name %q: %s", tc.name, name, tc.line)
				}
			}
		})
	}
}

// TestPlaintextHTTP_ReadsAsTheBooleanFlagItIs verifies that the setting reads
// its value as a boolean flag does, and remembers that the operator gave one.
//
// The flag defaults to true, which serves plain HTTP as every release before
// it did, but true given is not the default: it is the declaration startup
// reports at INFO, so the setting keeps three values where a bool keeps two.
// A bare --allow-plaintext-http means true, as it does for any boolean flag; a
// value strconv.ParseBool does not read is refused, which stops startup with
// the flag named; and the value it shows is true unless the operator chose
// the hardened mode, which is also what the flag package prints as the
// default and reads off a value it has not been handed yet.
func TestPlaintextHTTP_ReadsAsTheBooleanFlagItIs(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		value string
		want  plaintextHTTP
		shown string
	}{
		{value: "true", want: plaintextHTTPAllowed, shown: "true"},
		{value: "1", want: plaintextHTTPAllowed, shown: "true"},
		{value: "false", want: plaintextHTTPRefused, shown: "false"},
		{value: "0", want: plaintextHTTPRefused, shown: "false"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Parallel()
			var setting plaintextHTTP
			if err := setting.Set(tc.value); err != nil {
				t.Fatalf("Set(%q) = %v", tc.value, err)
			}
			if setting != tc.want || setting.String() != tc.shown {
				t.Errorf("Set(%q) left %d, shown %q; want %d, shown %q", tc.value, setting, setting.String(), tc.want, tc.shown)
			}
		})
	}

	t.Run("a value it does not read", func(t *testing.T) {
		t.Parallel()
		setting := plaintextHTTPAllowed
		if err := setting.Set("sometimes"); err == nil || setting != plaintextHTTPAllowed {
			t.Errorf("Set(%q) = %v and left %d, want an error and the setting as it was", "sometimes", err, setting)
		}
	})

	t.Run("the default", func(t *testing.T) {
		t.Parallel()
		var setting plaintextHTTP
		var unset *plaintextHTTP
		if setting != plaintextHTTPUnset || setting.String() != "true" || unset.String() != "true" {
			t.Errorf("the zero setting is %d, shown %q and %q through a nil pointer; want unset, shown true", setting, setting.String(), unset.String())
		}
		if !setting.IsBoolFlag() {
			t.Error("the setting is not a boolean flag, so a bare --allow-plaintext-http would want a value")
		}
	})
}

// TestPlaintextHTTP_OnTheCommandLine_ParsesAsABooleanFlag verifies the
// setting registered as the server registers it: absent it stays unset, bare
// it reads as true, with =false it is the hardened mode, and the default the
// flag package shows is true.
func TestPlaintextHTTP_OnTheCommandLine_ParsesAsABooleanFlag(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		args []string
		want plaintextHTTP
	}{
		{args: nil, want: plaintextHTTPUnset},
		{args: []string{"-allow-plaintext-http"}, want: plaintextHTTPAllowed},
		{args: []string{"-allow-plaintext-http=false"}, want: plaintextHTTPRefused},
	} {
		t.Run(strings.Join(append([]string{"args"}, tc.args...), " "), func(t *testing.T) {
			t.Parallel()
			flags := flag.NewFlagSet("server", flag.ContinueOnError)
			var setting plaintextHTTP
			flags.Var(&setting, "allow-plaintext-http", "")
			if err := flags.Parse(tc.args); err != nil || setting != tc.want {
				t.Errorf("Parse(%q) = %v and left %d, want %d", tc.args, err, setting, tc.want)
			}
			if registered := flags.Lookup("allow-plaintext-http"); registered.DefValue != "true" {
				t.Errorf("the flag's default is %q, want true", registered.DefValue)
			}
		})
	}
}
