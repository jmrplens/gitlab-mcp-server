// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package recipe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
)

// helperEnv names what this test binary does instead of running its tests,
// when it stands in for a program a recipe runs.
const helperEnv = "RECIPE_TEST_HELPER"

// The files a stand-in recipe writes where it runs.
const (
	startedFile = "started"
	endedFile   = "ended"
)

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
	case "pwd":
		fmt.Println(os.Getenv("PWD"))
	case "recipe":
		os.Exit(standInRecipe())
	case "signal":
		// Killed by its own signal, so it has no exit status.
		self, err := os.FindProcess(os.Getpid())
		if err == nil {
			_ = self.Kill()
		}
		time.Sleep(time.Minute)
	}
}

// standInRecipe is the program a make target runs in
// TestMakefile_ASIGTERMToMake_ReachesTheProgramOfTheTarget: it writes which
// process it is and what it was given to the file startedFile of the directory
// it runs in, waits for a signal, and returns the signal's status once it has
// written that to endedFile. One no signal reaches ends by itself.
func standInRecipe() int {
	ctx, stop := Signals(context.Background())
	defer stop()
	//#nosec G703 -- the path is a constant, and the arguments are only what the file holds
	if os.WriteFile(startedFile, fmt.Appendln(nil, os.Getpid(), os.Args[1:]), 0o600) != nil {
		return 1
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Minute):
		return 1
	}
	status, _ := Status(ctx)
	if os.WriteFile(endedFile, fmt.Append(nil, status), 0o600) != nil {
		return 1
	}
	return status
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

