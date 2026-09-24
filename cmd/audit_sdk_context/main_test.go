package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixtureWithAFinding is the smallest package this rule has anything to say
// about: one call that passes the context and one that does not.
const fixtureWithAFinding = fixtureHeader + `
func Get(ctx context.Context, c *gl.Client) {
	_, _, _ = c.Version.GetVersion(gl.WithContext(ctx))
	_, _, _ = c.Version.GetVersion()
}
`

// fixtureClean is the same package with both calls passing the context.
const fixtureClean = fixtureHeader + `
func Get(ctx context.Context, c *gl.Client) {
	_, _, _ = c.Version.GetVersion(gl.WithContext(ctx))
}
`

// runFixture runs the command over an in-memory fixture package and returns
// the exit code with what each stream said.
func runFixture(t *testing.T, source string, check, verbose bool) (int, string, string) {
	t.Helper()
	root := repoRoot(t)
	var stdout, stderr strings.Builder
	code := run(auditConfig{
		dir:      root,
		patterns: []string{"./" + fixtureDir},
		overlay:  fixtureOverlay(root, map[string]string{"fixture.go": source}),
	}, check, verbose, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// TestRun_FindingWithoutCheck_ReportsAndSucceeds: the report is worth having
// without the gate, so a plain run prints the finding and exits 0. The whole
// output is held, since every figure on the summary is read off the scan.
func TestRun_FindingWithoutCheck_ReportsAndSucceeds(t *testing.T) {
	code, stdout, stderr := runFixture(t, fixtureWithAFinding, false, false)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 without -check", code)
	}
	want := fixtureDir + "/fixture.go:18: c.Version.GetVersion " + reasonMissing + " (in Get)\n" +
		"\n" +
		"audit_sdk_context: 2 calls handing client-go request options in 1 packages, 1 without the caller's context " +
		"(0 forwarded to their own caller, 0 rebound after they were built, 0 excused by a declaration)\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty: a finding is not a broken run", stderr)
	}
}

// TestRun_FindingUnderCheck_Fails is the gate.
func TestRun_FindingUnderCheck_Fails(t *testing.T) {
	code, stdout, stderr := runFixture(t, fixtureWithAFinding, true, false)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 under -check", code)
	}
	if !strings.Contains(stdout, reasonMissing) {
		t.Fatalf("stdout does not name the finding:\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty: the finding goes to stdout", stderr)
	}
}

