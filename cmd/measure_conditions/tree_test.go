package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// treeListing is a listing of the three packages the tree tests measure.
func treeListing() string {
	const module = "example.com/m"
	return row(module+"/cmd/a", module, "a.go", "", "", "") +
		row(module+"/internal/b", module, "b.go", "", "", "") +
		row(module+"/internal/c", module, "c.go", "", "", "")
}

// treeScript answers for the three packages: one measured, one whose gate
// held its one condition, and one the script could not measure.
func treeScript() *fakeScript {
	return &fakeScript{
		outputs: map[string]string{
			"./cmd/a":      "Condition coverage: 9/10\n",
			"./internal/b": "Condition coverage: 3/4\n" + `b.go:7:2: condition "y" was never evaluated` + "\n" + "gobco: held:\n" + `  b.go:7:2: condition "y" was never evaluated` + "\n",
			"./internal/c": "gobco: gobco exited 1 on ./internal/c\n",
		},
		status: map[string]int{"./cmd/a": 0, "./internal/b": 3, "./internal/c": 1},
	}
}

// TestRunTree_Usage_IsRefusedWithStatus2 verifies every way the command line
// can be wrong, before anything is listed: an unknown flag, an argument, no
// records, no commit, a -parallel below one, and a gate or a mode the script
// would refuse.
func TestRunTree_Usage_IsRefusedWithStatus2(t *testing.T) {
	t.Parallel()
	const flagsWanted = "measure_conditions: tree takes -records and -commit, both non-empty, a -parallel of at least 1, and no arguments\n"
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "an unknown flag", args: []string{"-record", "r"}, wantStderr: "flag provided but not defined: -record\n"},
		{name: "an argument", args: []string{"-records", "r", "-commit", "c", "extra"}, wantStderr: flagsWanted},
		{name: "no records", args: []string{"-commit", "c"}, wantStderr: flagsWanted},
		{name: "no commit", args: []string{"-records", "r"}, wantStderr: flagsWanted},
		{name: "no parallelism", args: []string{"-records", "r", "-commit", "c", "-parallel", "0"}, wantStderr: flagsWanted},
		{
			name: "a gate the script would refuse", args: []string{"-records", "r", "-commit", "c", "-gate", "some"},
			wantStderr: "measure_conditions: -gate \"some\" is neither all nor beyond:GOOS/GOARCH, which is what scripts/coverage-conditions.sh reads\n",
		},
		{
			name: "a mode the script would refuse", args: []string{"-records", "r", "-commit", "c", "-mode", "Branch"},
			wantStderr: "measure_conditions: -mode \"Branch\" is neither condition nor branch, which is what scripts/coverage-conditions.sh reads\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls []listCall
			script := &fakeScript{}
			var stdout, stderr bytes.Buffer
			if got := runTree(tc.args, &stdout, &stderr, fakeLister(nil, "", &calls), script.run); got != 2 {
				t.Fatalf("runTree() = %d, want 2", got)
			}
			if stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), tc.wantStderr) || len(calls)+len(script.calls) != 0 {
				t.Errorf("stdout = %q, stderr = %q, %d listing(s) and %d script call(s); want nothing, %q and none", stdout.String(), stderr.String(), len(calls), len(script.calls), tc.wantStderr)
			}
		})
	}
}

// TestRunTree_Listing_FailuresStopBeforeMeasuring verifies that a tree that
// could not be listed, and one with no package below cmd/ and internal/,
// measure nothing and fail with the reason.
func TestRunTree_Listing_FailuresStopBeforeMeasuring(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		list       lister
		wantStderr string
	}{
		{
			name: "the listing failed", list: treeLister("", errors.New("planted failure")),
			wantStderr: "measure_conditions: the packages below cmd/ and internal/ could not be listed: go list: planted failure\n",
		},
		{
			name: "nothing below cmd/ and internal/", list: treeLister("", nil),
			wantStderr: "measure_conditions: there is no package below cmd/ and internal/, so there is nothing to measure\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			script := &fakeScript{}
			var stdout, stderr bytes.Buffer
			if got := runTree([]string{"-records", t.TempDir(), "-commit", "c"}, &stdout, &stderr, tc.list, script.run); got != 1 {
				t.Fatalf("runTree() = %d, want 1", got)
			}
			if stdout.Len() != 0 || stderr.String() != tc.wantStderr || len(script.calls) != 0 {
				t.Errorf("stdout = %q, stderr = %q after %d script call(s); want nothing, %q and none", stdout.String(), stderr.String(), len(script.calls), tc.wantStderr)
			}
		})
	}
}

