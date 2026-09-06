// target.go starts the server the way a client starts it, on each transport.
//
// The measured process is the real binary, built from cmd/server, and not a
// server assembled inside this command out of the same packages. A harness
// that reassembles the thing it measures reports on its own copy: it would
// miss the readiness gate, the authentication chain, the pool and the flag
// parsing, all of which are exactly what an operator is sizing for.
//
// What a "client" means differs by transport, and the difference is the point.
// On stdio every client is its own process with its own catalog, so N clients
// are N processes. On HTTP one process serves everyone and the pool holds one
// entry per credential, so N clients are N tokens against one process.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// benchToken is the credential prefix each client authenticates with. The
// suffix is the client index, which is what makes one pool entry per client:
// the pool keys on token and instance URL.
const benchToken = "bench-token-" //#nosec G101 -- not a credential, a stand-in the stub instance accepts

// Seams a test drives the slow failure paths through: the wait for /health,
// which is a minute against a binary that never serves it, and the port
// reservation, whose failures the kernel does not produce on demand.
var (
	healthWait  = 60 * time.Second
	reservePort = func(ctx context.Context) (net.Listener, error) {
		return (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	}
	// wirePipes attaches a fresh command's pipes, which cannot fail on a
	// command nothing else has touched; the seam lets a test see the client
	// that failure would have left behind.
	wirePipes = commandProcess
)

// clientConn is one connected MCP client.
type clientConn struct {
	rpc rpcClient
	// label distinguishes clients in error messages; nothing else reads it.
	label string
}

// target is a running server under measurement, on one transport.
type target interface {
	// start brings the server up to the point a client could connect, and
	// returns how long that took.
	start(ctx context.Context) (time.Duration, error)
	// addClient admits one more client and returns it with the time it took
	// to have something to talk to: nothing on HTTP, where the process is
	// already running, and the exec on stdio, where the client is what starts
	// the server. Protocol 2026-07-28 has no handshake, so there is no
	// ceremony between this and the client's first real request.
	addClient(ctx context.Context, index int) (*clientConn, time.Duration, error)
	// processes lists every process this target runs, for the sampler.
	processes() []*os.Process
	// goroutines kills the target with a traceback signal and counts what it
	// printed. It is the end of the target's life.
	goroutines() (int, error)
	// serverInfo is what the build says about itself, empty when the
	// transport offers no way to ask.
	serverInfo() ServerInfo
	// close stops everything, whether or not goroutines was called.
	close()
}

// procWait reaps one process exactly once, however many callers ask, and
// lets every caller wait for the same outcome.
//
// exec.Cmd.Wait is not safe to call twice while the first call is still in
// flight: the goroutines copying the child's output report to the first
// caller over a channel, and a second caller waits on that channel forever.
// Two callers is exactly what a target has, the traceback dump and close,
// and the second used to hang whenever the first was still waiting on a
// process that had not exited within the dump's own bound.
type procWait struct {
	once sync.Once
	done chan struct{}
}

// wait blocks until the process has been reaped, by this call or an earlier
// one.
func (w *procWait) wait(cmd *exec.Cmd) {
	w.once.Do(func() {
		w.done = make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(w.done)
		}()
	})
	<-w.done
}

// lockedBuffer collects a child's two output streams while it still runs.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *lockedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

