// main_test.go covers the command's own decisions: which matrix a set of flags
// selects, and the guard that keeps a smoke run from overwriting published
// measurements.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// setArgs installs a command line on a flag set of its own, so the test
// binary's own flags are untouched, for a caller that parses it itself.
func setArgs(t *testing.T, args ...string) {
	t.Helper()
	previousArgs, previousFlags := os.Args, flag.CommandLine
	t.Cleanup(func() { os.Args, flag.CommandLine = previousArgs, previousFlags })

	flag.CommandLine = flag.NewFlagSet("bench_resources", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = append([]string{"bench_resources"}, args...)
}

// withArgs runs parseFlags against a command line installed by setArgs.
func withArgs(t *testing.T, args ...string) options {
	t.Helper()
	setArgs(t, args...)
	return parseFlags()
}

// TestParseFlags_RecordsWhetherTheOutputPathWasChosen verifies -json is
// remembered as given rather than merely as a value.
//
// It is what lets the guard tell a partial run writing somewhere of its own
// from a partial run about to overwrite the published record: the two are
// indistinguishable by path, because passing the default path explicitly is a
// deliberate act and defaulting into it is not.
func TestParseFlags_RecordsWhetherTheOutputPathWasChosen(t *testing.T) {
	t.Run("not given", func(t *testing.T) {
		opts := withArgs(t)
		if opts.recordSet {
			t.Error("recordSet is true with no -json on the command line")
		}
		if opts.record != defaultRecord {
			t.Errorf("record = %q, want the default %q", opts.record, defaultRecord)
		}
	})

	t.Run("given a different path", func(t *testing.T) {
		opts := withArgs(t, "-json=/tmp/somewhere-else.json")
		if !opts.recordSet {
			t.Error("recordSet is false although -json was given")
		}
		if opts.record != "/tmp/somewhere-else.json" {
			t.Errorf("record = %q, want the path given", opts.record)
		}
	})

	t.Run("given the default path explicitly", func(t *testing.T) {
		// The distinction the guard rests on: same value, different intent.
		opts := withArgs(t, "-json="+defaultRecord)
		if !opts.recordSet {
			t.Error("recordSet is false although -json was given explicitly")
		}
	})
}

// TestMain_ChecksTheCommittedArtifactsAndExitsOnFailure drives main through
// its two ends: -check over the committed record, which is the CI gate and
// must find the committed charts current, and a redraw from a record that
// is not there, which must name the failure and exit non-zero.
func TestMain_ChecksTheCommittedArtifactsAndExitsOnFailure(t *testing.T) {
	if _, err := projectRootForTest(); err != nil {
		t.Skipf("not running inside the module: %v", err)
	}
	var exits []int
	previous := exitProcess
	exitProcess = func(code int) { exits = append(exits, code) }
	t.Cleanup(func() { exitProcess = previous })

	t.Run("check", func(t *testing.T) {
		setArgs(t, "-check")
		main()
		if len(exits) != 0 {
			t.Errorf("main exited with %v over the committed record; the charts or tables are stale", exits)
		}
	})
	t.Run("absent record", func(t *testing.T) {
		exits = nil
		setArgs(t, "-render", "-json="+filepath.Join(t.TempDir(), "absent.json"))
		main()
		if !reflect.DeepEqual(exits, []int{1}) {
			t.Errorf("main exited with %v, want a single 1", exits)
		}
	})
}

// TestParseFlags_Defaults_AreTheOnesTheDocumentationClaims verifies the values
// a plain run measures with, since the published page states them.
func TestParseFlags_Defaults_AreTheOnesTheDocumentationClaims(t *testing.T) {
	opts := withArgs(t)
	if opts.rounds != 3 {
		t.Errorf("rounds = %d, want 3", opts.rounds)
	}
	if opts.sampleInterval != 100*time.Millisecond {
		t.Errorf("sample interval = %v, want 100ms", opts.sampleInterval)
	}
	for name, got := range map[string]bool{
		"render": opts.render, "check": opts.check, "quick": opts.quick, "v": opts.verbose,
	} {
		t.Run("-"+name+" is off", func(t *testing.T) {
			if got {
				t.Errorf("-%s defaults to on", name)
			}
		})
	}
}

// TestParseFlags_MeasurementFlags_AreRead verifies the flags a run is steered
// with reach the options struct, since a silently ignored one would produce a
// record that does not match what was asked for.
func TestParseFlags_MeasurementFlags_AreRead(t *testing.T) {
	opts := withArgs(t,
		"-rounds=7",
		"-sample-interval=250ms",
		"-scenarios=http-dynamic,stdio-meta",
		"-quick",
		"-v",
		"-binary=/somewhere/gitlab-mcp-server",
	)
	if opts.rounds != 7 {
		t.Errorf("rounds = %d, want 7", opts.rounds)
	}
	if opts.sampleInterval != 250*time.Millisecond {
		t.Errorf("sample interval = %v, want 250ms", opts.sampleInterval)
	}
	if opts.scenarios != "http-dynamic,stdio-meta" {
		t.Errorf("scenarios = %q, want the two given", opts.scenarios)
	}
	if !opts.quick || !opts.verbose {
		t.Errorf("-quick/-v = %v/%v, want both on", opts.quick, opts.verbose)
	}
	if opts.binary != "/somewhere/gitlab-mcp-server" {
		t.Errorf("binary = %q, want the path given", opts.binary)
	}
}

// TestMatrixFor_FullRun_UsesThePublishedMatrix verifies a plain run measures
// everything the page publishes, at the requested number of rounds.
func TestMatrixFor_FullRun_UsesThePublishedMatrix(t *testing.T) {
	plans, err := matrixFor(options{rounds: 5, stepDuration: 7 * time.Second})
	if err != nil {
		t.Fatalf("matrixFor: %v", err)
	}
	if len(plans) != len(publishedMatrix(testSettings(5))) {
		t.Errorf("selected %d scenarios, want the whole matrix", len(plans))
	}
	for _, plan := range plans {
		if plan.isSeries() {
			if !reflect.DeepEqual(plan.Steps, defaultSeriesSteps) || plan.StepDuration != 7*time.Second {
				t.Errorf("%s steps %v over %s, want the default counts over the 7s asked for", plan.ID, plan.Steps, plan.StepDuration)
			}
			continue
		}
		if plan.Rounds != 5 {
			t.Errorf("%s runs %d rounds, want the 5 that were asked for", plan.ID, plan.Rounds)
		}
	}
}

// TestMatrixFor_SeriesKnobs verifies how -quick, -clients and -step-duration
// combine into the series plans: -quick pins the smoke counts and the short
// phase, a typed -clients wins over both, and a typed -step-duration survives
// -quick.
func TestMatrixFor_SeriesKnobs(t *testing.T) {
	cases := []struct {
		name         string
		opts         options
		wantSteps    []int
		wantDuration time.Duration
		wantErr      string
	}{
		{
			name:         "quick pins the smoke series",
			opts:         options{quick: true, rounds: 1, stepDuration: defaultStepDuration, recordSet: true},
			wantSteps:    quickSeriesSteps,
			wantDuration: quickStepDuration,
		},
		{
			name:         "quick keeps a typed step duration",
			opts:         options{quick: true, rounds: 1, stepDuration: 4 * time.Second, stepDurationSet: true, recordSet: true},
			wantSteps:    quickSeriesSteps,
			wantDuration: 4 * time.Second,
		},
		{
			name:         "clients override the quick list",
			opts:         options{quick: true, rounds: 1, stepDuration: defaultStepDuration, clients: "1,2", clientsSet: true, recordSet: true},
			wantSteps:    []int{1, 2},
			wantDuration: quickStepDuration,
		},
		{
			name:         "clients on the full matrix",
			opts:         options{rounds: 1, stepDuration: defaultStepDuration, clients: "3,30", clientsSet: true, recordSet: true},
			wantSteps:    []int{3, 30},
			wantDuration: defaultStepDuration,
		},
		{
			name:    "clients need a record of their own",
			opts:    options{rounds: 1, stepDuration: defaultStepDuration, clients: "1,2", clientsSet: true},
			wantErr: "-json",
		},
		{
			name:    "unusable clients",
			opts:    options{rounds: 1, stepDuration: defaultStepDuration, clients: "2,1", clientsSet: true, recordSet: true},
			wantErr: "-clients",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plans, err := matrixFor(tc.opts)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("matrixFor = %v, want an error naming %s", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("matrixFor: %v", err)
			}
			assertSeriesPlans(t, plans, tc.wantSteps, tc.wantDuration)
		})
	}
}

