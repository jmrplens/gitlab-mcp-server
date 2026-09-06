package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runFixture runs the audit over the named fixture packages, with the
// repository as the root and the streams captured.
func runFixture(t *testing.T, cfg auditRun, sources map[string]string) (code int, out, errOut string) {
	t.Helper()
	cfg.dir = repoRoot(t)
	cfg.patterns = []string{fixturePattern}
	cfg.overlay = fixtureOverlay(t, sources)
	var stdout, stderr strings.Builder
	code = execute(cfg, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// cleanFixture is a fixture with nothing to report, so the passing side of the
// gate is exercised on real source rather than on an empty report.
var cleanFixture = map[string]string{"mdsafe/mdsafe.go": mdsafeSource}

// TestExecute_CleanTree_PassesTheGate checks the answer a green run gives,
// which has to name what it judged rather than only say nothing was wrong.
func TestExecute_CleanTree_PassesTheGate(t *testing.T) {
	code, out, errOut := runFixture(t, auditRun{contexts: allContexts, check: true}, cleanFixture)

	if code != 0 {
		t.Errorf("exit code = %d, want 0\n%s%s", code, out, errOut)
	}
	if !strings.Contains(out, "check: PASS") {
		t.Errorf("a clean run does not say it passed:\n%s", out)
	}
	if !strings.Contains(out, "table-cell, heading, list-item, link-label, link-destination") {
		t.Errorf("the passing line does not name what was judged:\n%s", out)
	}
}

// TestExecute_CleanTree_FailUnresolved_Fails checks the stricter gate: a value
// the audit could not follow is a documented hole, and an operator who wants
// none can say so.
func TestExecute_CleanTree_FailUnresolved_Fails(t *testing.T) {
	code, out, _ := runFixture(t, auditRun{contexts: allContexts, check: true, failUnresolved: true}, cleanFixture)

	if code != 1 {
		t.Errorf("exit code = %d, want 1 with -fail-unresolved\n%s", code, out)
	}
	if !strings.Contains(out, "check: FAIL") {
		t.Errorf("the run does not say it failed:\n%s", out)
	}
}

// TestExecute_UnescapedValue_FailsTheGate checks the failing side, including
// that the failure names the counts a reader needs.
func TestExecute_UnescapedValue_FailsTheGate(t *testing.T) {
	code, out, _ := runFixture(t, auditRun{contexts: allContexts, check: true}, caseFixture)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "check: FAIL") || !strings.Contains(out, "reach a Markdown construct unescaped") {
		t.Errorf("the failure does not say what failed:\n%s", out)
	}
}

// TestExecute_WithoutCheck_ReportsWithoutFailing checks that the report mode
// is a report: it lists the same findings and exits 0, which is what makes it
// usable while the sweep is under way.
func TestExecute_WithoutCheck_ReportsWithoutFailing(t *testing.T) {
	code, out, _ := runFixture(t, auditRun{contexts: allContexts, verbose: true}, caseFixture)

	if code != 0 {
		t.Errorf("exit code = %d, want 0 without -check\n%s", code, out)
	}
	if !strings.Contains(out, "=== "+fixtureDir+"/mdcase ===") {
		t.Errorf("the report does not group the fixture's findings:\n%s", out)
	}
	if strings.Contains(out, "check:") {
		t.Errorf("the report mode printed a gate verdict:\n%s", out)
	}
}

// TestExecute_JSONPath_WritesTheWorkListOrSaysWhyNot covers both ends of the
// work list: the file the walk reads, and the failure that must not read as a
// clean run.
func TestExecute_JSONPath_WritesTheWorkListOrSaysWhyNot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backlog.json")

	code, out, _ := runFixture(t, auditRun{contexts: allContexts, jsonPath: path}, caseFixture)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0\n%s", code, out)
	}
	if !strings.Contains(out, "work list written to "+path) {
		t.Errorf("the run does not say where it wrote the work list:\n%s", out)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Errorf("work list at %s: %v", path, err)
	}

	failing, _, errOut := runFixture(t,
		auditRun{contexts: allContexts, jsonPath: filepath.Join(path, "nested.json")}, caseFixture)
	if failing != 2 {
		t.Errorf("exit code = %d, want 2 when the work list cannot be written", failing)
	}
	if !strings.Contains(errOut, toolName+":") {
		t.Errorf("the failure does not name the gate that failed: %q", errOut)
	}
}

