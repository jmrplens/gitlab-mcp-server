//go:build e2e

// server_test.go covers the launcher: the environment a child is given, the
// stderr it is judged by, and one run of the real binary against a stub GitLab
// to prove the three fit together.

package harness

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// testSettings builds the resolved configuration a child is launched from.
func testSettings(values map[string]string) settings {
	return settings{values: values}
}

// logCapture collects what the standard logger writes while one test runs.
//
// Guarded, because the standard logger is process-wide and this package has
// goroutines that write to it: the OTLP receiver says when it stopped, and the
// recorder says when no span has arrived. One of those writing while the test
// reads would be a data race the detector is right to report, and a mutex is
// the whole cost of not having one.
type logCapture struct {
	mu      sync.Mutex
	written strings.Builder
}

func (c *logCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.written.Write(p)
}

// String returns everything logged since the capture was installed.
func (c *logCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.written.String()
}

// captureLog points the standard logger at a buffer for the rest of this test
// and restores it afterwards.
//
// The harness reports through the standard logger everything it deliberately
// does not fail on: a child that would not stop, a run that wrote fewer
// coverage counters than it started children. A test of those lines has to
// read the log, because the line is the whole of the behavior.
func captureLog(t *testing.T) *logCapture {
	t.Helper()

	capture := &logCapture{}
	previous := log.Writer()
	log.SetOutput(capture)
	t.Cleanup(func() { log.SetOutput(previous) })
	return capture
}

// environMap turns a child's environment back into a map for assertions.
func environMap(t *testing.T, environ []string) map[string]string {
	t.Helper()
	vars := map[string]string{}
	for _, entry := range environ {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			t.Fatalf("environment entry %q is not KEY=VALUE", entry)
		}
		vars[key] = value
	}
	return vars
}

// TestChildEnv_ProcessCarriesYoloMode_TheChildNeverDoes is the assertion the
// whole "built from nothing" rule exists for.
//
// GITLAB_MCP_YOLO_MODE and AUTOPILOT skip the confirmation a destructive
// action asks for. A developer who has either exported, or an agent runtime
// that sets AUTOPILOT as a convention, would otherwise run the whole suite
// against a server nobody deploys, and every test that asserts a destructive
// call is refused without a confirmation would pass for the wrong reason.
//
// Both routes are covered: the process environment, which the builder must not
// read, and the session's own variables, where a forbidden key is dropped
// rather than honored.
func TestChildEnv_ProcessCarriesYoloMode_TheChildNeverDoes(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "true")
	t.Setenv("AUTOPILOT", "1")
	t.Setenv("YOLO_MODE", "yes")

	env := newChildEnv(testSettings(map[string]string{envGitLabURL: "http://gitlab.test", envGitLabToken: "glpat-x"}),
		t.TempDir(), map[string]string{"AUTOPILOT": "1", "GITLAB_MCP_TOOL_SURFACE": "meta"})

	vars := environMap(t, env.environ())
	for _, forbidden := range forbiddenChildKeys {
		if value, present := vars[forbidden]; present {
			t.Fatalf("the child was given %s=%q", forbidden, value)
		}
	}
	if vars["GITLAB_MCP_TOOL_SURFACE"] != "meta" {
		t.Fatalf("the session's own variables were dropped with the forbidden one: %v", vars)
	}
}

// TestChildEnv_ProcessEnvironment_IsNotInherited checks that nothing but the
// three variables a process cannot run without crosses over.
//
// The one that matters is the tool surface: a developer with
// GITLAB_MCP_TOOL_SURFACE exported would otherwise change which surface every
// test exercised, silently, and the run would still be green.
func TestChildEnv_ProcessEnvironment_IsNotInherited(t *testing.T) {
	t.Setenv("GITLAB_MCP_TOOL_SURFACE", "individual")
	t.Setenv("GITLAB_MCP_READ_ONLY", "true")
	t.Setenv("E2E_HARNESS_UNRELATED", "leaked")

	env := newChildEnv(testSettings(map[string]string{envGitLabURL: "http://gitlab.test", envGitLabToken: "glpat-x"}),
		t.TempDir(), nil)

	vars := environMap(t, env.environ())
	for _, key := range []string{"GITLAB_MCP_TOOL_SURFACE", "GITLAB_MCP_READ_ONLY", "E2E_HARNESS_UNRELATED"} {
		t.Run(key, func(t *testing.T) {
			if _, present := vars[key]; present {
				t.Fatalf("the child inherited %s from the test process", key)
			}
		})
	}
	if vars["PATH"] == "" {
		t.Fatal("the child was given no PATH, so it cannot run git or anything else it shells out to")
	}
}

