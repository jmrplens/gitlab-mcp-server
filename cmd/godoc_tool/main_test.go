package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMainEntry_DelegatesExitCodeToRunMain verifies main passes the process
// arguments and streams to runMain and exits with the code it returns. It
// drives the osExit seam so the failing invocation (no subcommand) does not
// terminate the test process, and points the real streams at /dev/null so the
// usage text does not leak into the test output.
func TestMainEntry_DelegatesExitCodeToRunMain(t *testing.T) {
	origArgs, origExit, origStdout, origStderr := os.Args, osExit, os.Stdout, os.Stderr
	t.Cleanup(func() {
		os.Args, osExit, os.Stdout, os.Stderr = origArgs, origExit, origStdout, origStderr
	})

	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = devnull.Close() })
	os.Stdout, os.Stderr = devnull, devnull

	os.Args = []string{"godoc_tool"}
	gotCode, called := 0, false
	osExit = func(code int) { gotCode, called = code, true }

	main()

	if !called {
		t.Fatal("main() did not call osExit")
	}
	if gotCode != 2 {
		t.Errorf("main() exit code = %d, want 2", gotCode)
	}
}

// assertRunMainExit drives runMain with args and asserts both the exit code
// and that stderr contains wantStderr (checked only when non-empty).
func assertRunMainExit(t *testing.T, args []string, wantCode int, wantStderr string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := runMain(args, &stdout, &stderr); code != wantCode {
		t.Errorf("runMain(%v) = %d, want %d (stderr %q)", args, code, wantCode, stderr.String())
	}
	if wantStderr != "" && !strings.Contains(stderr.String(), wantStderr) {
		t.Errorf("runMain(%v) stderr = %q, want it to contain %q", args, stderr.String(), wantStderr)
	}
}

// TestRunMain_ExitCodesAndMessages verifies the exit paths that do not write to
// stdout: the usage message with no subcommand, an audit flag error, the fix
// subcommand's missing-path and per-path-failure codes, and the unknown
// subcommand message.
func TestRunMain_ExitCodesAndMessages(t *testing.T) {
	testCases := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
	}{
		{name: "no subcommand prints usage", args: []string{"godoc_tool"}, wantCode: 2, wantErr: "usage: godoc_tool <audit|fix>"},
		{name: "audit rejects a bad format", args: []string{"godoc_tool", "audit", "--format=xml"}, wantCode: 1, wantErr: "unsupported format"},
		{name: "fix without a path", args: []string{"godoc_tool", "fix"}, wantCode: 2, wantErr: "at least one file or directory path is required"},
		{name: "fix with an unprocessable path", args: []string{"godoc_tool", "fix", filepath.Join(t.TempDir(), "missing.go")}, wantCode: 1, wantErr: "stat "},
		{name: "unknown subcommand", args: []string{"godoc_tool", "bogus"}, wantCode: 2, wantErr: `unknown subcommand "bogus"`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { dryRun = false })
			assertRunMainExit(t, tc.args, tc.wantCode, tc.wantErr)
		})
	}
}

// TestRunMain_AuditSuccess verifies the audit subcommand returns 0 and writes
// its report to the stdout stream when the run succeeds.
func TestRunMain_AuditSuccess(t *testing.T) {
	dir := writeModuleFixture(t)
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	if code := runMain([]string{"godoc_tool", "audit", "--format=json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("runMain(audit) = %d, want 0 (stderr %q)", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "{") {
		t.Errorf("stdout = %q, want a JSON report", stdout.String())
	}
}

// TestRunMain_FixDocumentsPaths verifies the fix subcommand returns 0 both when
// it documents a file directly and when --move-package-doc routes the paths to
// the package-comment mover.
func TestRunMain_FixDocumentsPaths(t *testing.T) {
	t.Run("documents a file", func(t *testing.T) {
		t.Cleanup(func() { dryRun = false })
		path := writeFixFile(t, t.TempDir(), "sample.go", "package sample\n\nfunc ListProjects() {}\n")

		var stdout, stderr bytes.Buffer
		var code int
		out := captureFixStdout(t, func() {
			code = runMain([]string{"godoc_tool", "fix", path}, &stdout, &stderr)
		})
		if code != 0 {
			t.Errorf("runMain(fix path) = %d, want 0 (stderr %q)", code, stderr.String())
		}
		if !strings.Contains(out, "documented "+path) {
			t.Errorf("stdout = %q, want the documented message", out)
		}
	})

	t.Run("move-package-doc uses the mover", func(t *testing.T) {
		t.Cleanup(func() { dryRun = false })
		dir := t.TempDir()
		writeFixFile(t, dir, "sample.go", "package sample\n\nfunc ListProjects() {}\n")

		var stdout, stderr bytes.Buffer
		var code int
		captureFixStdout(t, func() {
			code = runMain([]string{"godoc_tool", "fix", "--move-package-doc", dir}, &stdout, &stderr)
		})
		if code != 0 {
			t.Errorf("runMain(fix --move-package-doc) = %d, want 0 (stderr %q)", code, stderr.String())
		}
	})
}
