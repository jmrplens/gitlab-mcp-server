//go:build e2e

// server.go builds the server once and starts it as many times as the run
// needs, each child with an environment built from nothing.
//
// Building rather than importing is the point of the whole suite: what is
// under test is the program, which decides its own catalog, its own middleware
// chain and its own recovery from the environment it is handed. A test that
// assembled a server in its own process would be testing that assembly, which
// is what the suite this replaces did and why five classes of defect were
// invisible to it.
//
// The environment is built from nothing for the same reason the stdio module
// builds its own: a developer's exported GITLAB_MCP_TOOL_SURFACE would
// otherwise decide what the suite exercises, silently. Only PATH, TMPDIR and,
// on Windows, SYSTEMROOT are passed through, because a process with none of
// those cannot run at all.

package harness

import (
	"context"
	"fmt"
	"maps"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// binaryEnv names a pre-built server binary to drive instead of building one.
// The Docker targets set it so the three packages share one build.
const binaryEnv = "E2E_SERVER_BINARY"

// serverLogDir is where each child's stderr is kept, relative to the
// repository root. A child's log outlives the run: when a test fails because
// the server refused something, the reason is in the server's log and nowhere
// else.
var serverLogDir = filepath.Join("dist", "e2e-reports", "servers")

// stderrTailBytes is how much of a child's stderr a failure message carries.
// Enough for a panic with its first frames, short enough to read.
const stderrTailBytes = 4096

// childrenStarted counts every server process this run launched, restarts
// included.
//
// It is the denominator of the coverage report: one started child is one
// counter file at the end, unless the child was killed or crashed. Counting
// starts rather than sessions is deliberate, since a session whose child died
// is given a fresh process from the same shape and that process writes a file
// of its own.
var childrenStarted atomic.Int64

// The build is done once per process, whatever it is for.
var (
	buildOnce   sync.Once
	builtBinary string
	builtDir    string
	errBuild    error
)

// serverBinary returns the path of the server these tests drive, building it
// on the first call.
//
// It deliberately does not use t.TempDir: the build is shared by every test in
// the package, and the first test to arrive would own a directory removed when
// that test ended, leaving every later test pointing at a path that is gone.
// Main removes the directory after the last test instead.
func serverBinary(prebuilt string) (string, error) {
	if prebuilt != "" {
		if _, err := os.Stat(prebuilt); err != nil {
			return "", fmt.Errorf("%s names %s, which cannot be used: %w", binaryEnv, prebuilt, err)
		}
		return prebuilt, nil
	}

	buildOnce.Do(func() {
		// Not t.TempDir: the directory is shared by every test in the package,
		// and the first test to arrive would own one removed when it ended.
		dir, err := os.MkdirTemp("", "gitlab-mcp-e2e")
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

		root, rootErr := repoRoot()
		if rootErr != nil {
			errBuild = rootErr
			return
		}

		// The build arguments and the bound come from the race seam, so a
		// `go test -race` run drives an instrumented server rather than an
		// uninstrumented one.
		ctx, cancel := context.WithTimeout(context.Background(), serverBuildTimeout)
		defer cancel()
		args := serverBuildArgs(out)
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = root
		output, buildFailed := cmd.CombinedOutput()
		if buildFailed != nil {
			errBuild = fmt.Errorf("building cmd/server: %w\n%s", buildFailed, output)
			return
		}
		builtBinary = out
	})
	return builtBinary, errBuild
}

// removeBuiltBinary deletes what serverBinary built, if it built anything.
//
// Keyed on the directory rather than the binary so a failed build is cleaned
// up too: the directory is created before the compile runs. Each build is tens
// of megabytes and a machine running this suite through a day would otherwise
// keep one per run.
func removeBuiltBinary() {
	if builtDir == "" {
		return
	}
	_ = os.RemoveAll(builtDir)
}

// forbiddenChildKeys are the variables a child must never be given, whatever
// the process running the tests has in its own environment.
//
// They all do the same thing: skip the confirmation a destructive action asks
// for. A suite that inherited one would be testing a server nobody deploys,
// and the tests that assert a destructive call is refused without a
// confirmation would pass for the wrong reason.
var forbiddenChildKeys = []string{"GITLAB_MCP_YOLO_MODE", "YOLO_MODE", "AUTOPILOT"}

// childEnv is the environment one server child runs with.
type childEnv struct {
	// GitLabURL and Token are the instance the child talks to.
	GitLabURL string
	Token     string
	// SkipTLSVerify carries the run's TLS policy through to the child.
	SkipTLSVerify bool
	// Root is the child's home, its working directory and the one directory
	// it is allowed to read local files from or write downloads into.
	Root string
	// TempDir is the run's own temporary directory, passed through so the
	// child writes where the machine expects rather than into /tmp on a host
	// that has moved it.
	TempDir string
	// Path is the executable search path, without which the child cannot run
	// git or anything else it shells out to.
	Path string
	// SystemRoot is Windows' own, and empty everywhere else.
	SystemRoot string
	// Extra carries the per-session variables: the tool surface, the mode,
	// the capability surface and whatever else a session shape declares. A
	// forbidden key here is dropped rather than honored.
	Extra map[string]string
}

// newChildEnv builds the environment for a child from the run's settings and
// one session's own variables.
//
// PATH, TMPDIR and SYSTEMROOT are read from the process; nothing else is. That
// is the whole inheritance, and it is why a developer's exported surface or
// token cannot change what the suite exercises.
func newChildEnv(s settings, root string, extra map[string]string) childEnv {
	return childEnv{
		GitLabURL:     s.get(envGitLabURL),
		Token:         s.get(envGitLabToken),
		SkipTLSVerify: strings.EqualFold(s.get(envSkipTLSVerify), "true"),
		Root:          root,
		TempDir:       os.Getenv("TMPDIR"),
		Path:          os.Getenv("PATH"),
		SystemRoot:    os.Getenv("SYSTEMROOT"),
		Extra:         extra,
	}
}

// environWithoutCredential returns the child's environment with the token
// left out, which is the environment an HTTP child is given.
//
// It is a separate call rather than a flag on the struct because the two
// transports want opposite things and both are right: a stdio child holds the
// credential, and an HTTP one holds none and reads each caller's out of the
// request. Given both, the child could serve a call whose header never
// reached the request and the scenario that checks exactly that would pass on
// the environment instead.
func (c childEnv) environWithoutCredential() []string {
	environ := c.environ()
	kept := environ[:0]
	for _, entry := range environ {
		if !strings.HasPrefix(entry, envGitLabToken+"=") {
			kept = append(kept, entry)
		}
	}
	return kept
}

// environ returns the child's environment as exec wants it, sorted so two
// children built from the same inputs are started identically.
func (c childEnv) environ() []string {
	vars := map[string]string{
		"PATH":                             c.Path,
		"HOME":                             c.Root,
		envGitLabURL:                       c.GitLabURL,
		envGitLabToken:                     c.Token,
		envSkipTLSVerify:                   strconv.FormatBool(c.SkipTLSVerify),
		"GITLAB_MCP_ALLOWED_UPLOAD_DIRS":   c.Root,
		"GITLAB_MCP_ALLOWED_DOWNLOAD_DIRS": c.Root,
		"GITLAB_MCP_ALLOWED_IMPORT_DIRS":   c.Root,
	}
	if c.TempDir != "" {
		vars["TMPDIR"] = c.TempDir
	}
	if runtime.GOOS == "windows" {
		// os.UserHomeDir reads USERPROFILE there, and the server resolves its
		// env file and its implicit allow-list root through it. A child given
		// only HOME would be given a home it never looks at.
		vars["USERPROFILE"] = c.Root
		if c.SystemRoot != "" {
			vars["SYSTEMROOT"] = c.SystemRoot
		}
	}
	// Before the caller's own entries, so a session that needs to say
	// something else about the detector still can.
	maps.Copy(vars, raceChildEnv())
	for key, value := range c.Extra {
		if slices.Contains(forbiddenChildKeys, strings.ToUpper(key)) {
			continue
		}
		vars[key] = value
	}
	for _, key := range forbiddenChildKeys {
		delete(vars, key)
	}

	environ := make([]string, 0, len(vars))
	for _, key := range slices.Sorted(maps.Keys(vars)) {
		environ = append(environ, key+"="+vars[key])
	}
	return environ
}

// serverProcess is one server child: the environment it runs with, the process
// itself, and what it has written to stderr.
//
// It can be started more than once. A child that dies takes its session with
// it, and the session layer starts a fresh one from the same shape rather than
// failing every later test of the run for a crash one test caused.
type serverProcess struct {
	label string
	env   childEnv
	bin   string

	mu     sync.Mutex
	sink   *stderrSink
	starts int

	exited chan struct{}
	state  atomic.Pointer[os.ProcessState]
}

// newServerProcess describes a child without starting it.
func newServerProcess(label, bin string, env childEnv) *serverProcess {
	return &serverProcess{label: label, bin: bin, env: env}
}

// transport returns the transport that starts this child and speaks to it.
//
// Each call builds a fresh command, so the same serverProcess can be started
// again after its child has gone. The reaper is started once the transport has
// the process, which is the earliest moment there is one to reap.
//
// The context bounds the child's life: canceling it kills a server that
// outlived what it was started for, which a stdio client cannot otherwise do
// once its own stdin is gone.
func (p *serverProcess) transport(ctx context.Context) mcp.Transport {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.sink != nil {
		p.sink.close()
	}
	p.starts++
	childrenStarted.Add(1)
	sink := newStderrSink(p.label, p.starts)

	cmd := exec.CommandContext(ctx, p.bin) //#nosec G204 -- the path is this package's own build or E2E_SERVER_BINARY, which only the run's operator sets
	terminateOnCancel(cmd)
	cmd.Env = p.env.environ()
	cmd.Dir = p.env.Root
	cmd.Stderr = sink

	p.sink = sink
	p.exited = make(chan struct{})
	p.state.Store(nil)

	return &childTransport{proc: p, inner: &mcp.CommandTransport{Command: cmd}, exited: p.exited}
}

// httpTransport starts this child as an HTTP server and returns the transport
// that speaks to it over a loopback listener.
//
// The shape differs from the stdio one in the way that matters: there, the
// transport starts the process, because the pipes it speaks over are the
// process's own. Here the process has to be listening before a client can
// connect at all, so it is started first and waited for, and the transport is
// an ordinary streamable client pointed at the address.
//
// The credential travels in a header rather than in the environment, which is
// the whole difference HTTP mode makes to a deployment: the server holds no
// token and every request carries its caller's. Driving it any other way would
// be testing a server this binary cannot be configured to be.
func (p *serverProcess) httpTransport(ctx context.Context, addr string) (mcp.Transport, error) {
	p.mu.Lock()
	if p.sink != nil {
		p.sink.close()
	}
	p.starts++
	childrenStarted.Add(1)
	sink := newStderrSink(p.label, p.starts)

	//#nosec G204 -- the path is this package's own build or E2E_SERVER_BINARY, and the instance URL is the one the run was pointed at; both are the operator's
	cmd := exec.CommandContext(ctx, p.bin,
		"--http", "--http-addr", addr,
		"--gitlab-url", p.env.GitLabURL,
	)
	terminateOnCancel(cmd)
	cmd.Env = p.env.environWithoutCredential()
	cmd.Dir = p.env.Root
	cmd.Stderr = sink

	p.sink = sink
	p.exited = make(chan struct{})
	p.state.Store(nil)
	exited := p.exited
	p.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting the %s server on %s: %w", p.label, addr, err)
	}
	p.reap(cmd, exited)

	if err := waitForHTTPServer(ctx, addr, exited); err != nil {
		// The context outlives a readiness failure, since it is the session's
		// and not this call's, so nothing else would end a child that started
		// and never listened. Left alone it holds the port for the rest of
		// the run, and the next session asking for a free one can be handed
		// this same address.
		p.stopChild(cmd)
		return nil, fmt.Errorf("%w\nserver stderr:\n%s", err, p.stderrTail())
	}
	return &mcp.StreamableClientTransport{
		Endpoint:   "http://" + addr,
		HTTPClient: &http.Client{Transport: credentialHeaders(p.env)},
	}, nil
}

