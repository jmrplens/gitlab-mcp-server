// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	runs "github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// recipeCall is one call a fake recipe runner received.
type recipeCall struct {
	dir string
	pkg string
	env []string
}

// fakeRecipe answers for the recipe: each package prints the output and ends
// with the status its entry gives, and a package without one is measured and
// prints nothing. It records every call, calls onRun during each, when it is
// set, and is safe to call from several goroutines at once, as the tree's
// workers do.
type fakeRecipe struct {
	mu      sync.Mutex
	outputs map[string]string
	status  map[string]int
	calls   []recipeCall
	onRun   func()
}

// run is the fake's recipeRunner.
func (f *fakeRecipe) run(_ context.Context, dir, pkg string, env []string, output io.Writer) int {
	f.mu.Lock()
	f.calls = append(f.calls, recipeCall{dir: dir, pkg: pkg, env: env})
	f.mu.Unlock()
	if f.onRun != nil {
		f.onRun()
	}
	_, _ = io.WriteString(output, f.outputs[pkg])
	return f.status[pkg]
}

// platform is the platform a record written by this test binary names.
var platform = runtime.GOOS + "/" + runtime.GOARCH

// readFile returns the content of a file the test expects to exist.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// unlisted is a lister that lists nothing, so no package's conditions can be
// counted and every package is handed to the recipe.
func unlisted(context.Context, string, []string, ...string) ([]byte, error) {
	return nil, errors.New("not listed here")
}

// plantedNothing is the line a record of a planted package with nothing to
// measure holds, the files the go command builds of it named as given.
func plantedNothing(pkg, mode, files string) string {
	return "measure_conditions: nothing to measure in " + mode + " mode: gobco v1.3.4 counts no condition in the files the go command builds of " +
		pkg + " on " + platform + " (" + files + "), so cmd/internal/conditions was not run, and the package is measured with nothing to measure: 0 of 0\n"
}

// TestRunMeasure_PlantedPackages_NothingToMeasureIsMeasured verifies issue
// 1667 on the planted module, in each mode the job's environment can name: a
// package with no condition gobco counts in that mode is never handed to the
// recipe, and is recorded as measured, with the line that says why in place
// of the recipe's output, which streams to stdout as the recipe's would; a
// package with conditions is handed to the recipe as before, so a 0/0 the
// recipe refuses for one of them still fails the job.
func TestRunMeasure_PlantedPackages_NothingToMeasureIsMeasured(t *testing.T) {
	const refusal = "gobco: the report above measured no condition of ./internal/some (Condition coverage: 0/0); refusing to pass it on as a measurement\n"
	cases := []struct {
		mode        string
		env         string
		recipeed    []string
		nothing     map[string]string
		wantVerdict map[string]string
	}{
		{
			mode: "condition", env: "",
			recipeed: []string{"./internal/branchless", "./internal/some"},
			nothing:  map[string]string{"./cmd/none": "none.go", "./internal/testonly": "none but its tests"},
			wantVerdict: map[string]string{
				"./cmd/none": "measured, nothing to measure (no condition, the recipe not run)", "./internal/branchless": "measured (exit 0)",
				"./internal/some": "not measured (exit 1)", "./internal/testonly": "measured, nothing to measure (no condition, the recipe not run)",
			},
		},
		{
			mode: "branch", env: "branch",
			recipeed: []string{"./internal/some"},
			nothing:  map[string]string{"./cmd/none": "none.go", "./internal/branchless": "branchless.go", "./internal/testonly": "none but its tests"},
			wantVerdict: map[string]string{
				"./cmd/none": "measured, nothing to measure (no condition, the recipe not run)", "./internal/branchless": "measured, nothing to measure (no condition, the recipe not run)",
				"./internal/some": "not measured (exit 1)", "./internal/testonly": "measured, nothing to measure (no condition, the recipe not run)",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			t.Setenv("GOBCO_MODE", tc.env)
			recipe := &fakeRecipe{
				outputs: map[string]string{"./internal/branchless": "Condition coverage: 2/2\n", "./internal/some": refusal},
				status:  map[string]int{"./internal/branchless": 0, "./internal/some": 1},
			}
			records := t.TempDir()
			var stdout, stderr bytes.Buffer
			args := []string{"-packages", strings.Join(plantedOrder, " "), "-system", "s", "-records", records, "-dir", plantedModule}
			if got := runMeasure(t.Context(), args, &stdout, &stderr, goList, recipe.run); got != 1 {
				t.Fatalf("runMeasure() = %d, want 1 for the refused 0/0; stderr:\n%s", got, stderr.String())
			}
			called := make([]string, 0, len(recipe.calls))
			for _, call := range recipe.calls {
				called = append(called, call.pkg)
			}
			if !reflect.DeepEqual(called, tc.recipeed) {
				t.Errorf("the recipe ran over %q, want %q", called, tc.recipeed)
			}
			wantStdout, wantStderr := plantedStreams(tc.mode, tc.nothing, recipe.outputs, tc.wantVerdict)
			if got := stdout.String(); got != wantStdout {
				t.Errorf("stdout =\n%s\nwant\n%s", got, wantStdout)
			}
			if got := stderr.String(); got != wantStderr {
				t.Errorf("stderr =\n%s\nwant\n%s", got, wantStderr)
			}
			checkNothingRecords(t, records, tc.mode, tc.nothing)
		})
	}
}

