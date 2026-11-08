// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// diff_test.go holds the diff half (make check-em-dash) to the lines a
// branch adds, in real repositories git writes for each case, and the
// reading of git's diff to the shapes it can take.
package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/committext"
)

// dash is the character the gate refuses, never written out in this file.
var dash = committext.EmDash

// git runs git in dir for a fixture, isolated from the machine's and the
// user's configuration, and returns its stdout trimmed.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	global := filepath.Join(t.TempDir(), "gitconfig")
	writeFile(t, global, nil)
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+global, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.com",
		"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.com")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(string(out))
}

// commit writes files (a nil content removes the file), stages everything
// and commits it, and returns the commit.
func commit(t *testing.T, dir string, files map[string]*string) string {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if content == nil {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		writeFile(t, path, []byte(*content))
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "--quiet", "--allow-empty", "--no-gpg-sign", "-m", "fixture")
	return git(t, dir, "rev-parse", "HEAD")
}

// text is the address of s, for commit's file map.
func text(s string) *string { return &s }

// newRepository creates a repository whose first commit holds every path
// the gate excludes, as this tree does, plus files, and returns it with that
// commit, the base every case diffs against.
func newRepository(t *testing.T, files map[string]*string) (dir, base string) {
	t.Helper()
	dir = t.TempDir()
	git(t, dir, "init", "--quiet")
	all := map[string]*string{}
	for _, path := range excludedPaths {
		all[path] = text("generated\n")
	}
	for name, content := range files {
		all[name] = content
	}
	return dir, commit(t, dir, all)
}

// checkDiff runs the diff half in dir with args and vars, and returns its
// exit code and both streams.
func checkDiff(t *testing.T, dir string, vars map[string]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(t.Context(), append([]string{"diff"}, args...), dir, environment(vars), &out, &errOut)
	return code, out.String(), errOut.String()
}

// TestDiff_ABranchAddingNoEmDash_Passes verifies the passing answer, which
// names the range it judged.
func TestDiff_ABranchAddingNoEmDash_Passes(t *testing.T) {
	t.Parallel()

	dir, base := newRepository(t, map[string]*string{"README.md": text("one\n")})
	commit(t, dir, map[string]*string{"README.md": text("one\ntwo, with a comma\n")})
	code, stdout, stderr := checkDiff(t, dir, nil, base)
	if code != 0 || stdout != "OK: no line added between "+base+" and HEAD carries an em dash.\n" || stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want 0 and the OK line", code, stdout, stderr)
	}
}

// TestDiff_AddedLinesCarryingAnEmDash_AreReportedWhereTheyAre verifies the
// finding: each added line with its file and its number in the new file, in
// the order of the diff, and the failure that counts them.
func TestDiff_AddedLinesCarryingAnEmDash_AreReportedWhereTheyAre(t *testing.T) {
	t.Parallel()

	dir, base := newRepository(t, map[string]*string{"docs/a.md": text("one\ntwo\nthree\n")})
	commit(t, dir, map[string]*string{
		"docs/a.md":   text("one\ntwo " + dash + " inserted\nthree\nfour " + dash + "\n"),
		"cmd/x/x.go":  text("package x\n\n// A comment " + dash + " here.\n"),
		"docs/new.md": text("clean\n"),
	})
	code, stdout, stderr := checkDiff(t, dir, nil, base)
	want := "cmd/x/x.go:3: // A comment " + dash + " here.\n" +
		"docs/a.md:2: two " + dash + " inserted\n" +
		"docs/a.md:4: four " + dash + "\n"
	if code != 1 || stdout != want {
		t.Errorf("exit %d, stdout\n%s\nwant 1 and\n%s", code, stdout, want)
	}
	if !strings.Contains(stderr, "FAIL: 3 added line(s) carry an em dash (U+2014).") || !strings.Contains(stderr, "make check-em-dash") {
		t.Errorf("stderr does not count the findings and say how to re-check:\n%s", stderr)
	}
	if strings.Contains(stdout+stderr, "::error") {
		t.Errorf("an annotation was written off GitHub Actions:\n%s%s", stdout, stderr)
	}
}

// TestDiff_OnGitHubActions_AnnotatesEachFinding verifies that a finding
// lands on the diff the author is looking at, and not only in the log.
func TestDiff_OnGitHubActions_AnnotatesEachFinding(t *testing.T) {
	t.Parallel()

	dir, base := newRepository(t, nil)
	commit(t, dir, map[string]*string{"a.md": text("x " + dash + "\n")})
	_, stdout, _ := checkDiff(t, dir, map[string]string{"GITHUB_ACTIONS": "true"}, base)
	want := "a.md:1: x " + dash + "\n::error file=a.md,line=1::this line adds an em dash (U+2014); rewrite it\n"
	if stdout != want {
		t.Errorf("stdout %q, want %q", stdout, want)
	}
}

