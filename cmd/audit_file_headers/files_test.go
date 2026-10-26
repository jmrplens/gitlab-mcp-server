// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
)

// isolateGit points every git the test runs, the command's own included, at
// an empty global configuration, no system one and an empty user
// configuration directory, so no ignore file, attributes file, hook or
// template of the machine's or the user's reaches a fixture. The directory is
// needed beside the empty configuration because git reads the ignore and
// attributes files in it by default, without any configuration naming them.
func isolateGit(t *testing.T) {
	t.Helper()

	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

// gitFixture runs git in dir on behalf of a test and fails the test when git
// does.
func gitFixture(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
}

// newRepository returns a fresh repository holding files, each at its
// slash-separated path with its content, none of them added to the index.
func newRepository(t *testing.T, files map[string]string) string {
	t.Helper()

	isolateGit(t)
	dir := t.TempDir()
	gitFixture(t, dir, "init", "--quiet")
	writeFiles(t, dir, files)
	return dir
}

// writeFiles writes each file under dir, creating its directories.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestListFiles_Repository_ListsWhatGitHoldsAndWouldAdd verifies the files a
// run judges: the ones in git's index and the untracked ones git would add,
// sorted as one list although git prints the untracked ones first, and not
// the ones .gitignore excludes or the files of a checkout nested inside the
// tree, which git names as the directory alone.
func TestListFiles_Repository_ListsWhatGitHoldsAndWouldAdd(t *testing.T) {
	dir := newRepository(t, map[string]string{
		".gitignore":           "dist/\n*.tmp\n",
		"b/tracked.go":         "package b\n",
		"a/untracked.sh":       "echo a\n",
		"dist/built.js":        "a\n",
		"scratch.tmp":          "a\n",
		".github/scripts/x.sh": "echo x\n",
	})
	gitFixture(t, dir, "add", ".gitignore", "b/tracked.go")
	nested := filepath.Join(dir, "nested")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, nested, "init", "--quiet")
	writeFiles(t, nested, map[string]string{"inner.go": "package inner\n"})

	got, err := listFiles(context.Background(), dir)
	if err != nil {
		t.Fatalf("listFiles() error = %v", err)
	}
	want := []string{".github/scripts/x.sh", ".gitignore", "a/untracked.sh", "b/tracked.go", "nested/"}
	if !slices.Equal(got, want) {
		t.Errorf("listFiles() = %q, want %q", got, want)
	}
}

// TestListFiles_UserIgnoreFile_DoesNotReachTheFixture verifies the ignore
// file git reads by default from the user's configuration directory, which
// no global configuration names and which an empty one therefore does not
// turn off, stays away from a fixture: a contributor whose own ignore file
// names a kind of file the tests plant gets the answer everybody else gets.
func TestListFiles_UserIgnoreFile_DoesNotReachTheFixture(t *testing.T) {
	user := t.TempDir()
	writeFiles(t, user, map[string]string{"git/ignore": "*.sh\n"})
	t.Setenv("XDG_CONFIG_HOME", user)
	dir := newRepository(t, map[string]string{"a.sh": "echo a\n", "b.go": "package b\n"})

	got, err := listFiles(context.Background(), dir)
	if err != nil {
		t.Fatalf("listFiles() error = %v", err)
	}
	if want := []string{"a.sh", "b.go"}; !slices.Equal(got, want) {
		t.Errorf("listFiles() = %q, want %q", got, want)
	}
}

// TestListFiles_EmptyRepository_ListsNothing verifies a repository with no
// files is an empty list rather than one empty name.
func TestListFiles_EmptyRepository_ListsNothing(t *testing.T) {
	got, err := listFiles(context.Background(), newRepository(t, nil))
	if err != nil {
		t.Fatalf("listFiles() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("listFiles() = %q, want nothing", got)
	}
}

// TestListFiles_NotARepository_SaysWhatGitSaid verifies a directory git
// cannot read as a repository is an error carrying git's own reason.
func TestListFiles_NotARepository_SaysWhatGitSaid(t *testing.T) {
	isolateGit(t)
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	_, err := listFiles(context.Background(), dir)
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("listFiles() error = %v, want git's reason", err)
	}
}

// TestListFiles_NoGitOnPath_IsTheLookupsFailure verifies that git is found on
// PATH before it is run, and that a PATH holding none is reported as the
// lookup's failure rather than as a git that ran and refused.
func TestListFiles_NoGitOnPath_IsTheLookupsFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if _, err := listFiles(context.Background(), t.TempDir()); err == nil || !strings.HasPrefix(err.Error(), "find git: ") {
		t.Errorf("listFiles() with no git on PATH = %v, want the lookup's failure", err)
	}
}
