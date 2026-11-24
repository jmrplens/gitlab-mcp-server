// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package recipe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"
)

// Call is one program a recipe runs: its absolute path, its arguments, the
// directory and the environment it runs in, and where its output goes. A nil
// stream is discarded, and the program reads nothing. AddressSpace is the
// ceiling in bytes on its address space and on that of everything it starts,
// which only Linux is asked for; 0 sets none.
type Call struct {
	Program string
	Args    []string
	Dir     string
	Env     []string
	Stdout  io.Writer
	Stderr  io.Writer

	AddressSpace uint64
}

// Runner runs a call to its end and returns its exit status. An error means
// the program could not be started, so there is no status to read.
type Runner func(ctx context.Context, call Call) (int, error)

// grace is how long [Exec] waits for a signal after a program it ran failed.
// A signal sent to the whole process group ends the program at once and
// reaches this process through the runtime, which may take a moment longer,
// and a program that failed for that reason is not a measurement that failed.
// It is a variable so a test need not wait it out, and a tenth of a second.
var grace = time.Duration(1e8)

// runtimeGOOS is the platform, a variable so a test can ask for another's
// answer.
var runtimeGOOS = runtime.GOOS

// environment is the environment the call's program is given: the call's own,
// with PWD naming the directory the program runs in, as a shell that changed
// to it would have it. os/exec does that only for a call that gives no
// environment, and with the PWD of this process in its place the go command
// names a directory reached through a symbolic link by the path behind it,
// where one run in this process's own directory names it through the link: two
// listings of one module then cannot be cut against each other. Windows reads
// no PWD, as os/exec knows too. A directory that cannot be named leaves PWD
// empty, which its readers take for none.
func (c Call) environment() []string {
	if c.Env == nil || c.Dir == "" || runtimeGOOS == "windows" {
		return c.Env
	}
	dir, _ := filepath.Abs(c.Dir)
	return append(slices.Clone(c.Env), "PWD="+dir)
}

// Exec is the [Runner] production uses. The program is not ended when ctx is
// done: the recipe waits for it and then asks [Status] (see the package
// comment). A program a signal ended has no exit status of its own and
// reports 1.
func Exec(ctx context.Context, call Call) (int, error) {
	cmd := exec.CommandContext(context.WithoutCancel(ctx), call.Program, call.Args...) //#nosec G204 -- the callers resolve the program to an absolute path: the toolchain's own go command, or the tool a lookup named
	cmd.Dir = call.Dir
	cmd.Env = call.environment()
	cmd.Stdout = call.Stdout
	cmd.Stderr = call.Stderr
	start := cmd.Start
	if call.AddressSpace != 0 {
		start = func() error { return startLimited(cmd, call.AddressSpace) }
	}
	if err := start(); err != nil {
		return 0, err
	}
	err := cmd.Wait()
	if err == nil {
		return 0, nil
	}
	exit, exited := errors.AsType[*exec.ExitError](err)
	if !exited {
		return 0, err
	}
	select {
	case <-ctx.Done():
	case <-time.After(grace):
	}
	return max(exit.ExitCode(), 1), nil
}

// Output runs a call through run and returns what it wrote to its standard
// output, for the calls whose answer is read rather than passed on. A program
// that exits non-zero is an error, as one that cannot be started is.
func Output(ctx context.Context, run Runner, call Call) (string, error) {
	var out strings.Builder
	call.Stdout = &out
	status, err := run(ctx, call)
	if err != nil {
		return "", err
	}
	if status != 0 {
		return "", fmt.Errorf("%s exited %d", strings.Join(append([]string{call.Program}, call.Args...), " "), status)
	}
	return out.String(), nil
}

// Line returns what a program wrote without the end of its last line, so a path
// it named keeps every other character of its own, a blank at its end included.
func Line(out string) string {
	return strings.TrimSuffix(strings.TrimSuffix(out, "\n"), "\r")
}

// Words splits a variable that holds several words at the blanks a shell splits
// one at, a space, a tab and a line's end, so any other space belongs to the
// word it is in.
func Words(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return strings.ContainsRune(" \t\n", r) })
}

// Getenv returns the value environ, a list of KEY=value entries as
// [os.Environ] returns, gives key, and whether it is set. The last entry of a
// repeated key wins, as it does for the process the list is handed to.
func Getenv(environ []string, key string) (value string, set bool) {
	for _, entry := range environ {
		if name, entryValue, ok := strings.Cut(entry, "="); ok && name == key {
			value, set = entryValue, true
		}
	}
	return value, set
}

// InterruptedError is the cause of a context [Signals] ended.
type InterruptedError struct {
	Signal os.Signal
}

// Error names the signal.
func (e InterruptedError) Error() string {
	return "interrupted by " + e.Signal.String()
}

// Signals returns a context a SIGINT or SIGTERM ends, and the function that
// stops listening for them.
func Signals(parent context.Context) (ctx context.Context, stop func()) {
	ctx, cancel := context.WithCancelCause(parent)
	received := make(chan os.Signal, 1)
	signal.Notify(received, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-received:
			cancel(InterruptedError{Signal: sig})
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(received)
		cancel(nil)
	}
}

// Status reports whether a signal ended ctx, and the status a shell gives a
// run it ended: 143 for SIGTERM and 130 for the interrupt.
func Status(ctx context.Context) (status int, interrupted bool) {
	cause, ok := errors.AsType[InterruptedError](context.Cause(ctx))
	if !ok {
		return 0, false
	}
	if cause.Signal == syscall.SIGTERM {
		return 143, true
	}
	return 130, true
}
