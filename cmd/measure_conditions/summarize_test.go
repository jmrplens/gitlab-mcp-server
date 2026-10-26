package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// planOf writes the matrix plan for the packages crossed with the systems,
// as plan prints it.
func planOf(t *testing.T, packages, systems []string) string {
	t.Helper()
	planned, err := buildMatrix(packages, systems)
	if err != nil {
		t.Fatal(err)
	}
	return encodePlan(t, planned)
}

// recordFor is a record as the workflow writes it, for a platform of the
// test's choosing.
func recordFor(pkg, system, platform string, status int) string {
	return fmt.Sprintf("package=%s\nsystem=%s\nplatform=%s\nstatus=%d\n", pkg, system, platform, status)
}

// TestRunSummarize_Usage_IsRefusedWithStatus2 verifies every way the command
// line can be wrong: an unknown flag, an argument, each of the four flags
// left empty, a plan that is not JSON, a plan with no run in it, and a mode
// the script would refuse.
func TestRunSummarize_Usage_IsRefusedWithStatus2(t *testing.T) {
	t.Parallel()
	plan := planOf(t, []string{"./cmd/server"}, []string{"macos-latest"})
	full := func(overrides ...string) []string {
		values := map[string]string{"-plan": plan, "-records": "r", "-gate": "all", "-commit": "c"}
		for i := 0; i+1 < len(overrides); i += 2 {
			values[overrides[i]] = overrides[i+1]
		}
		return []string{"-plan", values["-plan"], "-records", values["-records"], "-gate", values["-gate"], "-commit", values["-commit"]}
	}
	const flagsWanted = "measure_conditions: summarize takes -plan, -records, -gate and -commit, all four non-empty, and no arguments\n"
	const planWanted = "measure_conditions: -plan is not a matrix plan with at least one run in it\n"
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "an unknown flag", args: []string{"-plans", plan}, wantStderr: "flag provided but not defined: -plans\n"},
		{name: "an argument", args: append(full(), "extra"), wantStderr: flagsWanted},
		{name: "no plan", args: full("-plan", ""), wantStderr: flagsWanted},
		{name: "no records", args: full("-records", ""), wantStderr: flagsWanted},
		{name: "no gate", args: full("-gate", ""), wantStderr: flagsWanted},
		{name: "no commit", args: full("-commit", ""), wantStderr: flagsWanted},
		{name: "a plan that is not JSON", args: full("-plan", "matrix="+plan), wantStderr: planWanted},
		{name: "a plan with no run", args: full("-plan", `{"include":[]}`), wantStderr: planWanted},
		{
			name: "a mode the script would refuse", args: append(full(), "-mode", "both"),
			wantStderr: "measure_conditions: -mode \"both\" is neither condition nor branch, which is what scripts/coverage-conditions.sh reads\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if got := runSummarize(tc.args, &stdout, &stderr); got != 2 {
				t.Fatalf("runSummarize() = %d, want 2", got)
			}
			if stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), tc.wantStderr) {
				t.Errorf("stdout = %q, stderr = %q; want nothing and %q", stdout.String(), stderr.String(), tc.wantStderr)
			}
		})
	}
}

// TestRunSummarize_EveryRunMeasured_WritesTheTableAndPasses verifies the run
// a dispatch hopes for: every run measured, nothing held and nothing named,
// so the summary is the figures alone and the status is 0.
func TestRunSummarize_EveryRunMeasured_WritesTheTableAndPasses(t *testing.T) {
	t.Parallel()
	records := t.TempDir()
	writeRun(t, records, recordFor("./cmd/server", "macos-latest", "darwin/arm64", 0), "Condition coverage: 12/12\n")
	plan := planOf(t, []string{"./cmd/server"}, []string{"macos-latest"})
	var stdout, stderr bytes.Buffer
	if got := runSummarize([]string{"-plan", plan, "-records", records, "-gate", "all", "-commit", "c0ffee"}, &stdout, &stderr); got != 0 {
		t.Fatalf("runSummarize() = %d, want 0; stderr:\n%s", got, stderr.String())
	}
	want := summaryIntro("all", "c0ffee") +
		"| `./cmd/server` | `macos-latest` (darwin/arm64) | 12 of 12 | 0 | 0 | `gobco-macos-latest-cmd-server/cmd-server` |\n" +
		totalsTable("| `macos-latest` | 1 of 1 | 12 of 12 | 100.0% |")
	if stdout.String() != want || stderr.Len() != 0 {
		t.Errorf("stdout =\n%s\nwant\n%s\nstderr = %q", stdout.String(), want, stderr.String())
	}
}

