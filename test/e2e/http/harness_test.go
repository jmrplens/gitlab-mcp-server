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
	"runtime"
	"slices"
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
		if runtime.GOOS == "windows" {
			// exec refuses a file with no executable extension there, and
			// go build -o writes exactly the name it is given.
			out += ".exe"
		}
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

// withInstancePolicy prepends the escape hatch that lets a server start
// without publishing a GitLab instance, unless the caller has published one
// itself.
//
// HTTP mode refuses to start when no --gitlab-url is given, because a
// deployment that has not said which GitLab it serves will make requests to
// whatever host a caller names in the GITLAB-URL header, carrying whatever
// token that caller supplied. Almost nothing in this module reaches GitLab at
// all: the behavior under test is decided before a request would leave the
// process. Those tests take the hatch so they exercise the transport rather
// than the instance policy.
//
// A test that publishes its own instance, and so is testing that policy, is
// left exactly as it was written.
func withInstancePolicy(flags []string) []string {
	for _, f := range flags {
		name, _, _ := strings.Cut(strings.TrimLeft(f, "-"), "=")
		if name == "gitlab-url" || name == "allow-any-gitlab-url" {
			return flags
		}
	}
	return append([]string{"--allow-any-gitlab-url"}, flags...)
}

// startServer launches the binary with the given extra flags and environment,
// waits for /health, and stops it when the test ends.
//
// GITLAB_URL points at whatever the caller passes; the tests here never reach
// GitLab, because every behavior under test is decided before the request
// would leave the process.
// The port is not chosen here. freePort hands back a number rather than a
// listener, so between the moment it releases the port and the moment the
// server binds it the port belongs to nobody, and every test in this module
// runs in parallel: two tests could be handed the same number, and the one
// that lost would either fail to bind or, worse, find the port answering and
// drive somebody else's server. Asking for port 0 hands the choice to the
// kernel, which cannot hand the same port to two live listeners, and the
// server says which one it got.
func startServer(t *testing.T, env map[string]string, flags ...string) *server {
	t.Helper()

	srv, err := tryStartServerOnPort(t, ephemeralPort, env, flags...)
	if err != nil {
		t.Fatalf("starting the server: %v", err)
	}
	return srv
}

// ephemeralPort asks the kernel for a port instead of naming one.
const ephemeralPort = 0

// errPortTaken says the server exited because something else holds its port.
// Only a caller that named the port can meet it.
var errPortTaken = errors.New("the port was taken")

// startServerOnPort is startServer with the port chosen by the caller, for
// tests that must configure something in front of it before it starts. The
// port is the caller's decision, so a collision fails rather than moving.
func startServerOnPort(t *testing.T, port int, env map[string]string, flags ...string) *server {
	t.Helper()

	srv, err := tryStartServerOnPort(t, port, env, flags...)
	if err != nil {
		t.Fatalf("starting the server on port %d: %v", port, err)
	}
	return srv
}

// tryStartServerOnPort starts the binary and reports why it did not come up,
// rather than ending the test, so a caller that chose the port at random can
// choose another.
func tryStartServerOnPort(t *testing.T, port int, env map[string]string, flags ...string) (*server, error) {
	t.Helper()

	bin := serverBinary(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	args := append([]string{"--http", "--http-addr=" + addr}, withInstancePolicy(flags)...)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, args...)
	prepareForTermination(cmd)
	cmd.Env = append(configFreeEnviron(),
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
		return nil, fmt.Errorf("launching the binary: %w", err)
	}

	logs := func() string {
		mu.Lock()
		defer mu.Unlock()
		return out.String()
	}

	// The process is waited on once, here, and the result published by closing
	// rather than by sending: everything that asks whether the server is still
	// there has to be able to ask, and a value can only be taken once. It was
	// sent once, and the cleanup then blocked forever on a channel the health
	// wait had already drained.
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		cancel()
		<-exited
	})

	// The address is read back from the server rather than assumed, because
	// with port 0 only the server knows it. This is also what ties the health
	// check to this process: the address came out of its own log, so nothing
	// else can be the thing that answers.
	bound, err := awaitListenAddr(logs, exited)
	if err != nil {
		return nil, err
	}

	srv := &server{baseURL: "http://" + bound, logs: logs}
	if err = awaitHealthy(t, srv, exited); err != nil {
		return nil, err
	}
	return srv, nil
}