// String returns everything written so far.
func (w *lockedBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// childEnv builds the environment for a measured process.
//
// It starts from the ambient environment with every setting this server reads
// removed. A developer machine exporting GITLAB_URL or TOOL_SURFACE would
// otherwise publish numbers for a configuration nobody chose, which is the
// same failure the generators avoid by pinning the surface rather than reading
// it.
func childEnv(plan scenarioPlan, stubURL, otlpURL string, stdio bool) []string {
	env := configFreeEnviron()
	// GOTRACEBACK=all is what makes the goroutine count readable at all: the
	// default traceback prints one goroutine, so every scenario would report
	// a count of one.
	env = append(env, "GOTRACEBACK=all", "GITLAB_MCP_LOG_LEVEL=error")
	if stdio {
		env = append(env,
			"GITLAB_URL="+stubURL,
			"GITLAB_TOKEN="+benchToken+"0",
			"GITLAB_MCP_TOOL_SURFACE="+plan.Surface,
		)
	}
	if plan.Telemetry {
		env = append(env,
			"GITLAB_MCP_TELEMETRY=true",
			"OTEL_EXPORTER_OTLP_ENDPOINT="+otlpURL,
			// Every OTEL_ duration is an integer number of milliseconds by
			// specification; a Go duration string parses as nothing and
			// silently keeps the ten-second default.
			"OTEL_EXPORTER_OTLP_TIMEOUT=2000",
			"OTEL_BSP_SCHEDULE_DELAY=200",
			"OTEL_METRIC_EXPORT_INTERVAL=200",
			"OTEL_BLRP_SCHEDULE_DELAY=200",
		)
	}
	return env
}

// configFreeEnviron is the process environment with every variable that
// configures this server removed.
//
// The old spellings come from the config package rather than being restated,
// so a setting added there is stripped here without anybody remembering to.
// The prefixed spellings fall under the GITLAB_ rule; AUTOPILOT is the one
// name read that carries neither prefix, as the alias other agent tooling
// sets for the yolo mode.
func configFreeEnviron() []string {
	return withoutConfig(os.Environ())
}

// withoutConfig filters one environment list, so the rule can be tested on a
// list of the test's own, including the entry without an "=" that execve
// permits and os.Environ never hands a Go program in practice.
func withoutConfig(environ []string) []string {
	legacy := make([]string, 0, len(config.PrefixedEnvNames())+1)
	for _, name := range config.PrefixedEnvNames() {
		legacy = append(legacy, config.LegacyEnvName(name))
	}
	legacy = append(legacy, "AUTOPILOT")
	kept := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(name, "GITLAB_") || strings.HasPrefix(name, "OTEL_") ||
			slices.Contains(legacy, name) {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

// limiterOffArgs turn the rate limiter off, which is what every scenario but
// the fairness one measures with.
//
// HTTP mode defaults to ten requests per second, which a benchmark firing
// parallel requests would spend its time being refused by, so a capacity
// figure measured with the limiter on would be the cost of saying no. The
// fairness scenario measures exactly that saying-no, and it substitutes this
// list rather than adding to it, so a reader of ps sees one setting for the
// bound and not two.
var limiterOffArgs = []string{"--rate-limit-rps=0"}

// httpTarget is one server process serving many credentials.
type httpTarget struct {
	binary  string
	plan    scenarioPlan
	stubURL string
	otlpURL string
	// pprofAddr, when set, is the loopback address the server is told to
	// serve its profiling handlers on; the series reads its profiles and
	// goroutine counts there. maxClients, when positive, sizes the pool so
	// that no credential of a series is evicted between its steps.
	pprofAddr  string
	maxClients int
	// boundArgs and boundEnv are how a bound is put in force, or taken out of
	// it, for one arm of the fairness scenario. A nil boundArgs is the
	// limiter-off list above, so every other scenario passes the arguments it
	// always passed; boundEnv is appended to the child environment, for a bound
	// whose switch is a variable rather than a flag.
	boundArgs []string
	boundEnv  []string

	addr string
	// mu guards cmd alone, which the sampler reads from a goroutine of its own
	// while start is still assigning it.
	mu     sync.Mutex
	cmd    *exec.Cmd
	output *lockedBuffer
	cancel context.CancelFunc
	info   ServerInfo
	reap   procWait
}

// start launches the process and waits for /health.
//
// The rate limiter is disabled unless the caller named a bound: refusals are a
// different measurement, made by the fairness scenario and by the httpe2e
// module, and one that would otherwise contaminate every capacity figure here.
func (t *httpTarget) start(ctx context.Context) (time.Duration, error) {
	port, err := freePort(ctx)
	if err != nil {
		return 0, err
	}
	t.addr = "127.0.0.1:" + strconv.Itoa(port)

	args := []string{
		"--http",
		"--http-addr=" + t.addr,
		"--gitlab-url=" + t.stubURL,
		"--tool-surface=" + t.plan.Surface,
	}
	if len(t.boundArgs) > 0 {
		args = append(args, t.boundArgs...)
	} else {
		args = append(args, limiterOffArgs...)
	}
	if t.plan.Telemetry {
		args = append(args, "--telemetry")
	}
	if t.pprofAddr != "" {
		args = append(args, "--pprof-addr="+t.pprofAddr)
	}
	if t.maxClients > 0 {
		args = append(args, "--max-http-clients="+strconv.Itoa(t.maxClients))
	}

	runCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	t.output = &lockedBuffer{}
	cmd := exec.CommandContext(runCtx, t.binary, args...) // #nosec G204 -- the binary is this command's own build of cmd/server
	cmd.Env = append(childEnv(t.plan, t.stubURL, t.otlpURL, false), t.boundEnv...)
	cmd.Stdout = t.output
	cmd.Stderr = t.output

	started := time.Now()
	if startErr := cmd.Start(); startErr != nil {
		cancel()
		return 0, fmt.Errorf("start server: %w", startErr)
	}
	// Published only once the process exists, and behind the mutex, because the
	// sampler is already polling processes() on a goroutine of its own: every
	// caller starts it before the target so that the idle reading is taken from
	// the first moment there is a process to read. Assigning the command before
	// Start filled in its Process was a read of two fields being written.
	t.setCommand(cmd)
	info, waitErr := t.waitHealthy(runCtx)
	if waitErr != nil {
		t.close()
		return 0, waitErr
	}
	t.info = info
	return time.Since(started), nil
}

// waitHealthy polls /health until the process answers, and reads the build it
// reports while it is there.
func (t *httpTarget) waitHealthy(ctx context.Context) (ServerInfo, error) {
	deadline := time.Now().Add(healthWait)
	client := &http.Client{Timeout: 5 * time.Second}
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+t.addr+"/health", http.NoBody)
		if err != nil {
			return ServerInfo{}, fmt.Errorf("build health request: %w", err)
		}
		resp, doErr := client.Do(req)
		if doErr == nil {
			info, decodeErr := decodeHealth(resp)
			_ = resp.Body.Close()
			if decodeErr == nil {
				return info, nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return ServerInfo{}, fmt.Errorf("server never became healthy:\n%s", t.output.String())
}

// addClient connects a client carrying its own credential, which is what makes
// the pool build one entry per client.
func (t *httpTarget) addClient(_ context.Context, index int) (*clientConn, time.Duration, error) {
	client := newHTTPRPC("http://"+t.addr+"/mcp", benchToken+strconv.Itoa(index))
	return &clientConn{rpc: client, label: "client " + strconv.Itoa(index)}, 0, nil
}

// setCommand publishes the started process to whoever is watching it.
func (t *httpTarget) setCommand(cmd *exec.Cmd) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cmd = cmd
}

// command is the started process, or nil before there is one. The lock is
// released before the caller does anything with it, since two of the three
// callers then block on the process for as long as it takes to die.
func (t *httpTarget) command() *exec.Cmd {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cmd
}

// processes reports the single server process.
func (t *httpTarget) processes() []*os.Process {
	cmd := t.command()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return []*os.Process{cmd.Process}
}

// goroutines dumps and counts the server's goroutines, ending its life.
func (t *httpTarget) goroutines() (int, error) {
	cmd := t.command()
	if cmd == nil || cmd.Process == nil {
		return 0, errors.New("no server process")
	}
	return dumpGoroutines(cmd.Process, func() { t.reap.wait(cmd) }, t.output.String)
}

// serverInfo returns the build /health reported.
func (t *httpTarget) serverInfo() ServerInfo { return t.info }

// close stops the process.
func (t *httpTarget) close() {
	if t.cancel != nil {
		t.cancel()
	}
	if cmd := t.command(); cmd != nil {
		t.reap.wait(cmd)
	}
}

// stdioTarget is N processes, one per client, which is what stdio is.
type stdioTarget struct {
	binary  string
	plan    scenarioPlan
	stubURL string
	otlpURL string

	mu      sync.Mutex
	cmds    []*exec.Cmd
	outputs []*lockedBuffer
	cancels []context.CancelFunc
	reaps   []*procWait
}

// start is a no-op: on stdio nothing exists until a client spawns it, and that
// is the honest zero for this transport.
func (t *stdioTarget) start(context.Context) (time.Duration, error) { return 0, nil }

// addClient spawns one server process and wires a client to its pipes.
//
// The reported duration is the exec: what it costs to have a process at all,
// before it has been asked for anything. The rest of the wait a stdio client
// experiences is its first request, which the ramp times separately.
func (t *stdioTarget) addClient(ctx context.Context, index int) (*clientConn, time.Duration, error) {
	runCtx, cancel := context.WithCancel(ctx)
	output := &lockedBuffer{}
	cmd := exec.CommandContext(runCtx, t.binary) // #nosec G204 -- the binary is this command's own build of cmd/server
	cmd.Env = childEnv(t.plan, t.stubURL, t.otlpURL, true)
	cmd.Stderr = output

	stdin, stdout, pipeErr := wirePipes(cmd)
	if pipeErr != nil {
		cancel()
		return nil, 0, pipeErr
	}

	started := time.Now()
	if startErr := cmd.Start(); startErr != nil {
		cancel()
		return nil, 0, fmt.Errorf("start stdio server %d: %w", index, startErr)
	}
	elapsed := time.Since(started)
	client := newStdioRPC(stdin, stdout)

	t.mu.Lock()
	t.cmds = append(t.cmds, cmd)
	t.outputs = append(t.outputs, output)
	t.cancels = append(t.cancels, cancel)
	t.reaps = append(t.reaps, &procWait{})
	t.mu.Unlock()

	return &clientConn{rpc: client, label: "process " + strconv.Itoa(index)}, elapsed, nil
}

// processes lists every spawned server.
func (t *stdioTarget) processes() []*os.Process {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]*os.Process, 0, len(t.cmds))
	for _, cmd := range t.cmds {
		if cmd.Process != nil {
			out = append(out, cmd.Process)
		}
	}
	return out
}

// goroutines counts the first process's goroutines and stops the rest.
//
// One process is enough because on stdio every process is a full server
// serving exactly one client: the count is per client by construction, and
// summing N identical processes would publish a number that says more about
// the benchmark's N than about the server.
func (t *stdioTarget) goroutines() (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.cmds) == 0 {
		return 0, errors.New("no server processes")
	}
	cmd, output, reap := t.cmds[0], t.outputs[0], t.reaps[0]
	if cmd.Process == nil {
		return 0, errors.New("first server process never started")
	}
	return dumpGoroutines(cmd.Process, func() { reap.wait(cmd) }, output.String)
}

// serverInfo is empty: stdio publishes no health document, and the version is
// taken from the HTTP scenarios instead of guessed at here.
func (t *stdioTarget) serverInfo() ServerInfo { return ServerInfo{} }

// close stops every process.
func (t *stdioTarget) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, cancel := range t.cancels {
		cancel()
	}
	for i, cmd := range t.cmds {
		t.reaps[i].wait(cmd)
	}
}

// freePort reserves a port by binding and releasing it. A race with another
// process remains, which is why start waits for health rather than assuming
// the port is ours the moment we ask.
func freePort(ctx context.Context) (int, error) {
	listener, err := reservePort(ctx)
	if err != nil {
		return 0, fmt.Errorf("reserve a port: %w", err)
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return 0, errors.New("the reserved listener is not a TCP address")
	}
	if closeErr := listener.Close(); closeErr != nil {
		return 0, fmt.Errorf("release the reserved port: %w", closeErr)
	}
	return addr.Port, nil
}