// TestRunSummarize_BranchMode_ReadsTheBranchFigures verifies a dispatch that
// asked gobco for branch coverage: the summary says so in its heading and in
// what it says the figures count, and reads each record's figure in the words
// gobco writes it in that mode.
func TestRunSummarize_BranchMode_ReadsTheBranchFigures(t *testing.T) {
	t.Parallel()
	records := t.TempDir()
	writeRun(t, records, recordFor("./cmd/server", "ubuntu-latest", "linux/amd64", 0), "Condition coverage: 9/9\nBranch coverage: 7/8\n")
	plan := planOf(t, []string{"./cmd/server"}, []string{"ubuntu-latest"})
	var stdout, stderr bytes.Buffer
	if got := runSummarize([]string{"-plan", plan, "-records", records, "-gate", "all", "-mode", "branch", "-commit", "c0ffee"}, &stdout, &stderr); got != 0 {
		t.Fatalf("runSummarize() = %d, want 0; stderr:\n%s", got, stderr.String())
	}
	want := "## Branch coverage, measured\n\n" +
		"`GOBCO_GATE=all` at `c0ffee`. Every figure is the one gobco printed on the runner named, through `scripts/coverage-conditions.sh`, the recipe behind `make coverage-conditions`; none is inferred from another platform. gobco counts each decision twice, once per outcome: the whole condition of an `if` or a `for`, and each case of a `switch`, so a total is twice the number of decisions. No figure fails this workflow: it is a measurement, not a gate.\n\n" +
		"| Package | System | Covered | Not both ways | Held by the gate | Raw output |\n" +
		"| --- | --- | ---: | ---: | ---: | --- |\n" +
		"| `./cmd/server` | `ubuntu-latest` (linux/amd64) | 7 of 8 | 0 | 0 | `gobco-ubuntu-latest-cmd-server/cmd-server` |\n" +
		totalsTable("| `ubuntu-latest` | 1 of 1 | 7 of 8 | 87.5% |")
	if stdout.String() != want || stderr.Len() != 0 {
		t.Errorf("stdout =\n%s\nwant\n%s\nstderr = %q", stdout.String(), want, stderr.String())
	}
}

// summaryIntro is the heading, the paragraph and the table head every
// summary of condition coverage opens with.
func summaryIntro(gate, commit string) string {
	return "## Condition coverage, measured\n\n" +
		"`GOBCO_GATE=" + gate + "` at `" + commit + "`. Every figure is the one gobco printed on the runner named, through `scripts/coverage-conditions.sh`, the recipe behind `make coverage-conditions`; none is inferred from another platform. gobco counts each condition twice, once per outcome, so a total is twice the number of conditions. No figure fails this workflow: it is a measurement, not a gate.\n\n" +
		"| Package | System | Covered | Not both ways | Held by the gate | Raw output |\n" +
		"| --- | --- | ---: | ---: | ---: | --- |\n"
}

// totalsTable is the totals section every summary carries after its table,
// with the rows given.
func totalsTable(rows ...string) string {
	return "\n### Total\n\n" +
		"A package that was not measured is in no total: the table above says which and why, and each share is of the packages its system measured.\n\n" +
		"| System | Packages measured | Covered | Share |\n" +
		"| --- | ---: | ---: | ---: |\n" +
		strings.Join(rows, "\n") + "\n"
}

