//go:build collectore2e

// Package collectore2e exports this server's telemetry into a real
// OpenTelemetry Collector and asserts on what that collector parsed.
//
// # What this module is for, and what it must not duplicate
//
// test/e2e/http already has an OTLP receiver: an in-process stub that keeps
// every payload it is sent. It runs on every push, needs no daemon, and proves
// two things a real collector cannot, because it never decodes anything. It can
// report the raw Authorization header, which a collector consumes. It can be
// searched byte by byte for a value that must never leave the process, which a
// collector forwards onward rather than handing back.
//
// Never decoding anything is also its blind spot, and this module is that blind
// spot's test. A stub answers 200 to a malformed protobuf, to a resource
// missing an attribute a pipeline requires, to a metric whose unit contradicts
// its name, and to a span kind that is out of range. Every one of those ships a
// server whose telemetry no real backend can read, with a green suite behind
// it. So nothing here asserts credentials or searches for leaks: it asserts
// that a genuine receiver accepted the export, and that what it parsed out is
// the shape an operator's dashboards will be built on.
//
// # Why its own build tag
//
// httpe2e and stdioe2e both run in CI on every push, which is the right place
// for a suite that needs no daemon. This one pulls a container image and starts
// a collector, so it belongs to the deliberate Docker-mode targets rather than
// to the fast path. A separate collectore2e tag is what keeps it out of both:
// out of the default go test ./... build, since no file here compiles without
// the tag, and out of the push-triggered jobs, which pass httpe2e and stdioe2e
// and never this one. Putting it behind httpe2e with a runtime skip would have
// worked too, and would have made every fast-path run pay a docker probe and
// then report a skip that means nothing to anyone reading CI output.
//
// One scheduled job does run the tag: the race gate
// (.github/workflows/race.yml), weekly and at every release, which wants
// exactly the modules that drive the binary as a process and can pay for an
// image pull at that cadence. That is also what keeps the build seam below
// honest, since a `go test -race` half nothing ever compiles is a half that
// rots.
//
// # Why the harness is duplicated rather than shared
//
// This is the third small build-and-drive harness in test/e2e, after http and
// stdio. Each module owning its own is the existing convention, and it is worth
// the hundred lines: the alternative is a shared package compiled with no build
// tag, which the default go test ./... would then build, and a change to it
// would reach across three suites at once.
//
// What it costs is that a lesson learned in one copy stays in that copy, and
// this one was two behind. A `go test -race` run of this module used to drive
// an uninstrumented server, because the -race flag reaches only the test binary
// and the build here did not pass it on; and the health wait polled a port for
// 45 seconds without ever asking whether the process it was waiting for was
// still alive, which is how a server that died on the way up is mistaken for a
// slow one — or, worse, how the port it freed is answered by something else and
// the module grades another test's telemetry. Both were already answered in
// test/e2e/http and test/e2e/stdio, and both are answered here now. Keeping the
// copies is a decision to port such a fix three times rather than to let one
// change reach three suites at once; it is not a decision to leave two of them
// wrong.
package collectore2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
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

