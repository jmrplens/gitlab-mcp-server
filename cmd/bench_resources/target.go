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

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// benchToken is the credential prefix each client authenticates with. The
// suffix is the client index, which is what makes one pool entry per client:
// the pool keys on token and instance URL.
const benchToken = "bench-token-" //#nosec G101 -- not a credential, a stand-in the stub instance accepts

// startAttempts is how many times an HTTP target will reserve an address and
// hand it to a process that has to bind it.
//
// One collision is ordinary on a machine doing anything else: the reservation
// is released before the child runs, so the port is the kernel's to give away
// again for as long as the child takes to start. Three collisions in a row on
// three ports the kernel picked independently is not a benchmark's problem to
// solve, and a retry that never gave up would turn a server that cannot start
// for its own reasons into a loop.
const startAttempts = 3

// errServerGone reports a measured process that ended before it answered
// /health, whatever it said on the way out.
//
// It is the one failure start retries, and it is deliberately not narrowed to
// "the address was taken": that reason arrives as the child's own text, which
// is the operating system's wording rather than ours and differs by platform,
// so matching it would make the retry a Unix-only behavior and the test for
// it unportable. Retrying any early exit costs a few milliseconds of restart
// where the cause was something else, and what start reports then is the last
// attempt's own output, so a server that is simply broken still says why.
var errServerGone = errors.New("the server exited before it answered /health")

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

// watch starts reaping the process, if nobody has started yet, and hands back
// the channel that closes once it is gone.
//
// The reaping starts as soon as there is a process rather than at close,
// because a child that dies on its own is how the port handover fails: the
// start below watches this so that it answers the moment the process is gone,
// instead of polling an address nothing is listening on until the health
// deadline. Sixty seconds of that is what one lost port cost the run this was
// written for.
func (w *procWait) watch(cmd *exec.Cmd) <-chan struct{} {
	w.once.Do(func() {
		w.done = make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(w.done)
		}()
	})
	return w.done
}

// wait blocks until the process has been reaped, by this call or an earlier
// one.
func (w *procWait) wait(cmd *exec.Cmd) { <-w.watch(cmd) }

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
var limiterOffArgs = []string{limiterOffArg}

// limiterOffArg turns the per-credential limiter off. It is a constant rather
// than a shared slice because each fairness plan owns its own argument list,
// and a plan that appended to a slice it shared with another would change that
// other plan's arm.
const limiterOffArg = "--rate-limit-rps=0"

// httpTarget is one server process serving many credentials.
type httpTarget struct {
	binary  string
	plan    scenarioPlan
	stubURL string
	otlpURL string
	// pprof asks for the profiling handlers, which the series reads its
	// profiles and goroutine counts from; start reserves the port and
	// publishes it as pprofAddr, so an attempt that lost one address does not
	// keep the other. maxClients, when positive, sizes the pool so that no
	// credential of a series is evicted between its steps.
	pprof      bool
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
	// mu guards cmd and the reaper watching it, which the sampler reads from a
	// goroutine of its own while start is still assigning them.
	mu     sync.Mutex
	cmd    *exec.Cmd
	reap   *procWait
	output *lockedBuffer
	cancel context.CancelFunc
	info   ServerInfo
}

// start launches the process and waits for /health, on a fresh address each
// time the one before it was lost.
//
// A port is reserved by binding and releasing it, and the child binds it a
// moment later, so anything else on the machine may take it in between. That
// window cannot be closed from here: the address has to be on the command line
// before the process exists, and the one way to have the child choose it
// instead would be to read the address back out of its log, which the server
// writes at info level while every measured process here runs at error, so
// every scenario would pay for a log line per tool call inside the figures it
// publishes. What can be done is to notice the collision and try again, which
// is what this does. The failure it answers is real and was hidden: a lost
// port failed a benchmark test after sixty seconds of polling, that failure
// failed the unit suite, and the suite failing left the request inventory
// unwritten and git status clean.
func (t *httpTarget) start(ctx context.Context) (time.Duration, error) {
	var lost error
	for range startAttempts {
		ready, err := t.startOnce(ctx)
		if err == nil {
			return ready, nil
		}
		if !errors.Is(err, errServerGone) {
			return 0, err
		}
		lost = err
		t.forgetProcess()
	}
	return 0, fmt.Errorf("%d attempts, each on a port of its own, and none stayed up: %w", startAttempts, lost)
}

// startOnce is one attempt: reserve what the child must bind, start it, and
// wait for it to answer.
//
// The rate limiter is disabled unless the caller named a bound: refusals are a
// different measurement, made by the fairness scenario and by the httpe2e
// module, and one that would otherwise contaminate every capacity figure here.
func (t *httpTarget) startOnce(ctx context.Context) (time.Duration, error) {
	// Both addresses come from this attempt, because the server exits when
	// either one is taken: a retry that reused the profiling port it had lost
	// would lose every attempt to the same collision.
	wanted := 1
	if t.pprof {
		wanted = 2
	}
	ports, err := freePorts(ctx, wanted)
	if err != nil {
		return 0, err
	}
	t.addr = "127.0.0.1:" + strconv.Itoa(ports[0])
	if t.pprof {
		t.pprofAddr = "127.0.0.1:" + strconv.Itoa(ports[1])
	}

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
	reap := &procWait{}
	t.setCommand(cmd, reap)
	info, waitErr := t.waitHealthy(runCtx, reap.watch(cmd))
	if waitErr != nil {
		t.close()
		return 0, waitErr
	}
	t.info = info
	return time.Since(started), nil
}