// plantedOrder is the planted module's packages, in the order a job of the
// planted tests names them.
var plantedOrder = []string{"./cmd/none", "./internal/branchless", "./internal/some", "./internal/testonly"}

// plantedStreams is what a job over plantedOrder writes to stdout and stderr
// in mode: the line of each package with nothing to measure, keyed to the
// files it names, or the recipe's output, and each package's verdict, with
// ./internal/some the one package not measured.
func plantedStreams(mode string, nothing, outputs, verdicts map[string]string) (stdout, stderr string) {
	var out, errs strings.Builder
	for _, pkg := range plantedOrder {
		if files, none := nothing[pkg]; none {
			out.WriteString(plantedNothing(pkg, mode, files))
		} else {
			out.WriteString(outputs[pkg])
		}
		errs.WriteString("measure_conditions: " + pkg + " on s: " + verdicts[pkg] + "\n")
	}
	errs.WriteString("measure_conditions: 1 of 4 package(s) could not be measured on s: ./internal/some; the gobco: lines above say why\n")
	return out.String(), errs.String()
}

// checkNothingRecords holds the record of each package with nothing to
// measure to what measurePackage writes for one: a measurement's status, and
// the line that says why in place of the recipe's output.
func checkNothingRecords(t *testing.T, records, mode string, nothing map[string]string) {
	t.Helper()
	for pkg, files := range nothing {
		dir := filepath.Join(records, packageSlug(pkg))
		if got, want := readFile(t, filepath.Join(dir, recordFile)), fmt.Sprintf("package=%s\nsystem=s\nplatform=%s\nstatus=0\n", pkg, platform); got != want {
			t.Errorf("record of %s = %q, want %q", pkg, got, want)
		}
		if got, want := readFile(t, filepath.Join(dir, outputFile)), plantedNothing(pkg, mode, files); got != want {
			t.Errorf("raw output of %s = %q, want %q", pkg, got, want)
		}
	}
}

// TestRunMeasure_ModeTheRecipeRefuses_CountsNothing verifies that a
// GOBCO_MODE the recipe will refuse counts no package's conditions, so even
// a package with none is handed to the recipe, which refuses the mode itself.
func TestRunMeasure_ModeTheRecipeRefuses_CountsNothing(t *testing.T) {
	t.Setenv("GOBCO_MODE", "decision")
	recipe := &fakeRecipe{outputs: map[string]string{"./cmd/none": "gobco: GOBCO_MODE=decision is neither condition nor branch; refusing to measure\n"}, status: map[string]int{"./cmd/none": 2}}
	var stdout, stderr bytes.Buffer
	args := []string{"-packages", "./cmd/none", "-system", "s", "-records", t.TempDir(), "-dir", plantedModule}
	if got := runMeasure(t.Context(), args, &stdout, &stderr, goList, recipe.run); got != 1 || len(recipe.calls) != 1 {
		t.Errorf("runMeasure() = %d after %d recipe call(s), want 1 after one; stderr:\n%s", got, len(recipe.calls), stderr.String())
	}
}