// TestChildEnv_LocalPaths_PointAtTheHarnessRoot checks the three allow-lists
// and the home the server resolves its own configuration through.
//
// They all name one directory, which is also the child's working directory: a
// tool that reads a local file, one that writes a download and one that reads
// an import archive are then confined to a directory this run created, rather
// than to whatever the test process happened to be started in.
func TestChildEnv_LocalPaths_PointAtTheHarnessRoot(t *testing.T) {
	root := t.TempDir()

	env := newChildEnv(testSettings(map[string]string{envGitLabURL: "http://gitlab.test", envGitLabToken: "glpat-x"}), root, nil)

	vars := environMap(t, env.environ())
	for _, key := range []string{
		"HOME",
		"GITLAB_MCP_ALLOWED_UPLOAD_DIRS",
		"GITLAB_MCP_ALLOWED_DOWNLOAD_DIRS",
		"GITLAB_MCP_ALLOWED_IMPORT_DIRS",
	} {
		t.Run(key, func(t *testing.T) {
			if vars[key] != root {
				t.Fatalf("%s = %q, want the harness root %q", key, vars[key], root)
			}
		})
	}
	if env.Root != root {
		t.Fatalf("child working directory = %q, want the harness root %q", env.Root, root)
	}
}

// TestChildEnv_Instance_IsTheOneTheRunResolved checks that the child talks to
// the instance the harness probed, with the same TLS policy.
//
// The TLS flag is spelled out rather than omitted when false, so the child's
// environment states the policy instead of relying on a default that could
// change.
func TestChildEnv_Instance_IsTheOneTheRunResolved(t *testing.T) {
	env := newChildEnv(testSettings(map[string]string{
		envGitLabURL:     "http://gitlab.test:8929",
		envGitLabToken:   "glpat-secret",
		envSkipTLSVerify: "TRUE",
	}), t.TempDir(), nil)

	vars := environMap(t, env.environ())
	if vars[envGitLabURL] != "http://gitlab.test:8929" || vars[envGitLabToken] != "glpat-secret" {
		t.Fatalf("the child was pointed somewhere else: %v", vars)
	}
	if vars[envSkipTLSVerify] != "true" {
		t.Fatalf("%s = %q, want true", envSkipTLSVerify, vars[envSkipTLSVerify])
	}
}

// TestChildEnv_Environ_IsSorted checks that two children built from the same
// inputs are started identically, so a difference between two runs is a
// difference somebody made.
func TestChildEnv_Environ_IsSorted(t *testing.T) {
	env := newChildEnv(testSettings(map[string]string{envGitLabURL: "http://gitlab.test", envGitLabToken: "glpat-x"}),
		t.TempDir(), map[string]string{"ZZZ_LAST": "1", "AAA_FIRST": "1"})

	environ := env.environ()

	if !slices.IsSorted(environ) {
		t.Fatalf("the child environment is not sorted: %v", environ)
	}
}