// TestDiff_AnEmDashAddedAndRemovedOnTheBranch_Passes verifies that the
// branch is judged net, so a dash its tip cleans up is not a finding.
func TestDiff_AnEmDashAddedAndRemovedOnTheBranch_Passes(t *testing.T) {
	t.Parallel()

	dir, base := newRepository(t, map[string]*string{"a.md": text("one\n")})
	commit(t, dir, map[string]*string{"a.md": text("one\ntwo " + dash + "\n")})
	commit(t, dir, map[string]*string{"a.md": text("one\ntwo, then\n")})
	if code, stdout, stderr := checkDiff(t, dir, nil, base); code != 0 {
		t.Errorf("exit %d, want 0\n%s%s", code, stdout, stderr)
	}
}

// TestDiff_WhatTheBranchDidNotWrite_IsNotJudged verifies the dashes already
// there: a line beside one that the branch edits, a file the branch moves, a
// file it deletes, and a path the gate excludes.
func TestDiff_WhatTheBranchDidNotWrite_IsNotJudged(t *testing.T) {
	t.Parallel()

	carried := "old " + dash + " prose\nmore old prose that makes the file long enough to be found moved\nand a third line\n"
	dir, base := newRepository(t, map[string]*string{
		"edited.md":  text("old " + dash + " prose\nsecond\n"),
		"moved.md":   text(carried),
		"deleted.md": text("gone " + dash + "\n"),
	})
	commit(t, dir, map[string]*string{
		"edited.md":         text("old " + dash + " prose\nsecond, edited\n"),
		"moved.md":          nil,
		"elsewhere/kept.md": text(carried),
		"deleted.md":        nil,
		excludedPaths[0]:    text("generated " + dash + "\n"),
	})
	if code, stdout, stderr := checkDiff(t, dir, nil, base); code != 0 {
		t.Errorf("exit %d, want 0\n%s%s", code, stdout, stderr)
	}
}

// TestDiff_AnAddedLineThatLooksLikeAFileHeader_IsStillJudged verifies the
// hunk state: an added line whose own text starts with "++ " reads as a file
// header to a parser that does not track it, which would then skip every
// line after it in the file.
func TestDiff_AnAddedLineThatLooksLikeAFileHeader_IsStillJudged(t *testing.T) {
	t.Parallel()

	dir, base := newRepository(t, nil)
	commit(t, dir, map[string]*string{"a.md": text("++ not a header\nafter " + dash + "\n")})
	code, stdout, _ := checkDiff(t, dir, nil, base)
	if code != 1 || stdout != "a.md:2: after "+dash+"\n" {
		t.Errorf("exit %d, stdout %q, want 1 and the line after the look-alike", code, stdout)
	}
}

// TestDiff_TheBase_IsTheArgumentThenTheVariableThenOriginMain verifies where
// the base comes from: an argument wins, $EM_DASH_BASE comes next, and
// origin/main is the default, which a repository without it cannot resolve.
func TestDiff_TheBase_IsTheArgumentThenTheVariableThenOriginMain(t *testing.T) {
	t.Parallel()

	dir, base := newRepository(t, nil)
	commit(t, dir, map[string]*string{"a.md": text("x " + dash + "\n")})
	head := git(t, dir, "rev-parse", "HEAD")

	if code, _, _ := checkDiff(t, dir, map[string]string{"EM_DASH_BASE": base}, head); code != 0 {
		t.Errorf("the argument HEAD lost to the variable: exit %d, want 0", code)
	}
	if code, stdout, _ := checkDiff(t, dir, map[string]string{"EM_DASH_BASE": base}); code != 1 || !strings.Contains(stdout, "a.md:1:") {
		t.Errorf("the variable was not used: exit %d, stdout %q", code, stdout)
	}
	code, _, stderr := checkDiff(t, dir, nil)
	want := "ERROR: cannot resolve the base commit 'origin/main'.\n" +
		"Fetch it (git fetch origin main), or name another: make check-em-dash EM_DASH_BASE=<ref>.\n"
	if code != 1 || stderr != want {
		t.Errorf("exit %d, stderr %q, want 1 and %q", code, stderr, want)
	}
}