// TestRunMeasure_GateTheRecipeRefuses_CountsNothing verifies that a
// GOBCO_GATE the recipe will refuse counts no package's conditions either,
// so a package with none is handed to the recipe, which refuses the gate
// itself, and the job fails rather than recording the package as measured.
func TestRunMeasure_GateTheRecipeRefuses_CountsNothing(t *testing.T) {
	t.Setenv("GOBCO_MODE", "")
	t.Setenv("GOBCO_GATE", "some")
	recipe := &fakeRecipe{outputs: map[string]string{"./cmd/none": "gobco: GOBCO_GATE=some is neither all nor beyond:GOOS/GOARCH; refusing to measure\n"}, status: map[string]int{"./cmd/none": 2}}
	var stdout, stderr bytes.Buffer
	args := []string{"-packages", "./cmd/none", "-system", "s", "-records", t.TempDir(), "-dir", plantedModule}
	if got := runMeasure(t.Context(), args, &stdout, &stderr, goList, recipe.run); got != 1 || len(recipe.calls) != 1 {
		t.Errorf("runMeasure() = %d after %d recipe call(s), want 1 after one; stderr:\n%s", got, len(recipe.calls), stderr.String())
	}
}

// TestCountingMode_Environment_CountsOnlyWhatTheRecipeReads verifies the
// mode measure counts in from the environment's two settings: the recipe's
// mode when it reads both, its default gate included, and none when it will
// refuse either of them.
func TestCountingMode_Environment_CountsOnlyWhatTheRecipeReads(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, mode, gate, want string
	}{
		{name: "both defaults", want: "condition"},
		{name: "every gate value", mode: "branch", gate: "all", want: "branch"},
		{name: "a platform gate", gate: "beyond:linux/amd64", want: "condition"},
		{name: "a gate the recipe refuses", gate: "some"},
		{name: "a platform gate missing its arch", gate: "beyond:linux"},
		{name: "a mode the recipe refuses", mode: "decision", gate: "all"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := countingMode(tc.mode, tc.gate); got != tc.want {
				t.Errorf("countingMode(%q, %q) = %q, want %q", tc.mode, tc.gate, got, tc.want)
			}
		})
	}
}

// TestRecipeMode_Environment_IsTheModeTheRecipeReads verifies the mode a
// package is counted in: gobco's default for an empty GOBCO_MODE, either mode
// the recipe reads as it is, and none for a value the recipe refuses.
func TestRecipeMode_Environment_IsTheModeTheRecipeReads(t *testing.T) {
	t.Parallel()
	for value, want := range map[string]string{"": "condition", "condition": "condition", "branch": "branch", "Branch": "", "decision": ""} {
		t.Run(strconv.Quote(value), func(t *testing.T) {
			t.Parallel()
			if got := recipeMode(value); got != want {
				t.Errorf("recipeMode(%q) = %q, want %q", value, got, want)
			}
		})
	}
}

// TestMeasurePackage_NothingToMeasure_RecordFailureIsAnError verifies that a
// record of a package with nothing to measure that cannot be written is the
// failure it is, and that the recipe is still not run over the package.
func TestMeasurePackage_NothingToMeasure_RecordFailureIsAnError(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	recipe := &fakeRecipe{}
	m := measurer{run: recipe.run, list: goList, dir: plantedModule, system: "s", records: file, mode: "condition"}
	status, nothing, err := measurePackage(context.Background(), m, "./cmd/none", io.Discard)
	if err == nil || !strings.HasPrefix(err.Error(), "mkdir ") || status != statusMeasured || !nothing || len(recipe.calls) != 0 {
		t.Errorf("measurePackage() = %d, %t, %v after %d recipe call(s); want the mkdir failure and no call", status, nothing, err, len(recipe.calls))
	}
}