// The protocol a 2026-07-28 client speaks. A request missing any of this is
// answered by the SDK before the instrumentation runs, so a harness that got it
// wrong could only ever observe rejections, and a rejected request produces no
// span at all.
const (
	protocolVersion = "2026-07-28"
	protocolMeta    = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`
	acceptHeader    = "application/json, text/event-stream"
)

var (
	buildOnce   sync.Once
	builtBinary string
	builtDir    string
	errBuild    error
)

// binaryEnv names a server already built, to drive instead of building one.
//
// The Makefile's e2e targets build cmd/server once and hand it to every package
// that drives it, and until this was read here the three transport modules were
// the packages that ignored it: a run that had already staged a binary still
// paid for a compile per module. What the variable means is the same in all
// four places; how it is resolved is not. The harness reads it through the
// run's settings, which overlay .env and test/e2e/.env.docker on the process
// environment, so a value written in one of those files reaches it. Here it is
// the process environment and nothing else — a Makefile target's export, or a
// caller's own — and an entry in .env reaches this module through neither.
const binaryEnv = "E2E_SERVER_BINARY"

// serverBinary returns the path of the server these tests drive: the one
// E2E_SERVER_BINARY names, or one built once for the whole package, and ends
// the test when neither can be had.
//
// Building rather than importing is the point of every module under test/e2e
// that drives a transport: the telemetry pipeline is assembled in package main
// from flags and environment variables, and a test that reassembled it would be
// testing its own copy of the wiring rather than the wiring that ships.
//
// A path that names nothing is refused rather than built around. Falling back
// to a compile would answer a typo by silently driving a different binary from
// the one the operator staged, and the run would no longer be testing what
// they meant to test.
func serverBinary(t *testing.T) string {
	t.Helper()
	if prebuilt := os.Getenv(binaryEnv); prebuilt != "" {
		if refusal := prebuiltBinaryRefusal(); refusal != "" {
			t.Fatalf("%s names %s, which cannot be used: %s", binaryEnv, prebuilt, refusal)
		}
		staged, err := stagedBinary(prebuilt)
		if err != nil {
			t.Fatalf("%s names %s, which cannot be used: %v", binaryEnv, prebuilt, err)
		}
		return staged
	}
	bin, err := buildServerBinary()
	if err != nil {
		t.Fatalf("%v", err)
	}
	return bin
}

// stagedBinary resolves what E2E_SERVER_BINARY names to an absolute path this
// harness can execute, or says why it cannot.
//
// Absolute, because the path is executed rather than only read, and a relative
// one names two different files: os.Stat resolves it against the process's
// working directory and exec resolves it against whatever Cmd.Dir a case
// chooses. Nothing here chooses one today, so resolving changes no behavior
// in this module; it keeps the file that was checked and the file that runs
// the same file the moment something does, which is the shape that bit the
// stdio module.
//
// Regular and executable, because a stat alone accepts a directory and a file
// nothing can run. An operator who pointed the variable at the staging
// directory rather than at the binary inside it got "fork/exec …: permission
// denied" out of cmd.Start, which names neither the variable nor what is wrong
// with what it names, and saying both is the whole reason this check exists
// rather than being left to exec.
func stagedBinary(prebuilt string) (string, error) {
	abs, err := filepath.Abs(prebuilt)
	if err != nil {
		return "", err
	}
	//#nosec G703 -- the path is E2E_SERVER_BINARY, chosen by whoever runs the tests, and statting it is the smaller half of what this run does with it: the next thing is to execute it as the server under test.
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("it is not a regular file (mode %s)", info.Mode())
	}
	// Windows has no executable bit — os.Stat reports 0666 or 0444 there, from
	// the read-only attribute — so this half of the check would refuse every
	// staged binary on that platform rather than the ones that cannot run.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("it is not executable (mode %s)", info.Mode())
	}
	return abs, nil
}

// buildServerBinary builds cmd/server once for the whole package.
//
// It takes no testing.T, and the build directory is not a t.TempDir, for one
// reason: the build is shared by every test in the package, so the first test
// to arrive would own a directory removed when that test ended, leaving every
// later test pointing at nothing. The package's TestMain removes it instead.
func buildServerBinary() (string, error) {
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gitlab-mcp-collectore2e")
		if err != nil {
			errBuild = fmt.Errorf("creating the collector e2e build directory: %w", err)
			return
		}
		builtDir = dir
		out := filepath.Join(dir, "gitlab-mcp-server")
		// The build arguments and the bound come from the race seam, so a
		// `go test -race` run drives an instrumented server rather than an
		// uninstrumented one (harness_race_test.go).
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
	return builtBinary, errBuild
}

// repoRoot walks up from the test's working directory to the module root.
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

// freePort reserves a port by binding and releasing it. A small race with
// another process remains, which is why every caller here waits for readiness
// rather than assuming the port is ours the instant we ask for it.
func freePort(t *testing.T) int {
	t.Helper()
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if closeErr := l.Close(); closeErr != nil {
		t.Fatalf("releasing the reserved port: %v", closeErr)
	}
	return port
}

// server is a running binary under test.
type server struct {
	baseURL string
	logs    func() string
}

// startServer launches the binary with the given flags and environment, waits
// for /health, and stops it when the test ends.
func startServer(t *testing.T, env map[string]string, flags ...string) *server {
	t.Helper()

	bin := serverBinary(t)
	addr := "127.0.0.1:" + strconv.Itoa(freePort(t))

	args := append([]string{"--http", "--http-addr=" + addr}, flags...)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(),
		"LOG_LEVEL=info",
		"TOOL_SURFACE=dynamic",
	)
	// Before the caller's own entries, so a test that needs to say something
	// else about GORACE still can.
	cmd.Env = append(cmd.Env, raceEnviron()...)
	// An instrumented binary writes its counters where E2E_COVER_DIR says; a
	// plain one ignores the variable, so this costs an ordinary run nothing
	// (coverage_test.go).
	cmd.Env = append(cmd.Env, coverEnviron(t)...)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	var out bytes.Buffer
	var mu sync.Mutex
	cmd.Stdout = &lockedWriter{mu: &mu, buf: &out}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("starting the server: %v", err)
	}

	srv := &server{
		baseURL: "http://" + addr,
		logs: func() string {
			mu.Lock()
			defer mu.Unlock()
			return out.String()
		},
	}

	// The process is waited on once, here, and the result published by closing
	// rather than by sending: everything that asks whether the server is still
	// there has to be able to ask, and a value can only be taken once. The
	// wait's own error is recorded beside it, which is safe to read once the
	// close has been observed.
	//
	// Cmd.Wait rather than os.Process.Wait, which is what the stdio module
	// reaps with: stdout and stderr here are an io.Writer, so exec runs its own
	// copy goroutines, and Cmd.Wait is the only thing that awaits them and
	// closes the parent ends of the pipes. Reaping the process directly would
	// close the channel before the last lines were copied and leak two
	// descriptors per start.
	var waitErr error
	exited := make(chan struct{})
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()

	t.Cleanup(func() {
		// Asked before the server is told to stop, because an exit after the
		// cancel is the one this harness asked for and says nothing. A process
		// already gone at this point went on its own, which under
		// GORACE=halt_on_error=1 is what a race report looks like: the report
		// can land after the export this test asserts on was already parsed by
		// the collector, so every assertion passes, the output goes to the
		// buffer of a test nobody prints, and the exit status is the only thing
		// left saying anything happened. waitHealthy watches for the same
		// thing and stops watching the moment the server answers, which is
		// before any test has driven a single call.
		select {
		case <-exited:
			t.Errorf("the server exited on its own during the test (%s), which no test here asks it to do:\n%s",
				describeExit(waitErr, cmd), srv.logs())
		default:
		}
		// Asked to stop, then waited for, and only then canceled. The cancel
		// is a kill, and a killed server flushes no last telemetry batch and
		// runs no coverage exit hook: an instrumented binary writes its
		// counter file when it exits normally, so every child of this module
		// was losing whatever it had executed. Waiting on the reaper is the
		// barrier the assertions need either way: the Wait it made is the one
		// that awaits the copy goroutines, so by the time this returns the
		// flush has been sent and whatever the server said about it is in the
		// buffer.
		stopServer(cmd, exited)
		cancel()
		<-exited
	})

	waitHealthy(t, srv, exited)
	return srv
}

// serverStopGrace is how long a cleanup lets the server leave on its own
// before it falls back to canceling the context, which kills it. Bounded
// rather than unbounded because a server that will not stop should end a test
// run rather than hang it.
const serverStopGrace = 10 * time.Second

// stopServer asks a running server to shut down and waits for it to go.
//
// os.Interrupt is the signal, because it is the one every platform's Go
// runtime can name and the server listens for it beside SIGTERM
// (signal.NotifyContext in cmd/server). Windows refuses to deliver it to
// another process, and this returns on that refusal, leaving the caller's
// cancel to stop the server the way it always did; this module needs a Docker
// daemon and so runs on Unix in practice, which is where the two things it
// buys are real: the last telemetry batch is flushed, and an instrumented
// binary runs the exit hook that writes its coverage counters.
func stopServer(cmd *exec.Cmd, exited <-chan struct{}) {
	if cmd.Process == nil {
		return
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		return
	}
	select {
	case <-exited:
	case <-time.After(serverStopGrace):
	}
}

// describeExit names how the process ended: its recorded status when there is
// one, and the error from the wait otherwise.
//
// The status is what to report. A server that handles its termination signal
// exits 0, so the wait error alone says "<nil>" about a process that is
// certainly gone, which is the least useful thing a message about an exit could
// say.
func describeExit(waitErr error, cmd *exec.Cmd) string {
	if state := cmd.ProcessState; state != nil {
		return state.String()
	}
	if waitErr != nil {
		return waitErr.Error()
	}
	return "exit status not recorded"
}

// lockedWriter serializes writes from the process's two pipes into one buffer
// the test can read while the process is still running.
type lockedWriter struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

// waitHealthy polls /health until the server answers or the deadline passes. A
// failure dumps the process output, because a server that refuses to start has
// already said why and nobody should have to go looking.
//
// The process is watched while it is polled, and that is not a refinement of
// the timeout. freePort above hands back a number rather than a listener, so
// between the release and the bind the port belongs to nobody: a server that
// died on the way up leaves it free for anything else to answer on, and a
// health check answered by somebody else is worse than one that fails, because
// the test would go on to drive a server configured for another test and grade
// its telemetry. Asking whether the process is still there is what tells those
// two apart, and it is why a dead server is reported in a moment rather than
// after 45 seconds of polling something that will never answer.
func waitHealthy(t *testing.T, s *server, exited <-chan struct{}) {
	t.Helper()
	healthClient := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			t.Fatalf("the server exited before it served. Output:\n%s", s.logs())
		default:
		}

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, s.baseURL+"/health", http.NoBody)
		if err != nil {
			t.Fatalf("building the health request: %v", err)
		}
		resp, err := healthClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("server never became healthy. Output:\n%s", s.logs())
}

// callAction drives one tools/call that reaches the instrumentation.
//
// Three things beyond a credential are required before a handler runs, and
// getting any of them wrong yields a refusal rather than a span: the _meta
// block carrying the protocol version, the Mcp-Method and Mcp-Name headers that
// protocol 2026-07-28 makes required on a POST, and the Mcp-Param-Action header
// mirroring the action argument, without which the transport answers -32020
// before any middleware sees the request.
// callTool posts one tools/call with the given tool name and arguments.
//
// callAction is this with the dynamic surface's envelope filled in. The meta
// and individual surfaces name a different tool and nest their parameters
// differently, and the whole point of covering them is that those shapes are
// not interchangeable: the surface decides what a call looks like, which is
// also what decides whether gitlab_mcp.action can be resolved from it.
func (s *server) callTool(t *testing.T, id int, tool, arguments string) {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) +
		`,"method":"tools/call","params":{` + protocolMeta +
		`,"name":"` + tool + `","arguments":` + arguments + `}}`

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the tools/call request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	req.Header.Set("Mcp-Method", "tools/call")
	req.Header.Set("Mcp-Name", tool)
	if action := topLevelAction(arguments); action != "" {
		req.Header.Set("Mcp-Param-Action", action)
	}
	req.Header.Set("PRIVATE-TOKEN", "glpat-collector-e2e-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	defer resp.Body.Close()

	payload, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		t.Fatalf("reading the MCP response: %v (got %d bytes)", readErr, len(payload))
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		t.Fatalf("the call was refused with %d, so no span exists to assert on. Server output:\n%s",
			resp.StatusCode, s.logs())
	}
	// Both ways a call can fail, because they are different mechanisms and
	// checking one is how a refusal travels unnoticed: a JSON-RPC error means
	// the request never reached a handler, and isError means a handler ran and
	// reported failure to the model.
	if bytes.Contains(payload, []byte(`"error":{`)) {
		t.Fatalf("%s was refused with a JSON-RPC error, so no handler ran:\n%s", tool, tailOfPayload(payload))
	}
	if bytes.Contains(payload, []byte(`"isError":true`)) {
		t.Fatalf("%s answered with an error result, so no handler ran:\n%s", tool, tailOfPayload(payload))
	}
}

// topLevelAction returns the action named at the top level of a call's
// arguments, or "".
//
// Protocol revision 2026-07-28 mirrors a call's parameters into Mcp-Param-*
// headers, and the stateless transport requires the ones the tool declares. The
// dynamic and meta surfaces both take an action there; the individual surface
// does not, so its calls carry no such header and must not be given one.
//
// Read with a decoder rather than by string matching, so a project path that
// happens to contain the word does not produce a header.
func topLevelAction(arguments string) string {
	var decoded struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal([]byte(arguments), &decoded); err != nil {
		return ""
	}
	return decoded.Action
}

func (s *server) callAction(t *testing.T, id int, action, projectID string) {
	t.Helper()

	// The action's own parameters go under params, which is the shape
	// gitlab_execute_action declares. They used to be siblings of action, and
	// the server refused every call with "unexpected additional properties":
	// the request never reached a handler, never called GitLab, and never
	// logged anything.
	//
	// Nothing here noticed for the life of the module, because every assertion
	// was about the MCP span and the middleware creates that before the handler
	// runs. A module whose reason for existing is not to be graded by our own
	// code was driving a request our own code rejected.
	body := `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) +
		`,"method":"tools/call","params":{` + protocolMeta +
		`,"name":"gitlab_execute_action","arguments":{"action":"` + action +
		`","params":{"project_id":"` + projectID + `"}}}}`

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the tools/call request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	req.Header.Set("Mcp-Method", "tools/call")
	req.Header.Set("Mcp-Name", "gitlab_execute_action")
	req.Header.Set("Mcp-Param-Action", action)
	req.Header.Set("PRIVATE-TOKEN", "glpat-collector-e2e-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	defer resp.Body.Close()
	// The body is drained and discarded on purpose. Whether GitLab would have
	// answered is not what this module tests. What is read here is whether the
	// call reached a handler at all, which the two checks below decide.
	payload, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		t.Fatalf("the call was refused with %d, so it never reached the instrumentation and no span exists to assert on. Server output:\n%s",
			resp.StatusCode, s.logs())
	}

	// A 200 is not enough, and assuming it was is how the malformed request
	// above survived. MCP reports a refused call inside a successful response,
	// so a schema rejection arrives as isError with the reason in the text and
	// every assertion in this module goes on passing against a server that
	// never ran a handler.
	if bytes.Contains(payload, []byte(`"isError":true`)) {
		t.Fatalf("the server answered with an error result, so no handler ran and no GitLab call was made:\n%s",
			tailOfPayload(payload))
	}
}