// waitForHTTPServer blocks until the child answers its own health endpoint, or
// until it exits without ever having done so.
//
// Polling rather than reading the address off the log: the server writes its
// listening line before the listener accepts, so a client that raced it saw a
// connection refused and reported a defect that was a schedule.
func waitForHTTPServer(ctx context.Context, addr string, exited <-chan struct{}) error {
	deadline := time.Now().Add(httpStartTimeout)
	client := &http.Client{Timeout: httpProbeTimeout}
	for {
		select {
		case <-exited:
			return fmt.Errorf("the server exited before it listened on %s", addr)
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/health", http.NoBody)
		if err != nil {
			return fmt.Errorf("building the health probe for %s: %w", addr, err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the server did not answer /health on %s within %s", addr, httpStartTimeout)
		}
		time.Sleep(httpProbeInterval)
	}
}

// credentialHeaders carries the caller's credential on every request, the way
// an HTTP deployment's client does.
type credentialHeaders childEnv

func (c credentialHeaders) RoundTrip(req *http.Request) (*http.Response, error) {
	// Cloned rather than mutated: the SDK may reuse a request across a retry,
	// and a header set on the caller's own value would outlive this hop.
	cloned := req.Clone(req.Context())
	cloned.Header.Set("PRIVATE-TOKEN", c.Token)
	return http.DefaultTransport.RoundTrip(cloned)
}

// freeLoopbackAddr reserves a loopback address the child can bind.
//
// The port is taken and released rather than left to the child, because the
// binary does not report the port it chose in a form a test can read, and
// asking it for one that is already taken fails at startup where the reason is
// plain. The window between release and bind is the ordinary one every such
// helper has.
func freeLoopbackAddr(ctx context.Context) (string, error) {
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("reserving a loopback port: %w", err)
	}
	addr := listener.Addr().String()
	return addr, listener.Close()
}

const (
	// httpStartTimeout bounds the wait for the child's first answer. Generous,
	// because it covers building the whole catalog on a loaded machine.
	httpStartTimeout = 90 * time.Second
	// httpProbeInterval is how often the health endpoint is asked.
	httpProbeInterval = 50 * time.Millisecond
	// httpProbeTimeout bounds one probe, so a hung connection does not eat the
	// whole start budget.
	httpProbeTimeout = 5 * time.Second
	// childStopTimeout bounds the wait for a child that was killed to be
	// collected, so a process that somehow ignores the kill costs one second
	// rather than the rest of the run.
	childStopTimeout = 1 * time.Second
	// childTerminateDelay is how long a child has to act on the termination
	// signal before exec kills it anyway. Enough for an HTTP child to finish
	// its own shutdown and for the runtime to write its coverage counters,
	// short enough that a wedged child costs seconds rather than the run.
	childTerminateDelay = 10 * time.Second
	// childrenExitBudget bounds the whole end-of-run wait, not one child's:
	// they are all signaled at once and shut down at once, so a per-child
	// bound would multiply by however many shapes the run started.
	//
	// It covers the phase after the cancel and not the Close loop that runs
	// first. Each session's Close reaches the SDK's own pipe teardown, which
	// waits for the exit, then sends SIGTERM and waits again, then kills and
	// waits again: up to three times its TerminateDuration, 15 s at the SDK's
	// default, for one stdio child, and serially. A healthy child exits on
	// EOF in a few hundred milliseconds, so the loop is only the shape of the
	// teardown when a child has wedged -- which is also when go test's own
	// timeout is likeliest to fire mid-teardown and take the counters with it.
	childrenExitBudget = 30 * time.Second
)

// terminateOnCancel makes canceling a child's context ask it to stop instead
// of killing it.
//
// exec.CommandContext's default Cancel is Process.Kill, and a killed process
// runs no exit hook. The Go runtime writes a coverage counter file from one,
// so with the default every child of an instrumented run contributed its
// meta-data and none of its counters, and the whole measurement read zero. The
// server takes SIGTERM through signal.NotifyContext and unwinds to a normal
// return from main, which is both a clean shutdown and the flush.
//
// Windows keeps the default kill. It has no SIGTERM; the portable substitute
// is a console control event delivered to a process group the child has to
// have been created in, and nothing runs this harness there: the
// cross-platform matrix compiles it and runs ./cmd/... and ./internal/...
// only. A termination seam no run exercises is worth less than the kill it
// would replace.
func terminateOnCancel(cmd *exec.Cmd) {
	if runtime.GOOS == "windows" {
		return
	}
	// Returned unwrapped: exec compares it with os.ErrProcessDone to tell a
	// child that had already exited from one that refused the signal.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = childTerminateDelay
}

// waitForExit blocks until this child has been collected or the deadline has
// passed, and reports whether it ended.
//
// A child that was never started counts as ended: the caller is asking whether
// anything is still running, and nothing is.
func (p *serverProcess) waitForExit(deadline time.Time) bool {
	p.mu.Lock()
	exited := p.exited
	p.mu.Unlock()
	if exited == nil {
		return true
	}

	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-exited:
		return true
	case <-timer.C:
		return false
	}
}

