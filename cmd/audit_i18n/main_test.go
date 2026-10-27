// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
)

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// runPlanted runs the command over a planted module and returns the exit code
// with what each stream said.
func runPlanted(t *testing.T, cfg auditConfig, check, verbose bool) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut strings.Builder
	code = run(cfg, check, verbose, &out, &errOut)
	return code, out.String(), errOut.String()
}

// TestRun_FindingsTree_ReportsAndFailsOnlyUnderCheck: the report is worth
// having without the gate, so a plain run prints the findings and exits 0,
// and under -check the same report fails.
func TestRun_FindingsTree_ReportsAndFailsOnlyUnderCheck(t *testing.T) {
	cfg := plantedConfig(t, "findings", findingsDeclarations)
	cases := []struct {
		name  string
		check bool
		want  int
	}{
		{name: "a report", check: false, want: 0},
		{name: "the gate", check: true, want: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runPlanted(t, cfg, tc.check, false)
			if code != tc.want {
				t.Errorf("exit = %d, want %d", code, tc.want)
			}
			if !strings.HasPrefix(stdout, "cmd/tool/main.go:14: I2 converts") || !strings.HasSuffix(stdout, "28 findings, 2 excused by a declaration\n") {
				t.Errorf("stdout =\n%s", stdout)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want empty: a finding is not a broken run", stderr)
			}
		})
	}
}

// TestRun_CleanTree_SucceedsUnderCheck. The output is held whole: a quiet
// clean run is the one summary line and nothing before it.
func TestRun_CleanTree_SucceedsUnderCheck(t *testing.T) {
	code, stdout, stderr := runPlanted(t, plantedConfig(t, "clean", nil), true, false)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stdout = %s", code, stdout)
	}
	want := "audit_i18n: 7 renders of the message catalog in 3 packages, against 5 English templates; 0 findings, 0 excused by a declaration\n"
	if stdout != want || stderr != "" {
		t.Errorf("stdout = %q, stderr = %q, want %q and nothing", stdout, stderr, want)
	}
}

// TestRun_TreeThatCannotBeLoaded_FailsOnStderr: a run that could not be made
// says so on stderr under the gate's name and writes no report, whether the
// patterns name nothing or the catalog package is not there.
func TestRun_TreeThatCannotBeLoaded_FailsOnStderr(t *testing.T) {
	cases := []struct {
		name string
		cfg  auditConfig
	}{
		{name: "a directory that is not there", cfg: auditConfig{dir: filepath.Join(t.TempDir(), "absent"), patterns: defaultPatterns, catalog: catalogPath}},
		{name: "a module with no catalog package", cfg: auditConfig{dir: plantedDir(t, "clean"), patterns: defaultPatterns, catalog: "example.com/clean/internal/missing"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runPlanted(t, tc.cfg, true, false)
			if code != 1 || stdout != "" || !strings.HasPrefix(stderr, toolName+": ") {
				t.Errorf("exit = %d, stdout = %q, stderr = %q; want 1, nothing and the gate's complaint", code, stdout, stderr)
			}
		})
	}
}

// TestAudit_RootThatCannotBeResolved_FailsRatherThanLoading: the root is made
// absolute before anything is loaded. Only Linux can be made to fail that
// way, so elsewhere the case is skipped.
func TestAudit_RootThatCannotBeResolved_FailsRatherThanLoading(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o750); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	t.Chdir(gone)
	if err := os.Remove(gone); err != nil {
		t.Skipf("this platform will not remove the working directory: %v", err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("this platform's getcwd still answers after the working directory is removed")
	}
	if _, err := audit(auditConfig{dir: ".", patterns: defaultPatterns, catalog: catalogPath}); err == nil {
		t.Fatal("audit error = nil, want the unresolved root")
	}
}

