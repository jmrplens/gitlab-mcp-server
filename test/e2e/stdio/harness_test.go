//go:build stdioe2e

// harness_test.go starts the real server binary over stdio and drives it the
// way a client does.
//
// stdio is this project's primary transport and, until this module existed,
// nothing anywhere started the binary and spoke it. The e2e suite drives an
// in-memory transport in the same process, which answers questions about tool
// behavior and none about the transport: no pipes, no process, no separation of
// stdout from stderr, and no environment-variable configuration, since HTTP mode
// takes flags instead.
//
// That gap is not theoretical. Two defects fixed in this batch were reachable
// on stdio and invisible to every gate: a nil dereference that killed the
// process on an ordinary tool call, and a keepalive ping that closed the
// session of any client speaking 2026-07-28 after 45 idle seconds — the latter
// held in place by a unit test asserting the ping should be there. Both were
// found by hand, against a binary, which is what this module automates.
//
// The binary is built once and started per test with its own environment,
// because stdio configuration is environment-driven and most of what is under
// test is configuration-dependent.
package stdioe2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// protocolVersion is the revision these tests speak unless a case says
// otherwise.
const protocolVersion = "2026-07-28"

// fakeGroupTotal is how many groups the fake instance holds. Deliberately more
// than one page: the number that made this worth testing was a real instance
// answering 20 of 137 under a description that said "List all".
const fakeGroupTotal = 137

var (
	buildOnce   sync.Once
	builtBinary string
	// builtDir is recorded separately from builtBinary because a failed build
	// leaves the directory created and the binary path empty — the one case
	// where cleanup matters most, since the run is about to end.
	builtDir string
	errBuild error
)

// serverBinary builds cmd/server once for the whole package and returns its
// path.
//
// Building rather than importing is the point of the module: what is under test
// is the process — its pipes, its streams, its exit — and a test that imported
// package main would be testing its own assembly of it instead.
func serverBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		// Not t.TempDir: the binary is built once under sync.Once for the whole
		// package, and the first test to arrive would own a directory removed
		// when that test ends.
		dir, err := os.MkdirTemp("", "gitlab-mcp-stdioe2e") //nolint:usetesting // see above
		if err != nil {
			errBuild = err
			return
		}
		builtDir = dir
		out := filepath.Join(dir, "gitlab-mcp-server")
		if runtime.GOOS == "windows" {
			// exec refuses a file with no executable extension there, and
			// go build -o writes exactly the name it is given.
			out += ".exe"
		}
		// The build arguments and the bound come from the race seam, so a
		// `go test -race` run builds an instrumented server rather than
		// driving an uninstrumented one (harness_race_test.go).
		ctx, cancel := context.WithTimeout(context.Background(), serverBuildTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", serverBuildArgs(out)...) //#nosec G204 -- every argument is a constant chosen by a build tag, plus a path this function got from os.MkdirTemp; nothing here comes from outside the test.
		cmd.Dir = repoRoot()
		if output, runErr := cmd.CombinedOutput(); runErr != nil {
			errBuild = fmt.Errorf("building cmd/server: %w\n%s", runErr, output)
			return
		}
		builtBinary = out
	})
	if errBuild != nil {
		t.Fatalf("building the server binary: %v", errBuild)
	}
	return builtBinary
}

// repoRoot walks up to the directory holding go.mod.
func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

// session is a running server process and the two pipes a client talks to it
// through.
type session struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	// exited is closed by the one goroutine that reaps the process. Everything
	// asking whether the server is still there reads it rather than waiting
	// again: a second Wait on a reaped child answers ErrProcessDone, which
	// cannot be told from a real failure.
	exited chan struct{}
	// state is what the reaper found, for the callers that report an exit code.
	state atomic.Pointer[os.ProcessState]

	mu     sync.Mutex
	stderr strings.Builder
	// notifications holds every notification call read past while waiting
	// for a response, in arrival order, for the tests that want them.
	notifications []map[string]any
}