// tailOfPayload returns the end of a response body, where the content sits
// after the server-info preamble every result carries.
func tailOfPayload(payload []byte) string {
	const want = 400
	flat := strings.ReplaceAll(string(payload), "\n", " ")
	if len(flat) <= want {
		return flat
	}
	return flat[len(flat)-want:]
}

// failingProject is the project id the fake instance answers with a 500, for
// the cases that need a failure this server counts as its own.
const failingProject = "always-failing-project"

// fakeGitLab is a running fake instance whose project payload can be changed.
//
// Only the project changes, and only because a subscription is defined by
// noticing that a read returned something different. The rest stays static:
// a fake that can drift in several places is a fake whose failures need
// diagnosing.
type fakeGitLab struct {
	url         string
	description atomic.Pointer[string]
}

// URL is where the server should point.
func (f *fakeGitLab) URL() string { return f.url }

// change makes the next read of the project return something different.
func (f *fakeGitLab) change(text string) {
	f.description.Store(&text)
}

// startFakeGitLab serves the endpoints the server probes when it builds a pool
// entry, so a credential is admitted and the call reaches the middleware.
//
// The instrumentation sits inside the authentication gate. Without an instance
// that accepts the token there is no span to collect, which is a deliberate
// property of the design and an obstacle to testing it.
func startFakeGitLab(t *testing.T) string {
	t.Helper()
	return startMutableFakeGitLab(t).URL()
}

