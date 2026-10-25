// main_test.go covers the command line: which arguments make a run, which
// are refused before one starts, and the exit code main hands the process.
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunMain_ArgumentsItCannotRead covers the usage errors: -h is not one,
// and an unknown flag, a positional argument, a mode missing or doubled, a
// release without its tag, a ref beside -twice and a relative -out are each
// refused with exit 2 before anything runs.
func TestRunMain_ArgumentsItCannotRead(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
		want int
		says string
	}{
		{name: "help", args: []string{"-h"}, want: 0},
		{name: "an unknown flag", args: []string{"-nope"}, want: 2},
		{name: "a positional argument", args: []string{"-twice", "./cmd/server"}, want: 2, says: "unexpected arguments"},
		{name: "no mode", args: nil, want: 2, says: "exactly one of -published and -twice"},
		{name: "both modes", args: []string{"-twice", "-published", "dist"}, want: 2, says: "exactly one of -published and -twice"},
		{name: "a release without its tag", args: []string{"-published", "dist"}, want: 2, says: "-published needs -tag"},
		{name: "a ref beside -twice", args: []string{"-twice", "-ref", "v1.2.3"}, want: 2, says: "-ref names the commit"},
		{name: "a relative -out", args: []string{"-twice", "-out", "dist/x"}, want: 2, says: "-out must be an absolute path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			if code := runMain(context.Background(), tc.args, &stdout, &stderr); code != tc.want || !strings.Contains(stderr.String(), tc.says) {
				t.Errorf("runMain(%q) = %d with %q on stderr, want %d and %q", tc.args, code, stderr.String(), tc.want, tc.says)
			}
		})
	}
}

// TestRunMain_RunsTheModeItIsGiven covers each mode reached from the command
// line, stopped at its first step so nothing is built: a release rebuilt
// from a directory that is no repository, and a tree with no VERSION built
// twice with no tag.
func TestRunMain_RunsTheModeItIsGiven(t *testing.T) {
	t.Parallel()

	empty := t.TempDir()
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "against a release", args: []string{"-dir", empty, "-published", empty, "-tag", "v1.2.3"}, want: "HEAD names no commit"},
		{name: "twice", args: []string{"-dir", empty, "-twice", "-out", empty}, want: "no -tag given"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			if code := runMain(context.Background(), tc.args, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("runMain(%q) = %d with %q on stderr, want 1 and %q", tc.args, code, stderr.String(), tc.want)
			}
		})
	}
}

// TestMain_HandsTheExitCodeToTheProcess covers main itself through the exit
// seam, with a tree that has nothing to build so it stops at once.
func TestMain_HandsTheExitCodeToTheProcess(t *testing.T) {
	previousArgs, previousExit := os.Args, exitProcess
	t.Cleanup(func() { os.Args, exitProcess = previousArgs, previousExit })

	os.Args = []string{toolName, "-twice", "-dir", filepath.Join(t.TempDir(), "absent")}
	code := -1
	exitProcess = func(c int) { code = c }
	main()
	if code != 1 {
		t.Fatalf("main exited %d, want 1 for a tree that does not exist", code)
	}
}
