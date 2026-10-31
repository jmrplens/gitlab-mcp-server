// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// main_test.go covers the command line: which check a run makes, the usage
// errors refused before either starts, and the exit code main hands the
// process.
package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestRunMain_RunsTheCheckItIsNamed covers the first argument: each check is
// reached by its name, help is not an error, and no check or an unknown one
// is refused with exit 2 and the usage.
func TestRunMain_RunsTheCheckItIsNamed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		args   []string
		want   int
		stdout string
		stderr string
	}{
		{name: "no check", args: nil, want: 2, stderr: "usage: release_checks order TAG"},
		{name: "an unknown check", args: []string{"interp"}, want: 2, stderr: `release_checks: unknown check "interp"`},
		{name: "a flag where the check goes", args: []string{"-v", "order"}, want: 2, stderr: `unknown check "-v"`},
		{name: "-h", args: []string{"-h"}, want: 0, stdout: "usage: release_checks order TAG"},
		{name: "-help", args: []string{"-help"}, want: 0, stdout: "release_checks elf-interp BINARY"},
		{name: "--help", args: []string{"--help"}, want: 0, stdout: "release_checks elf-interp BINARY"},
		{name: "help", args: []string{"help"}, want: 0, stdout: "release_checks elf-interp BINARY"},
		{name: "order", args: []string{"order", "v1.0.0"}, want: 0, stdout: "newest=true"},
		{name: "elf-interp", args: []string{"elf-interp"}, want: 2, stderr: "usage: release_checks elf-interp BINARY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code := runMain(tc.args, strings.NewReader(""), &stdout, &stderr)
			if code != tc.want {
				t.Errorf("runMain(%q) = %d, want %d (stderr %q)", tc.args, code, tc.want, stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.stdout) || !strings.Contains(stderr.String(), tc.stderr) {
				t.Errorf("runMain(%q) wrote %q on stdout and %q on stderr, want %q and %q", tc.args, stdout.String(), stderr.String(), tc.stdout, tc.stderr)
			}
			if tc.stdout == "" && stdout.Len() != 0 {
				t.Errorf("runMain(%q) wrote %q on stdout, want nothing", tc.args, stdout.String())
			}
		})
	}
}

// TestMain_HandsTheExitCodeToTheProcess covers main itself: the code runMain
// decides is the one the process exits with. It replaces os.Args and the exit
// seam, so it does not run in parallel.
func TestMain_HandsTheExitCodeToTheProcess(t *testing.T) {
	previousArgs, previousExit := os.Args, exitProcess
	t.Cleanup(func() { os.Args, exitProcess = previousArgs, previousExit })

	os.Args = []string{toolName}
	code := -1
	exitProcess = func(c int) { code = c }
	main()
	if code != 2 {
		t.Errorf("main() exited %d with no check named, want 2", code)
	}
}
