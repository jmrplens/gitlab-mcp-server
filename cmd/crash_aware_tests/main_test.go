// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// main_test.go drives the whole command against a stand-in gotestsum: this
// test binary started again, which writes a recorded stream where it is told
// to and exits with the status it is told to. Each case is one of the paths
// the script took, and the job summary is read back from the file GitHub
// names.
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// standinEnv names the directory the stand-in reads its instructions from.
// Its presence is what makes this binary the stand-in rather than the tests.
const standinEnv = "CRASH_AWARE_TESTS_STANDIN"

// TestMain runs the tests, or, in a process the tests started as gotestsum,
// the stand-in.
func TestMain(m *testing.M) {
	if dir, standin := os.LookupEnv(standinEnv); standin {
		os.Exit(standIn(dir, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// standIn does what gotestsum does, as far as the command can tell: it
// records its arguments, writes to the --jsonfile path the stream recorded
// for that run (first.json or rerun.json under dir, when one is there), and
// exits with the status written beside it (first.status or rerun.status, 0
// when none is).
func standIn(dir string, args []string) int {
	calls, err := os.OpenFile(filepath.Join(dir, "calls"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 125
	}
	_, _ = calls.WriteString(strings.Join(args, " ") + "\n")
	_ = calls.Close()

	if len(args) < 3 || args[0] != "--jsonfile" || args[2] != "--" {
		return 126
	}
	run := strings.TrimSuffix(filepath.Base(args[1]), ".json")
	if data, readErr := os.ReadFile(filepath.Join(dir, run+".json")); readErr == nil {
		if os.WriteFile(args[1], data, 0o600) != nil { //#nosec G703 -- the --jsonfile path the test's own run named, below a t.TempDir
			return 127
		}
	}
	_, _ = os.Stdout.WriteString("stand-in gotestsum " + run + "\n")
	status, err := os.ReadFile(filepath.Join(dir, run+".status"))
	if err != nil {
		return 0
	}
	code, err := strconv.Atoi(strings.TrimSpace(string(status)))
	if err != nil {
		return 124
	}
	return code
}

// suite is one scripted run of the command: the streams and statuses the
// stand-in hands back, keyed by run ("first", "rerun").
type suite struct {
	streams  map[string]string
	statuses map[string]int
}

// result is what one run of the command left behind.
type result struct {
	code    int
	stdout  string
	stderr  string
	summary string
	calls   []string
	dir     string
}

// recorded reads one recorded stream under testdata/.
func recorded(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// plant writes one file of the stand-in's instructions: name under dir.
func plant(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil { //#nosec G703 -- a t.TempDir and a name these tests spell
		t.Fatal(err)
	}
}

// onlyOnPath leaves PATH holding one directory, which holds a program named
// name that is never started, and returns the path a lookup of name finds
// there. It changes the process's PATH, so a test calling it does not run in
// parallel.
func onlyOnPath(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		// A lookup on Windows finds a program by an extension PATHEXT
		// lists, never by its bare name.
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, nil, 0o700); err != nil { //#nosec G306 G703 -- a program the lookup must find executable, below the test's own t.TempDir
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return path
}

// absentCommand answers the gotestsum seam with a program that is not there.
func absentCommand(ctx context.Context, absent string, args []string) *exec.Cmd {
	return exec.CommandContext(ctx, absent, args...) //#nosec G204,G702 -- a path below a t.TempDir that holds no file, so nothing starts
}

// runWith runs the command over ./... with -count=1, gotestsum answered by
// the stand-in as s scripts it, and a job summary file to append to.
func runWith(t *testing.T, s suite) result {
	t.Helper()
	standin := t.TempDir()
	for run, name := range s.streams {
		plant(t, standin, run+".json", recorded(t, name))
	}
	for run, status := range s.statuses {
		plant(t, standin, run+".status", []byte(strconv.Itoa(status)))
	}
	useStandIn(t, standin)

	dir := filepath.Join(t.TempDir(), "unit-suite")
	summary := filepath.Join(t.TempDir(), "summary.md")
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), []string{dir, "-count=1", "-timeout=30m", "./..."}, summary, &stdout, &stderr)
	r := result{code: code, stdout: stdout.String(), stderr: stderr.String(), dir: dir}
	if data, err := os.ReadFile(summary); err == nil {
		r.summary = string(data)
	}
	if data, err := os.ReadFile(filepath.Join(standin, "calls")); err == nil {
		r.calls = strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}
	return r
}

// useStandIn points the gotestsum seam at this test binary for the rest of
// the test. The tests that call it do not run in parallel, since the seam is
// one variable.
func useStandIn(t *testing.T, dir string) {
	t.Helper()
	previous := gotestsumCommand
	t.Cleanup(func() { gotestsumCommand = previous })
	coverage := t.TempDir()
	gotestsumCommand = func(ctx context.Context, args []string) *exec.Cmd {
		// #nosec G204 G702 -- the program is this test binary, started again as the stand-in
		cmd := exec.CommandContext(ctx, os.Args[0], args...)
		// A binary built for a coverage run warns on stderr when it has
		// nowhere to write its counters, which is a stream these tests read.
		cmd.Env = append(os.Environ(), standinEnv+"="+dir, "GOCOVERDIR="+coverage)
		return cmd
	}
}

// TestRun_APassingSuite_ExitsWithGotestsumsStatusAndWritesNoSummary
// verifies that a run with nothing to rerun is gotestsum's run: its status
// is the command's, and the job summary is left alone.
func TestRun_APassingSuite_ExitsWithGotestsumsStatusAndWritesNoSummary(t *testing.T) {
	r := runWith(t, suite{streams: map[string]string{"first": "pass.json"}})
	if r.code != 0 || r.summary != "" {
		t.Errorf("exit %d, summary %q, want 0 and none\n%s%s", r.code, r.summary, r.stdout, r.stderr)
	}
	want := "--jsonfile " + filepath.Join(r.dir, "first.json") + " -- -count=1 -timeout=30m ./..."
	if len(r.calls) != 1 || r.calls[0] != want {
		t.Errorf("gotestsum was called %q, want once as %q", r.calls, want)
	}
	if !strings.Contains(r.stdout, "stand-in gotestsum first\n") {
		t.Errorf("gotestsum's own output did not reach stdout: %q", r.stdout)
	}
}

// TestRun_ANonZeroStatusWithNothingFailing_IsKept verifies that a status
// gotestsum gives for a reason the stream does not show still decides the
// run, as the script's exit "$first_status" did.
func TestRun_ANonZeroStatusWithNothingFailing_IsKept(t *testing.T) {
	r := runWith(t, suite{streams: map[string]string{"first": "pass.json"}, statuses: map[string]int{"first": 3}})
	if r.code != 3 {
		t.Errorf("exit %d, want 3", r.code)
	}
}

// TestRun_AFailingPackage_FailsAndListsItsTestsAndTheCrashesBesideIt
// verifies the failing path: the run fails without a rerun, and the summary
// lists each failing package with its tests, then the crash a failure left
// unexamined.
func TestRun_AFailingPackage_FailsAndListsItsTestsAndTheCrashesBesideIt(t *testing.T) {
	r := runWith(t, suite{streams: map[string]string{"first": "failing.json"}, statuses: map[string]int{"first": 1}})
	if r.code != 1 || len(r.calls) != 1 {
		t.Fatalf("exit %d after %d gotestsum calls, want 1 after one\n%s%s", r.code, len(r.calls), r.stdout, r.stderr)
	}
	want := "### Test suite: failing packages\n\n" +
		"- `" + gamma + "`\n" +
		"  - TestDelta\n" +
		"  - TestGamma\n" +
		"  - TestGamma/empty\n" +
		"\n" +
		"Also crashed inside the Go runtime or stopped without a word, not rerun because the failures above decide the run:\n" +
		"- `" + server + "`: fatal error: acquireSudog: found s.elem != nil in cache\n"
	if r.summary != want {
		t.Errorf("summary\n%s\nwant\n%s", r.summary, want)
	}
	if !strings.HasSuffix(r.stdout, want) {
		t.Errorf("stdout does not end with the summary:\n%s", r.stdout)
	}
}

// TestRun_AFailingPackageAlone_ListsNoCrashes verifies that the crash
// paragraph is left out when nothing crashed.
func TestRun_AFailingPackageAlone_ListsNoCrashes(t *testing.T) {
	r := runWith(t, suite{streams: map[string]string{"first": "rerun-fail.json"}, statuses: map[string]int{"first": 1}})
	want := "### Test suite: failing packages\n\n- `" + toolutil + "`\n  - TestWrapErr\n"
	if r.code != 1 || r.summary != want {
		t.Errorf("exit %d, summary\n%s\nwant 1 and\n%s", r.code, r.summary, want)
	}
}

// TestRun_ACrashThatPassesWhenRerun_PassesWithAWarning verifies the path
// the runner exists for: the crashed and the dead package are run again,
// alone and with the same flags, and passing there passes the run with a
// warning and a record in the job summary.
func TestRun_ACrashThatPassesWhenRerun_PassesWithAWarning(t *testing.T) {
	r := runWith(t, suite{
		streams:  map[string]string{"first": "crash.json", "rerun": "rerun-pass.json"},
		statuses: map[string]int{"first": 1},
	})
	if r.code != 0 {
		t.Fatalf("exit %d, want 0\n%s%s", r.code, r.stdout, r.stderr)
	}
	wantRerun := "--jsonfile " + filepath.Join(r.dir, "rerun.json") + " -- -count=1 -timeout=30m " + server + " " + toolutil
	if len(r.calls) != 2 || r.calls[1] != wantRerun {
		t.Errorf("gotestsum was called %q, want the rerun as %q", r.calls, wantRerun)
	}
	announcement := "\nThe Go runtime crashed in the following package(s), or their binary stopped without a word; rerunning them alone once (issue 467, golang/go#81238):\n" +
		"  " + server + ": fatal error: unexpected signal during runtime execution\n" +
		"  " + toolutil + ": " + deadLine + "\n\n"
	if !strings.Contains(r.stdout, announcement) {
		t.Errorf("stdout does not announce the rerun:\n%s", r.stdout)
	}
	if strings.Contains(r.summary, "rerunning them alone once") {
		t.Errorf("the announcement reached the job summary:\n%s", r.summary)
	}
	want := crashTable(
		"| `"+server+"` | `fatal error: unexpected signal during runtime execution` | passed |",
		"| `"+toolutil+"` | `"+deadLine+"` | passed |",
	)
	if r.summary != want {
		t.Errorf("summary\n%s\nwant\n%s", r.summary, want)
	}
	warning := "::warning title=Go runtime crash::the Go runtime crashed in 2 package(s) and every one passed when rerun alone; see the job summary (issue 467)\n"
	if !strings.HasSuffix(r.stdout, warning) {
		t.Errorf("stdout does not end with the warning:\n%s", r.stdout)
	}
}

// crashTable is the job summary of a rerun with rows as its table body.
func crashTable(rows ...string) string {
	return "### Test suite: Go runtime crash\n\n" +
		"The first run crashed inside the Go runtime, or a test binary stopped without a word (issue 467, golang/go#81238), and those packages were run once more on their own.\n\n" +
		"| Package | First run | Rerun |\n" +
		"| --- | --- | --- |\n" +
		strings.Join(rows, "\n") + "\n"
}

// TestRun_ARerunThatDoesNotPass_FailsTheRun verifies each way a rerun fails
// the run: a package crashing again, one failing, a rerun gotestsum exits
// non-zero on with a clean stream, and a rerun that wrote no stream at all.
func TestRun_ARerunThatDoesNotPass_FailsTheRun(t *testing.T) {
	cases := []struct {
		name     string
		s        suite
		outcomes [2]string
	}{
		{
			name:     "a crash again and a pass",
			s:        suite{streams: map[string]string{"first": "crash.json", "rerun": "rerun-crash.json"}, statuses: map[string]int{"rerun": 1}},
			outcomes: [2]string{"crashed again", "passed"},
		},
		{
			name:     "a pass and a failure",
			s:        suite{streams: map[string]string{"first": "crash.json", "rerun": "rerun-fail.json"}, statuses: map[string]int{"rerun": 1}},
			outcomes: [2]string{"passed", "failed"},
		},
		{
			name:     "a clean stream and a non-zero status",
			s:        suite{streams: map[string]string{"first": "crash.json", "rerun": "rerun-pass.json"}, statuses: map[string]int{"rerun": 2}},
			outcomes: [2]string{"passed", "passed"},
		},
		{
			name:     "no stream",
			s:        suite{streams: map[string]string{"first": "crash.json"}},
			outcomes: [2]string{"no stream", "no stream"},
		},
	}
	// sequential: each case points the one gotestsum seam at its own stand-in
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := runWith(t, tc.s)
			if r.code != 1 {
				t.Errorf("exit %d, want 1\n%s%s", r.code, r.stdout, r.stderr)
			}
			want := crashTable(
				"| `"+server+"` | `fatal error: unexpected signal during runtime execution` | "+tc.outcomes[0]+" |",
				"| `"+toolutil+"` | `"+deadLine+"` | "+tc.outcomes[1]+" |",
			) + "\nThe rerun did not pass, so the run fails.\n"
			if r.summary != want {
				t.Errorf("summary\n%s\nwant\n%s", r.summary, want)
			}
			if strings.Contains(r.stdout, "::warning") {
				t.Errorf("a failed rerun still warned:\n%s", r.stdout)
			}
		})
	}
}

// TestRun_AFirstRunWithNoStream_NeverPasses verifies that a first run that
// wrote no stream fails, with gotestsum's status when it gave one and 1 when
// it exited 0, which the script passed.
func TestRun_AFirstRunWithNoStream_NeverPasses(t *testing.T) {
	for name, tc := range map[string]struct{ status, want int }{
		"gotestsum exited 0": {status: 0, want: 1},
		"gotestsum exited 2": {status: 2, want: 2},
	} {
		t.Run(name, func(t *testing.T) {
			r := runWith(t, suite{statuses: map[string]int{"first": tc.status}})
			if r.code != tc.want {
				t.Errorf("exit %d, want %d", r.code, tc.want)
			}
			want := "ERROR: gotestsum wrote no JSON stream to " + filepath.Join(r.dir, "first.json") + " (exit " + strconv.Itoa(tc.status) + ")\n"
			if r.stderr != want {
				t.Errorf("stderr %q, want %q", r.stderr, want)
			}
		})
	}
}

// TestRun_AStreamThatCannotBeRead_Fails verifies that a stream path that is
// not a file stops the run with the reason.
func TestRun_AStreamThatCannotBeRead_Fails(t *testing.T) {
	useStandIn(t, t.TempDir())
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "first.json"), 0o750); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), []string{dir, "./..."}, "", &stdout, &stderr)
	if code != 1 || !strings.HasPrefix(stderr.String(), "ERROR: read "+filepath.Join(dir, "first.json")+": ") {
		t.Errorf("exit %d, stderr %q, want 1 and the read error", code, stderr.String())
	}
}

