//go:build httpe2e

// harness_test.go starts the real server binary over HTTP and drives it the way
// a client does.
//
// The existing e2e suite exercises tool behavior through an in-memory MCP
// transport, which is the right shape for that question and answers none of
// this one: cross-origin decisions, preflight, per-address rate limiting,
// authentication modes and the JSON-RPC shape of a rejection all live in the
// HTTP handler chain in package main, which no test could reach. Every bug this
// module pins was found by hand first, twice.
//
// The binary is built once and started per test with its own flags, because the
// behaviors under test are configuration-dependent by nature: the same request
// must be refused with one flag set and accepted with another.
package httpe2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"testing"
	"time"
)

// The protocol headers a 2026-07-28 client must send. Requests omitting them
// are answered by the SDK before anything under test runs, which makes for
// confusing failures.
const (
	protocolVersion = "2026-07-28"
	toolsListBody   = `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`
	// legacyToolsListBody is the same call as a pre-2026-07-28 client sends it.
	// SEP-2575's per-request _meta carries its own protocolVersion, and the
	// transport refuses a request whose header disagrees with it (-32020), so a
	// body announcing 2026-07-28 cannot be used to exercise an older revision.
	legacyToolsListBody = `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	acceptHeader        = "application/json, text/event-stream"
)

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
// path. Building rather than importing is deliberate: the handler chain being
// tested is assembled in package main and cannot be imported, and a test that
// reassembled it would be testing its own copy.
func serverBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		// t.TempDir cannot be used here: the binary is built once for the
		// whole package under sync.Once, and the first test to arrive would
		// own a directory removed when that test ends, leaving every later
		// test pointing at a path that no longer exists.
		dir, err := os.MkdirTemp("", "gitlab-mcp-httpe2e") //nolint:usetesting // see above
		if err != nil {
			errBuild = err
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
// another process remains, which is why startServer retries readiness rather
// than assuming the port is ours the instant we ask for it.
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
	// client reaches this server. It is not always http.DefaultClient: a
	// server listening on a unix socket needs a dialer that talks to the
	// path, and one serving TLS needs a root that trusts its certificate.
	// Both are configurations a deployment can choose, so both must be
	// reachable from a test.
	client *http.Client
}

// httpClient returns the client for this server, defaulting to the shared one.
func (s *server) httpClient() *http.Client {
	if s.client != nil {
		return s.client
	}
	return http.DefaultClient
}

// startServer launches the binary with the given extra flags and environment,
// waits for /health, and stops it when the test ends.
//
// GITLAB_URL points at whatever the caller passes; the tests here never reach
// GitLab, because every behavior under test is decided before the request
// would leave the process.
func startServer(t *testing.T, env map[string]string, flags ...string) *server {
	t.Helper()
	return startServerOnPort(t, freePort(t), env, flags...)
}

// startServerOnPort is startServer with the port chosen by the caller, for
// tests that must configure something in front of it before it starts.
func startServerOnPort(t *testing.T, port int, env map[string]string, flags ...string) *server {
	t.Helper()

	bin := serverBinary(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

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

// waitHealthy polls /health until the server answers or the deadline passes.
// A failure dumps the process output, because a server that refuses to start
// has already said why and the test should not make anyone go looking.
func waitHealthy(t *testing.T, s *server) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, s.baseURL+"/health", http.NoBody)
		if err != nil {
			t.Fatalf("building the health request: %v", err)
		}
		resp, err := s.httpClient().Do(req)
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

// request is one HTTP call to the server under test.
type request struct {
	method  string
	path    string
	body    string
	headers map[string]string
}

// response is what a test asserts against.
type response struct {
	status int
	header http.Header
	body   string
}

// do issues the request and returns the response, failing the test only when
// the call could not be made at all — a rejection is data, not an error.
func (s *server) do(t *testing.T, r request) response {
	t.Helper()

	method := r.method
	if method == "" {
		method = http.MethodPost
	}
	path := r.path
	if path == "" {
		path = "/mcp"
	}
	var body io.Reader = http.NoBody
	if r.body != "" {
		body = strings.NewReader(r.body)
	}

	req, err := http.NewRequestWithContext(t.Context(), method, s.baseURL+path, body)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	if r.body != "" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", acceptHeader)
		req.Header.Set("MCP-Protocol-Version", protocolVersion)
		// Protocol 2026-07-28 makes Mcp-Method REQUIRED: the SDK rejects a
		// POST without it before any handler runs. Without this the harness
		// could only ever observe rejections, so no test here reached a
		// successful call — which is how a real 200 path went unexercised.
		if rpcMethod := jsonRPCMethod(r.body); rpcMethod != "" {
			req.Header.Set("Mcp-Method", rpcMethod)
			if name := jsonRPCToolName(r.body); name != "" {
				req.Header.Set("Mcp-Name", name)
			}
		}
	}
	for k, v := range r.headers {
		// An empty value deletes the header rather than sending it empty, so a
		// test can express "this client sends no such header" for one the
		// harness sets by default. A pre-negotiation client sends no
		// MCP-Protocol-Version at all, and that is a case worth covering.
		if v == "" {
			req.Header.Del(k)
			continue
		}
		req.Header.Set(k, v)
	}

	resp, err := s.httpClient().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("reading the response body: %v", err)
	}
	return response{status: resp.StatusCode, header: resp.Header, body: string(raw)}
}

// jsonRPCMethod reads the "method" out of a JSON-RPC request body so the
// harness can send the Mcp-Method header a real client sends. A body that is
// not JSON-RPC yields "", leaving the header unset — which is itself a case
// some tests want.
// jsonRPCPayload returns the JSON-RPC message from a response body, whether it
// arrived as a bare JSON document or inside an SSE frame.
//
// The transport decides which: a refusal is written as JSON, while a successful
// call is streamed as text/event-stream unless --json-response is set. A test
// that unmarshals the body directly therefore works only for the failure cases,
// which is why the successful paths went untested for so long.
func jsonRPCPayload(t *testing.T, body string) string {
	t.Helper()
	for line := range strings.SplitSeq(body, "\n") {
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			return strings.TrimSpace(after)
		}
	}
	return body
}

func jsonRPCMethod(body string) string {
	var msg struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal([]byte(body), &msg); err != nil {
		return ""
	}
	return msg.Method
}

// jsonRPCToolName reads params.name out of a tools/call body, which protocol
// 2026-07-28 also carries in a header.
func jsonRPCToolName(body string) string {
	var msg struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(body), &msg); err != nil || msg.Method != "tools/call" {
		return ""
	}
	return msg.Params.Name
}

// mcpPOST is the common case: a tools/list call with the headers a real client
// sends, plus whatever the test adds.
func mcpPOST(headers map[string]string) request {
	return request{method: http.MethodPost, path: "/mcp", body: toolsListBody, headers: headers}
}

// runServerExpectingExit starts the binary and waits for it to exit, returning
// its combined output. It is for startup-validation tests, where the server is
// supposed to refuse to run.
func runServerExpectingExit(t *testing.T, bin string, args ...string) (string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "LOG_LEVEL=info")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// itoa keeps strconv out of every test file that needs one port number.
func itoa(n int) string { return strconv.Itoa(n) }

// fakeGitLab is a stand-in instance the server can actually reach.
//
// Several behaviors only appear when GitLab answers: the failure budget is
// charged when GitLab *rejects* a credential, and deliberately not when the
// instance is merely unreachable, since counting an outage would lock out
// clients holding valid tokens. Pointing --gitlab-url at a domain that does not
// resolve therefore exercises the wrong path.
type fakeGitLab struct {
	url   string
	calls func() int
}

// startFakeGitLab serves the endpoints the server probes when it builds a pool
// entry. userStatus is what /api/v4/user answers, so a test can choose between
// "GitLab rejects this credential" and "GitLab accepts it".
func startFakeGitLab(t *testing.T, userStatus int, userBody string) *fakeGitLab {
	t.Helper()

	var mu sync.Mutex
	calls := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		if userStatus != http.StatusOK {
			http.Error(w, http.StatusText(userStatus), userStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(userBody))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		// Scope and tier probes hit other paths; answering 404 means
		// "unavailable", which every caller handles.
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &fakeGitLab{
		url: srv.URL,
		calls: func() int {
			mu.Lock()
			defer mu.Unlock()
			return calls
		},
	}
}

// startScopedFakeGitLab serves a GitLab that reports each token's real scopes,
// which is what the server introspects to decide how much authority a
// credential carries.
//
// startFakeGitLab answers 404 for the introspection endpoints, so every token
// there looks equally authoritative — which is exactly why the read_api
// admission could not be tested with it. scopesFor maps a bearer token to the
// scopes GitLab reports for it; a token it does not know gets 404, the
// "instance would not say" case.
func startScopedFakeGitLab(t *testing.T, scopesFor map[string][]string) *fakeGitLab {
	t.Helper()

	var mu sync.Mutex
	calls := 0

	bearer := func(r *http.Request) string {
		value := r.Header.Get("Authorization")
		if token, ok := strings.CutPrefix(value, "Bearer "); ok {
			return token
		}
		return r.Header.Get("PRIVATE-TOKEN")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		if _, known := scopesFor[bearer(r)]; !known {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"username":"scoped"}`))
	})
	// The PAT introspection endpoint. The server tries this one first, then
	// /oauth/token/info; either answering is enough.
	mux.HandleFunc("/api/v4/personal_access_tokens/self", func(w http.ResponseWriter, r *http.Request) {
		scopes, known := scopesFor[bearer(r)]
		if !known {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		body, err := json.Marshal(map[string]any{"id": 1, "scopes": scopes, "active": true})
		if err != nil {
			t.Errorf("marshaling the scope response: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &fakeGitLab{
		url: srv.URL,
		calls: func() int {
			mu.Lock()
			defer mu.Unlock()
			return calls
		},
	}
}

// startHangingGitLab serves an instance that accepts connections and never
// answers, for testing that upstream probes are bounded.
func startHangingGitLab(t *testing.T) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		// Block until the client gives up or the test ends. Returning would
		// defeat the point; panicking would take the test binary with it.
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// rawRequest writes bytes straight onto a TCP connection, bypassing net/http's
// client-side validation.
//
// Go refuses to send a header value containing CR, LF or a NUL, and refuses to
// parse a URL with a control character in it — which means the interesting
// attacks cannot be expressed through http.Client at all. An attacker has no
// such restriction, so the request is spelled out by hand. The reply is read
// with a deadline: a server that answers nothing is as much a finding as one
// that answers wrongly.
func (s *server) raw(t *testing.T, wire string) string {
	t.Helper()

	addr := strings.TrimPrefix(s.baseURL, "http://")
	var d net.Dialer
	conn, err := d.DialContext(t.Context(), "tcp", addr)
	if err != nil {
		t.Fatalf("dialing %s: %v", addr, err)
	}
	defer conn.Close()

	if deadlineErr := conn.SetDeadline(time.Now().Add(10 * time.Second)); deadlineErr != nil {
		t.Fatalf("setting the deadline: %v", deadlineErr)
	}
	if _, writeErr := conn.Write([]byte(wire)); writeErr != nil {
		// A server that closes on a malformed request line is a valid answer.
		return ""
	}

	var reply strings.Builder
	buf := make([]byte, 4096)
	for {
		n, readErr := conn.Read(buf)
		reply.Write(buf[:n])
		if readErr != nil || reply.Len() > 64*1024 {
			break
		}
	}
	return reply.String()
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