// startMutableFakeGitLab is the same instance, returned so a test can change
// what it answers.
func startMutableFakeGitLab(t *testing.T) *fakeGitLab {
	t.Helper()

	fake := &fakeGitLab{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"username":"collector-e2e"}`))
	})
	// The endpoint the driven action actually calls, answered with an empty
	// page so the tool call succeeds.
	//
	// It used to fall through to the 404 below, which made every driven call
	// fail. That was invisible while the assertions were about the MCP span
	// alone, and it hid two things the moment anything looked further: a failed
	// call makes no GitLab request, so no client span exists to check the trace
	// tree against, and it takes the "tool call completed" record with it, so
	// there is no correlated log record either. The fake answering the call is
	// what makes those two observable at all.
	mux.HandleFunc("/api/v4/projects/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		// One project that fails, so the failure path has somewhere to happen.
		// Without it every call here either succeeds or is refused for a
		// caller fault, and this server deliberately records no error.type for
		// those: a model naming an action that does not exist is an ordinary
		// event, not a malfunction. So nothing exercised error.type at all.
		case strings.Contains(r.URL.Path, failingProject):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"500 Internal Server Error"}`))
		case strings.HasSuffix(r.URL.Path, "/issues"):
			w.Header().Set("X-Total", "0")
			_, _ = w.Write([]byte(`[]`))
		case strings.Count(strings.Trim(r.URL.EscapedPath(), "/"), "/") == 3:
			// The project itself, which the gitlab://project/{ref} resource
			// reads. Enough fields for the handler to render it; the test is
			// about what telemetry says, not about the payload.
			description := ""
			if current := fake.description.Load(); current != nil {
				description = *current
			}
			_, _ = w.Write([]byte(`{"id":1,"name":"some-project",` +
				`"path_with_namespace":"some-group/some-project","default_branch":"main",` +
				`"description":"` + description + `",` +
				`"visibility":"private","web_url":"http://example.invalid/some-group/some-project"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		// Scope and tier probes hit other paths; 404 means "unavailable",
		// which every caller handles.
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	fake.url = srv.URL
	return fake
}

// TestMain removes the binary this package builds once the suite has finished.
//
// serverBinary deliberately does not use t.TempDir, because the binary outlives
// the test that happened to build it, so nothing else is in a position to clean
// up. The server binary is tens of megabytes, /tmp is not always cleared between
// runs, and a machine running this module through a working day accumulates a
// copy per run. test/e2e/http carries the same teardown for the same reason.
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
// Keyed on the directory rather than on the binary so a build that failed is
// cleaned up too: MkdirTemp succeeds before the compile does, so a failing build
// leaves a directory and no binary.
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

// legacyProtocolVersion is the newest revision that has sessions.
//
// Revision 2026-07-28 is stateless only, so a server started with
// --stateless=false does not advertise it and answers "unsupported protocol
// version" to a client that insists. Anything about session identity or session
// duration has to be driven over this one.
const legacyProtocolVersion = "2025-11-25"

// session is an established MCP session on a stateful deployment.
type session struct {
	srv *server
	id  string
}

// openSession performs the handshake and returns the session.
//
// The initialized notification is not optional: the SDK treats a session as
// incomplete until it arrives, and a tools/call before it is refused, which
// would look from here exactly like the refusals this module has already
// mistaken for success twice.
func (s *server) openSession(t *testing.T) *session {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{` +
		`"protocolVersion":"` + legacyProtocolVersion + `",` +
		`"capabilities":{},"clientInfo":{"name":"collector-e2e","version":"1"}}}`

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the initialize request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("MCP-Protocol-Version", legacyProtocolVersion)
	req.Header.Set("PRIVATE-TOKEN", "glpat-collector-e2e-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST initialize: %v", err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)

	id := resp.Header.Get("Mcp-Session-Id")
	if id == "" {
		t.Fatalf("the deployment issued no session id (status %d); it is not running statefully.\nResponse:\n%s\nServer:\n%s",
			resp.StatusCode, tailOfPayload(payload), s.logs())
	}

	sess := &session{srv: s, id: id}
	sess.notify(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	return sess
}

// notify posts a notification, which has no response to check.
func (sess *session) notify(t *testing.T, body string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, sess.srv.baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the notification: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("MCP-Protocol-Version", legacyProtocolVersion)
	req.Header.Set("Mcp-Session-Id", sess.id)
	req.Header.Set("PRIVATE-TOKEN", "glpat-collector-e2e-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST notification: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
}

// call posts one request inside the session and fails on either kind of
// refusal, for the reason callTool does.
func (sess *session) call(t *testing.T, id int, method, params string) []byte {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"` + method + `","params":` + params + `}`

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, sess.srv.baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the %s request: %v", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("MCP-Protocol-Version", legacyProtocolVersion)
	req.Header.Set("Mcp-Session-Id", sess.id)
	req.Header.Set("PRIVATE-TOKEN", "glpat-collector-e2e-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", method, err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)

	if bytes.Contains(payload, []byte(`"error":{`)) {
		t.Fatalf("%s was refused with a JSON-RPC error:\n%s", method, tailOfPayload(payload))
	}
	return payload
}

// close ends the session, which is what makes its duration observable.
//
// A session that is merely abandoned ends when the idle timeout fires, which is
// minutes away and longer than any test should wait.
func (sess *session) close(t *testing.T) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, sess.srv.baseURL+"/mcp", nil)
	if err != nil {
		t.Fatalf("building the delete: %v", err)
	}
	req.Header.Set("MCP-Protocol-Version", legacyProtocolVersion)
	req.Header.Set("Mcp-Session-Id", sess.id)
	req.Header.Set("PRIVATE-TOKEN", "glpat-collector-e2e-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /mcp: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
}

// post sends one JSON-RPC request and returns the response body, asserting
// nothing about it.
//
// The helpers built on this decide what counts as failure, because they differ:
// most calls must succeed, and the one driving an unknown prompt must not.
func (s *server) post(t *testing.T, method, name, body string) []byte {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the %s request: %v", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	req.Header.Set("Mcp-Method", method)
	if name != "" {
		req.Header.Set("Mcp-Name", name)
	}
	req.Header.Set("PRIVATE-TOKEN", "glpat-collector-e2e-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", method, err)
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		t.Fatalf("%s was refused with %d, so no span exists to assert on. Server output:\n%s",
			method, resp.StatusCode, s.logs())
	}
	return payload
}

// readResource reads one resource and fails if the read was refused.
func (s *server) readResource(t *testing.T, id int, uri string) {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) +
		`,"method":"resources/read","params":{` + protocolMeta + `,"uri":"` + uri + `"}}`

	payload := s.post(t, "resources/read", uri, body)
	if bytes.Contains(payload, []byte(`"error":{`)) {
		t.Fatalf("the read was refused, so no handler ran:\n%s", tailOfPayload(payload))
	}
}

// getPromptExpectingRefusal asks for a prompt by a name this server does not
// have, and requires the refusal.
//
// Named for what it does because it inverts the rule every other helper here
// follows. The subject is the refusal: a name that names nothing is how a
// caller would mint metric series, so the assertion is about what the refused
// call recorded, and a call that unexpectedly succeeded would mean the fixture
// had stopped being an unknown name.
func (s *server) getPromptExpectingRefusal(t *testing.T, id int, name string) {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) +
		`,"method":"prompts/get","params":{` + protocolMeta +
		`,"name":"` + name + `","arguments":{}}}`

	payload := s.post(t, "prompts/get", name, body)
	if !bytes.Contains(payload, []byte(`"error":{`)) {
		t.Fatalf("prompts/get %q was served; the fixture is no longer an unknown name:\n%s",
			name, tailOfPayload(payload))
	}
}

// metricsPath is where the collector writes the metric documents.
func metricsPath(c *collector) string {
	return filepath.Join(c.outDir, metricsFile)
}

// callToolTolerant posts a tools/call and requires only that it reached the
// instrumentation.
//
// The counterpart to callTool, for the cases whose subject is a refusal: safe
// mode answers with a preview and read-only removes the tool, so both come back
// as failures and neither is a defect. Named for the difference rather than
// taking a flag, because a boolean at the call site says nothing about which
// way it points.
func (s *server) callToolTolerant(t *testing.T, id int, tool, arguments string) {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) +
		`,"method":"tools/call","params":{` + protocolMeta +
		`,"name":"` + tool + `","arguments":` + arguments + `}}`

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the tools/call request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	req.Header.Set("Mcp-Method", "tools/call")
	req.Header.Set("Mcp-Name", tool)
	if action := topLevelAction(arguments); action != "" {
		req.Header.Set("Mcp-Param-Action", action)
	}
	req.Header.Set("PRIVATE-TOKEN", "glpat-collector-e2e-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		t.Fatalf("the call was refused with %d before any instrumentation ran. Server output:\n%s",
			resp.StatusCode, s.logs())
	}
}

// callWithTraceContext posts a tools/call carrying W3C trace context in
// params._meta.
//
// Unprefixed keys, which is the exception MCP grants: "the keys traceparent,
// tracestate, and baggage are reserved for OpenTelemetry trace context
// propagation". Sending a DNS-prefixed variant instead would be wrong rather
// than merely unusual, and would test nothing.
func (s *server) callWithTraceContext(t *testing.T, id int, traceparent string) {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) +
		`,"method":"tools/call","params":{` +
		`"_meta":{"io.modelcontextprotocol/protocolVersion":"` + protocolVersion + `",` +
		`"io.modelcontextprotocol/clientCapabilities":{},` +
		`"traceparent":"` + traceparent + `"},` +
		`"name":"gitlab_execute_action","arguments":{"action":"issue.list",` +
		`"params":{"project_id":"some-group/some-project"}}}}`

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the tools/call request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	req.Header.Set("Mcp-Method", "tools/call")
	req.Header.Set("Mcp-Name", "gitlab_execute_action")
	req.Header.Set("Mcp-Param-Action", "issue.list")
	req.Header.Set("PRIVATE-TOKEN", "glpat-collector-e2e-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(resp.Body)
	if bytes.Contains(payload, []byte(`"error":{`)) || bytes.Contains(payload, []byte(`"isError":true`)) {
		t.Fatalf("the call was refused, so no span carries the caller's context:\n%s", tailOfPayload(payload))
	}
}