// waitHealthy polls /health until the process answers, and reads the build it
// reports while it is there.
//
// gone closes when the process has been reaped, which ends the wait at once:
// a server that is no longer running will not answer, and the sixty seconds
// this would otherwise spend asking an address nothing holds are sixty seconds
// a reader spends waiting for a failure that already happened. A nil channel
// never closes, which is what a caller holding no process passes.
func (t *httpTarget) waitHealthy(ctx context.Context, gone <-chan struct{}) (ServerInfo, error) {
	// The requests are bound to the process's life as well as to the caller's
	// context, because the interesting collision is with something that is
	// listening: the connection is accepted and then nothing answers it, so a
	// poll costs the client's whole timeout while the answer, that this is no
	// longer our process, arrived seconds ago.
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		select {
		case <-gone:
			stop()
		case <-ctx.Done():
		}
	}()

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
		select {
		case <-gone:
			// Everything the process wrote is in the buffer by now: the wait
			// that closes this channel returns only once the copies of its two
			// streams are finished, so the reason it gave is here to report.
			return ServerInfo{}, fmt.Errorf("%w on %s:\n%s", errServerGone, t.addr, t.output.String())
		default:
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

// setCommand publishes the started process, and the reaper watching it, to
// whoever is watching them.
func (t *httpTarget) setCommand(cmd *exec.Cmd, reap *procWait) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cmd, t.reap = cmd, reap
}

// forgetProcess drops the process an attempt left behind, after that attempt
// has stopped and reaped it, so the next one publishes its own rather than
// inheriting a reaper that has already fired.
func (t *httpTarget) forgetProcess() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cmd, t.reap, t.cancel = nil, nil, nil
}

// command is the started process and the reaper that collects it, or nil
// before there is one. The lock is released before the caller does anything
// with them, since two of the three callers then block on the process for as
// long as it takes to die.
func (t *httpTarget) command() (*exec.Cmd, *procWait) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cmd, t.reap
}

// processes reports the single server process.
func (t *httpTarget) processes() []*os.Process {
	cmd, _ := t.command()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return []*os.Process{cmd.Process}
}

// goroutines dumps and counts the server's goroutines, ending its life.
func (t *httpTarget) goroutines() (int, error) {
	cmd, reap := t.command()
	if cmd == nil || cmd.Process == nil {
		return 0, errors.New("no server process")
	}
	return dumpGoroutines(cmd.Process, func() { reap.wait(cmd) }, t.output.String)
}

// serverInfo returns the build /health reported.
func (t *httpTarget) serverInfo() ServerInfo { return t.info }

// close stops the process.
func (t *httpTarget) close() {
	if t.cancel != nil {
		t.cancel()
	}
	if cmd, reap := t.command(); cmd != nil && reap != nil {
		reap.wait(cmd)
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

// freePorts reserves count ports by binding them, and releases them together.
//
// Together, rather than one after another, because a port released a moment
// ago is one the kernel may hand out again: two reservations made in turn can
// return the same number, and a server given that number twice, once to serve
// on and once for its profiling handlers, refuses to start with the address
// already in use. Holding every listener until the last one is bound is what
// makes them distinct; it is the one half of this the command can settle by
// itself.
//
// The other half it cannot. Each port is the kernel's to give away again from
// the moment this returns, so what comes back is a candidate rather than a
// reservation, and start is where that is dealt with: it notices a child that
// exited without serving and asks for another, rather than assuming the
// address was still ours by the time the child got to it.
func freePorts(ctx context.Context, count int) ([]int, error) {
	held := make([]net.Listener, 0, count)
	ports := make([]int, 0, count)
	for range count {
		listener, err := reservePort(ctx)
		if err != nil {
			releaseAll(held)
			return nil, fmt.Errorf("reserve a port: %w", err)
		}
		held = append(held, listener)
		addr, ok := listener.Addr().(*net.TCPAddr)
		if !ok {
			releaseAll(held)
			return nil, errors.New("the reserved listener is not a TCP address")
		}
		ports = append(ports, addr.Port)
	}
	for i, listener := range held {
		if closeErr := listener.Close(); closeErr != nil {
			releaseAll(held[i+1:])
			return nil, fmt.Errorf("release the reserved port: %w", closeErr)
		}
	}
	return ports, nil
}

// releaseAll closes what a failed reservation had bound so far. Whatever it
// reports is beside the point: the caller is already returning the failure
// that brought it here, and a port left bound would be a port the harness
// itself takes out of circulation.
func releaseAll(held []net.Listener) {
	for _, listener := range held {
		_ = listener.Close()
	}
}