// TestExecute_Failures_ExitTwo checks that a gate which could not do its job
// says so with the exit code its siblings use, rather than reading as a gate
// that passed.
func TestExecute_Failures_ExitTwo(t *testing.T) {
	cases := []struct {
		name string
		cfg  auditRun
		want string
	}{
		{name: "an unknown context", cfg: auditRun{contexts: "cells"}, want: "unknown Markdown context"},
		{name: "a tree that cannot be loaded", cfg: auditRun{contexts: allContexts, dir: filepath.Join(t.TempDir(), "absent")}, want: "load"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			if cfg.dir == "" {
				cfg.dir = repoRoot(t)
			}
			cfg.patterns = []string{fixturePattern}
			var out, errOut strings.Builder

			if got := execute(cfg, &out, &errOut); got != 2 {
				t.Errorf("exit code = %d, want 2", got)
			}
			if !strings.Contains(errOut.String(), tc.want) {
				t.Errorf("error %q does not say %q", errOut.String(), tc.want)
			}
		})
	}
}

// TestExecute_UnresolvableRoot_ExitsTwo checks the one failure that needs the
// working directory to have been removed under the process, through the seam
// that stands in for it.
func TestExecute_UnresolvableRoot_ExitsTwo(t *testing.T) {
	original := absRoot
	t.Cleanup(func() { absRoot = original })
	absRoot = func(string) (string, error) { return "", errors.New("no working directory") }
	var out, errOut strings.Builder

	code := execute(auditRun{contexts: allContexts, dir: "."}, &out, &errOut)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "no working directory") {
		t.Errorf("error %q does not name the failure", errOut.String())
	}
}

// TestRun_CommandLine_ParsesItsFlags covers the command line itself: the help
// that is not a failure, the flag that is, and a real run driven the way the
// Makefile drives it.
func TestRun_CommandLine_ParsesItsFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{name: "help", args: []string{"-h"}, want: 0},
		{name: "a flag that does not exist", args: []string{"-no-such-flag"}, want: 2},
		{name: "an unknown context", args: []string{"-contexts", "cells"}, want: 2},
		{name: "a report over the packages named after the flags", args: []string{"-dir", repoRoot(t), "-contexts", "heading", "./internal/toolutil"}, want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut strings.Builder

			if got := run(tc.args, &out, &errOut); got != tc.want {
				t.Errorf("run(%v) = %d, want %d\n%s%s", tc.args, got, tc.want, out.String(), errOut.String())
			}
		})
	}
}

// TestGate_Report_DecidesWhatFails checks the verdict itself, since what the
// gate counts as a failure is the whole of its behavior in CI.
func TestGate_Report_DecidesWhatFails(t *testing.T) {
	cases := []struct {
		name           string
		summary        Summary
		failUnresolved bool
		want           int
		says           string
	}{
		{name: "nothing at all", summary: Summary{Contexts: "table-cell"}, want: 0, says: "check: PASS"},
		{name: "an unescaped value", summary: Summary{Findings: 1}, want: 1, says: "check: FAIL"},
		{name: "a directive that excuses nothing", summary: Summary{Stale: 1}, want: 1, says: "check: FAIL"},
		{name: "an unresolved value is not a failure", summary: Summary{Unresolved: 3, Contexts: "table-cell"}, want: 0, says: "3 unresolved"},
		{name: "unless the operator asks", summary: Summary{Unresolved: 3}, failUnresolved: true, want: 1, says: "check: FAIL"},
		{name: "an excused value is not one either", summary: Summary{Excused: 2, Contexts: "table-cell"}, want: 0, says: "2 excused"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder

			got := gate(&out, Report{Summary: tc.summary}, tc.failUnresolved)

			if got != tc.want {
				t.Errorf("gate = %d, want %d", got, tc.want)
			}
			if !strings.Contains(out.String(), tc.says) {
				t.Errorf("gate said %q, want it to say %q", out.String(), tc.says)
			}
		})
	}
}

// TestMain_HelpFlag_ExitsThroughTheSeam checks the one line main carries, so
// the command's entry point is exercised rather than assumed.
func TestMain_HelpFlag_ExitsThroughTheSeam(t *testing.T) {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer devNull.Close()
	originalArgs, originalExit, originalOut, originalErr := os.Args, exit, os.Stdout, os.Stderr
	t.Cleanup(func() { os.Args, exit, os.Stdout, os.Stderr = originalArgs, originalExit, originalOut, originalErr })
	code := -1
	exit = func(got int) { code = got }
	os.Args = []string{toolName, "-h"}
	os.Stdout, os.Stderr = devNull, devNull

	main()

	if code != 0 {
		t.Errorf("main exited %d, want 0", code)
	}
}