// TestRunTree_EveryPackage_IsMeasuredAndSummarized verifies the whole-tree
// run end to end, two packages at a time: every package listed is measured
// once, in the module root, with the gate in its environment; the summary is
// the workflow's, a row per package in the order listed, each record below
// its own directory of -records, and the total of the packages measured,
// with the one the script could not measure named and left out of it; and
// stderr says what became of each package. A package gobco could not
// measure is the summary's to report, not a failure of the run.
func TestRunTree_EveryPackage_IsMeasuredAndSummarized(t *testing.T) {
	t.Parallel()
	script := treeScript()
	records := t.TempDir()
	var stdout, stderr bytes.Buffer
	args := []string{"-records", records, "-commit", "c0ffee", "-gate", "beyond:linux/amd64", "-parallel", "2", "-dir", "root"}
	if got := runTree(args, &stdout, &stderr, treeLister(treeListing(), nil), script.run); got != 0 {
		t.Fatalf("runTree() = %d, want 0; stderr:\n%s", got, stderr.String())
	}
	slices.SortFunc(script.calls, func(a, b scriptCall) int { return strings.Compare(a.pkg, b.pkg) })
	env := []string{"GOBCO_GATE=beyond:linux/amd64", "GOBCO_MODE=condition"}
	wantCalls := []scriptCall{{dir: "root", pkg: "./cmd/a", env: env}, {dir: "root", pkg: "./internal/b", env: env}, {dir: "root", pkg: "./internal/c", env: env}}
	if !reflect.DeepEqual(script.calls, wantCalls) {
		t.Errorf("script calls = %+v, want %+v", script.calls, wantCalls)
	}
	local := "`local` (" + platform + ")"
	want := summaryIntro("beyond:linux/amd64", "c0ffee") +
		"| `./cmd/a` | " + local + " | 9 of 10 | 0 | 0 | `cmd-a` |\n" +
		"| `./internal/b` | " + local + " | 3 of 4 | 1 | 1 | `internal-b` |\n" +
		"| `./internal/c` | " + local + " | not measured (exit 1) | - | - | `internal-c` |\n" +
		totalsTable("| `local` | 2 of 3 | 12 of 14 | 85.7% |") +
		"\n### Not evaluated both ways\n\n" +
		"| Package | System | File | Line | Condition | gobco says | Held |\n" +
		"| --- | --- | --- | ---: | --- | --- | --- |\n" +
		"| `./internal/b` | `local` | `b.go` | 7 | `y` | never evaluated | yes |\n" +
		"\n### Not measured\n" +
		"\n#### `./internal/c` on `local`\n\n" +
		"`scripts/coverage-conditions.sh` exited 1, which means it could not measure, and said:\n\n" +
		"```text\ngobco: gobco exited 1 on ./internal/c\n```\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
	lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
	wantFirst := "measure_conditions: measuring the 3 packages below cmd/ and internal/, 2 at a time, with GOBCO_GATE=beyond:linux/amd64 and GOBCO_MODE=condition"
	if len(lines) != 4 || lines[0] != wantFirst {
		t.Fatalf("stderr lines = %q, want %q and one line per package", lines, wantFirst)
	}
	progress := slices.Sorted(slices.Values(lines[1:]))
	wantProgress := []string{
		"measure_conditions: ./cmd/a: measured (exit 0)",
		"measure_conditions: ./internal/b: measured (exit 3)",
		"measure_conditions: ./internal/c: not measured (exit 1)",
	}
	if !reflect.DeepEqual(progress, wantProgress) {
		t.Errorf("progress lines = %q, want %q in any order", progress, wantProgress)
	}
}