// TestRun_GotestsumThatCannotStart_Fails verifies that a gotestsum that is
// not there is a failure that says so, the first run and the rerun alike.
// gotestsum is missing the way it is on a machine without it, the real seam
// looking it up on a PATH that holds none, so the test changes PATH and does
// not run in parallel.
func TestRun_GotestsumThatCannotStart_Fails(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := run(t.Context(), []string{t.TempDir(), "./..."}, "", &stdout, &stderr)
	if code != 1 || !strings.HasPrefix(stderr.String(), "ERROR: run gotestsum: find gotestsum: ") {
		t.Errorf("exit %d, stderr %q, want 1 and the start error", code, stderr.String())
	}
}

// TestRun_ARerunThatCannotStart_FailsWithNoStream verifies the rerun half
// of the same failure: the first run crashed, gotestsum is gone by the
// rerun, and every package is reported with no stream.
func TestRun_ARerunThatCannotStart_FailsWithNoStream(t *testing.T) {
	standin := t.TempDir()
	plant(t, standin, "first.json", recorded(t, "crash.json"))
	useStandIn(t, standin)
	standinCommand := gotestsumCommand
	absent := filepath.Join(t.TempDir(), "gotestsum")
	gotestsumCommand = func(ctx context.Context, args []string) *exec.Cmd {
		if strings.HasSuffix(args[1], "rerun.json") {
			return absentCommand(ctx, absent, args)
		}
		return standinCommand(ctx, args)
	}

	var stdout, stderr bytes.Buffer
	summary := filepath.Join(t.TempDir(), "summary.md")
	code := run(t.Context(), []string{t.TempDir(), "./..."}, summary, &stdout, &stderr)
	written, readErr := os.ReadFile(summary)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if code != 1 || !strings.Contains(stderr.String(), "ERROR: run gotestsum: ") || !strings.Contains(string(written), "| no stream |") {
		t.Errorf("exit %d, stderr %q, summary %q, want 1, the start error and no stream", code, stderr.String(), written)
	}
}