// startSession launches the binary with the given environment and returns a
// live session.
//
// The environment is replaced rather than extended, apart from PATH: stdio
// configuration comes from the environment, so a developer's own GITLAB_URL or
// TOOL_SURFACE would otherwise decide what the test exercises. This is the same
// class of mistake as a generator reading TOOL_SURFACE at the call site, and it
// is worth being deliberate about here, where a leaked variable would silently
// change which tool surface is under test.
func startSession(t *testing.T, env map[string]string) *session {
	t.Helper()
	return startSessionIn(t, "", env)
}

// startSessionInDir is startSession with the process's working directory
// chosen by the caller.
//
// It exists because the working directory is untrusted input on stdio: an MCP
// client sets it to whatever workspace it has open, so its contents arrive
// with a cloned repository rather than from the person running the server.
// Whether the server reads anything out of it is a property of the process
// and can only be tested by choosing one.
func startSessionInDir(t *testing.T, dir string, env map[string]string) *session {
	t.Helper()
	return startSessionIn(t, dir, env)
}

// startSessionWithArgs is startSession for a case that needs the binary to be
// given flags.
//
// stdio configuration is environment-driven, so almost nothing here passes
// arguments; --transport is the exception, because what it reads is a property
// of the process that only exists once there is a process.
func startSessionWithArgs(t *testing.T, env map[string]string, args ...string) *session {
	t.Helper()
	return startSessionIn(t, "", env, args...)
}

// startSessionIn is what the three wrappers above share. An empty dir keeps the
// Go default, which is this package's own directory, so a caller that does not
// care about the working directory is unaffected by one that does.
func startSessionIn(t *testing.T, dir string, env map[string]string, args ...string) *session {
	t.Helper()

	bin := serverBinary(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, args...)
	prepareForTermination(cmd)
	cmd.Dir = dir

	environ := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	// Before the caller's own entries, so a test that needs to say something
	// else about GORACE still can.
	environ = append(environ, raceEnviron()...)
	for k, v := range env {
		environ = append(environ, k+"="+v)
	}
	// os.UserHomeDir reads HOME on Unix and USERPROFILE on Windows, and the
	// server resolves its home through it: the env file it loads and the home
	// directory it drops as an implicit allow-list root both hang off that
	// answer. Mirror the effective HOME into the platform's own variable, or a
	// test that sets HOME configures a home the server never looks at.
	environ = withPlatformHome(environ)
	cmd.Env = environ

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		t.Fatalf("stderr pipe: %v", err)
	}
	if startErr := cmd.Start(); startErr != nil {
		cancel()
		t.Fatalf("starting the server: %v", startErr)
	}

	s := &session{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
		exited: make(chan struct{}),
	}

	// The one reaper. Started with the process rather than on demand, because
	// alive() has to answer before anybody has asked the process to stop, and
	// a check that has to wait first cannot answer that question.
	go func() {
		state, _ := cmd.Process.Wait()
		s.state.Store(state)
		close(s.exited)
	}()

	// stderr is drained on its own goroutine: a full pipe would block the
	// server, and the contents are an assertion of their own — logs belong
	// here and nowhere near stdout.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := stderrPipe.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.stderr.Write(buf[:n])
				s.mu.Unlock()
			}
			if readErr != nil {
				return
			}
		}
	}()

	t.Cleanup(func() {
		_ = stdin.Close()
		cancel()
		// Not Cmd.Wait: the reaper above already collected the child, so this
		// would answer ErrProcessDone and could not be told from a real
		// failure. Waiting on the reaper is the same barrier without the
		// ambiguity.
		select {
		case <-s.exited:
		case <-time.After(10 * time.Second):
		}
	})
	return s
}

// send writes one JSON-RPC message.
func (s *session) send(t *testing.T, msg string) {
	t.Helper()
	if _, err := io.WriteString(s.stdin, msg+"\n"); err != nil {
		// A broken pipe here means the process is gone. Saying so is worth the
		// line: the raw error names the syscall and not the cause, and "the
		// server exited" is the finding in every case that produces it.
		t.Fatalf("the server is no longer running (%v)\nstderr: %s", err, s.stderrText())
	}
}

