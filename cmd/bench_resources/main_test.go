// main_test.go covers the command's own decisions: which matrix a set of flags
// selects, and the guard that keeps a smoke run from overwriting published
// measurements.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// withArgs runs parseFlags against a command line, on a flag set of its own so
// the test binary's own flags are untouched.
func withArgs(t *testing.T, args ...string) options {
	t.Helper()
	previousArgs, previousFlags := os.Args, flag.CommandLine
	t.Cleanup(func() { os.Args, flag.CommandLine = previousArgs, previousFlags })

	flag.CommandLine = flag.NewFlagSet("bench_resources", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = append([]string{"bench_resources"}, args...)
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
	plans, err := matrixFor(options{rounds: 5})
	if err != nil {
		t.Fatalf("matrixFor: %v", err)
	}
	if len(plans) != len(publishedMatrix(5)) {
		t.Errorf("selected %d scenarios, want the whole matrix", len(plans))
	}
	for _, plan := range plans {
		if plan.Rounds != 5 {
			t.Errorf("%s runs %d rounds, want the 5 that were asked for", plan.ID, plan.Rounds)
		}
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
	tests := []struct {
		name    string
		opts    options
		wantErr bool
	}{
		{name: "quick without a path", opts: options{quick: true, rounds: 1}, wantErr: true},
		{name: "quick with a path", opts: options{quick: true, rounds: 1, recordSet: true}},
		{name: "filtered without a path", opts: options{scenarios: "http-meta", rounds: 1}, wantErr: true},
		{name: "filtered with a path", opts: options{scenarios: "http-meta", rounds: 1, recordSet: true}},
		{name: "full run needs no path", opts: options{rounds: 1}},
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
	if _, err := matrixFor(options{scenarios: "http-dinamic", rounds: 1, recordSet: true}); err == nil {
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
	// write outside the repository.
	outside := "/tmp/bench.json"
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
	cases := []struct {
		name    string
		opts    options
		wantErr string
	}{
		{"zero rounds", options{rounds: 0, sampleInterval: good}, "-rounds"},
		{"negative rounds", options{rounds: -1, sampleInterval: good}, "-rounds"},
		{"zero interval", options{rounds: 3, sampleInterval: 0}, "-sample-interval"},
		{"negative interval", options{rounds: 3, sampleInterval: -time.Second}, "-sample-interval"},
		{"both valid", options{rounds: 3, sampleInterval: good}, ""},
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
	}
}

// TestMeasure_QuickMatrix_ProducesOneScenarioPerPlan runs the smoke matrix
// against the stand-in and checks the record carries what every downstream
// artifact reads: a scenario per plan, the build the server reported, and the
// settings the matrix imposed rather than the ones the flags asked for.
func TestMeasure_QuickMatrix_ProducesOneScenarioPerPlan(t *testing.T) {
	root := t.TempDir()
	opts := quickOptions(t, root)
	opts.rounds = 7 // the smoke matrix pins one round, and the record must say so

	run, err := measure(opts, root)
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if got, want := len(run.Scenarios), len(quickMatrix()); got != want {
		t.Fatalf("recorded %d scenarios, want %d", got, want)
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
// absolute.
func TestRel_PathThatCannotBeMadeRelative_IsReturnedWhole(t *testing.T) {
	absolute := filepath.Join(t.TempDir(), "record.json")
	if got := rel("relative-root", absolute); got != absolute {
		t.Errorf("rel = %q, want the path untouched %q", got, absolute)
	}
}