// TestRun_ASummaryThatCannotBeWritten_IsWarnedAboutOnce verifies that a job
// summary path that cannot be opened costs the record and not the verdict:
// the run goes on to the same exit, and the warning is printed once.
func TestRun_ASummaryThatCannotBeWritten_IsWarnedAboutOnce(t *testing.T) {
	standin := t.TempDir()
	plant(t, standin, "first.json", recorded(t, "failing.json"))
	useStandIn(t, standin)

	summary := filepath.Join(t.TempDir(), "missing", "summary.md")
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), []string{t.TempDir(), "./..."}, summary, &stdout, &stderr)
	if code != 1 || strings.Count(stderr.String(), "WARNING: could not write the job summary") != 1 {
		t.Errorf("exit %d, stderr %q, want 1 and one warning", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "### Test suite: failing packages") {
		t.Errorf("the summary did not reach stdout:\n%s", stdout.String())
	}
}

// TestRun_OffCI_ReportsOnStdoutAlone verifies that with no job summary
// named, the report is printed and nothing else is attempted.
func TestRun_OffCI_ReportsOnStdoutAlone(t *testing.T) {
	standin := t.TempDir()
	plant(t, standin, "first.json", recorded(t, "rerun-fail.json"))
	useStandIn(t, standin)

	var stdout, stderr bytes.Buffer
	code := run(t.Context(), []string{t.TempDir(), "./..."}, "", &stdout, &stderr)
	if code != 1 || stderr.Len() != 0 || !strings.HasSuffix(stdout.String(), "- `"+toolutil+"`\n  - TestWrapErr\n") {
		t.Errorf("exit %d, stdout %q, stderr %q, want 1, the report and nothing on stderr", code, stdout.String(), stderr.String())
	}
}