// TestRunMeasure_Usage_IsRefusedWithStatus2 verifies every way the command
// line can be wrong: an unknown flag, an argument, each of the three flags
// left empty, and a package name the plan would refuse too.
func TestRunMeasure_Usage_IsRefusedWithStatus2(t *testing.T) {
	t.Parallel()
	full := func(overrides ...string) []string {
		values := map[string]string{"-packages": "./cmd/server", "-system": "s", "-records": "r"}
		for i := 0; i+1 < len(overrides); i += 2 {
			values[overrides[i]] = overrides[i+1]
		}
		return []string{"-packages", values["-packages"], "-system", values["-system"], "-records", values["-records"]}
	}
	const flagsWanted = "measure_conditions: measure takes -packages, -system and -records, all three non-empty, and no arguments\n"
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "an unknown flag", args: []string{"-package", "./a"}, wantStderr: "flag provided but not defined: -package\n"},
		{name: "an argument", args: append(full(), "extra"), wantStderr: flagsWanted},
		{name: "no package", args: full("-packages", " "), wantStderr: flagsWanted},
		{name: "no system", args: full("-system", ""), wantStderr: flagsWanted},
		{name: "no records", args: full("-records", ""), wantStderr: flagsWanted},
		{
			name: "a package outside the module", args: full("-packages", "../x"),
			wantStderr: "measure_conditions: -packages: \"../x\" is not a package directory below the module root, such as ./cmd/server\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			recipe := &fakeRecipe{}
			var stdout, stderr bytes.Buffer
			if got := runMeasure(t.Context(), tc.args, &stdout, &stderr, unlisted, recipe.run); got != 2 {
				t.Fatalf("runMeasure() = %d, want 2", got)
			}
			if stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), tc.wantStderr) || len(recipe.calls) != 0 {
				t.Errorf("stdout = %q, stderr = %q, %d recipe call(s); want nothing, %q and none", stdout.String(), stderr.String(), len(recipe.calls), tc.wantStderr)
			}
		})
	}
}

// TestRunMeasure_Packages_AreMeasuredOneAfterAnother verifies a job's
// measuring end to end: each package is run in the module root -dir names,
// in the order named, with nothing added to the environment the step gives
// it; its output streams to stdout and is kept beside its record, below the
// package's own directory of -records; stderr says what became of each; and
// a package the recipe could not measure fails the job only after every
// other package has been measured, named with the reason's whereabouts.
func TestRunMeasure_Packages_AreMeasuredOneAfterAnother(t *testing.T) {
	t.Parallel()
	recipe := &fakeRecipe{
		outputs: map[string]string{"./cmd/a": "Condition coverage: 1/2\n", "./internal/b": "gobco: refused\n", "./internal/c": "Condition coverage: 3/4\n"},
		status:  map[string]int{"./cmd/a": 0, "./internal/b": 1, "./internal/c": 3},
	}
	records := t.TempDir()
	var stdout, stderr bytes.Buffer
	args := []string{"-packages", "./cmd/a ./internal/b,./internal/c", "-system", "ubuntu-latest", "-records", records, "-dir", "root"}
	if got := runMeasure(t.Context(), args, &stdout, &stderr, unlisted, recipe.run); got != 1 {
		t.Fatalf("runMeasure() = %d, want 1; stderr:\n%s", got, stderr.String())
	}
	wantCalls := []recipeCall{{dir: "root", pkg: "./cmd/a"}, {dir: "root", pkg: "./internal/b"}, {dir: "root", pkg: "./internal/c"}}
	if !reflect.DeepEqual(recipe.calls, wantCalls) {
		t.Errorf("recipe calls = %+v, want %+v", recipe.calls, wantCalls)
	}
	if want := "Condition coverage: 1/2\ngobco: refused\nCondition coverage: 3/4\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	wantStderr := "measure_conditions: ./cmd/a on ubuntu-latest: measured (exit 0)\n" +
		"measure_conditions: ./internal/b on ubuntu-latest: not measured (exit 1)\n" +
		"measure_conditions: ./internal/c on ubuntu-latest: measured (exit 3)\n" +
		"measure_conditions: 1 of 3 package(s) could not be measured on ubuntu-latest: ./internal/b; the gobco: lines above say why\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr =\n%s\nwant\n%s", stderr.String(), wantStderr)
	}
	for _, each := range []struct {
		slug, pkg, output string
		status            int
	}{
		{slug: "cmd-a", pkg: "./cmd/a", output: "Condition coverage: 1/2\n", status: 0},
		{slug: "internal-b", pkg: "./internal/b", output: "gobco: refused\n", status: 1},
		{slug: "internal-c", pkg: "./internal/c", output: "Condition coverage: 3/4\n", status: 3},
	} {
		t.Run(each.pkg, func(t *testing.T) {
			t.Parallel()
			wantRecord := fmt.Sprintf("package=%s\nsystem=ubuntu-latest\nplatform=%s\nstatus=%d\n", each.pkg, platform, each.status)
			if got := readFile(t, filepath.Join(records, each.slug, recordFile)); got != wantRecord {
				t.Errorf("record = %q, want %q", got, wantRecord)
			}
			if got := readFile(t, filepath.Join(records, each.slug, outputFile)); got != each.output {
				t.Errorf("raw output = %q, want %q", got, each.output)
			}
		})
	}
}

