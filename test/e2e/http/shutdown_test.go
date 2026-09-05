//go:build httpe2e

// shutdown_test.go drives the real binary's answer to SIGTERM while clients
// hold subscriptions/listen requests open.
//
// It is here rather than in a unit test because the symptom is a process: how
// long the binary takes to exit after a signal, and what exit status it leaves
// behind. The in-process e2e suite builds a server directly, so it has no
// process, no signal and no HTTP drain; a unit test can prove the stream
// registry cancels what it holds and cannot prove that the registry is wired
// into the configuration a deployment actually runs.
package httpe2e

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// shutdownGrace is how long the process may take to exit after SIGTERM.
//
// It sits below the binary's own httpShutdownTimeout of 15 seconds, because
// waiting out that budget is the failure under test: a server that stops
// promptly does so in milliseconds, and one that leaves a stream open cannot
// finish before the drain deadline expires. The gap is wide enough that a
// loaded machine does not decide the outcome, and the assertions that carry the
// meaning are the ones about the completion results and the exit status.
const shutdownGrace = 10 * time.Second

// shutdownListenBody is a subscriptions/listen carrying only a list-changed
// subscription, which is the shape every capability surface accepts.
//
// It names no resource on purpose. The go-sdk client opens exactly this request
// during its handshake whenever it registers a tools-list-changed handler, so it
// is what an ordinary 2026-07-28 client leaves open against a server that
// advertises no resource subscriptions at all, which is the configuration where
// the stream registry was missing.
func shutdownListenBody(id int) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"subscriptions/listen","params":{`+
		`"notifications":{"toolsListChanged":true},`+
		`"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",`+
		`"io.modelcontextprotocol/clientCapabilities":{},`+
		`"io.modelcontextprotocol/clientInfo":{"name":"probe","version":"1"}}}}`, id)
}

// TestShutdown_OpenListenStreams_ProcessExitsCleanlyOnEverySurface pins that
// SIGTERM stops the server while clients hold listen streams open, whatever the
// capability surface is.
//
// A subscription at protocol 2026-07-28 is a request the client leaves open, and
// the SDK's handler blocks until that request's context ends. Only the stream
// registry ends it, and the registry used to be built beside the resource
// subscription runtime, which does not exist on --capability-surface=minimal.
// There the SDK went on acknowledging and holding list-changed listens with
// nothing able to close them, so SIGTERM was answered by the full 15-second HTTP
// drain and then "http server shutdown: context deadline exceeded" and exit 1.
// A supervisor reads that as a failed stop, and its next step is SIGKILL.
//
// The ceilings on how many listens may be open at once do not help here: they
// bound how many streams can pile up, not whether the ones that exist can be
// ended.
//
// The full surface is the control. It always had the registry, so it passed
// before this change and passing here says the failure on minimal was the
// missing wiring rather than shutdown in general.
//
// What the assertions rest on is the completion result arriving on each stream
// before the process exits: that is the server actively closing them, where a
// process dying at its deadline would drop the connections with no result at
// all. The elapsed time is only a backstop, since a duration alone can pass on
// a fast machine for the wrong reason.
func TestShutdown_OpenListenStreams_ProcessExitsCleanlyOnEverySurface(t *testing.T) {
	tests := []struct {
		name string
		// surface is the --capability-surface the server runs with.
		surface string
	}{
		{
			name:    "the minimal surface, which builds no subscription runtime",
			surface: "minimal",
		},
		{
			// The control: this configuration always closed its streams.
			name:    "the full surface, which builds one",
			surface: "full",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
			srv := shutdownStartServer(t,
				"--gitlab-url="+gitlab.url,
				"--capability-surface="+tt.surface,
			)

			streams := srv.openListenStreams(t, 3)

			elapsed, code := srv.terminateAndWait(t, shutdownGrace)

			for _, stream := range streams {
				if !stream.completed() {
					t.Errorf("stream %d never received its completion result, so the process ended without closing it:\n%s",
						stream.id, stream.text())
				}
			}
			if code != 0 {
				t.Errorf("exit status %d after %s, want 0:\n%s", code, terminationSignalName, srv.logs())
			}
			if strings.Contains(srv.logs(), "context deadline exceeded") {
				t.Errorf("shutdown ran out of its drain budget:\n%s", srv.logs())
			}
			t.Logf("exited %s after %s", elapsed, terminationSignalName)
		})
	}
}

// shutdownServer is a running binary this file can signal and then wait on.
//
// The shared harness deliberately does not offer that: it ends its servers by
// canceling an exec context, which is a SIGKILL, and how the process answers a
// SIGTERM is the entire subject here.
type shutdownServer struct {
	// probe carries the harness's own server shape, so waitHealthy and the
	// shared HTTP client can be reused unchanged.
	probe *server
	cmd   *exec.Cmd
	logs  func() string

	waitOnce sync.Once
	waitErr  error
}

// shutdownStartServer launches the binary with the given flags and waits for
// /health, leaving the process reachable by signal.
func shutdownStartServer(t *testing.T, flags ...string) *shutdownServer {
	t.Helper()

	bin := serverBinary(t)
	addr := "127.0.0.1:" + itoa(freePort(t))

	args := append([]string{"--http", "--http-addr=" + addr}, flags...)
	// Not exec.CommandContext: its cancellation is a kill, and this file needs
	// the process to survive until it is signaled deliberately.
	cmd := exec.Command(bin, args...) //nolint:noctx // see above
	prepareForTermination(cmd)
	cmd.Env = append(os.Environ(),
		"LOG_LEVEL=info",
		"TOOL_SURFACE=dynamic",
	)

	var mu sync.Mutex
	var out bytes.Buffer
	cmd.Stdout = &lockedWriter{mu: &mu, buf: &out}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the server: %v", err)
	}

	logs := func() string {
		mu.Lock()
		defer mu.Unlock()
		return out.String()
	}
	srv := &shutdownServer{
		probe: &server{baseURL: "http://" + addr, logs: logs},
		cmd:   cmd,
		logs:  logs,
	}

	t.Cleanup(func() {
		// Whatever the test concluded, nothing is left running: a server that
		// ignored its signal is exactly the failure this file looks for, and
		// leaking it would take the next test's port with it.
		_ = cmd.Process.Kill()
		_ = srv.wait()
	})

	waitHealthy(t, srv.probe)
	return srv
}

// wait reaps the process once, so the cleanup and the test can both call it.
func (s *shutdownServer) wait() error {
	s.waitOnce.Do(func() { s.waitErr = s.cmd.Wait() })
	return s.waitErr
}

// terminateAndWait sends SIGTERM and reports how long the process took to exit
// and what status it left. A process still running at the deadline fails the
// test, since that is the defect this file exists for.
func (s *shutdownServer) terminateAndWait(t *testing.T, grace time.Duration) (time.Duration, int) {
	t.Helper()

	started := time.Now()
	if err := signalTermination(s.cmd.Process); err != nil {
		t.Fatalf("sending %s: %v", terminationSignalName, err)
	}

	// The wait runs on its own goroutine so the deadline can be enforced here,
	// on the test's goroutine, where failing is allowed.
	reaped := make(chan struct{})
	go func() {
		_ = s.wait()
		close(reaped)
	}()

	select {
	case <-reaped:
	case <-time.After(grace):
		t.Fatalf("the server was still running %s after %s; it had to be killed:\n%s", grace, terminationSignalName, s.logs())
	}
	return time.Since(started), s.cmd.ProcessState.ExitCode()
}

// openListenStreams opens n subscriptions/listen requests and returns once each
// one has been acknowledged.
//
// Waiting for the acknowledgment is what makes the shutdown that follows land on
// established streams. Signaling earlier would exercise a listen that arrives
// during the drain instead, which is a different case with a different answer.
func (s *shutdownServer) openListenStreams(t *testing.T, n int) []*shutdownStream {
	t.Helper()

	streams := make([]*shutdownStream, 0, n)
	for id := 1; id <= n; id++ {
		streams = append(streams, s.openListenStream(t, id))
	}
	for _, stream := range streams {
		select {
		case <-stream.acknowledged:
		case <-time.After(60 * time.Second):
			t.Fatalf("stream %d was never acknowledged, so there would be nothing for shutdown to close:\n%s\n%s",
				stream.id, stream.text(), s.logs())
		}
	}
	return streams
}

// openListenStream sends one listen request and reads its stream in the
// background until the server ends it.
func (s *shutdownServer) openListenStream(t *testing.T, id int) *shutdownStream {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		s.probe.baseURL+"/mcp", strings.NewReader(shutdownListenBody(id)))
	if err != nil {
		t.Fatalf("building the listen request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	req.Header.Set("Mcp-Method", "subscriptions/listen")
	req.Header.Set("PRIVATE-TOKEN", "glpat-whatever")

	resp, err := s.probe.httpClient().Do(req)
	if err != nil {
		t.Fatalf("opening listen stream %d: %v", id, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("listen stream %d answered %d, want 200", id, resp.StatusCode)
	}

	stream := &shutdownStream{id: id, acknowledged: make(chan struct{})}
	t.Cleanup(func() { _ = resp.Body.Close() })

	// The reader records and never asserts: it runs off the test's goroutine,
	// where a failure could not be reported, and the recorded frames are read
	// back on the test's goroutine once the process has exited.
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			stream.record(scanner.Text())
		}
	}()

	return stream
}

// shutdownStream is one open subscriptions/listen request and what the server
// has written on it.
type shutdownStream struct {
	id int
	// acknowledged closes when the server confirms the subscription, which is
	// its first message on the stream.
	acknowledged chan struct{}

	mu     sync.Mutex
	frames []string
	acked  bool
}

func (s *shutdownStream) record(line string) {
	s.mu.Lock()
	s.frames = append(s.frames, line)
	ack := !s.acked && strings.Contains(line, "notifications/subscriptions/acknowledged")
	if ack {
		s.acked = true
	}
	s.mu.Unlock()

	if ack {
		close(s.acknowledged)
	}
}

// completed reports whether the server answered the open request rather than
// dropping the connection under it.
//
// The result carries the request's own id, which distinguishes it from the
// acknowledgment: that arrives as a notification and carries no id at all.
func (s *shutdownStream) completed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := fmt.Sprintf(`"id":%d`, s.id)
	for _, frame := range s.frames {
		if strings.Contains(frame, want) && strings.Contains(frame, `"result"`) {
			return true
		}
	}
	return false
}

// text renders what arrived on the stream, for a failure message.
func (s *shutdownStream) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.frames, "\n")
}