// assertSeriesPlans checks every series in a matrix carries the given counts
// and steady-phase length, and that there is at least one.
func assertSeriesPlans(t *testing.T, plans []scenarioPlan, wantSteps []int, wantDuration time.Duration) {
	t.Helper()
	seen := 0
	for _, plan := range plans {
		if !plan.isSeries() {
			continue
		}
		seen++
		if !reflect.DeepEqual(plan.Steps, wantSteps) || plan.StepDuration != wantDuration {
			t.Errorf("%s steps %v over %s, want %v over %s", plan.ID, plan.Steps, plan.StepDuration, wantSteps, wantDuration)
		}
	}
	if seen == 0 {
		t.Error("the matrix holds no series")
	}
}

// TestMatrixFor_PartialRun_RefusesToOverwriteTheRecord verifies a smoke or
// filtered run insists on being told where to write.
//
// Without this, `-quick` would replace measured, published numbers with a
// two-scenario smoke test and the documentation would keep presenting them as
// the figures. The guard is on the flag having been given, not on its value,
// so passing the default path explicitly is still allowed: that is someone
// saying what they mean.
func TestMatrixFor_PartialRun_RefusesToOverwriteTheRecord(t *testing.T) {
	const step = defaultStepDuration
	tests := []struct {
		name    string
		opts    options
		wantErr bool
	}{
		{name: "quick without a path", opts: options{quick: true, rounds: 1, stepDuration: step}, wantErr: true},
		{name: "quick with a path", opts: options{quick: true, rounds: 1, stepDuration: step, recordSet: true}},
		{name: "filtered without a path", opts: options{scenarios: "http-meta", rounds: 1, stepDuration: step}, wantErr: true},
		{name: "filtered with a path", opts: options{scenarios: "http-meta", rounds: 1, stepDuration: step, recordSet: true}},
		{name: "full run needs no path", opts: options{rounds: 1, stepDuration: step}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := matrixFor(tc.opts)
			if tc.wantErr {
				if err == nil {
					t.Fatal("matrixFor allowed a partial matrix to write to the published record")
				}
				if !strings.Contains(err.Error(), "-json") {
					t.Errorf("error %q does not say how to fix it", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("matrixFor: %v", err)
			}
		})
	}
}

// TestMatrixFor_UnknownScenario_ReportsIt verifies a mistyped scenario name
// stops the run instead of measuring a smaller matrix.
func TestMatrixFor_UnknownScenario_ReportsIt(t *testing.T) {
	if _, err := matrixFor(options{scenarios: "http-dinamic", rounds: 1, stepDuration: defaultStepDuration, recordSet: true}); err == nil {
		t.Error("matrixFor accepted a scenario name that does not exist")
	}
}

// TestResolveAndRel_RoundTrip verifies flag paths are resolved against the
// module root and reported back relative to it, so the command's output names
// files the way the repository does.
func TestResolveAndRel_RoundTrip(t *testing.T) {
	root := t.TempDir()

	absolute := resolve(root, "docs/reference/benchmarks")
	if !strings.HasPrefix(absolute, root) {
		t.Errorf("resolve = %q, want it under %q", absolute, root)
	}
	if got := rel(root, absolute); got != "docs/reference/benchmarks" {
		t.Errorf("rel = %q, want the relative path back", got)
	}

	// An absolute flag value is left alone, which is what lets a smoke run
	// write outside the repository. Built from a temporary directory so it
	// is rooted on every platform: "/tmp" carries no volume on Windows.
	outside := filepath.Join(t.TempDir(), "bench.json")
	if got := resolve(root, outside); got != outside {
		t.Errorf("resolve rewrote an absolute path: %q", got)
	}
}

// TestProgressFunc_QuietUnlessAsked verifies the per-client reporter is silent
// by default, so a full run prints one line per scenario rather than hundreds.
func TestProgressFunc_QuietUnlessAsked(t *testing.T) {
	quiet := progressFunc(false)
	quiet("this must not panic %d", 1)

	loud := progressFunc(true)
	loud("nor this %d", 2)
}

// TestOptionsValidate_RejectsValuesThatWouldMeasureNothing verifies the two
// flags that reach the measurement machinery are refused before it starts.
//
// Both used to be taken verbatim. A non-positive -sample-interval reached
// time.NewTicker, which panics, several minutes into a run; a non-positive
// -rounds produced a plan with no load rounds, whose zero-sample latencies
// were then written over the published record. Rejecting them at the front
// costs a line and saves a run.
func TestOptionsValidate_RejectsValuesThatWouldMeasureNothing(t *testing.T) {
	const good = 100 * time.Millisecond
	const step = defaultStepDuration
	cases := []struct {
		name    string
		opts    options
		wantErr string
	}{
		{"zero rounds", options{rounds: 0, sampleInterval: good, stepDuration: step}, "-rounds"},
		{"negative rounds", options{rounds: -1, sampleInterval: good, stepDuration: step}, "-rounds"},
		{"zero interval", options{rounds: 3, sampleInterval: 0, stepDuration: step}, "-sample-interval"},
		{"negative interval", options{rounds: 3, sampleInterval: -time.Second, stepDuration: step}, "-sample-interval"},
		{"zero step duration", options{rounds: 3, sampleInterval: good, stepDuration: 0}, "-step-duration"},
		{"negative budget", options{rounds: 3, sampleInterval: good, stepDuration: step, memoryBudget: -1}, "-memory-budget"},
		{"unusable clients", options{rounds: 3, sampleInterval: good, stepDuration: step, clients: "5,1", clientsSet: true}, "-clients"},
		{"all valid", options{rounds: 3, sampleInterval: good, stepDuration: step, clients: "1,5", clientsSet: true}, ""},
		// A redraw reads the committed record and reaches neither a ticker
		// nor a round, so refusing it over an unused flag would reject a
		// legitimate command.
		{"render ignores them", options{render: true}, ""},
		{"check ignores them", options{check: true}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("validate() = %v, want no error", err)
			case tc.wantErr != "" && err == nil:
				t.Errorf("validate() = nil, want an error naming %s", tc.wantErr)
			case tc.wantErr != "" && err != nil && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("validate() = %v, want it to name %s", err, tc.wantErr)
			}
		})
	}
}

