// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
)

// plantedSummary is the summary line of a run over the whole planted module,
// every figure of which is read off the scan.
const plantedSummary = "audit_test_closed_servers: 110 closes of an httptest server judged in 6 packages; " +
	"64 made while its address is still used, 0 excused by a declaration\n"

// cleanSummary is the summary line of a run over the planted package that
// holds nothing to report.
const cleanSummary = "audit_test_closed_servers: 44 closes of an httptest server judged in 1 packages; " +
	"0 made while its address is still used, 0 excused by a declaration\n"

// plantedDir is the fixture module every run here is made over.
func plantedDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "planted"))
	if err != nil {
		t.Fatalf("fixture module: %v", err)
	}
	return dir
}

// plantedConfig is a run over the named packages of the fixture module under
// every analysis tag, with no declarations.
func plantedConfig(t *testing.T, patterns ...string) auditConfig {
	t.Helper()
	return auditConfig{dir: plantedDir(t), patterns: patterns, tags: analysisTags, declared: map[string]sourcegate.Declaration{}}
}

// runPlanted runs the command over the fixture module and returns the exit
// code with what each stream said.
func runPlanted(t *testing.T, check, verbose bool, patterns ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr strings.Builder
	code := run(plantedConfig(t, patterns...), check, verbose, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// TestRun_FindingsWithoutCheck_ReportAndSucceed: the report is worth having
// without the gate, so a plain run prints every finding and exits 0, and the
// summary is the last line, after a blank one.
func TestRun_FindingsWithoutCheck_ReportAndSucceed(t *testing.T) {
	code, stdout, stderr := runPlanted(t, false, false, "./...")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 without -check", code)
	}
	if !strings.HasSuffix(stdout, "\n\n"+plantedSummary) {
		t.Fatalf("stdout does not end with a blank line and %q:\n%s", plantedSummary, stdout)
	}
	if !strings.HasPrefix(stdout, "declared/declared_test.go:18: srv is closed while its address is still used, through url at line 19 (in TestPlanted_ExcusedByADeclaration_Excused)\n") {
		t.Fatalf("stdout does not open with the first finding in file order:\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty: a finding is not a broken run", stderr)
	}
}

// TestRun_FindingsUnderCheck_Fail is the gate.
func TestRun_FindingsUnderCheck_Fail(t *testing.T) {
	code, stdout, stderr := runPlanted(t, true, false, "./...")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 under -check", code)
	}
	if !strings.HasSuffix(stdout, plantedSummary) {
		t.Fatalf("stdout does not end with the summary:\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty: the findings go to stdout", stderr)
	}
}

// TestRun_NothingToReport_SucceedsUnderCheck holds the output whole: a quiet
// clean run is the one summary line and nothing before it, and a verbose one
// puts a blank line before it.
func TestRun_NothingToReport_SucceedsUnderCheck(t *testing.T) {
	for _, tt := range []struct {
		name    string
		verbose bool
		want    string
	}{
		{name: "quiet", want: cleanSummary},
		{name: "verbose", verbose: true, want: "\n" + cleanSummary},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runPlanted(t, true, tt.verbose, "./clean/...")
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stdout = %s", code, stdout)
			}
			if stdout != tt.want {
				t.Fatalf("stdout = %q, want %q", stdout, tt.want)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
		})
	}
}

// TestRun_ALoadThatFails_FailsOnStderr: a run that could not be made says so
// on stderr, names the tool, and prints no report.
func TestRun_ALoadThatFails_FailsOnStderr(t *testing.T) {
	code, stdout, stderr := runPlanted(t, false, false, "./nowhere/...")
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

// TestAudit_RootThatCannotBeResolved_FailsRatherThanLoading verifies a run
// whose working directory is gone stops on that error instead of naming files
// against wherever the process happens to be. Only Linux can be made to fail
// that way, so the other platforms skip.
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
// flag is a usage error, a help request is not a failure, and the directory,
// the patterns and the flags reach the run they describe.
func TestRunMain_ParsesTheCommandLine(t *testing.T) {
	dir := plantedDir(t)
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name: "unknown flag", args: []string{"-nope"}, wantCode: 2,
			wantStderr: "flag provided but not defined: -nope\nUsage of audit_test_closed_servers:\n",
		},
		{name: "help", args: []string{"-h"}, wantCode: 0, wantStderr: "Usage of audit_test_closed_servers:\n"},
		{name: "a clean package under check", args: []string{"-dir", dir, "-check", "./clean/..."}, wantCode: 0, wantStdout: cleanSummary},
		{name: "verbose", args: []string{"-dir", dir, "-v", "./clean/..."}, wantCode: 0, wantStdout: "\n" + cleanSummary},
		{
			name: "findings under check", args: []string{"-dir", dir, "-check", "./flagged/..."}, wantCode: 1,
			wantStdout: "flagged/external_test.go:21: ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			if code := runMain(tt.args, &stdout, &stderr); code != tt.wantCode {
				t.Fatalf("exit = %d, want %d; stderr = %q", code, tt.wantCode, stderr.String())
			}
			assertBegins(t, "stdout", stdout.String(), tt.wantStdout)
			assertBegins(t, "stderr", stderr.String(), tt.wantStderr)
		})
	}
}

// assertBegins holds one output stream to what it has to begin with, and an
// empty want to a stream that carries nothing at all.
func assertBegins(t *testing.T, stream, got, want string) {
	t.Helper()
	if want == "" {
		if got != "" {
			t.Fatalf("%s = %q, want nothing", stream, got)
		}
		return
	}
	if !strings.HasPrefix(got, want) {
		t.Fatalf("%s = %q, want it to begin %q", stream, got, want)
	}
}