// TestDiff_ARangeGitCannotDiff_Fails verifies that git's own refusal is the
// answer: two histories with no merge base have no three-dot diff.
func TestDiff_ARangeGitCannotDiff_Fails(t *testing.T) {
	t.Parallel()

	dir, _ := newRepository(t, nil)
	git(t, dir, "checkout", "--quiet", "--orphan", "other")
	other := commit(t, dir, map[string]*string{"b.md": text("b\n")})
	git(t, dir, "checkout", "--quiet", "-")
	code, _, stderr := checkDiff(t, dir, nil, other)
	if code != 1 || !strings.HasPrefix(stderr, "ERROR: git diff "+other+"...HEAD: ") {
		t.Errorf("exit %d, stderr %q, want 1 and git's refusal", code, stderr)
	}
}

// TestDiff_AnExclusionThatNamesNoFile_Fails verifies the declaration rule: an
// entry that excuses nothing is stale, and a stale one is how a real finding
// gets excused later by accident.
func TestDiff_AnExclusionThatNamesNoFile_Fails(t *testing.T) {
	t.Parallel()

	dir, base := newRepository(t, nil)
	commit(t, dir, map[string]*string{excludedPaths[1]: nil})
	code, stdout, stderr := checkDiff(t, dir, nil, base)
	want := "ERROR: the exclusion list names " + excludedPaths[1] + ", which is not in the tree.\n" +
		"Remove the entry from excludedPaths in cmd/audit_pr_text/diff.go, or restore the file.\n"
	if code != 1 || stdout != "" || stderr != want {
		t.Errorf("exit %d, stdout %q, stderr %q, want 1 and %q", code, stdout, stderr, want)
	}
}

// TestDiff_TwoBases_IsAUsageError verifies that the command line names one
// base at most.
func TestDiff_TwoBases_IsAUsageError(t *testing.T) {
	t.Parallel()

	code, _, stderr := checkDiff(t, t.TempDir(), nil, "a", "b")
	if code != 2 || !strings.HasPrefix(stderr, "Usage:") {
		t.Errorf("exit %d, stderr %q, want 2 and the usage", code, stderr)
	}
}

// TestParseDiff_EachShapeGitWrites_IsRead verifies the reader over the
// shapes a real repository cannot always produce on every platform: a
// quoted path, a deleted file, a pure deletion hunk, a no-newline marker and
// a hunk header whose context carries a plus sign.
func TestParseDiff_EachShapeGitWrites_IsRead(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/gone.md b/gone.md\n" +
		"deleted file mode 100644\n" +
		"--- a/gone.md\n" +
		"+++ /dev/null\n" +
		"@@ -1 +0,0 @@\n" +
		"-old " + dash + "\n" +
		"diff --git \"a/odd\\tname.md\" \"b/odd\\tname.md\"\n" +
		"--- \"a/odd\\tname.md\"\n" +
		"+++ \"b/odd\\tname.md\"\n" +
		"@@ -3,0 +4,2 @@ ## A heading +5\n" +
		"+new " + dash + "\n" +
		"+second " + dash + "\n" +
		"\\ No newline at end of file\n" +
		"diff --git a/kept.md b/kept.md\n" +
		"--- a/kept.md\n" +
		"+++ b/kept.md\n" +
		"@@ -2,2 +1,0 @@\n" +
		"-was " + dash + "\n" +
		"-also\n" +
		"@@ -9 +8 @@\n" +
		"-before\n" +
		"+after " + dash + "\n" +
		"diff --git \"a/bad\\q.md\" \"b/bad\\q.md\"\n" +
		"+++ \"b/bad\\q.md\n" +
		"@@ -0,0 +1 @@\n" +
		"+unreadable path " + dash + "\n"
	got := parseDiff([]byte(diff))
	want := []finding{
		{path: "odd\tname.md", line: 4, text: "new " + dash},
		{path: "odd\tname.md", line: 5, text: "second " + dash},
		{path: "kept.md", line: 8, text: "after " + dash},
		// A path git quoted in a way that does not decode is reported as git
		// wrote it, rather than dropped with its findings.
		{path: `"b/bad\q.md`, line: 1, text: "unreadable path " + dash},
	}
	if !slices.Equal(got, want) {
		t.Errorf("findings %q, want %q", got, want)
	}
}

// TestGitCommand_RunsGitInTheDirectory verifies the seam's own answer
// without starting it.
func TestGitCommand_RunsGitInTheDirectory(t *testing.T) {
	t.Parallel()

	cmd := gitCommand(t.Context(), "/some/dir", "rev-parse", "HEAD")
	if got := strings.Join(cmd.Args, " "); got != "git rev-parse HEAD" || cmd.Dir != "/some/dir" {
		t.Errorf("command %q in %q, want git rev-parse HEAD in /some/dir", got, cmd.Dir)
	}
}