// TestRunMeasure_EveryPackageMeasured_Passes verifies that a job whose every
// package was measured passes, whatever its gate held.
func TestRunMeasure_EveryPackageMeasured_Passes(t *testing.T) {
	t.Parallel()
	recipe := &fakeRecipe{outputs: map[string]string{"./a": "Condition coverage: 1/2\n"}, status: map[string]int{"./a": 3}}
	var stdout, stderr bytes.Buffer
	if got := runMeasure(t.Context(), []string{"-packages", "./a", "-system", "s", "-records", t.TempDir()}, &stdout, &stderr, unlisted, recipe.run); got != 0 {
		t.Fatalf("runMeasure() = %d, want 0; stderr:\n%s", got, stderr.String())
	}
	if want := "measure_conditions: ./a on s: measured (exit 3)\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

// TestRunMeasure_Failures_StopTheJobAndSayWhy verifies that a record that
// could not be written in any of its three steps stops the job at that package
// with the reason, so the package after it is never run, and that the package
// is left without a record readRecords could read.
func TestRunMeasure_Failures_StopTheJobAndSayWhy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		prepare func(t *testing.T, records string) string
		want    string
	}{
		{
			name: "the records directory is a file",
			prepare: func(t *testing.T, records string) string {
				t.Helper()
				file := filepath.Join(records, "file")
				if err := os.WriteFile(file, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return file
			},
			want: "measure_conditions: mkdir ",
		},
		{
			name: "the raw output cannot be written",
			prepare: func(t *testing.T, records string) string {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(records, "a", outputFile), 0o750); err != nil {
					t.Fatal(err)
				}
				return records
			},
			want: "measure_conditions: open " + filepath.Join("a", outputFile),
		},
		{
			name: "the record cannot be written",
			prepare: func(t *testing.T, records string) string {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(records, "a", recordFile), 0o750); err != nil {
					t.Fatal(err)
				}
				return records
			},
			want: "measure_conditions: open " + filepath.Join("a", recordFile),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			records := tc.prepare(t, t.TempDir())
			recipe := &fakeRecipe{outputs: map[string]string{"./a": "x\n"}, status: map[string]int{"./a": 0}}
			var stdout, stderr bytes.Buffer
			if got := runMeasure(t.Context(), []string{"-packages", "./a ./later", "-system", "s", "-records", records}, &stdout, &stderr, unlisted, recipe.run); got != 1 {
				t.Fatalf("runMeasure() = %d, want 1", got)
			}
			got := strings.ReplaceAll(stderr.String(), records+string(filepath.Separator), "")
			if !strings.HasPrefix(got, tc.want) || len(recipe.calls) != 1 {
				t.Errorf("stderr = %q after %d recipe call(s), want one call and a line starting %q", got, len(recipe.calls), tc.want)
			}
			if found, _ := readRecords(records); len(found) != 0 {
				t.Errorf("readRecords() = %+v, want no record of a package whose measuring failed", found)
			}
		})
	}
}