// stopChild ends a child that started and never became usable, and waits for
// the reaper to record it.
//
// A kill rather than an interrupt, because the child never reached the state
// where it answers anything and there is nothing to shut down gracefully;
// Windows has no interrupt to send it either.
//
// The wait is waitForExit's, so there is one bounded wait on p.exited in this
// package rather than a second spelling of the same select: this one is the
// same wait under childStopTimeout with a kill in front of it.
func (p *serverProcess) stopChild(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	p.waitForExit(time.Now().Add(childStopTimeout))
}

// reap starts the one goroutine that collects the child.
//
// It waits through os.Process rather than exec.Cmd because the transport calls
// Cmd.Wait itself when the session closes: two Cmd.Wait calls cannot be told
// apart from a real failure, while a second os.Process.Wait answers
// ErrProcessDone deterministically. The cost is that the transport's own Close
// returns that error, which the session layer ignores, and the benefit is that
// the harness can say whether the server is still running before anybody has
// asked it to stop. Without that, a child killed by a panic looks exactly like
// a slow one.
func (p *serverProcess) reap(cmd *exec.Cmd, exited chan struct{}) {
	go func() {
		state, _ := cmd.Process.Wait()
		p.state.Store(state)
		close(exited)
	}()
}

// alive reports whether the child is still running.
func (p *serverProcess) alive() bool {
	p.mu.Lock()
	exited := p.exited
	p.mu.Unlock()
	if exited == nil {
		return false
	}
	select {
	case <-exited:
		return false
	default:
		return true
	}
}

