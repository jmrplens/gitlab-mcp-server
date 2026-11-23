// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package recipe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// helperEnv names what this test binary does instead of running its tests,
// when it stands in for a program a recipe runs.
const helperEnv = "RECIPE_TEST_HELPER"

// TestMain runs the tests, or the program helperEnv asks for.
func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "":
		os.Exit(m.Run())
	case "exit":
		fmt.Println("out", os.Args[1], mustGetwd())
		fmt.Fprintln(os.Stderr, "err", os.Getenv("RECIPE_TEST_VALUE"))
		status, err := strconv.Atoi(os.Args[1])
		if err != nil {
			os.Exit(125)
		}
		os.Exit(status)
	case "address-space":
		soft, hard, err := AddressSpace()
		fmt.Println(soft, hard, err)
	case "signal":
		// Killed by its own signal, so it has no exit status.
		self, err := os.FindProcess(os.Getpid())
		if err == nil {
			_ = self.Kill()
		}
		time.Sleep(time.Minute)
	}
}

// mustGetwd is the helper's working directory, with its links resolved.
func mustGetwd() string {
	dir, err := os.Getwd()
	if err != nil {
		os.Exit(124)
	}
	if dir, err = filepath.EvalSymlinks(dir); err != nil {
		os.Exit(124)
	}
	return filepath.Base(dir)
}

// helper is a call that runs this test binary as the program named.
func helper(t *testing.T, name string, args ...string) Call {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Call{Program: self, Args: args, Env: append(os.Environ(), helperEnv+"="+name)}
}

// noGrace takes the wait after a failed program away for one test.
func noGrace(t *testing.T) {
	t.Helper()
	previous := grace
	grace = 0
	t.Cleanup(func() { grace = previous })
}

// TestExec_AProgram_RunsInItsDirectoryWithItsEnvironmentAndStatus runs a real
// program: its arguments, directory and environment are the call's, both
// streams reach their writers, and its exit status comes back as the status,
// 0 included.
func TestExec_AProgram_RunsInItsDirectoryWithItsEnvironmentAndStatus(t *testing.T) {
	noGrace(t)
	for _, status := range []int{0, 3} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "here")
			if err := os.Mkdir(dir, 0o750); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			call := helper(t, "exit", strconv.Itoa(status))
			call.Dir, call.Stdout, call.Stderr = dir, &stdout, &stderr
			call.Env = append(call.Env, "RECIPE_TEST_VALUE=given")
			got, err := Exec(t.Context(), call)
			if err != nil || got != status {
				t.Errorf("Exec() = %d, %v; want %d", got, err, status)
			}
			if want := fmt.Sprintf("out %d here\n", status); stdout.String() != want {
				t.Errorf("stdout = %q, want %q", stdout.String(), want)
			}
			if stderr.String() != "err given\n" {
				t.Errorf("stderr = %q, want the environment's value", stderr.String())
			}
		})
	}
}

// TestExec_AProgramThatCannotStart_IsAnError verifies that a program with no
// status to read is an error, and that one a signal ended reports 1.
func TestExec_AProgramThatCannotStart_IsAnError(t *testing.T) {
	noGrace(t)
	if status, err := Exec(t.Context(), Call{Program: filepath.Join(t.TempDir(), "absent")}); err == nil || status != 0 {
		t.Errorf("Exec(absent) = %d, %v; want an error", status, err)
	}
	if status, err := Exec(t.Context(), helper(t, "signal")); err != nil || status != 1 {
		t.Errorf("Exec(a killed program) = %d, %v; want 1", status, err)
	}
}

// refusingWriter is a stream that takes nothing.
type refusingWriter struct{}

// Write is refused.
func (refusingWriter) Write([]byte) (int, error) { return 0, os.ErrClosed }

// TestExec_AStreamThatTakesNothing_IsAnError verifies that a program that ran
// and whose output was lost is an error rather than the status of a run whose
// report nobody has.
func TestExec_AStreamThatTakesNothing_IsAnError(t *testing.T) {
	noGrace(t)
	call := helper(t, "exit", "0")
	call.Stdout = refusingWriter{}
	if status, err := Exec(t.Context(), call); !errors.Is(err, os.ErrClosed) || status != 0 {
		t.Errorf("Exec() = %d, %v; want the stream's error", status, err)
	}
}

// TestExec_AContextAlreadyDone_StillRunsTheProgramToItsEnd verifies that a
// program is never ended by the context, and that one that fails under a done
// context is not waited on for a signal.
func TestExec_AContextAlreadyDone_StillRunsTheProgramToItsEnd(t *testing.T) {
	previous := grace
	grace = time.Hour
	t.Cleanup(func() { grace = previous })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if status, err := Exec(ctx, helper(t, "exit", "4")); err != nil || status != 4 {
		t.Errorf("Exec() under a done context = %d, %v; want the program's 4", status, err)
	}
}

// TestExec_AFailedProgram_IsGivenAMomentForTheSignalThatEndedIt verifies the
// wait after a failure: a signal that arrives within it ends it and is what
// Status then reports, which is how an interrupt of the whole process group
// reads as one rather than as the program's own failure.
func TestExec_AFailedProgram_IsGivenAMomentForTheSignalThatEndedIt(t *testing.T) {
	previous := grace
	grace = time.Hour
	t.Cleanup(func() { grace = previous })
	ctx, cancel := context.WithCancelCause(t.Context())
	time.AfterFunc(time.Millisecond, func() { cancel(InterruptedError{Signal: os.Interrupt}) })
	if status, err := Exec(ctx, helper(t, "exit", "1")); err != nil || status != 1 {
		t.Errorf("Exec() = %d, %v; want 1", status, err)
	}
	if status, interrupted := Status(ctx); !interrupted || status != 130 {
		t.Errorf("Status() = %d, %t; want 130", status, interrupted)
	}
}