// TestMeasurePackage_Record_ReadsBackAsTheSummaryReadsIt holds the writer of
// a record to its reader: what measurePackage leaves is what readRecords
// finds, field for field, with the platform the measuring ran on.
func TestMeasurePackage_Record_ReadsBackAsTheSummaryReadsIt(t *testing.T) {
	t.Parallel()
	recipe := &fakeRecipe{outputs: map[string]string{"./internal/tools/x": "Condition coverage: 5/6\n"}, status: map[string]int{"./internal/tools/x": 3}}
	records := t.TempDir()
	run := measurer{run: recipe.run, list: unlisted, dir: ".", system: "macos-latest", records: records, env: []string{"GOBCO_GATE=all"}, mode: "condition"}
	status, nothing, err := measurePackage(context.Background(), run, "./internal/tools/x", io.Discard)
	if err != nil || status != 3 || nothing {
		t.Fatalf("measurePackage() = %d, %t, %v; want 3, something measured", status, nothing, err)
	}
	got, problems := readRecords(records)
	want := []record{{pkg: "./internal/tools/x", system: "macos-latest", platform: platform, status: 3, output: "Condition coverage: 5/6\n"}}
	if len(problems) != 0 || !reflect.DeepEqual(got, want) {
		t.Errorf("readRecords() = %+v, %q; want %+v", got, problems, want)
	}
	if wantCalls := []recipeCall{{dir: ".", pkg: "./internal/tools/x", env: []string{"GOBCO_GATE=all"}}}; !reflect.DeepEqual(recipe.calls, wantCalls) {
		t.Errorf("recipe calls = %+v, want %+v", recipe.calls, wantCalls)
	}
}

// TestRunRecipe_ThePackage_IsHandedToTheRecipeInThisProcess runs the
// production runner as far as the recipe refuses what it is given, which needs
// neither gobco nor the network: the environment added reaches the recipe, the
// package is named from the directory given, everything it says reaches the
// writer, and its status comes back as it is.
func TestRunRecipe_ThePackage_IsHandedToTheRecipeInThisProcess(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		env  []string
		want int
		says string
	}{
		{"a mode it refuses", []string{"GOBCO_MODE=decision"}, 2, "gobco: GOBCO_MODE=decision is neither condition nor branch; refusing to measure\n"},
		{"a package that is not there", []string{"GOBCO_MODE=condition", "GOBCO_GATE="}, 1, "gobco: go cannot load ./absent under build tags (none): "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			status := runRecipe(t.Context(), plantedModule, "./absent", tc.env, &output)
			if status != tc.want || !strings.HasPrefix(output.String(), tc.says) {
				t.Errorf("runRecipe() = %d, output %q; want %d and %q", status, output.String(), tc.want, tc.says)
			}
		})
	}
}

// TestRunMeasure_AnInterrupt_EndsTheJobAndLeavesNoRecord verifies that a
// signal while a package is measured ends the job with the status a shell
// gives it, measures no other package, says nothing of the one it ended in,
// and leaves no record of it: what it printed is no reason the package could
// not be measured.
func TestRunMeasure_AnInterrupt_EndsTheJobAndLeavesNoRecord(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancelCause(t.Context())
	recipe := &fakeRecipe{status: map[string]int{"./a": 130}, onRun: func() { cancel(runs.InterruptedError{Signal: os.Interrupt}) }}
	records := t.TempDir()
	var stdout, stderr bytes.Buffer
	if got := runMeasure(ctx, []string{"-packages", "./a ./later", "-system", "s", "-records", records}, &stdout, &stderr, unlisted, recipe.run); got != 130 {
		t.Errorf("runMeasure() = %d, want 130", got)
	}
	if len(recipe.calls) != 1 || stderr.Len() != 0 {
		t.Errorf("%d recipe call(s), stderr = %q; want one and nothing said", len(recipe.calls), stderr.String())
	}
	if found, problems := readRecords(records); len(found) != 0 || len(problems) != 0 {
		t.Errorf("readRecords() = %+v, %q; want no record", found, problems)
	}
}
