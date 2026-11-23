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

// Exec is the [Runner] production uses. The program is not ended when ctx is
// done: the recipe waits for it and then asks [Status] (see the package
// comment). A program a signal ended has no exit status of its own and
// reports 1.
func Exec(ctx context.Context, call Call) (int, error) {
	cmd := exec.CommandContext(context.WithoutCancel(ctx), call.Program, call.Args...) //#nosec G204 -- the callers resolve the program to an absolute path: the toolchain's own go command, or the tool a lookup named
	cmd.Dir = call.Dir
	cmd.Env = call.Env
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
