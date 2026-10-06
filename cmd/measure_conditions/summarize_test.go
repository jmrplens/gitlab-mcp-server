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
	data, err := json.Marshal(planned)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// recordFor is a record as the workflow writes it, for a platform of the
// test's choosing.
func recordFor(pkg, system, platform string, status int) string {
	return fmt.Sprintf("package=%s\nsystem=%s\nplatform=%s\nstatus=%d\n", pkg, system, platform, status)
}

// TestRunSummarize_Usage_IsRefusedWithStatus2 verifies every way the command
// line can be wrong: an unknown flag, an argument, each of the four flags
// left empty, a plan that is not JSON and a plan with no run in it.
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
		"| `./cmd/server` | `macos-latest` (darwin/arm64) | 12 of 12 | 0 | 0 | `gobco-macos-latest-cmd-server` |\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Errorf("stdout =\n%s\nwant\n%s\nstderr = %q", stdout.String(), want, stderr.String())
	}
}

// summaryIntro is the heading, the paragraph and the table head every
// summary opens with.
func summaryIntro(gate, commit string) string {
	return "## Condition coverage, measured\n\n" +
		"`GOBCO_GATE=" + gate + "` at `" + commit + "`. Every figure is the one gobco printed on the runner named, through `scripts/coverage-conditions.sh`, the recipe behind `make coverage-conditions`; none is inferred from another platform. gobco counts each condition twice, once per outcome, so a total is twice the number of conditions. No figure fails this workflow: it is a measurement, not a gate.\n\n" +
		"| Package | System | Covered | Not both ways | Held by the gate | Raw output |\n" +
		"| --- | --- | ---: | ---: | ---: | --- |\n"
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
		"| `./cmd/server` | `windows-latest` (windows/amd64) | 8 of 12 | 3 | 1 | `gobco-windows-latest-cmd-server` |\n" +
		"| `./cmd/server` | `macos-latest` (darwin/arm64) | 12 of 12 | 0 | 0 | `gobco-macos-latest-cmd-server` |\n" +
		"| `./cmd/server` | `ubuntu-latest` (linux/amd64) | not measured (exit 1) | - | - | `gobco-ubuntu-latest-cmd-server` |\n" +
		"| `./internal/toolutil` | `windows-latest` | no record | - | - | `gobco-windows-latest-internal-toolutil` |\n" +
		"| `./internal/toolutil` | `macos-latest` (darwin/arm64) | unreadable record | - | - | `gobco-macos-latest-internal-toolutil` |\n" +
		"| `./internal/toolutil` | `ubuntu-latest` (linux/amd64) | not measured (exit 2) | - | - | `gobco-ubuntu-latest-internal-toolutil` |\n" +
		"\n### Not evaluated both ways\n\n" +
		"| Package | System | File | Line | Condition | gobco says | Held |\n" +
		"| --- | --- | --- | ---: | --- | --- | --- |\n" +
		"| `./cmd/server` | `windows-latest` | `conn_refused_windows.go` | 12 | `errors.Is(err, x)` | 2 times true but never false | yes |\n" +
		"| `./cmd/server` | `windows-latest` | `main.go` | 40 | `a \\|\\| b` | never evaluated | no |\n" +
		"| `./cmd/server` | `windows-latest` | `main.go` | 52 | `strings.Contains(s, \"x\t\")` | once true but never false | no |\n" +
		"\n### Not measured\n" +
		"\n#### `./cmd/server` on `ubuntu-latest`\n\n" +
		"`scripts/coverage-conditions.sh` exited 1, which means it could not measure, and said:\n\n" +
		"````text\ngobco: a line quoting ``` three backticks\ngobco: gobco exited 1 on ./cmd/server\n````\n" +
		"\n#### `./internal/toolutil` on `windows-latest`\n\n" +
		"No record came back in `gobco-windows-latest-internal-toolutil`: the run ended before its measuring step wrote one (a setup step failed, or the run was cancelled, timed out or lost its runner), or it did not upload the one it wrote, and its job log says which.\n" +
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
// that could not measure is shown with: the script's own lines when it wrote
// any, and otherwise the whole of a short output or the end of a long one.
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