// readMessage returns the next line the server writes to stdout, decoded.
//
// It fails rather than blocking forever when the server says nothing, since a
// server that has died mid-handshake is the exact failure this module exists to
// catch and a hung test reports it as a timeout with no detail.
func (s *session) readMessage(t *testing.T, within time.Duration) map[string]any {
	t.Helper()

	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := s.stdout.ReadString('\n')
		ch <- result{line, err}
	}()

	select {
	case r := <-ch:
		if r.err != nil && r.line == "" {
			t.Fatalf("the server closed stdout without answering: %v (stderr: %s)", r.err, s.stderrText())
		}
		var decoded map[string]any
		if unmarshalErr := json.Unmarshal([]byte(r.line), &decoded); unmarshalErr != nil {
			// The most useful failure this module can produce: something that
			// is not JSON-RPC on stdout breaks every client, whatever it is.
			t.Fatalf("stdout carried a line that is not JSON: %q\nstderr: %s", r.line, s.stderrText())
		}
		return decoded
	case <-time.After(within):
		t.Fatalf("the server did not answer within %s (stderr: %s)", within, s.stderrText())
		return nil
	}
}

// call sends a request and returns the response carrying its id.
//
// Matching on the id is what makes this a call rather than "the next line".
// The server speaks unprompted: after initialize it announces the catalog it
// registered once the transport was connected, as tools, prompts and resources
// list_changed notifications, and a reader that took the next line for the
// answer was one message behind from then on. Every later assertion then ran
// against a notification, which has no error and so never failed, and the real
// responses piled up unread in the pipe. On Windows, whose pipe is smaller than
// Linux's, four of those were enough to block the server's write and hold its
// shutdown forever, which is how this surfaced. Notifications read past are
// kept in notifications; a response to some other request is a failure, since
// nothing here sends two requests without reading the first one back.
func (s *session) call(t *testing.T, msg string) map[string]any {
	t.Helper()

	want := requestIDOf(t, msg)
	s.send(t, msg)
	for {
		got := s.readMessage(t, 30*time.Second)
		if _, isCall := got["method"]; isCall && got["id"] == nil {
			s.mu.Lock()
			s.notifications = append(s.notifications, got)
			s.mu.Unlock()
			continue
		}
		if !sameID(got["id"], want) {
			s.mu.Lock()
			passed := len(s.notifications)
			s.mu.Unlock()
			t.Fatalf("waiting for the response to id %v, read a message for id %v after passing %d notification(s): %v\nstderr: %s",
				want, got["id"], passed, got, s.stderrText())
		}
		return got
	}
}

// requestIDOf reads the id out of an outgoing request, failing on a message
// that has none: a notification is sent with send, not call.
func requestIDOf(t *testing.T, msg string) any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(msg), &decoded); err != nil {
		t.Fatalf("call was given a message that is not JSON: %q: %v", msg, err)
	}
	id, ok := decoded["id"]
	if !ok || id == nil {
		t.Fatalf("call was given a message without an id; use send for a notification: %q", msg)
	}
	return id
}

// sameID compares two JSON-RPC ids as the decoder produced them, a number or a
// string, without caring which of the two spellings each side used.
func sameID(a, b any) bool {
	return fmt.Sprint(a) == fmt.Sprint(b)
}

// alive reports whether the server process is still running.
func (s *session) alive() bool {
	select {
	case <-s.exited:
		return false
	default:
		return true
	}
}

// stderrText returns everything the server has logged so far.
func (s *session) stderrText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stderr.String()
}

