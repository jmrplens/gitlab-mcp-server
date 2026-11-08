// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// main_test.go covers the command line: the subcommands it knows, the usage
// it prints, and the exit code main hands the process.
package main

import (
	"bytes"
	"os"
	"testing"
)

// TestRun_TheCommandLine_IsReadAsTheScriptReadIt verifies the usage paths:
// help on stdout with 0, and no subcommand, an unknown one, or arguments the
// description half does not take, on stderr with 2.
func TestRun_TheCommandLine_IsReadAsTheScriptReadIt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{name: "help", args: []string{"help"}, code: 0, stdout: usage},
		{name: "-h", args: []string{"-h"}, code: 0, stdout: usage},
		{name: "--help", args: []string{"--help"}, code: 0, stdout: usage},
		{name: "nothing", code: 2, stderr: usage},
		{name: "an unknown subcommand", args: []string{"lines"}, code: 2, stderr: usage},
		{name: "a description with an argument", args: []string{"description", "7"}, code: 2, stderr: usage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code := run(t.Context(), tc.args, t.TempDir(), environment(nil), &stdout, &stderr)
			if code != tc.code || stdout.String() != tc.stdout || stderr.String() != tc.stderr {
				t.Errorf("exit %d, stdout %q, stderr %q, want %d, %q and %q", code, stdout.String(), stderr.String(), tc.code, tc.stdout, tc.stderr)
			}
		})
	}
}

// TestUsage_NamesBothSubcommandsAndTheirVariables verifies that the usage
// says how each half is pointed at what it judges.
func TestUsage_NamesBothSubcommandsAndTheirVariables(t *testing.T) {
	t.Parallel()

	if absent := lacking(usage, "audit_pr_text diff [BASE_REF]", "audit_pr_text description", "$EM_DASH_BASE", "$PR_NUMBER", "PR_TITLE_FILE", "PR_BODY_FILE"); len(absent) > 0 {
		t.Errorf("usage lacks %q", absent)
	}
}

// TestMain_HandsTheExitCodeToTheProcess covers main, which reads its
// arguments from the process: with none it prints the usage and exits 2. It
// changes os.Args and the exit seam, so it does not run in parallel.
func TestMain_HandsTheExitCodeToTheProcess(t *testing.T) {
	previousExit, previousArgs := exitProcess, os.Args
	t.Cleanup(func() { exitProcess, os.Args = previousExit, previousArgs })
	os.Args = []string{"audit_pr_text"}

	code := -1
	exitProcess = func(c int) { code = c }
	main()
	if code != 2 {
		t.Errorf("main exited %d with no arguments, want 2", code)
	}
}