// TestStderrSink_MoreThanTheTail_KeepsTheEnd checks that a talkative child
// does not grow the harness without bound and that what is kept is the end.
//
// The end is what matters: a server that died says why in its last lines, and
// the first four kilobytes of a startup log say nothing about it.
func TestStderrSink_MoreThanTheTail_KeepsTheEnd(t *testing.T) {
	sink := &stderrSink{}
	for range 10 {
		if _, err := sink.Write([]byte(strings.Repeat("a", 1024))); err != nil {
			t.Fatalf("Write() error = %v, want nil", err)
		}
	}
	if _, err := sink.Write([]byte("the last thing it said")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	tail := sink.tail()
	if len(tail) > stderrTailBytes {
		t.Fatalf("tail length = %d, want at most %d", len(tail), stderrTailBytes)
	}
	if !strings.HasSuffix(tail, "the last thing it said") {
		t.Fatalf("the tail does not end with the last thing written: %q", tail[max(0, len(tail)-64):])
	}
}

// TestStderrSink_NoLogFile_StillRecordsTheTail checks that a sink which could
// not open its file keeps working.
//
// Losing the log is a nuisance; losing the server because its stderr could not
// be stored would be a failure of the harness itself.
func TestStderrSink_NoLogFile_StillRecordsTheTail(t *testing.T) {
	sink := &stderrSink{}
	sink.close()

	if _, err := sink.Write([]byte("logged anyway")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}
	if sink.tail() != "logged anyway" {
		t.Fatalf("tail = %q, want the written text", sink.tail())
	}
}

// TestServerBinary_MissingPrebuiltPath_IsRefused checks that a pointer to a
// binary that is not there is reported rather than built around.
//
// E2E_SERVER_BINARY exists so the three packages share one build; a typo in it
// would otherwise be answered by silently building a second one, and the run
// would no longer be testing what the operator staged.
func TestServerBinary_MissingPrebuiltPath_IsRefused(t *testing.T) {
	_, err := serverBinary(filepath.Join(t.TempDir(), "not-there"))
	if err == nil {
		t.Fatal("serverBinary() accepted a path that does not exist")
	}
	if !strings.Contains(err.Error(), binaryEnv) {
		t.Fatalf("the error should name %s: %v", binaryEnv, err)
	}
}

// TestServerProcess_RealBinary_ServesTheDefaultSurface starts the server this
// suite drives and speaks MCP to it.
//
// This is the smallest whole test of what S04 delivers: the binary is built,
// launched with an environment built from nothing, and answers tools/list with
// the two tools the default dynamic surface registers. It also proves the
// reaper, which is what lets a later failure say "the server exited" instead
// of timing out.
//
// The GitLab behind it is a stub answering only what the server asks at
// startup, so the test needs no instance and no credentials.
func TestServerProcess_RealBinary_ServesTheDefaultSurface(t *testing.T) {
	stub := startStubGitLab(t)

	bin, err := serverBinary("")
	if err != nil {
		t.Fatalf("building the server: %v", err)
	}

	root := t.TempDir()
	env := newChildEnv(testSettings(map[string]string{
		envGitLabURL:   stub.URL,
		envGitLabToken: "glpat-harness-smoke",
	}), root, map[string]string{"GITLAB_MCP_LOG_LEVEL": "info"})

	proc := newServerProcess("smoke", bin, env)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "harness-smoke", Version: "1"}, nil)
	session, err := client.Connect(ctx, proc.transport(ctx), nil)
	if err != nil {
		t.Fatalf("connecting to the server: %v\nstderr: %s", err, proc.stderrTail())
	}

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v\nstderr: %s", err, proc.stderrTail())
	}
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	for _, want := range []string{"gitlab_find_action", "gitlab_execute_action"} {
		t.Run(want, func(t *testing.T) {
			if !slices.Contains(names, want) {
				t.Fatalf("the default surface served %v, want it to include %s", names, want)
			}
		})
	}
	if !proc.alive() {
		t.Fatalf("the server is not running after answering: %s\nstderr: %s", proc.exitStatus(), proc.stderrTail())
	}

	// Closing the session closes the child's stdin, which is how a stdio
	// client asks a server to stop. The reaper is what notices.
	const exitWindow = 30 * time.Second
	_ = session.Close()
	deadline := time.Now().Add(exitWindow)
	for proc.alive() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if proc.alive() {
		t.Fatalf("the server was still running %s after its stdin closed\nstderr: %s", exitWindow, proc.stderrTail())
	}
}

// TestServerProcess_ContextCancel_EndsTheChildWithACleanExit checks that the
// end of a run asks the child to stop rather than killing it.
//
// The difference is invisible in a run's verdict and decides everything about
// what the run measured: the Go runtime writes an instrumented binary's
// coverage counters from an exit hook, and a killed process runs none of
// those. So it is asserted on the recorded exit status, which is the server's
// own when it was asked and `signal: killed` when it was not.
//
// Windows is skipped rather than covered: the harness keeps exec's kill there
// because Windows has no SIGTERM, and nothing runs this suite on it.
func TestServerProcess_ContextCancel_EndsTheChildWithACleanExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the harness keeps exec's kill here, since Windows has no SIGTERM and no run of this suite happens here")
	}
	stub := startStubGitLab(t)

	bin, err := serverBinary("")
	if err != nil {
		t.Fatalf("building the server: %v", err)
	}

	env := newChildEnv(testSettings(map[string]string{
		envGitLabURL:   stub.URL,
		envGitLabToken: "glpat-harness-terminate",
	}), t.TempDir(), nil)

	proc := newServerProcess("terminate", bin, env)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "harness-terminate", Version: "1"}, nil)
	session, err := client.Connect(ctx, proc.transport(ctx), nil)
	if err != nil {
		t.Fatalf("connecting to the server: %v\nstderr: %s", err, proc.stderrTail())
	}
	// Closing the session is what the run does after the cancel, and what
	// collects the command exec is watching.
	defer func() { _ = session.Close() }()

	if _, err = session.ListTools(ctx, nil); err != nil {
		t.Fatalf("tools/list: %v\nstderr: %s", err, proc.stderrTail())
	}

	cancel()

	const exitWindow = 30 * time.Second
	if !proc.waitForExit(time.Now().Add(exitWindow)) {
		t.Fatalf("the server was still running %s after its context was cancelled\nstderr: %s", exitWindow, proc.stderrTail())
	}
	if got := proc.exitStatus(); got != "exit status 0" {
		t.Fatalf("the server ended as %q, want a clean exit: a killed child runs no exit hook\nstderr: %s",
			got, proc.stderrTail())
	}
}