// TestRunMain_ParsesTheCommandLine holds what each argument does: an unknown
// flag is a usage error, a help request is not a failure, and the patterns
// and flags reach the run they describe, over the real catalog and the real
// declaration table.
func TestRunMain_ParsesTheCommandLine(t *testing.T) {
	root := repoRoot(t)
	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "unknown flag", args: []string{"-nope"}, wantCode: 2, wantStderr: "flag provided but not defined: -nope\nUsage of audit_i18n:\n"},
		{name: "help", args: []string{"-h"}, wantCode: 0, wantStderr: "Usage of audit_i18n:\n"},
		{
			name: "one package under check and verbose", args: []string{"-dir", root, "-check", "-v", "./cmd/audit_md_escaping"}, wantCode: 0,
			wantStdout: "cmd/audit_md_escaping/catalog.go:",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			if code := runMain(tc.args, &stdout, &stderr); code != tc.wantCode {
				t.Fatalf("exit = %d, want %d; stderr = %q", code, tc.wantCode, stderr.String())
			}
			assertBegins(t, "stdout", stdout.String(), tc.wantStdout)
			assertBegins(t, "stderr", stderr.String(), tc.wantStderr)
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

// TestMain_ExitsWithTheCodeRunMainDecided: main hands runMain's code to the
// process through the exit seam and does nothing else.
func TestMain_ExitsWithTheCodeRunMainDecided(t *testing.T) {
	previous, previousArgs := sourcegate.Exit, os.Args
	t.Cleanup(func() { sourcegate.Exit, os.Args = previous, previousArgs })
	var codes []int
	sourcegate.Exit = func(code int) { codes = append(codes, code) }
	os.Args = []string{toolName, "-nope"}

	main()

	if !slices.Equal(codes, []int{2}) {
		t.Fatalf("exit codes = %v, want the 2 runMain returned for an unknown flag", codes)
	}
}

// TestMain_WritesTheReportToStdoutAndTheComplaintToStderr: main hands runMain
// the process's standard output and standard error in that order. Each case
// is held on both streams, because handed over the other way round the text
// is not lost, only moved.
func TestMain_WritesTheReportToStdoutAndTheComplaintToStderr(t *testing.T) {
	root := repoRoot(t)
	previousExit, previousArgs := sourcegate.Exit, os.Args
	previousStdout, previousStderr := os.Stdout, os.Stderr
	t.Cleanup(func() {
		sourcegate.Exit, os.Args = previousExit, previousArgs
		os.Stdout, os.Stderr = previousStdout, previousStderr
	})
	sourcegate.Exit = func(int) {}
	cases := []struct {
		name       string
		args       []string
		wantStdout string
		wantStderr string
	}{
		{name: "a report", args: []string{"-dir", root, "./cmd/audit_md_escaping"}, wantStdout: "audit_i18n: 0 renders of the message catalog in 2 packages"},
		{name: "an unknown flag", args: []string{"-nope"}, wantStderr: "flag provided but not defined: -nope\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			stdout := createFile(t, filepath.Join(dir, "stdout"))
			stderr := createFile(t, filepath.Join(dir, "stderr"))
			os.Args = append([]string{toolName}, tc.args...)
			os.Stdout, os.Stderr = stdout, stderr
			main()
			os.Stdout, os.Stderr = previousStdout, previousStderr
			assertBegins(t, "stdout", closeAndRead(t, stdout), tc.wantStdout)
			assertBegins(t, "stderr", closeAndRead(t, stderr), tc.wantStderr)
		})
	}
}

// createFile creates an empty file to stand in for one of the process's
// streams.
func createFile(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("prepare %s: %v", path, err)
	}
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

// TestDefaultPatterns_CoverTheRepositorysLibrarySource states what the gate
// is over, so narrowing it is a visible change rather than a quiet one.
func TestDefaultPatterns_CoverTheRepositorysLibrarySource(t *testing.T) {
	if want := []string{"./internal/...", "./cmd/..."}; !slices.Equal(defaultPatterns, want) {
		t.Fatalf("defaultPatterns = %v, want %v", defaultPatterns, want)
	}
}

// TestCatalogPath_UnderTheModulePath_MovesWithIt holds the written-out path
// to the module, so a module move still has one place to land.
func TestCatalogPath_UnderTheModulePath_MovesWithIt(t *testing.T) {
	if want := goprogram.ModulePath + "/internal/i18n"; catalogPath != want {
		t.Errorf("catalogPath = %q, want %q", catalogPath, want)
	}
}

// TestAudit_PatternsTheCallerPassed_AreNotChanged: the catalog is loaded
// beside the patterns without writing into the caller's slice, which is the
// package's own default table on every run the command line makes.
func TestAudit_PatternsTheCallerPassed_AreNotChanged(t *testing.T) {
	backing := []string{"./internal/reader", "untouched"}
	cfg := plantedConfig(t, "findings", nil)
	cfg.patterns = backing[:1]
	if _, err := audit(cfg); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if backing[1] != "untouched" {
		t.Errorf("the caller's patterns were written past their length: %v", backing)
	}
}
