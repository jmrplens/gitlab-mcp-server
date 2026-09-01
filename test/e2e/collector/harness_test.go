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
// # Why the harness is duplicated rather than shared
//
// This is the third small build-and-drive harness in test/e2e, after http and
// stdio. Each module owning its own is the existing convention, and it is worth
// the hundred lines: the alternative is a shared package compiled with no build
// tag, which the default go test ./... would then build, and a change to it
// would reach across three suites at once.
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

// serverBinary builds cmd/server once for the whole package.
//
// Building rather than importing is the point of every module under test/e2e
// that drives a transport: the telemetry pipeline is assembled in package main
// from flags and environment variables, and a test that reassembled it would be
// testing its own copy of the wiring rather than the wiring that ships.
func serverBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		// Not t.TempDir: the binary is built once for the package under
		// sync.Once, so the first test to arrive would own a directory removed
		// when that test ends, leaving every later test pointing at nothing.
		dir, err := os.MkdirTemp("", "gitlab-mcp-collectore2e") //nolint:usetesting // see above
		if err != nil {
			errBuild = fmt.Errorf("creating the collector e2e build directory: %w", err)
			return
		}
		builtDir = dir
		out := filepath.Join(dir, "gitlab-mcp-server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "build", "-o", out, "./cmd/server")
		cmd.Dir = repoRoot()
		if output, runErr := cmd.CombinedOutput(); runErr != nil {
			errBuild = fmt.Errorf("building cmd/server: %w\n%s", runErr, output)
			return
		}
		builtBinary = out
	})
	if errBuild != nil {
		t.Fatalf("%v", errBuild)
	}
	return builtBinary
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

	t.Cleanup(func() {
		// Cancel first, then wait: the process flushes its last telemetry
		// batch on the way out, and a test that killed it without waiting
		// would race that flush against its own assertions.
		cancel()
		_ = cmd.Wait()
	})

	waitHealthy(t, srv)
	return srv
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
func waitHealthy(t *testing.T, s *server) {
	t.Helper()
	healthClient := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
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