// TestServerProcess_HTTPContextCancel_EndsTheChildWithACleanExit is the twin
// of the test above for the transport where the cancel is the whole of the
// ending.
//
// A stdio child is also ended by its client closing the pipe it speaks over,
// so the seam there has a second thing holding it up. An HTTP child's client
// holds no pipe of the process at all: canceling the session's context is the
// only thing that ever stops one, so terminateOnCancel could be dropped from
// httpTransport with every other test in this package still green, and every
// HTTP child of an instrumented run would be killed and contribute nothing.
//
// No MCP client is needed to ask the question: httpTransport returns only
// after the child has answered its own health endpoint, which is the whole
// proof that a process was started and got as far as serving.
func TestServerProcess_HTTPContextCancel_EndsTheChildWithACleanExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the harness keeps exec's kill here, since Windows has no SIGTERM and no run of this suite happens here")
	}
	stub := startStubGitLab(t)

	bin, err := serverBinary("")
	if err != nil {
		t.Fatalf("building the server: %v", err)
	}

	env := newChildEnv(testSettings(map[string]string{
		envGitLabURL:   stub.URL,
		envGitLabToken: "glpat-harness-http-terminate",
	}), t.TempDir(), nil)

	proc := newServerProcess("http-terminate", bin, env)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	addr, err := freeLoopbackAddr(ctx)
	if err != nil {
		t.Fatalf("reserving an address for the child: %v", err)
	}
	if _, err = proc.httpTransport(ctx, addr); err != nil {
		t.Fatalf("starting the server on %s: %v", addr, err)
	}

	cancel()

	const exitWindow = 30 * time.Second
	if !proc.waitForExit(time.Now().Add(exitWindow)) {
		t.Fatalf("the HTTP server was still running %s after its context was cancelled\nstderr: %s",
			exitWindow, proc.stderrTail())
	}
	if got := proc.exitStatus(); got != "exit status 0" {
		t.Fatalf("the HTTP server ended as %q, want a clean exit: a killed child runs no exit hook\nstderr: %s",
			got, proc.stderrTail())
	}
}

// TestServerProcess_WaitForExit_EachChildState_IsAnswered covers the bound the
// end of a run leans on: it must not wait forever for a child that will not
// stop, and must not report one that already has as still running.
func TestServerProcess_WaitForExit_EachChildState_IsAnswered(t *testing.T) {
	stopped := make(chan struct{})
	close(stopped)

	cases := []struct {
		name   string
		proc   *serverProcess
		exited bool
	}{
		{name: "never started", proc: &serverProcess{label: "never-started"}, exited: true},
		{name: "already collected", proc: &serverProcess{label: "collected", exited: stopped}, exited: true},
		{name: "still running", proc: &serverProcess{label: "running", exited: make(chan struct{})}, exited: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// A deadline already past for the running case, since what is
			// being checked is that it returns rather than how long it waits.
			deadline := time.Now()
			if testCase.exited {
				deadline = deadline.Add(time.Minute)
			}
			if got := testCase.proc.waitForExit(deadline); got != testCase.exited {
				t.Fatalf("waitForExit() = %t, want %t", got, testCase.exited)
			}
		})
	}
}

// startStubGitLab serves the handful of endpoints the server asks at startup,
// and nothing else.
//
// It is deliberately minimal: what is under test here is the launcher, and a
// stub that answered tool calls would invite assertions about tools that
// belong against a real instance.
func startStubGitLab(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		writeStubJSON(w, map[string]any{"version": "18.0.0", "revision": "abcdef", "enterprise": false})
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		writeStubJSON(w, map[string]any{"id": 7, "username": "harness", "name": "Harness", "is_admin": true})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		// The license and the token's own scopes are both asked for and both
		// optional: a 404 is what an unlicensed instance and an older GitLab
		// answer, and the server is expected to carry on.
		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// writeStubJSON answers one stub request.
func writeStubJSON(w http.ResponseWriter, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	encoded, err := json.Marshal(body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(encoded)
}

// TestRemoveBuiltBinary_NothingBuilt_DoesNothing checks that the cleanup Main
// runs is safe for a package whose tests never needed a server.
func TestRemoveBuiltBinary_NothingBuilt_DoesNothing(t *testing.T) {
	before := builtDir
	t.Cleanup(func() { builtDir = before })

	builtDir = ""
	removeBuiltBinary()

	if _, err := os.Stat(before); before != "" && err != nil {
		t.Fatalf("the built directory was removed by a call that should have done nothing: %v", err)
	}
}