// TestRun_ACrashLineCarryingAPipe_KeepsTheTableReadable verifies that a
// pipe in a crash line is escaped in its table cell, where GitHub would
// otherwise read it as the end of the cell.
func TestRun_ACrashLineCarryingAPipe_KeepsTheTableReadable(t *testing.T) {
	standin := t.TempDir()
	plant(t, standin, "first.json", streamOf(t,
		event{Action: "output", Package: server, Output: "fatal error: bad | pointer\n"},
		event{Action: "fail", Package: server},
	))
	plant(t, standin, "rerun.json", recorded(t, "rerun-pass.json"))
	useStandIn(t, standin)

	summary := filepath.Join(t.TempDir(), "summary.md")
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), []string{t.TempDir(), "./..."}, summary, &stdout, &stderr)
	written, readErr := os.ReadFile(summary)
	if readErr != nil {
		t.Fatal(readErr)
	}
	row := "| `" + server + "` | `fatal error: bad \\| pointer` | passed |"
	if code != 0 || !strings.Contains(string(written), row) {
		t.Errorf("exit %d, summary\n%s\nwant 0 and the row %q", code, written, row)
	}
}

// TestRun_TheCommandLine_IsRefusedWhenItNamesNoSuite verifies the usage
// errors: no arguments, flags and no package, and a stream directory that
// cannot be made.
func TestRun_TheCommandLine_IsRefusedWhenItNamesNoSuite(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{name: "no arguments", code: 2, stderr: usage + "\n"},
		{name: "flags and no package", args: []string{"dir", "-count=1"}, code: 2, stderr: "ERROR: no packages given\n" + usage + "\n"},
		{name: "a directory under a file", args: []string{filepath.Join(file, "dir"), "./..."}, code: 1, stderr: "ERROR: create " + filepath.Join(file, "dir") + ": "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code := run(t.Context(), tc.args, "", &stdout, &stderr)
			if code != tc.code || !strings.HasPrefix(stderr.String(), tc.stderr) || stdout.Len() != 0 {
				t.Errorf("exit %d, stdout %q, stderr %q, want %d and stderr starting %q", code, stdout.String(), stderr.String(), tc.code, tc.stderr)
			}
		})
	}
}