// TestRunSummarize_ShardedPlan_ReadsEveryPackageOfEveryJob verifies the
// summary of a -tree plan: every package of every shard is a row of its own,
// read from its directory inside its shard's artifact, the total of a system
// adds up the packages it measured and leaves out the one it did not, and a
// package the plan names with no record is reported missing under its own
// record path.
func TestRunSummarize_ShardedPlan_ReadsEveryPackageOfEveryJob(t *testing.T) {
	t.Parallel()
	records := t.TempDir()
	writeRun(t, filepath.Join(records, "gobco-ubuntu-latest-tree-01", "cmd-a"), recordFor("./cmd/a", "ubuntu-latest", "linux/amd64", 0), "Condition coverage: 3/4\n")
	writeRun(t, filepath.Join(records, "gobco-ubuntu-latest-tree-01", "internal-c"), recordFor("./internal/c", "ubuntu-latest", "linux/amd64", 1), "gobco: could not instrument\n")
	writeRun(t, filepath.Join(records, "gobco-ubuntu-latest-tree-02", "internal-b"), recordFor("./internal/b", "ubuntu-latest", "linux/amd64", 3),
		"Condition coverage: 5/6\n"+`b.go:1:2: condition "x" was never evaluated`+"\n"+"gobco: held:\n"+`  b.go:1:2: condition "x" was never evaluated`+"\n")
	plan := encodePlan(t, buildShards([]string{"./cmd/a", "./internal/b", "./internal/c", "./internal/d"}, []string{"ubuntu-latest"}, 2))
	var stdout, stderr bytes.Buffer
	if got := runSummarize([]string{"-plan", plan, "-records", records, "-gate", "all", "-commit", "c0ffee"}, &stdout, &stderr); got != 0 {
		t.Fatalf("runSummarize() = %d, want 0; stderr:\n%s", got, stderr.String())
	}
	want := summaryIntro("all", "c0ffee") +
		"| `./cmd/a` | `ubuntu-latest` (linux/amd64) | 3 of 4 | 0 | 0 | `gobco-ubuntu-latest-tree-01/cmd-a` |\n" +
		"| `./internal/c` | `ubuntu-latest` (linux/amd64) | not measured (exit 1) | - | - | `gobco-ubuntu-latest-tree-01/internal-c` |\n" +
		"| `./internal/b` | `ubuntu-latest` (linux/amd64) | 5 of 6 | 1 | 1 | `gobco-ubuntu-latest-tree-02/internal-b` |\n" +
		"| `./internal/d` | `ubuntu-latest` | no record | - | - | `gobco-ubuntu-latest-tree-02/internal-d` |\n" +
		totalsTable("| `ubuntu-latest` | 2 of 4 | 8 of 10 | 80.0% |") +
		"\n### Not evaluated both ways\n\n" +
		"| Package | System | File | Line | Condition | gobco says | Held |\n" +
		"| --- | --- | --- | ---: | --- | --- | --- |\n" +
		"| `./internal/b` | `ubuntu-latest` | `b.go` | 1 | `x` | never evaluated | yes |\n" +
		"\n### Not measured\n" +
		"\n#### `./internal/c` on `ubuntu-latest`\n\n" +
		"`scripts/coverage-conditions.sh` exited 1, which means it could not measure, and said:\n\n" +
		"```text\ngobco: could not instrument\n```\n" +
		"\n#### `./internal/d` on `ubuntu-latest`\n\n" +
		"No record came back in `gobco-ubuntu-latest-tree-02/internal-d`: the run ended before its measuring step wrote one (a setup step failed, or the run was cancelled, timed out or lost its runner), or it did not upload the one it wrote, and its job log says which.\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Errorf("stdout =\n%s\nwant\n%s\nstderr = %q", stdout.String(), want, stderr.String())
	}
}

// encodePlan writes a plan as the plan subcommand prints it after matrix=.
func encodePlan(t *testing.T, planned matrix) string {
	t.Helper()
	data, err := json.Marshal(planned)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestShare_Figures_AreRoundedDownToATenth verifies that a share never reads
// higher than it is: 799 of 1000 is 79.9% and not 80.0%, two thirds is 66.6%,
// a whole is 100.0%, nothing covered of something is 0.0%, and nothing
// counted is a dash rather than a division by zero.
func TestShare_Figures_AreRoundedDownToATenth(t *testing.T) {
	t.Parallel()
	cases := []struct {
		covered, outcomes int
		want              string
	}{
		{covered: 799, outcomes: 1000, want: "79.9%"},
		{covered: 2, outcomes: 3, want: "66.6%"},
		{covered: 5, outcomes: 5, want: "100.0%"},
		{covered: 0, outcomes: 4, want: "0.0%"},
		{covered: 0, outcomes: 0, want: "-"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			if got := share(tc.covered, tc.outcomes); got != tc.want {
				t.Errorf("share(%d, %d) = %q, want %q", tc.covered, tc.outcomes, got, tc.want)
			}
		})
	}
}

