// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// git_test.go covers the clone a release is rebuilt in, and the two answers
// read from git: the commit a ref names and the abbreviation of it.
package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitFixture runs git in dir on behalf of a test and returns what it printed
// on stdout. It is isolated from the machine's and the user's configuration,
// so no signing key, hook or line-ending rule of theirs reaches the fixture,
// and it commits under an identity of its own.
func gitFixture(t *testing.T, dir string, args ...string) string {
	t.Helper()

	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+global, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(string(out))
}

// commitAll commits everything in dir and returns the commit.
func commitAll(t *testing.T, dir, message string) string {
	t.Helper()

	gitFixture(t, dir, "add", "--all")
	gitFixture(t, dir, "commit", "--quiet", "--no-gpg-sign", "--allow-empty", "--message", message)
	return gitFixture(t, dir, "rev-parse", "HEAD")
}

// fixtureRepo makes a repository holding files in one commit tagged v1.2.3,
// and returns its directory and that commit.
func fixtureRepo(t *testing.T, files map[string]string) (dir, commit string) {
	t.Helper()

	dir = t.TempDir()
	writeFiles(t, dir, files)
	gitFixture(t, dir, "init", "--quiet")
	commit = commitAll(t, dir, "fixture")
	gitFixture(t, dir, "tag", "v1.2.3")
	return dir, commit
}

// TestCloneAt_ChecksOutTheCommitTheRefNames covers the clone a release is
// rebuilt in: it is at the commit the ref names rather than at the
// repository's head, the release's tag is there for Go to stamp the version
// from, and nothing in it differs from that commit, which Go would stamp as a
// modified tree.
func TestCloneAt_ChecksOutTheCommitTheRefNames(t *testing.T) {
	t.Parallel()

	repo, tagged := fixtureRepo(t, fixtureModule)
	writeFiles(t, repo, map[string]string{"later.txt": "after the release\n"})
	commitAll(t, repo, "later")

	dest := filepath.Join(t.TempDir(), "src")
	commit, err := cloneAt(context.Background(), repo, "v1.2.3", dest)
	if err != nil {
		t.Fatalf("cloneAt: %v", err)
	}
	if commit != tagged {
		t.Errorf("cloneAt = %s, want the tagged commit %s", commit, tagged)
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "head", args: []string{"rev-parse", "HEAD"}, want: tagged},
		{name: "tag", args: []string{"tag", "--points-at", "HEAD"}, want: "v1.2.3"},
		{name: "status", args: []string{"status", "--porcelain"}, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := gitFixture(t, dest, tc.args...); got != tc.want {
				t.Errorf("git %s in the clone = %q, want %q", strings.Join(tc.args, " "), got, tc.want)
			}
		})
	}
	if _, statErr := os.Stat(filepath.Join(dest, "later.txt")); !os.IsNotExist(statErr) {
		t.Errorf("the clone holds a file committed after the ref: %v", statErr)
	}
}

// TestCloneAt_ARefThatNamesNoCommit covers a ref git cannot resolve, which is
// named in the error before anything is cloned.
func TestCloneAt_ARefThatNamesNoCommit(t *testing.T) {
	t.Parallel()

	repo, _ := fixtureRepo(t, fixtureModule)
	dest := filepath.Join(t.TempDir(), "src")
	if _, err := cloneAt(context.Background(), repo, "v9.9.9", dest); err == nil || !strings.Contains(err.Error(), "v9.9.9 names no commit") {
		t.Errorf("cloneAt = %v, want the ref refused", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("a refused ref left a clone behind: %v", err)
	}
}

// TestCloneAt_ACloneThatCannotBeMade covers git refusing the clone itself,
// here because the destination already holds a file.
func TestCloneAt_ACloneThatCannotBeMade(t *testing.T) {
	t.Parallel()

	repo, _ := fixtureRepo(t, fixtureModule)
	dest := t.TempDir()
	writeFiles(t, dest, map[string]string{"occupied": "x"})
	if _, err := cloneAt(context.Background(), repo, "HEAD", dest); err == nil || !strings.Contains(err.Error(), "git clone") {
		t.Errorf("cloneAt = %v, want git's refusal of the clone", err)
	}
}

// TestGitOrNone_TheAnswerOrNone covers both answers: what git prints in a
// repository, and "none" where git has nothing to say, which is GoReleaser's
// own value for a commit outside a repository.
func TestGitOrNone_TheAnswerOrNone(t *testing.T) {
	t.Parallel()

	repo, commit := fixtureRepo(t, fixtureModule)
	if got := gitOrNone(context.Background(), repo, "rev-parse", "HEAD"); got != commit {
		t.Errorf("gitOrNone in a repository = %q, want %q", got, commit)
	}
	if got := gitOrNone(context.Background(), t.TempDir(), "rev-parse", "HEAD"); got != "none" {
		t.Errorf("gitOrNone outside a repository = %q, want none", got)
	}
}

// TestShortCommitOr_GivenOrAbbreviated covers the two sources of
// {{.ShortCommit}}: the value the command line gives, which wins, and git's
// own abbreviation of the head, which is what GoReleaser asks git for.
func TestShortCommitOr_GivenOrAbbreviated(t *testing.T) {
	t.Parallel()

	repo, commit := fixtureRepo(t, fixtureModule)
	if got := shortCommitOr(context.Background(), repo, "abcdef12"); got != "abcdef12" {
		t.Errorf("shortCommitOr with a value given = %q", got)
	}
	got := shortCommitOr(context.Background(), repo, "")
	if len(got) < 7 || !strings.HasPrefix(commit, got) {
		t.Errorf("shortCommitOr = %q, want an abbreviation of %s", got, commit)
	}
}

// TestRunGit_AFailureCarriesGitsOwnMessage covers the error a git command
// returns: the command it ran and what git said on stderr, which is the only
// place git says why.
func TestRunGit_AFailureCarriesGitsOwnMessage(t *testing.T) {
	t.Parallel()

	repo, _ := fixtureRepo(t, fixtureModule)
	_, err := runGit(context.Background(), repo, "rev-parse", "--verify", "absent")
	if err == nil {
		t.Fatal("runGit resolved a ref that does not exist")
	}
	for _, want := range []string{"git rev-parse --verify absent", "fatal:"} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(err.Error(), want) {
				t.Errorf("the error %q does not contain %q", err, want)
			}
		})
	}
}

// TestRunGit_NoGitOnPath_IsTheLookupsFailure verifies that git is found on
// PATH before it is run, and that a PATH holding none is reported as the
// lookup's failure rather than as a git that ran and refused.
func TestRunGit_NoGitOnPath_IsTheLookupsFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if _, err := runGit(context.Background(), t.TempDir(), "--version"); err == nil || !strings.HasPrefix(err.Error(), "find git: ") {
		t.Errorf("runGit() with no git on PATH = %v, want the lookup's failure", err)
	}
}