// TestOutput_EachAnswer_IsTheOutputOrAnError verifies that a call's output is
// returned when it exits 0, and that a status and a program that could not
// start are errors, the first naming the program and the status.
func TestOutput_EachAnswer_IsTheOutputOrAnError(t *testing.T) {
	t.Parallel()
	planted := errors.New("planted")
	cases := []struct {
		name    string
		status  int
		err     error
		want    string
		wantErr string
	}{
		{name: "answered", want: "answer"},
		{name: "failed", status: 2, wantErr: "prog a b exited 2"},
		{name: "not started", err: planted, wantErr: "planted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			run := func(_ context.Context, call Call) (int, error) {
				_, _ = call.Stdout.Write([]byte("answer"))
				return tc.status, tc.err
			}
			got, err := Output(t.Context(), run, Call{Program: "prog", Args: []string{"a", "b"}})
			if tc.wantErr == "" {
				if err != nil || got != tc.want {
					t.Errorf("Output() = %q, %v; want %q", got, err, tc.want)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr || got != "" {
				t.Errorf("Output() = %q, %v; want the error %q", got, err, tc.wantErr)
			}
		})
	}
}

// TestGetenv_AnEnvironment_AnswersTheLastEntryOfAKey verifies that a key is
// read whole, that its last entry wins, that an empty value is set, and that
// an entry that is no KEY=value answers nothing.
func TestGetenv_AnEnvironment_AnswersTheLastEntryOfAKey(t *testing.T) {
	t.Parallel()
	environ := []string{"A=1", "AB=2", "A=3", "EMPTY=", "bare", "EQ=x=y"}
	cases := []struct {
		key, want string
		set       bool
	}{{"A", "3", true}, {"AB", "2", true}, {"EMPTY", "", true}, {"EQ", "x=y", true}, {"bare", "", false}, {"B", "", false}}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()
			if got, set := Getenv(environ, tc.key); got != tc.want || set != tc.set {
				t.Errorf("Getenv(%q) = %q, %t; want %q, %t", tc.key, got, set, tc.want, tc.set)
			}
		})
	}
}

// TestStatus_EachCause_IsTheStatusAShellGivesIt verifies the statuses of the
// two signals, and that a context that is not done, or is done for another
// reason, is no interrupt.
func TestStatus_EachCause_IsTheStatusAShellGivesIt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		cause error
		want  int
		hit   bool
	}{
		{name: "SIGTERM", cause: InterruptedError{Signal: syscall.SIGTERM}, want: 143, hit: true},
		{name: "the interrupt", cause: fmt.Errorf("wrapped: %w", InterruptedError{Signal: os.Interrupt}), want: 130, hit: true},
		{name: "another reason", cause: errors.New("planted")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancelCause(t.Context())
			if status, hit := Status(ctx); status != 0 || hit {
				t.Errorf("Status() before = %d, %t; want none", status, hit)
			}
			cancel(tc.cause)
			if status, hit := Status(ctx); status != tc.want || hit != tc.hit {
				t.Errorf("Status() = %d, %t; want %d, %t", status, hit, tc.want, tc.hit)
			}
		})
	}
	if got := (InterruptedError{Signal: syscall.SIGTERM}).Error(); got != "interrupted by terminated" {
		t.Errorf("Error() = %q", got)
	}
}

// TestSignals_ASignal_EndsTheContextAndStopLetsItGo sends this process a real
// signal, where a process can be sent one, and verifies the context ends with
// it as its cause; and that stop ends a context no signal reached for no
// cause of its own.
func TestSignals_ASignal_EndsTheContextAndStopLetsItGo(t *testing.T) {
	ctx, stop := Signals(t.Context())
	stop()
	<-ctx.Done()
	if status, hit := Status(ctx); hit {
		t.Errorf("Status() after stop = %d, want no interrupt", status)
	}
	if runtime.GOOS == "windows" {
		t.Skip("a process cannot send itself a signal on Windows")
	}
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			// Held by the test as well, so a signal that arrives once the
			// recipe has stopped listening does not end the test binary.
			held := make(chan os.Signal, 1)
			signal.Notify(held, sig)
			defer signal.Stop(held)
			interrupted, stopSignals := Signals(t.Context())
			defer stopSignals()
			self, err := os.FindProcess(os.Getpid())
			if err != nil {
				t.Fatal(err)
			}
			if err = self.Signal(sig); err != nil {
				t.Fatal(err)
			}
			select {
			case <-interrupted.Done():
			case <-time.After(30 * time.Second):
				t.Fatal("the signal did not end the context")
			}
			want := 130
			if sig == syscall.SIGTERM {
				want = 143
			}
			if status, hit := Status(interrupted); !hit || status != want {
				t.Errorf("Status() = %d, %t; want %d", status, hit, want)
			}
			if cause := context.Cause(interrupted); !strings.HasPrefix(cause.Error(), "interrupted by ") {
				t.Errorf("cause = %v", cause)
			}
		})
	}
}