// TestRunSummarize_EveryOutcome_IsReportedAndProblemsFailIt drives the summary
// through everything a run can leave: a measurement whose gate held one of
// the three conditions it names, one of them written across lines, a clean
// measurement, a run that could not measure and said why, one that said
// nothing of its own, no record at all, a record whose status and output
// disagree, a second record for one run, a record the plan did not ask for
// and a record that cannot be parsed. The table, the conditions, each on one
// row of one line, the reasons and the problems are all written, and the
// problems make the status 1.
func TestRunSummarize_EveryOutcome_IsReportedAndProblemsFailIt(t *testing.T) {
	t.Parallel()
	records := t.TempDir()
	windows := "Condition coverage: 8/12\n" +
		`conn_refused_windows.go:12:9: condition "errors.Is(err, x)" was 2 times true but never false` + "\n" +
		`main.go:40:3: condition "a || b" was never evaluated` + "\n" +
		`main.go:52:5: condition "strings.Contains(s,\n\t\t\"x\t\")" was once true but never false` + "\r\n" +
		"gobco: GOBCO_GATE=beyond:linux/amd64: 1 condition(s) in the files were not evaluated both ways:\n" +
		`  conn_refused_windows.go:12:9: condition "errors.Is(err, x)" was 2 times true but never false` + "\n"
	writeRun(t, filepath.Join(records, "a-win"), recordFor("./cmd/server", "windows-latest", "windows/amd64", 3), windows)
	writeRun(t, filepath.Join(records, "b-mac"), recordFor("./cmd/server", "macos-latest", "darwin/arm64", 0), "Condition coverage: 12/12\n")
	writeRun(t, filepath.Join(records, "b-mac-dup"), recordFor("./cmd/server", "macos-latest", "darwin/arm64", 0), "Condition coverage: 12/12\n")
	writeRun(t, filepath.Join(records, "c-ubu"), recordFor("./cmd/server", "ubuntu-latest", "linux/amd64", 1),
		"ok  \texample.com/m\n--- FAIL: TestX\ngobco: a line quoting ``` three backticks\r\ngobco: gobco exited 1 on ./cmd/server\n")
	writeRun(t, filepath.Join(records, "e-tu-mac"), recordFor("./internal/toolutil", "macos-latest", "darwin/arm64", 0), "no figure here\n")
	var lines, tail []string
	for i := 1; i <= 25; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
		if i > 25-reasonTail {
			tail = append(tail, fmt.Sprintf("line %d", i))
		}
	}
	writeRun(t, filepath.Join(records, "f-tu-ubu"), recordFor("./internal/toolutil", "ubuntu-latest", "linux/amd64", 2), strings.Join(lines, "\n")+"\n")
	writeRun(t, filepath.Join(records, "x-unplanned"), recordFor("./x", "macos-latest", "darwin/arm64", 0), "Condition coverage: 1/2\n")
	writeRun(t, filepath.Join(records, "zz-broken"), "nonsense\n", "")

	plan := planOf(t, []string{"./cmd/server", "./internal/toolutil"}, runners)
	var stdout, stderr bytes.Buffer
	if got := runSummarize([]string{"-plan", plan, "-records", records, "-gate", "beyond:linux/amd64", "-commit", "abc123"}, &stdout, &stderr); got != 1 {
		t.Fatalf("runSummarize() = %d, want 1; stderr:\n%s", got, stderr.String())
	}
	problems := []string{
		`the record in zz-broken cannot be read: line "nonsense" is not key=value`,
		"two records name ./cmd/server on macos-latest",
		"the record of ./internal/toolutil on macos-latest cannot be read: the script's status says it measured, and its output carries no Condition coverage figure",
		"a record names ./x on macos-latest, which the plan did not ask for",
	}
	want := summaryIntro("beyond:linux/amd64", "abc123") +
		"| `./cmd/server` | `windows-latest` (windows/amd64) | 8 of 12 | 3 | 1 | `gobco-windows-latest-cmd-server/cmd-server` |\n" +
		"| `./cmd/server` | `macos-latest` (darwin/arm64) | 12 of 12 | 0 | 0 | `gobco-macos-latest-cmd-server/cmd-server` |\n" +
		"| `./cmd/server` | `ubuntu-latest` (linux/amd64) | not measured (exit 1) | - | - | `gobco-ubuntu-latest-cmd-server/cmd-server` |\n" +
		"| `./internal/toolutil` | `windows-latest` | no record | - | - | `gobco-windows-latest-internal-toolutil/internal-toolutil` |\n" +
		"| `./internal/toolutil` | `macos-latest` (darwin/arm64) | unreadable record | - | - | `gobco-macos-latest-internal-toolutil/internal-toolutil` |\n" +
		"| `./internal/toolutil` | `ubuntu-latest` (linux/amd64) | not measured (exit 2) | - | - | `gobco-ubuntu-latest-internal-toolutil/internal-toolutil` |\n" +
		totalsTable(
			"| `windows-latest` | 1 of 2 | 8 of 12 | 66.6% |",
			"| `macos-latest` | 1 of 2 | 12 of 12 | 100.0% |",
			"| `ubuntu-latest` | 0 of 2 | 0 of 0 | - |",
		) +
		"\n### Not evaluated both ways\n\n" +
		"| Package | System | File | Line | Condition | gobco says | Held |\n" +
		"| --- | --- | --- | ---: | --- | --- | --- |\n" +
		"| `./cmd/server` | `windows-latest` | `conn_refused_windows.go` | 12 | `errors.Is(err, x)` | 2 times true but never false | yes |\n" +
		"| `./cmd/server` | `windows-latest` | `main.go` | 40 | `a \\|\\| b` | never evaluated | no |\n" +
		"| `./cmd/server` | `windows-latest` | `main.go` | 52 | `strings.Contains(s, \"x\t\")` | once true but never false | no |\n" +
		"\n### Not measured\n" +
		"\n#### `./cmd/server` on `ubuntu-latest`\n\n" +
		"`scripts/coverage-conditions.sh` exited 1, which means it could not measure, and said:\n\n" +
		"````text\n--- FAIL: TestX\ngobco: a line quoting ``` three backticks\ngobco: gobco exited 1 on ./cmd/server\n````\n" +
		"\n#### `./internal/toolutil` on `windows-latest`\n\n" +
		"No record came back in `gobco-windows-latest-internal-toolutil/internal-toolutil`: the run ended before its measuring step wrote one (a setup step failed, or the run was cancelled, timed out or lost its runner), or it did not upload the one it wrote, and its job log says which.\n" +
		"\n#### `./internal/toolutil` on `ubuntu-latest`\n\n" +
		"`scripts/coverage-conditions.sh` exited 2, which means it could not measure, and said:\n\n" +
		"```text\n" + strings.Join(tail, "\n") + "\n```\n" +
		"\n### Records this summary could not use\n\n" +
		"- " + strings.Join(problems, "\n- ") + "\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
	if wantStderr := "measure_conditions: " + strings.Join(problems, "\nmeasure_conditions: ") + "\n"; stderr.String() != wantStderr {
		t.Errorf("stderr =\n%s\nwant\n%s", stderr.String(), wantStderr)
	}
}