// TestRunTree_BranchMode_CountsBranchesThroughTheScript verifies the tree in
// gobco's branch mode: the mode reaches every run of the script beside the
// gate, and the summary is of branch coverage, read from the figure gobco
// writes in that mode, so a package whose report carries only a condition
// figure is a record the summary could not use and fails the run.
func TestRunTree_BranchMode_CountsBranchesThroughTheScript(t *testing.T) {
	t.Parallel()
	script := &fakeScript{
		outputs: map[string]string{
			"./cmd/a": "Branch coverage: 4/4\n",
			"./internal/b": "Branch coverage: 5/6\n" + `b.go:3:4: condition "x && y" was once true but never false` + "\n" +
				"gobco: held:\n" + `  b.go:3:4: condition "x && y" was once true but never false` + "\n",
			"./internal/c": "Condition coverage: 9/10\n",
		},
		status: map[string]int{"./cmd/a": 0, "./internal/b": 3, "./internal/c": 0},
	}
	var stdout, stderr bytes.Buffer
	args := []string{"-records", t.TempDir(), "-commit", "c0ffee", "-mode", "branch"}
	if got := runTree(args, &stdout, &stderr, treeLister(treeListing(), nil), script.run); got != 1 {
		t.Fatalf("runTree() = %d, want 1; stderr:\n%s", got, stderr.String())
	}
	for _, call := range script.calls {
		if want := []string{"GOBCO_GATE=all", "GOBCO_MODE=branch"}; !reflect.DeepEqual(call.env, want) {
			t.Errorf("the script ran over %s with %q, want %q", call.pkg, call.env, want)
		}
	}
	local := "`local` (" + platform + ")"
	const problem = "the record of ./internal/c on local cannot be read: the script's status says it measured, and its output carries no Branch coverage figure"
	want := "## Branch coverage, measured\n\n" +
		"`GOBCO_GATE=all` at `c0ffee`. Every figure is the one gobco printed on the runner named, through `scripts/coverage-conditions.sh`, the recipe behind `make coverage-conditions`; none is inferred from another platform. gobco counts each decision twice, once per outcome: the whole condition of an `if` or a `for`, and each case of a `switch`, so a total is twice the number of decisions. No figure fails this workflow: it is a measurement, not a gate.\n\n" +
		"| Package | System | Covered | Not both ways | Held by the gate | Raw output |\n" +
		"| --- | --- | ---: | ---: | ---: | --- |\n" +
		"| `./cmd/a` | " + local + " | 4 of 4 | 0 | 0 | `cmd-a` |\n" +
		"| `./internal/b` | " + local + " | 5 of 6 | 1 | 1 | `internal-b` |\n" +
		"| `./internal/c` | " + local + " | unreadable record | - | - | `internal-c` |\n" +
		totalsTable("| `local` | 2 of 3 | 9 of 10 | 90.0% |") +
		"\n### Not evaluated both ways\n\n" +
		"| Package | System | File | Line | Condition | gobco says | Held |\n" +
		"| --- | --- | --- | ---: | --- | --- | --- |\n" +
		"| `./internal/b` | `local` | `b.go` | 3 | `x && y` | once true but never false | yes |\n" +
		"\n### Records this summary could not use\n\n" +
		"- " + problem + "\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
	if !strings.HasPrefix(stderr.String(), "measure_conditions: measuring the 3 packages below cmd/ and internal/, 1 at a time, with GOBCO_GATE=all and GOBCO_MODE=branch\n") ||
		!strings.HasSuffix(stderr.String(), "measure_conditions: "+problem+"\n") {
		t.Errorf("stderr =\n%s\nwant the mode named first and the unusable record last", stderr.String())
	}
}

// TestRunTree_StaleRecord_IsAProblemOfTheSummary verifies that a record an
// earlier run left for a package the tree no longer holds is not counted: the
// summary names it as a record the plan did not ask for, and the run fails,
// while every package of the tree is still measured and its own earlier
// record replaced.
func TestRunTree_StaleRecord_IsAProblemOfTheSummary(t *testing.T) {
	t.Parallel()
	records := t.TempDir()
	writeRun(t, filepath.Join(records, "cmd-gone"), recordFor("./cmd/gone", localSystem, "linux/amd64", 0), "Condition coverage: 1/2\n")
	writeRun(t, filepath.Join(records, "cmd-a"), recordFor("./cmd/a", localSystem, "linux/amd64", 1), "an earlier run\n")
	var stdout, stderr bytes.Buffer
	if got := runTree([]string{"-records", records, "-commit", "c"}, &stdout, &stderr, treeLister(treeListing(), nil), treeScript().run); got != 1 {
		t.Fatalf("runTree() = %d, want 1", got)
	}
	const problem = "a record names ./cmd/gone on local, which the plan did not ask for"
	if !strings.Contains(stdout.String(), "- "+problem+"\n") || !strings.HasSuffix(stderr.String(), "measure_conditions: "+problem+"\n") {
		t.Errorf("stdout =\n%s\nstderr =\n%s\nwant the stale record named in both", stdout.String(), stderr.String())
	}
	if got := readFile(t, filepath.Join(records, "cmd-a", outputFile)); got != "Condition coverage: 9/10\n" {
		t.Errorf("the record of ./cmd/a holds %q, want this run's output", got)
	}
}

// TestRunTree_UnwritableRecords_FailWithEveryPackagesReason verifies that a
// -records the records cannot be written below fails the run with the
// reason of each package, after all of them were run, and writes no summary.
func TestRunTree_UnwritableRecords_FailWithEveryPackagesReason(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	script := treeScript()
	var stdout, stderr bytes.Buffer
	if got := runTree([]string{"-records", file, "-commit", "c"}, &stdout, &stderr, treeLister(treeListing(), nil), script.run); got != 1 {
		t.Fatalf("runTree() = %d, want 1", got)
	}
	if stdout.Len() != 0 || len(script.calls) != 3 || strings.Count(stderr.String(), "mkdir "+file) != 3 {
		t.Errorf("stdout = %q, %d script call(s), stderr =\n%s\nwant no summary, three calls and three mkdir failures", stdout.String(), len(script.calls), stderr.String())
	}
}