// exitStatus describes how the child ended, in terms that make sense whether
// or not the reaper had recorded it when it was asked.
func (p *serverProcess) exitStatus() string {
	if state := p.state.Load(); state != nil {
		return state.String()
	}
	return "exit status not recorded"
}

// stderrTail returns the end of what the child has logged, which is what a
// failed call carries: a transport error says a pipe closed, and the server's
// last lines say why.
func (p *serverProcess) stderrTail() string {
	p.mu.Lock()
	sink := p.sink
	p.mu.Unlock()
	if sink == nil {
		return ""
	}
	return sink.tail()
}

// childTransport starts the child through the SDK's command transport and
// hands the reaper the process the moment there is one.
type childTransport struct {
	proc   *serverProcess
	inner  *mcp.CommandTransport
	exited chan struct{}
}

// Connect starts the child and begins watching it.
func (t *childTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	conn, err := t.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	t.proc.reap(t.inner.Command, t.exited)
	return conn, nil
}

// stderrSink keeps the tail of one child's stderr in memory and the whole of
// it on disk.
//
// In memory because a failing call has to be able to quote the server's last
// words, and on disk because the interesting failure is usually the one that
// happened three tests earlier.
type stderrSink struct {
	mu      sync.Mutex
	tailBuf []byte
	file    *os.File
}

// newStderrSink opens the log for one start of one child.
//
// A log that cannot be opened is not fatal, and the sink returns no error for
// that reason: the tail stays in memory, which is what a failing call quotes,
// and a run that could not create a directory under dist/ should not lose its
// server over it.
func newStderrSink(label string, start int) *stderrSink {
	sink := &stderrSink{}
	root, err := repoRoot()
	if err != nil {
		return sink
	}
	dir := filepath.Join(root, serverLogDir)
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return sink
	}
	name := fmt.Sprintf("%s-%d.log", sanitizeNamePart(label, 60), start)
	file, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return sink
	}
	sink.file = file
	return sink
}

// Write records what the child logged. It never fails: stderr that cannot be
// stored is worth less than a server killed by its own logging.
func (s *stderrSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file != nil {
		_, _ = s.file.Write(p)
	}
	s.tailBuf = append(s.tailBuf, p...)
	if len(s.tailBuf) > stderrTailBytes {
		s.tailBuf = slices.Clone(s.tailBuf[len(s.tailBuf)-stderrTailBytes:])
	}
	return len(p), nil
}

// tail returns what the sink kept.
func (s *stderrSink) tail() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.tailBuf)
}

// close releases the log file.
func (s *stderrSink) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
}