// waitForStderr returns the server's stderr once it contains needle, and fails
// the test if it does not within the window.
//
// stderr is copied into the buffer by a goroutine of this harness, so a line
// the server wrote before answering on stdout can still be in the pipe when
// the test reads the buffer: the stdout reply and the stderr copy are two
// goroutines with no ordering between them. A test that asserted on
// stderrText right after a call therefore found it empty now and then,
// including for the OTEL_SDK_DISABLED veto, which the server logs before it
// serves anything. Waiting for the line the assertion is about removes the
// race without loosening the assertion; an absence assertion that follows
// should anchor on a line the server always writes after the one it denies.
func (s *session) waitForStderr(t *testing.T, needle string, within time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		text := s.stderrText()
		if strings.Contains(text, needle) {
			return text
		}
		if time.Now().After(deadline) {
			t.Fatalf("stderr did not carry %q within %s\nstderr: %s", needle, within, text)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// request builds a JSON-RPC request carrying the per-request _meta a
// 2026-07-28 client sends.
func request(id int, method, params string) string {
	meta := `"_meta":{"io.modelcontextprotocol/protocolVersion":"` + protocolVersion + `",` +
		`"io.modelcontextprotocol/clientCapabilities":{},` +
		`"io.modelcontextprotocol/clientInfo":{"name":"stdio-e2e","version":"1"}}`
	if params == "" {
		params = "{" + meta + "}"
	} else {
		params = params[:len(params)-1] + "," + meta + "}"
	}
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, params)
}

// fakeGitLab is a running fake instance and what a test can observe about it.
type fakeGitLab struct {
	// URL is the base address to point the server at.
	URL string
	// entered is closed the first time a request reaches the blocking
	// endpoint, so a test can wait for a call to be genuinely in flight
	// instead of guessing at a duration.
	entered chan struct{}
	arrived sync.Once
	// scopes is what the personal-access-token endpoint reports for the token
	// in use; nil leaves the endpoint unanswered, which is the "detection
	// unavailable" case every other test relies on. Set it before the server
	// starts, since the server asks once at startup.
	scopes []string
}

// awaitInFlightCall blocks until a call has reached the blocking endpoint.
func (f *fakeGitLab) awaitInFlightCall(t *testing.T, within time.Duration) {
	t.Helper()

	select {
	case <-f.entered:
	case <-time.After(within):
		t.Fatalf("no call reached the blocking endpoint within %s, so nothing was in flight to test", within)
	}
}

// startFakeGitLab serves the handful of endpoints the server probes at startup
// plus one project, so a tool call has something to answer with.
func startFakeGitLab(t *testing.T) *fakeGitLab {
	t.Helper()

	fake := &fakeGitLab{entered: make(chan struct{})}
	blocked := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"version":"17.0.0","revision":"abcdef"}`)
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"id":7,"username":"someone","name":"Some One"}`)
	})
	mux.HandleFunc("/api/v4/personal_access_tokens/self", func(w http.ResponseWriter, _ *http.Request) {
		if fake.scopes == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		scopes, _ := json.Marshal(fake.scopes)
		writeJSON(w, `{"id":1,"name":"stdio-e2e","active":true,"scopes":`+string(scopes)+`}`)
	})
	mux.HandleFunc("/api/v4/projects/42", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"id":42,"name":"proj","path_with_namespace":"g/proj","web_url":"http://example.invalid/g/proj"}`)
	})
	// A collection larger than one page, answering exactly what was asked for
	// and reporting the rest through the pagination headers GitLab sends. It
	// honors per_page rather than ignoring it, so the number of items that
	// come back is itself the assertion about what the server requested.
	mux.HandleFunc("/api/v4/groups", func(w http.ResponseWriter, r *http.Request) {
		perPage := 20 // GitLab's own default, which is what an unset per_page gets
		if requested, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && requested > 0 {
			perPage = min(requested, fakeGroupTotal)
		}
		w.Header().Set("X-Total", strconv.Itoa(fakeGroupTotal))
		w.Header().Set("X-Total-Pages", strconv.Itoa((fakeGroupTotal+perPage-1)/perPage))
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Per-Page", strconv.Itoa(perPage))
		if perPage < fakeGroupTotal {
			w.Header().Set("X-Next-Page", "2")
		}

		groups := make([]string, 0, perPage)
		for id := 1; id <= perPage; id++ {
			groups = append(groups, fmt.Sprintf(`{"id":%d,"name":"g%d","path":"g%d","full_path":"g%d","visibility":"private"}`, id, id, id, id))
		}
		writeJSON(w, "["+strings.Join(groups, ",")+"]")
	})
	// Project 99 never answers until the test is over, so a case can hold a
	// tool call in flight while it shuts the server down. It returns as soon as
	// the caller goes away, so a dead server does not leave it parked.
	mux.HandleFunc("/api/v4/projects/99", func(w http.ResponseWriter, r *http.Request) {
		// Announcing arrival is what lets a caller know the call is genuinely
		// in flight. A test that slept instead was asserting on the scheduler:
		// if the request had not started yet, closing stdin exercised the idle
		// path and the case passed without testing anything it claimed to.
		fake.arrived.Do(func() { close(fake.entered) })
		select {
		case <-blocked:
		case <-r.Context().Done():
		}
		writeJSON(w, `{"id":99,"name":"slow","path_with_namespace":"g/slow"}`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	fake.URL = srv.URL
	t.Cleanup(srv.Close)
	// Registered after srv.Close so it runs before it: Close waits for
	// outstanding requests, and one parked on this channel would hold it.
	t.Cleanup(func() { close(blocked) })
	return fake
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

// baseEnv is the environment every case starts from: a fake GitLab, a token
// that looks real.
func baseEnv(gitlabURL string) map[string]string {
	return map[string]string{
		"GITLAB_URL":   gitlabURL,
		"GITLAB_TOKEN": "glpat-stdio-e2e-token",
		"LOG_LEVEL":    "info",
	}
}

// terminate sends SIGTERM and reports whether the process exited within the
// given window.
//
// Shutdown is a contract too: a client that stops its server expects the
// process to go away, and a supervisor that waits on it expects the same. A
// server that has to be killed is a server that may not have finished what it
// was doing.
func (s *session) terminate(t *testing.T, within time.Duration) bool {
	t.Helper()
	_, exited := s.terminateAndWait(t, within)
	return exited
}

// terminateAndWait sends SIGTERM and returns the exit status.
func (s *session) terminateAndWait(t *testing.T, within time.Duration) (code int, exited bool) {
	t.Helper()

	if err := signalTermination(s.cmd.Process); err != nil {
		t.Fatalf("sending %s: %v", terminationSignalName, err)
	}
	return s.waitExit(t, within)
}

// closeStdinAndWait closes the client's end of stdin and returns the exit
// status.
//
// This is the shutdown signal the stdio binding calls primary and portable, and
// the one a client that simply goes away produces.
func (s *session) closeStdinAndWait(t *testing.T, within time.Duration) (code int, exited bool) {
	t.Helper()

	if err := s.stdin.Close(); err != nil {
		t.Fatalf("closing stdin: %v", err)
	}
	return s.waitExit(t, within)
}

// waitExit reaps the process and returns its exit code.
//
// It reaps through Process.Wait rather than Cmd.Wait deliberately: Cmd.Wait
// closes the stdout and stderr pipes once it sees the process exit, and a read
// still in flight on either would fail rather than return what it had. Reading
// the status directly leaves the pipes alone, and the drain goroutine ends on
// its own when the process does.
func (s *session) waitExit(t *testing.T, within time.Duration) (code int, exited bool) {
	t.Helper()

	done := make(chan *os.ProcessState, 1)
	go func() {
		<-s.exited
		done <- s.state.Load()
	}()

	select {
	case state := <-done:
		if state == nil {
			t.Fatalf("the process could not be reaped\nstderr: %s", s.stderrText())
		}
		return state.ExitCode(), true
	case <-time.After(within):
		return 0, false
	}
}

// TestMain removes the binary this package builds once its tests are done.
//
// serverBinary deliberately cannot use t.TempDir: the binary is built under
// sync.Once for the whole package, and the first test to arrive would own a
// directory removed when that test ends, leaving every later test pointing at a
// path that no longer exists. TestMain is the scope that actually matches — it
// outlives every test and runs after all of them — so the reason for opting out
// of t.TempDir is not a reason to leak.
//
// It is worth doing rather than leaving to the operating system. Each build is
// around 68 MB, /tmp is not always cleared between runs, and a machine that
// runs this suite through a working day accumulates a copy per run. Enough of
// them had piled up here to be worth several gigabytes.
//
// The exit code is preserved, so a failing suite still fails.
func TestMain(m *testing.M) {
	code := m.Run()
	removeBuiltBinary()
	os.Exit(code)
}

// removeBuiltBinary deletes the temporary directory serverBinary created, if it
// created one.
//
// Keyed on the directory rather than the binary so a build that failed is
// cleaned up too: MkdirTemp succeeds before the compile does, so the failing
// case leaves a directory and no binary.
//
// Failure is ignored: this runs after the tests have reported, so there is
// nobody left to tell, and a leaked temporary file is not worth turning a
// passing suite red.
func removeBuiltBinary() {
	if builtDir == "" {
		return
	}
	_ = os.RemoveAll(builtDir)
}