// TestRunMain_NoPatterns_AuditsTheWholeModule: with nothing named, the run is
// over every package of the module -dir names, wherever it sits.
func TestRunMain_NoPatterns_AuditsTheWholeModule(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":           "module example.com/tiny\n\ngo 1.27\n",
		"internal/a/a.go":  "package a\n",
		"elsewhere/c/c.go": "package c\n",
	})
	var stdout, stderr strings.Builder
	if code := runMain([]string{"-dir", dir, "-check"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stdout = %s; stderr = %s", code, stdout.String(), stderr.String())
	}
	if want := "audit_test_closed_servers: 0 closes of an httptest server judged in 2 packages; "; !strings.HasPrefix(stdout.String(), want) {
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
	previous := sourcegate.Exit
	t.Cleanup(func() { sourcegate.Exit = previous })
	var codes []int
	sourcegate.Exit = func(code int) { codes = append(codes, code) }
	previousArgs := os.Args
	t.Cleanup(func() { os.Args = previousArgs })
	os.Args = []string{toolName, "-nope"}
	previousStderr := os.Stderr
	t.Cleanup(func() { os.Stderr = previousStderr })
	os.Stderr = createFile(t, filepath.Join(t.TempDir(), "stderr"))

	main()

	if !slices.Equal(codes, []int{2}) {
		t.Fatalf("exit codes = %v, want the 2 runMain returned for an unknown flag", codes)
	}
}

// TestMain_WritesTheReportToStdoutAndTheComplaintToStderr: main hands runMain
// the process's standard output and standard error in that order. Each case is
// held on both streams, because handed over the other way round the text is
// not lost, only moved.
func TestMain_WritesTheReportToStdoutAndTheComplaintToStderr(t *testing.T) {
	dir := plantedDir(t)
	previousExit, previousArgs := sourcegate.Exit, os.Args
	previousStdout, previousStderr := os.Stdout, os.Stderr
	t.Cleanup(func() {
		sourcegate.Exit, os.Args = previousExit, previousArgs
		os.Stdout, os.Stderr = previousStdout, previousStderr
	})
	sourcegate.Exit = func(int) {}
	tests := []struct {
		name       string
		args       []string
		wantStdout string
		wantStderr string
	}{
		{name: "a report", args: []string{"-dir", dir, "./clean/..."}, wantStdout: cleanSummary},
		{name: "an unknown flag", args: []string{"-nope"}, wantStderr: "flag provided but not defined: -nope\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams := t.TempDir()
			stdout := createFile(t, filepath.Join(streams, "stdout"))
			stderr := createFile(t, filepath.Join(streams, "stderr"))
			os.Args = append([]string{toolName}, tt.args...)
			os.Stdout, os.Stderr = stdout, stderr
			main()
			os.Stdout, os.Stderr = previousStdout, previousStderr
			assertBegins(t, "stdout", closeAndRead(t, stdout), tt.wantStdout)
			assertBegins(t, "stderr", closeAndRead(t, stderr), tt.wantStderr)
		})
	}
}

// createFile creates an empty file to stand in for one of the process's
// streams. The file is closed when the test ends, before the temporary
// directory holding it is removed: Windows will not remove a file that is
// still open, and a test that only hands the file to main never closes it.
func createFile(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("prepare %s: %v", path, err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

// closeAndRead closes a file that stood in for a stream and returns what was
// written to it.
func closeAndRead(t *testing.T, file *os.File) string {
	t.Helper()
	if err := file.Close(); err != nil {
		t.Fatalf("close %s: %v", file.Name(), err)
	}
	content, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("read %s: %v", file.Name(), err)
	}
	return string(content)
}

// TestDefaultPatterns_AreTheWholeModule states what the gate is over, so
// narrowing it is a visible change rather than a quiet one.
func TestDefaultPatterns_AreTheWholeModule(t *testing.T) {
	if want := []string{"./..."}; !slices.Equal(defaultPatterns, want) {
		t.Fatalf("defaultPatterns = %v, want %v", defaultPatterns, want)
	}
}

// TestAnalysisTags_AreTheMakefilesAnalysisTags holds the tags every load sets
// to GO_ANALYSIS_TAGS in the Makefile, the list golangci-lint is run with, so
// a suite added behind a new tag cannot be added there and left unread here.
func TestAnalysisTags_AreTheMakefilesAnalysisTags(t *testing.T) {
	makefile, err := os.Open(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatalf("open the Makefile: %v", err)
	}
	defer makefile.Close()
	const prefix = "GO_ANALYSIS_TAGS="
	var listed []string
	lines := bufio.NewScanner(makefile)
	for lines.Scan() {
		if value, found := strings.CutPrefix(lines.Text(), prefix); found {
			listed = strings.Split(strings.TrimSpace(value), ",")
		}
	}
	if scanErr := lines.Err(); scanErr != nil {
		t.Fatalf("read the Makefile: %v", scanErr)
	}
	if listed == nil {
		t.Fatalf("the Makefile sets no %s", strings.TrimSuffix(prefix, "="))
	}
	slices.Sort(listed)
	if !slices.Equal(analysisTags, listed) {
		t.Fatalf("analysisTags = %v, want GO_ANALYSIS_TAGS sorted, %v", analysisTags, listed)
	}
}