// TestRun_NothingToReport_SucceedsUnderCheck. The output is held whole: a
// quiet clean run is the one summary line and nothing before it.
func TestRun_NothingToReport_SucceedsUnderCheck(t *testing.T) {
	code, stdout, stderr := runFixture(t, fixtureClean, true, false)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	want := "audit_sdk_context: 1 calls handing client-go request options in 1 packages, 0 without the caller's context " +
		"(0 forwarded to their own caller, 0 rebound after they were built, 0 excused by a declaration)\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

// TestRun_SourceThatDoesNotTypeCheck_FailsOnStderr: a package that did not
// type-check resolves nothing, so a report over it would be a clean answer
// about source nobody understood. The loader refuses it and this says so.
func TestRun_SourceThatDoesNotTypeCheck_FailsOnStderr(t *testing.T) {
	code, stdout, stderr := runFixture(t, "package fixture\n\nfunc Live() string { return undefinedSymbol }\n", false, false)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.HasPrefix(stderr, toolName+": ") {
		t.Fatalf("stderr = %q, want it to name the tool", stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want nothing: no report was made", stdout)
	}
}

// TestRun_PatternMatchingNothing_FailsRatherThanPassing guards the silence a
// gate must not have: auditing no package is not a clean run.
func TestRun_PatternMatchingNothing_FailsRatherThanPassing(t *testing.T) {
	var stdout, stderr strings.Builder
	code := run(auditConfig{
		dir:      repoRoot(t),
		patterns: []string{"./cmd/audit_sdk_context/nothing-is-here/..."},
	}, true, false, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if stderr.String() == "" {
		t.Fatal("stderr is empty, want the refusal")
	}
}

// TestAudit_RootThatCannotBeResolved_FailsRatherThanLoading: the root is made
// absolute before anything is loaded, so a run whose working directory is
// gone stops on that error instead of naming files against wherever the
// process happens to be.
//
// Only Linux can be made to fail that way. Windows refuses to remove a
// process's working directory, and macOS keeps answering getcwd from the path
// it remembers, so on both the premise cannot be set up and the case is
// skipped rather than reported as a defect in the operating system.
func TestAudit_RootThatCannotBeResolved_FailsRatherThanLoading(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o750); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	t.Chdir(gone)
	if err := os.Remove(gone); err != nil {
		t.Skipf("this platform will not remove the working directory, so filepath.Abs cannot be made to fail here: %v", err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("this platform's getcwd still answers after the working directory is removed, so filepath.Abs cannot fail here")
	}

	report, err := audit(auditConfig{dir: ".", patterns: []string{"./..."}})
	if err == nil {
		t.Fatalf("audit error = nil, want the unresolved root; report = %+v", report)
	}
}

// TestRunMain_ParsesTheCommandLine holds what each argument does: an unknown
// flag is a usage error, a help request is not a failure, and the patterns
// and flags reach the run they describe. The package named is this command's
// own, which is real source and clean.
func TestRunMain_ParsesTheCommandLine(t *testing.T) {
	root := repoRoot(t)
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
	}{
		{name: "unknown flag", args: []string{"-nope"}, wantCode: 2},
		{name: "help", args: []string{"-h"}, wantCode: 0},
		{
			name: "one package under check and verbose", args: []string{"-dir", root, "-check", "-v", "./cmd/audit_sdk_context"}, wantCode: 0,
			wantStdout: "\naudit_sdk_context: 0 calls handing client-go request options in 1 packages",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			if code := runMain(tt.args, &stdout, &stderr); code != tt.wantCode {
				t.Fatalf("exit = %d, want %d; stderr = %q", code, tt.wantCode, stderr.String())
			}
			if !strings.HasPrefix(stdout.String(), tt.wantStdout) {
				t.Fatalf("stdout = %q, want it to begin %q", stdout.String(), tt.wantStdout)
			}
		})
	}
}

// TestRunMain_NoPatterns_AuditsTheDefaultOnes: with nothing named, the run is
// over internal/ and cmd/ of the directory -dir names. A module holding one
// package under each, and one outside both, is loaded instead of this whole
// repository, so the count on the summary can only come from the default
// patterns.
func TestRunMain_NoPatterns_AuditsTheDefaultOnes(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":           "module example.com/tiny\n\ngo 1.27\n",
		"internal/a/a.go":  "package a\n",
		"cmd/b/main.go":    "package main\n\nfunc main() {}\n",
		"elsewhere/c/c.go": "package c\n",
	})
	var stdout, stderr strings.Builder
	if code := runMain([]string{"-dir", dir, "-check"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stdout = %s; stderr = %s", code, stdout.String(), stderr.String())
	}
	if want := "audit_sdk_context: 0 calls handing client-go request options in 2 packages,"; !strings.HasPrefix(stdout.String(), want) {
		t.Fatalf("stdout = %q, want it to begin %q", stdout.String(), want)
	}
}

// writeTree writes the named files below a new temporary directory and
// returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, source := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("prepare %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatalf("prepare %s: %v", name, err)
		}
	}
	return dir
}

// TestMain_ExitsWithTheCodeRunMainDecided: main hands runMain's code to the
// process and does nothing else.
func TestMain_ExitsWithTheCodeRunMainDecided(t *testing.T) {
	previous := exitProcess
	t.Cleanup(func() { exitProcess = previous })
	var codes []int
	exitProcess = func(code int) { codes = append(codes, code) }
	previousArgs := os.Args
	t.Cleanup(func() { os.Args = previousArgs })
	os.Args = []string{toolName, "-nope"}

	main()

	if !slices.Equal(codes, []int{2}) {
		t.Fatalf("exit codes = %v, want the 2 runMain returned for an unknown flag", codes)
	}
}

// TestDefaultPatterns_CoverTheRepositorysLibrarySource states what the gate
// is over, so narrowing it is a visible change rather than a quiet one.
func TestDefaultPatterns_CoverTheRepositorysLibrarySource(t *testing.T) {
	if want := []string{"./internal/...", "./cmd/..."}; !slices.Equal(defaultPatterns, want) {
		t.Fatalf("defaultPatterns = %v, want %v", defaultPatterns, want)
	}
}