// standinPath is the stand-in server under testdata/standin, built once by
// TestMain for every test that measures a process. Empty on Windows, where
// those tests skip: the harness samples /proc or ps and asks for a goroutine
// count with SIGQUIT, and Windows has neither.
var standinPath string

// TestMain builds the stand-in before the tests and removes it after them.
//
// Built here rather than per test because a t.TempDir goes with the test that
// made it, and built at all because the process-spawning half of this command
// cannot be exercised against anything in-process: the readiness poll, the
// pipe wiring, the resident-set sampling and the traceback signal all need a
// child. Linking the real server for that would cost a minute per run to
// learn nothing about the harness.
func TestMain(m *testing.M) {
	os.Exit(runWithStandin(m))
}

// runWithStandin builds the stand-in, runs the tests and cleans up after them.
func runWithStandin(m *testing.M) int {
	if runtime.GOOS != "windows" {
		dir, err := os.MkdirTemp("", "bench-standin")
		if err != nil {
			fmt.Fprintf(os.Stderr, "create a build directory for the stand-in: %v\n", err)
			return 1
		}
		defer func() { _ = os.RemoveAll(dir) }()
		path := filepath.Join(dir, "standin")
		// Bounded like buildServer: the Go test timeout cannot stop a build
		// started before m.Run, so a stalled one would hold the package until
		// an outer runner gave up.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		build := exec.CommandContext(ctx, "go", "build", "-o", path, "./testdata/standin")
		if out, buildErr := build.CombinedOutput(); buildErr != nil {
			fmt.Fprintf(os.Stderr, "build testdata/standin: %v\n%s", buildErr, out)
			return 1
		}
		standinPath = path
	}
	return m.Run()
}

