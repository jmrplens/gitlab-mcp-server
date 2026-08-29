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
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// protocolVersion is the revision these tests speak unless a case says
// otherwise.
const protocolVersion = "2026-07-28"

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

	mu     sync.Mutex
	stderr strings.Builder
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

	bin := serverBinary(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin)

	environ := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	for k, v := range env {
		environ = append(environ, k+"="+v)
	}
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

	s := &session{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout)}

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
		_ = cmd.Wait()
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

// call sends a request and returns its response.
func (s *session) call(t *testing.T, msg string) map[string]any {
	t.Helper()
	s.send(t, msg)
	return s.readMessage(t, 30*time.Second)
}

// alive reports whether the server process is still running.
func (s *session) alive() bool {
	return s.cmd.ProcessState == nil || !s.cmd.ProcessState.Exited()
}

// stderrText returns everything the server has logged so far.
func (s *session) stderrText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stderr.String()
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

// startFakeGitLab serves the handful of endpoints the server probes at startup
// plus one project, so a tool call has something to answer with.
func startFakeGitLab(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"version":"17.0.0","revision":"abcdef"}`)
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"id":7,"username":"someone","name":"Some One"}`)
	})
	mux.HandleFunc("/api/v4/projects/42", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"id":42,"name":"proj","path_with_namespace":"g/proj","web_url":"http://example.invalid/g/proj"}`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

// baseEnv is the environment every case starts from: a fake GitLab, a token
// that looks real, and no auto-update, which would otherwise reach the network
// from a test.
func baseEnv(gitlabURL string) map[string]string {
	return map[string]string{
		"GITLAB_URL":   gitlabURL,
		"GITLAB_TOKEN": "glpat-stdio-e2e-token",
		"AUTO_UPDATE":  "false",
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

	if err := s.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("sending SIGTERM: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_, _ = s.cmd.Process.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(within):
		return false
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