// awaitListenAddr reads the address the server bound out of its own log.
//
// The server logs the listener's address, not the one it was asked for, so
// this is the only way to learn the port when the harness asked for 0, and the
// most direct way to know the process under test is the one on that port.
func awaitListenAddr(logs func() string, exited <-chan struct{}) (string, error) {
	deadline := time.Now().Add(45 * time.Second)
	for {
		if addr := listenAddrIn(logs()); addr != "" {
			return addr, nil
		}
		select {
		case <-exited:
			// One last read: the line and the exit can land together, and a
			// server that bound and then died has still said where.
			if addr := listenAddrIn(logs()); addr != "" {
				return addr, nil
			}
			return "", startupFailure(logs())
		default:
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("the server never said what it bound. Output:\n%s", logs())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// listenAddrIn finds the bound address in the server's JSON log output.
func listenAddrIn(logs string) string {
	for line := range strings.SplitSeq(logs, "\n") {
		var record struct {
			Msg  string `json:"msg"`
			Addr string `json:"addr"`
		}
		if json.Unmarshal([]byte(line), &record) != nil {
			continue
		}
		if record.Msg == "listening on tcp" && record.Addr != "" {
			return record.Addr
		}
	}
	return ""
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

// waitHealthy polls /health until the server answers, for a caller holding a
// server this harness did not start and so has no process to watch.
func waitHealthy(t *testing.T, s *server) {
	t.Helper()
	if err := awaitHealthy(t, s, nil); err != nil {
		t.Fatal(err)
	}
}

// awaitHealthy polls /health until the server answers or the deadline passes.
// A failure dumps the process output, because a server that refuses to start
// has already said why and the test should not make anyone go looking.
func awaitHealthy(t *testing.T, s *server, exited <-chan struct{}) error {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		// Checked before the probe, because a server that has already given up
		// leaves its port free for anything else to answer on, and a health
		// check answered by somebody else is worse than one that fails: the
		// test would go on to drive a server configured for another test.
		select {
		case <-exited:
			return startupFailure(s.logs())
		default:
		}

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, s.baseURL+"/health", http.NoBody)
		if err != nil {
			return fmt.Errorf("building the health request: %w", err)
		}
		resp, err := s.httpClient().Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("the server never became healthy. Output:\n%s", s.logs())
}

// startupFailure names why a server exited before it served, so that a port
// lost to another test is retried and anything else is reported.
func startupFailure(logs string) error {
	if strings.Contains(logs, "address already in use") ||
		strings.Contains(logs, "Only one usage of each socket address") {
		return fmt.Errorf("%w. Output:\n%s", errPortTaken, logs)
	}
	return fmt.Errorf("the server exited before it served. Output:\n%s", logs)
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
		// Host is not a header on an outgoing request in Go: the client reads
		// Request.Host and ignores a map entry of that name entirely. A test
		// that sets it through the map believes it spoofed the Host and sent
		// nothing, which made one privacy assertion vacuous.
		if strings.EqualFold(k, "Host") {
			req.Host = v
			continue
		}
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
	cmd.Env = append(configFreeEnviron(), "LOG_LEVEL=info")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// genericServerSettings are the settings this server reads under names generic
// enough to belong to something else in the same shell. They are listed here
// rather than imported because this module builds the binary rather than
// linking it, so the list is duplicated on purpose and a name added to the
// server without being added here only weakens the isolation, never the build.
var genericServerSettings = []string{
	"AUTH_MODE", "CAPABILITY_SURFACE", "CLIENT_COMPAT", "EXCLUDE_TOOLS",
	"LOG_LEVEL", "MAX_HTTP_CLIENTS", "META_PARAM_SCHEMA", "META_TOOLS",
	"OAUTH_CACHE_TTL", "OAUTH_CLIENT_UID", "POOL_IDLE_TIMEOUT", "PUBLIC_URL",
	"RATE_LIMIT_BURST", "RATE_LIMIT_RPS", "TOOL_SURFACE", "TRUSTED_ORIGINS",
	"UPLOAD_MAX_FILE_SIZE",
}

// configFreeEnviron is the process environment with every variable that
// configures this server removed, so a test's flags decide its behavior and
// nothing else does.
//
// The server reads its settings from the environment when a flag is absent, and
// these tests start it as a child of whatever shell the developer or the runner
// is using. A GITLAB_URL exported there therefore reaches the server as a
// published instance, which silently defeats every test that pins what a
// deployment publishing none does: they pass on a clean machine and fail on a
// configured one, which is the worst way for a test to be wrong.
//
// Filtered rather than replaced, because the child still needs PATH, HOME and
// the rest of the machine to run at all.
func configFreeEnviron() []string {
	kept := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(name, "GITLAB_") || strings.HasPrefix(name, "OTEL_") ||
			slices.Contains(genericServerSettings, name) {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
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