// standinBinary returns the stand-in server, skipping where nothing can be
// measured.
func standinBinary(t *testing.T) string {
	t.Helper()
	if standinPath == "" {
		t.Skip("the harness samples /proc or ps and signals SIGQUIT, neither of which Windows has")
	}
	return standinPath
}

// quickOptions are the flags of a smoke run against the stand-in: one round, a
// fast sampler, and a record path of its own so the guard against overwriting
// the published record lets it through.
func quickOptions(t *testing.T, root string) options {
	t.Helper()
	return options{
		binary:         standinBinary(t),
		record:         filepath.Join(root, "record.json"),
		recordSet:      true,
		rounds:         1,
		sampleInterval: 20 * time.Millisecond,
		quick:          true,
		// Two counts and the shortest phase the flag accepts in whole
		// seconds, so the series costs the smoke run a few seconds.
		clients:      "1,2",
		clientsSet:   true,
		stepDuration: time.Second,
		profiles:     filepath.Join(root, "profiles"),
	}
}

// TestMeasure_QuickMatrix_ProducesOneScenarioPerPlan runs the smoke matrix
// against the stand-in and checks the record carries what every downstream
// artifact reads: a scenario per point plan, the series with a step per
// count, the build the server reported, and the settings the matrix imposed
// rather than the ones the flags asked for.
func TestMeasure_QuickMatrix_ProducesOneScenarioPerPlan(t *testing.T) {
	root := t.TempDir()
	opts := quickOptions(t, root)
	opts.rounds = 7 // the smoke matrix pins one round, and the record must say so

	run, err := measure(opts, root)
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if got, want := len(run.Scenarios), len(pointPlans(quickMatrix(testSettings(1)))); got != want {
		t.Fatalf("recorded %d scenarios, want %d", got, want)
	}
	if len(run.Series) != 1 || len(run.Series[0].Steps) != 2 {
		t.Fatalf("recorded series %+v, want one series of two steps", run.Series)
	}
	if run.Server.Version != "standin" || run.Server.Commit == "" {
		t.Errorf("server info = %+v, want the build /health reported", run.Server)
	}
	if run.Settings.Rounds != 1 || !run.Settings.Quick || run.Settings.SampleIntervalMs != 20 {
		t.Errorf("settings = %+v, want the smoke matrix's one round, quick, 20 ms", run.Settings)
	}
	if run.Schema != resultSchema || run.GeneratedAt == "" || run.Host.OS == "" {
		t.Errorf("record header incomplete: schema %d, generated %q, host %+v", run.Schema, run.GeneratedAt, run.Host)
	}
	for _, scenario := range run.Scenarios {
		t.Run(scenario.ID, func(t *testing.T) {
			if len(scenario.Latency) != 3 {
				t.Errorf("%d latency rows, want one per method", len(scenario.Latency))
			}
			if scenario.Goroutines == 0 {
				t.Errorf("no goroutine count: notes %v", scenario.Notes)
			}
		})
	}
}

