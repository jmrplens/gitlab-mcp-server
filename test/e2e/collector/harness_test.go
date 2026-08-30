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
			errBuild = err
			return
		}
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
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, s.baseURL+"/health", http.NoBody)
		if err != nil {
			t.Fatalf("building the health request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
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
func (s *server) callAction(t *testing.T, id int, action, projectID string) {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) +
		`,"method":"tools/call","params":{` + protocolMeta +
		`,"name":"gitlab_execute_action","arguments":{"action":"` + action +
		`","project_id":"` + projectID + `"}}}`

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
	// answered is not what this module tests; a call that reached the handler
	// produced a span either way, and asserting on the result here would make
	// this a test of the fake instance below.
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		t.Fatalf("the call was refused with %d, so it never reached the instrumentation and no span exists to assert on. Server output:\n%s",
			resp.StatusCode, s.logs())
	}
}

// startFakeGitLab serves the endpoints the server probes when it builds a pool
// entry, so a credential is admitted and the call reaches the middleware.
//
// The instrumentation sits inside the authentication gate. Without an instance
// that accepts the token there is no span to collect, which is a deliberate
// property of the design and an obstacle to testing it.
func startFakeGitLab(t *testing.T) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"username":"collector-e2e"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		// Scope and tier probes hit other paths; 404 means "unavailable",
		// which every caller handles.
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}