// TestGotestsumCommand_RunsGotestsumFromThePath verifies the seam's own
// answer, the program CI installs with go install tool, without starting it:
// the gotestsum a lookup finds on PATH, run by its absolute path with the
// arguments as given. It changes PATH, so it does not run in parallel.
func TestGotestsumCommand_RunsGotestsumFromThePath(t *testing.T) {
	gotestsum := onlyOnPath(t, "gotestsum")

	cmd := gotestsumCommand(t.Context(), []string{"--jsonfile", "x.json", "--", "./..."})
	if cmd.Err != nil || cmd.Path != gotestsum || !slices.Equal(cmd.Args, []string{gotestsum, "--jsonfile", "x.json", "--", "./..."}) {
		t.Errorf("command %q at %q (error %v), want %s with the arguments as given", cmd.Args, cmd.Path, cmd.Err, gotestsum)
	}
}

// TestGotestsumCommand_NoGotestsumOnPath_IsTheLookupsFailure verifies that
// gotestsum is found on PATH before it is run, and that a PATH holding none
// is the error running the command returns: the lookup's failure rather than
// a gotestsum that ran and exited. It changes PATH, so it does not run in
// parallel.
func TestGotestsumCommand_NoGotestsumOnPath_IsTheLookupsFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := gotestsumCommand(t.Context(), []string{"--version"}).Run()
	if !errors.Is(err, exec.ErrNotFound) || !strings.HasPrefix(err.Error(), "find gotestsum: ") {
		t.Errorf("running gotestsumCommand() with no gotestsum on PATH = %v, want the lookup's failure", err)
	}
}

// TestMain_HandsTheExitCodeToTheProcess covers main, which reads its
// arguments from the process: with none it prints the usage and exits 2. It
// changes os.Args and the exit seam, so it does not run in parallel.
func TestMain_HandsTheExitCodeToTheProcess(t *testing.T) {
	previousExit, previousArgs := exitProcess, os.Args
	t.Cleanup(func() { exitProcess, os.Args = previousExit, previousArgs })
	os.Args = []string{"crash_aware_tests"}

	code := -1
	exitProcess = func(c int) { code = c }
	main()
	if code != 2 {
		t.Errorf("main exited %d with no arguments, want 2", code)
	}
}

// TestExitStatus_MapsWhatGotestsumGaveToAnExitCode verifies the mapping of
// a status the first run ended with to the command's own exit: a killed
// gotestsum, whose status reads -1, is a failure rather than an exit the
// process would wrap around.
func TestExitStatus_MapsWhatGotestsumGaveToAnExitCode(t *testing.T) {
	t.Parallel()

	for status, want := range map[int]int{-1: 1, 0: 0, 1: 1, 3: 3} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			t.Parallel()

			if got := exitStatus(status); got != want {
				t.Errorf("exitStatus(%d) = %d, want %d", status, got, want)
			}
		})
	}
}