// TestExecute_MeasureRenderCheck_AgreeWithEachOther drives the command the
// way make does: a measurement that writes the record and every artifact, then
// -render over the same record, then -check, which must find nothing stale in
// what the two previous calls wrote.
func TestExecute_MeasureRenderCheck_AgreeWithEachOther(t *testing.T) {
	root, tree := renderTree(t)
	opts := quickOptions(t, root)
	// execute resolves relative paths against the real module root, so the
	// temporary tree's paths go in absolute.
	opts.docCharts = filepath.Join(root, filepath.FromSlash(tree.docCharts))
	opts.siteCharts = filepath.Join(root, filepath.FromSlash(tree.siteCharts))
	opts.docPage = filepath.Join(root, filepath.FromSlash(tree.docPage))
	opts.sitePageEN = filepath.Join(root, filepath.FromSlash(tree.sitePageEN))
	opts.sitePageES = filepath.Join(root, filepath.FromSlash(tree.sitePageES))

	if err := execute(opts); err != nil {
		t.Fatalf("execute (measure): %v", err)
	}
	run, err := readRun(opts.record)
	if err != nil {
		t.Fatalf("the measurement left no readable record: %v", err)
	}
	assertChartsWritten(t, root, tree, run)

	redraw := opts
	redraw.render = true
	if renderErr := execute(redraw); renderErr != nil {
		t.Errorf("execute (render): %v", renderErr)
	}
	check := opts
	check.check = true
	if checkErr := execute(check); checkErr != nil {
		t.Errorf("execute (check) over what execute just wrote: %v", checkErr)
	}
}

