// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package conditions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// Release is the gobco release every figure is taken with, and Gobco the
// module it is run and installed from at that release.
const (
	Release = "v1.3.4"
	Gobco   = "github.com/rillig/gobco@v1.3.4"
)

// The statuses a run ends with, besides an interrupt's: the package was
// measured and the gate, if any, held nothing; nothing was measured; a
// setting could not be read; and the package was measured and the gate held
// at least one condition, which no other path reports.
const (
	Measured    = 0
	NotMeasured = 1
	Refused     = 2
	GateHeld    = 3
)

// installCommand is the command line that puts the pinned gobco on PATH,
// which a failed run through go run offers.
const installCommand = "go run ./cmd/coverage_recipes install-gobco"

// errInterrupted is what a call made after a signal returns: nothing is
// started once a run is ending.
var errInterrupted = errors.New("interrupted")

// Tools is what the recipe reaches outside itself: the go command, how a
// program is run, how gobco is looked up on PATH, and how the pause between
// two attempts at installing it is waited out.
type Tools struct {
	Go       string
	Run      recipe.Runner
	LookPath func(name string) (string, error)
	Pause    func(ctx context.Context, wait time.Duration)
}

// DefaultTools are the tools production measures with: the toolchain's own go
// command and the PATH of this process.
func DefaultTools() Tools {
	return Tools{Go: golist.Executable(), Run: recipe.Exec, LookPath: exec.LookPath, Pause: pause}
}

// Request is one package to measure: the directory the package is named from,
// the package and the build tags it is measured under, the environment the
// settings are read from and every program is given, and where the report and
// the messages go.
type Request struct {
	Dir     string
	Package string
	Tags    string
	Environ []string
	Stdout  io.Writer
	Stderr  io.Writer
}

// pause waits for wait, or until ctx is done.
func pause(ctx context.Context, wait time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(wait):
	}
}

// lockedWriter serializes the writes of the goroutines that copy a program's
// two streams, which a caller may have given one writer.
type lockedWriter struct {
	mu *sync.Mutex
	io.Writer
}

// Write writes under the lock the streams share.
func (w lockedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Writer.Write(data)
}

// Measure runs gobco over one package and returns the status of the run, as
// the package comment describes each. Stdout and Stderr of the request may be
// one writer.
func Measure(ctx context.Context, tools Tools, req Request) int {
	var mu sync.Mutex
	req.Stdout, req.Stderr = lockedWriter{mu: &mu, Writer: req.Stdout}, lockedWriter{mu: &mu, Writer: req.Stderr}
	m := &measurement{tools: tools, Request: req}
	status := m.measure(ctx)
	if interrupted, hit := recipe.Status(ctx); hit {
		return interrupted
	}
	return status
}

// measurement is one run: what was asked, how it was read, what the go
// command answered about the package, and what was staged of it.
type measurement struct {
	Request

	tools    Tools
	settings settings
	platform platform
	pkg      golist.PackageFiles
	// gated are the files a beyond: gate holds.
	gated  []string
	staged staging
}

// measure takes the run from its settings to the gate's verdict. The settings
// are read before anything runs, so a value that cannot be read is refused
// rather than discovered after a run of several minutes.
func (m *measurement) measure(ctx context.Context) int {
	read, err := readSettings(m.Environ, m.Tags)
	if err != nil {
		m.refusef(ctx, "%v; refusing to measure", err)
		return Refused
	}
	m.settings = read
	if !m.load(ctx) {
		return NotMeasured
	}
	if err = m.plan(ctx); err != nil {
		m.refusef(ctx, "%v", err)
		return NotMeasured
	}
	target, cleanup, err := m.stage()
	defer cleanup()
	if err != nil {
		m.refusef(ctx, "%s could not be staged: %v; refusing to measure", m.Package, err)
		return NotMeasured
	}
	report, passed := m.runGobco(ctx, target)
	if !passed {
		return NotMeasured
	}
	return m.judge(ctx, report)
}

// sayf prints one message of the recipe's own to stdout.
func (m *measurement) sayf(format string, args ...any) {
	fmt.Fprintf(m.Stdout, "gobco: "+format+"\n", args...)
}

// refusef prints one message of the recipe's own to stderr, unless a signal is
// ending the run, when what failed failed because of it.
func (m *measurement) refusef(ctx context.Context, format string, args ...any) {
	if ctx.Err() == nil {
		fmt.Fprintf(m.Stderr, "gobco: "+format+"\n", args...)
	}
}

