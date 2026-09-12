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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

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
		cmd := exec.CommandContext(ctx, "go", serverBuildArgs(out)...) //#nosec G204 -- every argument is a constant chosen by a build tag, plus a path from os.MkdirTemp
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
	sink := newStderrSink(p.label, p.starts)

	cmd := exec.CommandContext(ctx, p.bin) //#nosec G204 -- the path is this package's own build or E2E_SERVER_BINARY, which only the run's operator sets
	cmd.Env = p.env.environ()
	cmd.Dir = p.env.Root
	cmd.Stderr = sink

	p.sink = sink
	p.exited = make(chan struct{})
	p.state.Store(nil)

	return &childTransport{proc: p, inner: &mcp.CommandTransport{Command: cmd}, exited: p.exited}
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