// TestExec_ADirectory_IsThePWDOfTheProgram verifies that PWD names the
// directory a program is run in and not the one its environment was taken
// from, whether the call names it whole or from this process's own; that a
// call that names none, whose program runs where this process does, and one on
// Windows, where PWD is read by nothing, hand their environment on as it is;
// and that a call without an environment is given this process's, as os/exec
// gives it.
func TestExec_ADirectory_IsThePWDOfTheProgram(t *testing.T) {
	root := t.TempDir()
	below := filepath.Join(root, "below")
	if err := os.Mkdir(below, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	t.Setenv(helperEnv, "pwd")
	t.Setenv("PWD", root)
	// os/exec names the directory itself then, but on Windows.
	withoutEnv := below
	if runtime.GOOS == "windows" {
		withoutEnv = root
	}
	cases := []struct {
		name, goos, dir string
		noEnv           bool
		want            string
	}{
		{name: "named whole", goos: "linux", dir: below, want: below},
		{name: "named from here", goos: "linux", dir: "below", want: below},
		{name: "none", goos: "linux", want: "elsewhere"},
		{name: "on Windows", goos: "windows", dir: below, want: "elsewhere"},
		{name: "no environment", goos: "linux", dir: below, noEnv: true, want: withoutEnv},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			previous := runtimeGOOS
			runtimeGOOS = tc.goos
			t.Cleanup(func() { runtimeGOOS = previous })
			var stdout bytes.Buffer
			// No test is run by a helper that lost the variable naming it.
			call := helper(t, "pwd", "-test.run=^$")
			call.Dir, call.Stdout, call.Env = tc.dir, &stdout, append(call.Env, "PWD=elsewhere")
			if tc.noEnv {
				call.Env = nil
			}
			if status, err := Exec(t.Context(), call); err != nil || status != 0 {
				t.Fatalf("Exec() = %d, %v; want 0", status, err)
			}
			if got := strings.TrimSpace(stdout.String()); got != tc.want {
				t.Errorf("PWD = %q, want %q", got, tc.want)
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

// TestLine_AnAnswer_LosesTheEndOfItsLineAlone verifies that a path a program
// names keeps a blank at its end, which a directory may hold.
func TestLine_AnAnswer_LosesTheEndOfItsLineAlone(t *testing.T) {
	t.Parallel()
	for out, want := range map[string]string{"": "", "\n": "", "/repo \n": "/repo ", " /repo\r\n": " /repo", "/repo": "/repo", "a\nb\n": "a\nb"} {
		t.Run(strconv.Quote(out), func(t *testing.T) {
			t.Parallel()
			if got := Line(out); got != want {
				t.Errorf("Line(%q) = %q, want %q", out, got, want)
			}
		})
	}
}

// noBreakSpace is a space no shell splits a word at.
const noBreakSpace = string(rune(0xa0))

// TestWords_AVariable_IsSplitAtTheBlanksAShellSplitsAt verifies that a run of
// spaces, tabs and line ends separates two words, and that a no-break space
// belongs to the word it is in.
func TestWords_AVariable_IsSplitAtTheBlanksAShellSplitsAt(t *testing.T) {
	t.Parallel()
	for value, want := range map[string][]string{
		"":                                 {},
		" \t\n":                            {},
		" -run=A  -v\t-x\n-y":              {"-run=A", "-v", "-x", "-y"},
		"-run=^A$|" + noBreakSpace + " -v": {"-run=^A$|" + noBreakSpace, "-v"},
		"--output=a" + noBreakSpace + "b":  {"--output=a" + noBreakSpace + "b"},
	} {
		t.Run(strconv.Quote(value), func(t *testing.T) {
			t.Parallel()
			if got := Words(value); !slices.Equal(got, want) {
				t.Errorf("Words(%q) = %q, want %q", value, got, want)
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

// goStandIn is a go command that builds and runs one program, this test
// binary, and runs it as `go run` does: as a child it waits for, which a
// signal that ends the go command does not reach.
const goStandIn = `#!/bin/sh
case $1 in
build)
  out=
  while [ "$#" -gt 0 ]; do
    if [ "$1" = -o ]; then out=$2; fi
    shift
  done
  mkdir -p "$(dirname "$out")" && ln -s "$RECIPE_TEST_SELF" "$out"
  ;;
run)
  shift 2
  "$RECIPE_TEST_SELF" "$@" &
  wait "$!"
  ;;
esac
`

// makeInDirectory is a make of this repository's Makefile in a directory of
// its own, where it builds into, with goStandIn as the go command and the
// stand-in recipe as everything it builds, and the files the stand-in writes.
// It skips the test where that make cannot run: macOS ships make 3.81, older
// than the 4.2 the Makefile needs, and Windows has no signal to send one.
func makeInDirectory(t *testing.T, args ...string) (run *exec.Cmd, started, ended string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("the Makefile's recipes are held on Linux; this is %s", runtime.GOOS)
	}
	makeBin, err := exec.LookPath("make")
	if err != nil {
		t.Skipf("make is not installed here: %v", err)
	}
	makefile, err := filepath.Abs(filepath.Join("..", "..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir, stubs := t.TempDir(), t.TempDir()
	//#nosec G306 -- the stand-in is a script make has to execute
	if err = os.WriteFile(filepath.Join(stubs, "go"), []byte(goStandIn), 0o700); err != nil {
		t.Fatal(err)
	}
	started, ended = filepath.Join(dir, startedFile), filepath.Join(dir, endedFile)
	run = exec.CommandContext(t.Context(), makeBin, append([]string{"-f", makefile}, args...)...) //#nosec G204 -- make from PATH, over this repository's own Makefile
	run.Dir = dir
	run.Env = append(slices.DeleteFunc(os.Environ(), func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")
		return slices.Contains([]string{"MAKEFLAGS", "MFLAGS", "MAKELEVEL"}, name)
	}), helperEnv+"=recipe", "RECIPE_TEST_SELF="+self, "PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	return run, started, ended
}

// TestMakefile_ASIGTERMToMake_ReachesTheProgramOfTheTarget sends make a SIGTERM
// while the program of a target runs, and verifies that the program was given
// what the target says, received the signal and ended before make did. make
// hands the signal to the process it started and to nothing below it, and `go
// run`, which the targets once ran their programs through, ends on it without
// passing it on: the program went on by itself, a staged copy still in the
// module, and started gremlins after the run it belonged to had been ended.
func TestMakefile_ASIGTERMToMake_ReachesTheProgramOfTheTarget(t *testing.T) {
	cases := []struct {
		target string
		args   []string
		want   string
	}{
		{target: "coverage-conditions", args: []string{"PKG=./a", "TAGS=e2e"}, want: "[conditions ./a e2e]"},
		{target: "coverage-mutants", args: []string{"PKG=./a", "MUTANT_BUDGET=1", "MUTANT_BUDGET_FLOOR=2", "MUTANT_DEADLINE_MAX=3"}, want: "[mutants ./a 1 2 3]"},
		{target: "check-spec-conditions", args: []string{"BASE=main"}, want: "[spec-conditions main]"},
		{target: "sonar", want: "[]"},
		{target: "sonar-status", want: "[-no-scan]"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			run, started, ended := makeInDirectory(t, append([]string{tc.target}, tc.args...)...)
			if err := run.Start(); err != nil {
				t.Fatal(err)
			}
			pid, got := awaitStandIn(t, started)
			if got != tc.want {
				t.Errorf("make %s gave its program %s, want %s", tc.target, got, tc.want)
			}
			if err := run.Process.Signal(syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			if err := run.Wait(); err == nil {
				t.Errorf("make %s ended as a run that was not interrupted", tc.target)
			}
			if status, err := os.ReadFile(ended); err != nil || string(status) != "143" {
				t.Errorf("the program of make %s did not end on the signal: it wrote %q, %v", tc.target, status, err)
				// It is a child of no process of this test by now, and it
				// would outlive the test by a minute.
				if left, findErr := os.FindProcess(pid); findErr == nil {
					_ = left.Kill()
				}
			}
		})
	}
}

// TestMakefile_ATrimmedBuild_LeavesItsProgramTheToolchainsRoot runs a target
// with -trimpath in GOFLAGS and no GOROOT, over a module whose command prints
// the root go/build answers, and verifies that the command still knows the one
// it was built with, which is where it finds go.
func TestMakefile_ATrimmedBuild_LeavesItsProgramTheToolchainsRoot(t *testing.T) {
	run, _, _ := makeInDirectory(t, "-s", "coverage-conditions", "PKG=./a")
	goBin := golist.Executable()
	// The Makefile asks for the toolchain the go directive names, so it names
	// this one.
	toolchain, _, _ := strings.Cut(strings.TrimPrefix(runtime.Version(), "go"), " ")
	plant(t, run.Dir, "go.mod", "module example.com/m\n\ngo "+toolchain+"\n", 0o600)
	plant(t, run.Dir, "cmd/coverage_recipes/main.go", "package main\n\nimport (\n\t\"fmt\"\n\t\"go/build\"\n)\n\nfunc main() { fmt.Println(build.Default.GOROOT) }\n", 0o600)
	run.Env = append(slices.DeleteFunc(run.Env, func(entry string) bool { return strings.HasPrefix(entry, "GOROOT=") }),
		"GOFLAGS=-trimpath", "GOWORK=off", "PATH="+filepath.Dir(goBin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	var said bytes.Buffer
	run.Stderr = &said
	out, err := run.Output()
	if want := filepath.Dir(filepath.Dir(goBin)) + "\n"; err != nil || string(out) != want {
		t.Errorf("the program of make coverage-conditions printed %q (%v, %s), want %q", out, err, said.String(), want)
	}
}

// awaitStandIn waits for the stand-in recipe to say it runs, and returns which
// process it is and the arguments it was given.
func awaitStandIn(t *testing.T, started string) (pid int, args string) {
	t.Helper()
	for range 3000 {
		if said, err := os.ReadFile(started); err == nil && bytes.HasSuffix(said, []byte("\n")) {
			number, rest, _ := strings.Cut(strings.TrimSuffix(string(said), "\n"), " ")
			if pid, err = strconv.Atoi(number); err != nil {
				t.Fatalf("the stand-in said %q", said)
			}
			return pid, rest
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("make did not run the program of its target")
	return 0, ""
}