// goOutput runs the go command in the request's directory, with env added to
// its environment, and returns what it wrote to its standard output. Its
// standard error is the caller's.
func (m *measurement) goOutput(ctx context.Context, env []string, args ...string) (string, error) {
	if ctx.Err() != nil {
		return "", errInterrupted
	}
	return recipe.Output(ctx, m.tools.Run, recipe.Call{
		Program: m.tools.Go, Args: args, Dir: m.Dir, Env: append(slices.Clone(m.Environ), env...), Stderr: m.Stderr,
	})
}

// context names the build this run measures.
func (m *measurement) context() string {
	return m.platform.target + " under build tags " + m.tagsShown()
}

// tagsShown is how a message names the build tags.
func (m *measurement) tagsShown() string {
	if m.Tags == "" {
		return "(none)"
	}
	return m.Tags
}

// runGobco runs gobco in target and returns the report it printed, which is
// passed through as it is written and kept, so the figure it ends on can be
// judged. gobco prints its figures even when its go test fails, so a failure
// is said out loud rather than left to an exit status a reader of the report
// may not see, in one line a caller can pick out (the gate over action specs
// reports it as the reason). A staged run then names what the staging itself
// changed, most likely first.
//
// gobco runs from PATH or through go run, as the package comment says, and a
// non-zero exit of gobco run from PATH is reported as 1, which is what go run
// reports for any non-zero exit of the program it ran, so a failed run reads
// as it always has whichever way gobco ran, and never as the gate's 3.
//
// gobco runs its go test with its own environment less GOPATH (goTest.env in
// main.go, v1.3.4), so where GOPATH names a directory other than the default
// that go test reads another module cache and downloads every dependency into
// it, which needs the network as much as go run does. GOMODCACHE, which gobco
// passes on, names the cache go uses here, so its go test reads the one go
// mod download filled. That goes when a gobco release passes GOPATH on.
func (m *measurement) runGobco(ctx context.Context, target string) (report string, passed bool) {
	var printed bytes.Buffer
	call := recipe.Call{
		Dir: target, Env: append(slices.Clone(m.Environ), "GOMODCACHE="+m.platform.modCache),
		Stdout: io.MultiWriter(m.Stdout, &printed), Stderr: m.Stderr,
	}
	onPath, _ := pinnedOnPath(ctx, m.tools, m.Environ)
	if onPath != "" {
		call.Program, call.Args = onPath, m.settings.args
	} else {
		call.Program, call.Args = m.tools.Go, append([]string{"run", Gobco}, m.settings.args...)
	}
	if ctx.Err() != nil {
		return "", false
	}
	status, err := m.tools.Run(ctx, call)
	if err != nil {
		m.refusef(ctx, "gobco could not be started on %s: %v; refusing to measure", m.Package, err)
		return "", false
	}
	if status == 0 {
		return printed.String(), true
	}
	if onPath != "" {
		status = 1
	}
	m.explainFailure(ctx, status, onPath != "")
	return "", false
}

// explainFailure says that a failed run's figures are no measurement, what
// the staging changed that could be why, and, last, since the run cannot tell
// it from the others, the cause a run from PATH cannot have: go's own message
// above names it when it is.
func (m *measurement) explainFailure(ctx context.Context, status int, fromPath bool) {
	const unmeasured = "so any figure above is not a measurement: its go test failed"
	if !m.staged.needed() {
		m.refusef(ctx, "gobco exited %d on %s, %s, or it could not instrument the package", status, m.Package, unmeasured)
	} else {
		m.refusef(ctx, "gobco exited %d on the staged copy of %s, %s there, or it could not instrument the copy", status, m.Package, unmeasured)
		if len(m.staged.constrained) > 0 {
			m.refusef(ctx, "the copy blanked the build constraint lines of %s, so a test that reads its own files' headers finds them blank there and fails; that is the likeliest cause (the e2e harness's source checks fail this way), and a package whose tests do that cannot be measured this way until gobco honors build tags (issue 1017)", joined(m.staged.constrained))
		}
		m.refusef(ctx, "a test that reads something the copy leaves out (.git, .claude/worktrees, node_modules, dist) fails there too, and the package's own tests run where it is are what to compare against")
	}
	if !fromPath {
		m.refusef(ctx, "it ran through go run, which asks the module proxy on every run and fails without the network (issue 1515); %s puts gobco %s on PATH, and a run from there needs none", installCommand, Release)
	}
}

// joined is how a message names several files.
func joined(names []string) string {
	return strings.Join(names, " ")
}