// TestExecute_RefusesWhatItCannotRun covers the two ways the command stops
// before measuring or drawing: flags that would measure nothing, and a record
// that is not there to redraw from.
func TestExecute_RefusesWhatItCannotRun(t *testing.T) {
	t.Run("invalid flags", func(t *testing.T) {
		err := execute(options{rounds: 0, sampleInterval: time.Millisecond})
		if err == nil || !strings.Contains(err.Error(), "-rounds") {
			t.Errorf("execute = %v, want the -rounds refusal", err)
		}
	})
	t.Run("absent record", func(t *testing.T) {
		absent := filepath.Join(t.TempDir(), "absent.json")
		err := execute(options{render: true, record: absent})
		if err == nil || !strings.Contains(err.Error(), absent) {
			t.Errorf("execute = %v, want it to name %s", err, absent)
		}
	})
}

// writeModuleFile creates one file of the throwaway module below.
func writeModuleFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	//#nosec G703 -- both halves of the path are this test's own: a t.TempDir and a literal
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestBuildServer_CompilesCmdServerUnderTheRoot points buildServer at a
// throwaway module with a trivial cmd/server, which exercises the real build
// path in a second rather than the minute the actual server takes to link.
func TestBuildServer_CompilesCmdServerUnderTheRoot(t *testing.T) {
	root := t.TempDir()
	writeModuleFile(t, filepath.Join(root, "go.mod"), "module standin.example/server\n\ngo 1.22\n")
	writeModuleFile(t, filepath.Join(root, "cmd", "server", "main.go"), "package main\n\nfunc main() {}\n")

	built, err := buildServer(root)
	if err != nil {
		t.Fatalf("buildServer: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(built)) })
	info, statErr := os.Stat(built)
	if statErr != nil {
		t.Fatalf("the built binary is not there: %v", statErr)
	}
	if info.Size() == 0 {
		t.Error("the built binary is empty")
	}
}

// TestBuildServer_NothingToBuild_ReportsTheCompilerOutput checks a root with
// no module in it fails with the compiler's own words attached, since that is
// what somebody running the benchmark from the wrong directory needs to see.
func TestBuildServer_NothingToBuild_ReportsTheCompilerOutput(t *testing.T) {
	_, err := buildServer(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "build ./cmd/server") {
		t.Errorf("buildServer = %v, want the build failure", err)
	}
}

// TestRel_PathThatCannotBeMadeRelative_IsReturnedWhole covers the fallback
// for a path filepath.Rel refuses, which happens when only one of the two is
// absolute, and for a path outside the root, which is better named whole
// than as a climb of "../" from wherever the checkout is.
func TestRel_PathThatCannotBeMadeRelative_IsReturnedWhole(t *testing.T) {
	absolute := filepath.Join(t.TempDir(), "record.json")
	if got := rel("relative-root", absolute); got != absolute {
		t.Errorf("rel = %q, want the path untouched %q", got, absolute)
	}
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "bench.json")
	if got := rel(root, outside); got != outside {
		t.Errorf("rel of a path outside the root = %q, want it whole", got)
	}
	if got := rel(root, filepath.Dir(root)); got != filepath.Dir(root) {
		t.Errorf("rel of the parent = %q, want it whole", got)
	}
}

// TestParseFlags_SeriesFlags_AreReadAndRemembered verifies the series
// knobs reach the options and that the two -quick overrides are remembered
// as typed, which is what lets a typed value survive -quick.
func TestParseFlags_SeriesFlags_AreReadAndRemembered(t *testing.T) {
	opts := withArgs(t, "-clients=1,2", "-step-duration=3s", "-memory-budget=512", "-profiles=/tmp/p", "-no-render")
	if opts.clients != "1,2" || !opts.clientsSet {
		t.Errorf("clients = %q set=%v, want 1,2 remembered as typed", opts.clients, opts.clientsSet)
	}
	if opts.stepDuration != 3*time.Second || !opts.stepDurationSet {
		t.Errorf("step duration = %s set=%v, want 3s remembered as typed", opts.stepDuration, opts.stepDurationSet)
	}
	if opts.memoryBudget != 512 || opts.profiles != "/tmp/p" || !opts.noRender {
		t.Errorf("budget %d profiles %q noRender %v, want what was typed", opts.memoryBudget, opts.profiles, opts.noRender)
	}
	defaults := withArgs(t)
	if defaults.stepDuration != defaultStepDuration || defaults.profiles != defaultProfiles || defaults.clientsSet || defaults.stepDurationSet {
		t.Errorf("defaults %+v, want the documented ones with nothing remembered as typed", defaults)
	}
}

// TestLocateRoot_MeasureOnlyRunNeedsNoRepository verifies a measure-only
// run with a prebuilt binary works from a directory with no go.mod above it,
// which is the shape the series runs in on the host with the memory, and
// that every other run still insists on the root.
func TestLocateRoot_MeasureOnlyRunNeedsNoRepository(t *testing.T) {
	outside := t.TempDir()
	t.Chdir(outside)

	t.Run("no render with a binary", func(t *testing.T) {
		got, err := locateRoot(options{noRender: true, binary: "/somewhere/server"})
		if err != nil {
			t.Fatalf("locateRoot: %v", err)
		}
		if resolved, _ := filepath.EvalSymlinks(outside); filepath.Clean(got) != resolved && filepath.Clean(got) != outside {
			t.Errorf("locateRoot = %q, want the working directory %q", got, outside)
		}
	})
	t.Run("no render without a binary", func(t *testing.T) {
		if _, err := locateRoot(options{noRender: true}); err == nil {
			t.Error("locateRoot found a root to build the server in where there is none")
		}
	})
	t.Run("a rendering run", func(t *testing.T) {
		if _, err := locateRoot(options{render: true}); err == nil {
			t.Error("locateRoot found a root to render into where there is none")
		}
	})
	t.Run("working directory unreadable", func(t *testing.T) {
		previous := getwd
		getwd = func() (string, error) { return "", errors.New("gone") }
		t.Cleanup(func() { getwd = previous })
		if _, err := locateRoot(options{noRender: true, binary: "/somewhere/server"}); err == nil || !strings.Contains(err.Error(), "working directory") {
			t.Errorf("locateRoot = %v, want the working directory failure", err)
		}
	})
}

// TestExecute_MeasureOnly_WritesTheRecordAndNothingElse runs the smoke
// matrix with -no-render from a directory that is not a checkout, and checks
// the record and the profiles are the only things written.
func TestExecute_MeasureOnly_WritesTheRecordAndNothingElse(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	opts := quickOptions(t, root)
	opts.noRender = true
	opts.docPage = filepath.Join(root, "doc.md")

	if err := execute(opts); err != nil {
		t.Fatalf("execute -no-render: %v", err)
	}
	run, err := readRun(opts.record)
	if err != nil {
		t.Fatalf("no readable record: %v", err)
	}
	if len(run.Series) != 1 || run.Series[0].Steps[0].Profiles.CPU == "" {
		t.Errorf("record %+v, want the series with its profiles", run.Series)
	}
	if _, statErr := os.Stat(opts.docPage); statErr == nil {
		t.Error("a page was written by a run asked not to render")
	}
}

// TestExecute_Refusals covers the failures execute reports before or after
// measuring: a root it cannot find, a matrix it cannot build, and a record it
// cannot write.
func TestExecute_Refusals(t *testing.T) {
	t.Run("no root", func(t *testing.T) {
		t.Chdir(t.TempDir())
		if err := execute(options{render: true, record: "x.json"}); err == nil {
			t.Error("execute rendered from a directory with no repository above it")
		}
	})
	t.Run("matrix cannot be built", func(t *testing.T) {
		opts := quickOptions(t, t.TempDir())
		opts.clients = "5,1"
		if err := execute(opts); err == nil || !strings.Contains(err.Error(), "-clients") {
			t.Errorf("execute = %v, want the -clients refusal", err)
		}
	})
	t.Run("measurement fails", func(t *testing.T) {
		t.Setenv("STANDIN_FAIL", methodToolsList)
		opts := quickOptions(t, t.TempDir())
		if err := execute(opts); err == nil || !strings.Contains(err.Error(), "scenario stdio-dynamic") {
			t.Errorf("execute = %v, want the first scenario's failure", err)
		}
	})
	t.Run("record cannot be written", func(t *testing.T) {
		root := t.TempDir()
		opts := quickOptions(t, root)
		opts.record = root // a directory, not a file
		opts.noRender = true
		if err := execute(opts); err == nil || !strings.Contains(err.Error(), "write") {
			t.Errorf("execute = %v, want the write failure", err)
		}
	})
}

// TestMeasure_Failures covers the ways measurement stops: a server built
// from the checkout that exits at once, so the first scenario fails, and a
// series that measures no step.
func TestMeasure_Failures(t *testing.T) {
	t.Run("built server that serves nothing", func(t *testing.T) {
		root := t.TempDir()
		writeModuleFile(t, filepath.Join(root, "go.mod"), "module standin.example/server\n\ngo 1.22\n")
		writeModuleFile(t, filepath.Join(root, "cmd", "server", "main.go"), "package main\n\nfunc main() {}\n")
		opts := quickOptions(t, root)
		opts.binary = ""
		_, err := measure(opts, root)
		if err == nil || !strings.Contains(err.Error(), "scenario stdio-dynamic") {
			t.Errorf("measure = %v, want the first scenario's failure against a server that exits", err)
		}
	})
	t.Run("nothing to build", func(t *testing.T) {
		root := t.TempDir()
		opts := quickOptions(t, root)
		opts.binary = ""
		if _, err := measure(opts, root); err == nil || !strings.Contains(err.Error(), "build ./cmd/server") {
			t.Errorf("measure = %v, want the build failure from a root with no module", err)
		}
	})
	t.Run("a series that measures nothing", func(t *testing.T) {
		t.Setenv("STANDIN_FAIL", methodToolsList)
		root := t.TempDir()
		opts := quickOptions(t, root)
		opts.scenarios = "http-dynamic-series"
		_, err := measure(opts, root)
		if err == nil || !strings.Contains(err.Error(), "series http-dynamic-series") {
			t.Errorf("measure = %v, want the series failure", err)
		}
	})
	t.Run("nothing to measure", func(t *testing.T) {
		opts := quickOptions(t, t.TempDir())
		opts.clients = "0"
		if _, err := measure(opts, t.TempDir()); err == nil {
			t.Error("measure accepted a credential count of zero")
		}
	})
}

// TestPointRounds_SeriesOnlyMatrix_ReportsNone verifies a matrix of only
// series records no rounds, since a series has none, rather than a number
// from a scenario that did not run.
func TestPointRounds_SeriesOnlyMatrix_ReportsNone(t *testing.T) {
	series := []scenarioPlan{seriesPlan(surfaceMeta, 4, testSettings(3))}
	if got := pointRounds(series); got != 0 {
		t.Errorf("pointRounds = %d, want 0 for a series-only matrix", got)
	}
	if got := pointRounds(publishedMatrix(testSettings(3))); got != 3 {
		t.Errorf("pointRounds = %d, want the point scenarios' 3", got)
	}
}

// TestMemoryBudgetMiB_FlagWinsOverTheHost verifies an explicit budget is
// taken verbatim and the default is a share of what the host reports. The
// host's answer is pinned, because the kernel's figure moves by a few kibibytes
// between two reads on a busy machine and a test that read it once itself and
// once through the function under test was comparing two different hosts.
func TestMemoryBudgetMiB_FlagWinsOverTheHost(t *testing.T) {
	restore := hostAvailableMemoryMiB
	hostAvailableMemoryMiB = func() float64 { return 1000 }
	t.Cleanup(func() { hostAvailableMemoryMiB = restore })

	if got := memoryBudgetMiB(2048); got != 2048 {
		t.Errorf("memoryBudgetMiB(2048) = %v, want the flag", got)
	}
	if got, want := memoryBudgetMiB(0), round(1000*defaultBudgetFraction); got != want {
		t.Errorf("memoryBudgetMiB(0) = %v, want %v, eighty percent of what the host reports", got, want)
	}
}

// TestBuildServer_NoTemporaryDirectory_IsReported points the build at a
// temporary directory that does not exist, which is the one failure before
// the compiler runs.
func TestBuildServer_NoTemporaryDirectory_IsReported(t *testing.T) {
	// os.TempDir reads TMPDIR on Unix and TMP, TEMP or USERPROFILE on
	// Windows, so all three point at the absent directory and the build
	// fails before the compiler runs on every platform.
	absent := filepath.Join(t.TempDir(), "absent")
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, absent)
	}
	if _, err := buildServer(t.TempDir()); err == nil || !strings.Contains(err.Error(), "build directory") {
		t.Errorf("buildServer = %v, want the build directory failure", err)
	}
}