// TestReasonLines_Outputs_KeepWhatTheScriptSaid verifies the reason a run
// that could not measure is shown with: the script's own lines, and the
// lines gobco or its go test said the failure in before the script's verdict
// (a panic, a top-level failing test, a package that did not build or set
// up, a compiler's position), each in the order printed and without the
// conditions a report beside the failure names; and when none of those is
// there, the whole of a short output or the end of a long one. The three
// failures are the three ways the tree's unmeasured packages failed: an
// export_test.go symbol gobco cannot resolve, the bridge it writes with an
// empty import path, and tests that read the source gobco rewrote.
func TestReasonLines_Outputs_KeepWhatTheScriptSaid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{name: "the script's lines", output: "noise\ngobco: one\r\nmore noise\ngobco: two\n", want: "gobco: one\ngobco: two"},
		{name: "a short output", output: "a\r\nb\n\n", want: "a\nb"},
		{name: "nothing", output: "", want: ""},
		{name: "a prefix without its space", output: "gobco:x\n", want: "gobco:x"},
		{
			name: "gobco panicked",
			output: "panic: a_test.go:49:27: undefined: a.PublishedActionIDs\n\ngoroutine 1 [running]:\nmain.ok(...)\n" +
				"\t/go/pkg/mod/github.com/rillig/gobco@v1.3.4/util.go:100\nmain.(*instrumenter).resolveTypes(0x1)\n" +
				"\t/go/pkg/mod/github.com/rillig/gobco@v1.3.4/instrumenter.go:143 +0x445\nexit status 2\n" +
				"gobco: gobco exited 1 on ./a, so any figure above is not a measurement\n",
			want: "panic: a_test.go:49:27: undefined: a.PublishedActionIDs\ngobco: gobco exited 1 on ./a, so any figure above is not a measurement",
		},
		{
			name: "the test package did not set up",
			output: "# example.com/m/a\n/tmp/gobco-1/module/a/gobco_bridge_test.go:3:9: invalid import path: \r\n" +
				"FAIL\texample.com/m/a [setup failed]\nFAIL\ngo test .: exit status 1\nexit status 1\ngobco: gobco exited 1 on ./a\n",
			want: "/tmp/gobco-1/module/a/gobco_bridge_test.go:3:9: invalid import path: \nFAIL\texample.com/m/a [setup failed]\ngobco: gobco exited 1 on ./a",
		},
		{
			name:   "the test package did not build",
			output: "FAIL\texample.com/m/b [build failed]\nFAIL\tsomething else\ngobco: gobco exited 1 on ./b\n",
			want:   "FAIL\texample.com/m/b [build failed]\ngobco: gobco exited 1 on ./b",
		},
		{
			name: "a test failed beside a report",
			output: "--- FAIL: TestCases (0.00s)\n    --- FAIL: TestCases/one (0.00s)\n        c_test.go:20: no case labels were read\n" +
				"FAIL\nFAIL\texample.com/m/c\t1.1s\n\nCondition coverage: 3/4\n" + `c.go:4:9: condition "x == 1" was never evaluated` + "\n" +
				"exit status 1\ngobco: gobco exited 1 on ./c\n",
			want: "--- FAIL: TestCases (0.00s)\ngobco: gobco exited 1 on ./c",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := reasonLines(tc.output); got != tc.want {
				t.Errorf("reasonLines(%q) = %q, want %q", tc.output, got, tc.want)
			}
		})
	}
}

// TestCodeSpan_Backticks_AreFencedPastTheLongestRun verifies the code span
// rule: a delimiter one backtick longer than the longest run inside, and a
// space of padding exactly where the text starts or ends with a backtick.
func TestCodeSpan_Backticks_AreFencedPastTheLongestRun(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"x":       "`x`",
		"a`b":     "``a`b``",
		"a``b`c":  "```a``b`c```",
		"`a":      "`` `a ``",
		"a`":      "`` a` ``",
		"plain b": "`plain b`",
	}
	for text, want := range cases {
		t.Run(text, func(t *testing.T) {
			t.Parallel()
			if got := codeSpan(text); got != want {
				t.Errorf("codeSpan(%q) = %q, want %q", text, got, want)
			}
		})
	}
}

// TestWriteRow_Pipes_AreEscapedInEveryCell verifies that a pipe in any cell,
// inside a code span or not, cannot break the row into more columns.
func TestWriteRow_Pipes_AreEscapedInEveryCell(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	writeRow(&out, "a|b", "`c||d`", "e")
	if want := "| a\\|b | `c\\|\\|d` | e |\n"; out.String() != want {
		t.Errorf("writeRow() = %q, want %q", out.String(), want)
	}
}
