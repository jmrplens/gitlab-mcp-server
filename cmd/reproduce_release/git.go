// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// gitQuiet keeps a git command here from printing what nothing reads: the
// diff git show would print after the commit, and the progress of a clone and
// a checkout.
const gitQuiet = "--quiet"

// runGit runs git in dir and returns what it printed on stdout, trimmed. A
// failure carries what git said on stderr, which is the only place it says
// why.
//
// The program is resolved to an absolute path first, which is how every other
// command here runs a tool it did not build (cmd/gen_orbit_record): the
// lookup is the one exec would do, decided here rather than by whatever PATH
// holds when the command starts.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("find git: %w", err)
	}
	// #nosec G204 G702 -- the program is the absolute path just resolved, with arguments this command builds from the operator's own -dir and -ref
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// gitOrNone is what git prints in dir, or "none" when git has nothing to say
// there (no repository, or no git), which is what GoReleaser renders a commit
// as outside a repository.
func gitOrNone(ctx context.Context, dir string, args ...string) string {
	if out, err := runGit(ctx, dir, args...); err == nil {
		return out
	}
	return "none"
}

// shortCommitOr is what {{.ShortCommit}} renders as: given when the command
// line gives it, and otherwise git's own abbreviation of the head of dir,
// asked the way GoReleaser asks it (internal/pipe/git, getCommit). The
// abbreviation's length depends on how many objects the clone holds, so a
// release stamped with it may need the value it was stamped with given back.
func shortCommitOr(ctx context.Context, dir, given string) string {
	if given != "" {
		return given
	}
	return gitOrNone(ctx, dir, "show", "--format=%h", gitQuiet, "HEAD")
}

// cloneAt clones repo into dest with the commit ref names checked out, and
// returns that commit.
//
// The clone shares repo's objects rather than copying them, and carries its
// tags, which is what the go command stamps a tagged commit's version from.
// It is a clone rather than the operator's own checkout so that nothing in
// it differs from the commit: the go command stamps a tree with changes as
// modified, and a release is built from one without.
func cloneAt(ctx context.Context, repo, ref, dest string) (string, error) {
	commit, err := runGit(ctx, repo, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%s names no commit in %s: %w", ref, repo, err)
	}
	for _, args := range [][]string{
		{"clone", gitQuiet, "--shared", "--no-checkout", repo, dest},
		{"-C", dest, "checkout", gitQuiet, "--detach", commit},
	} {
		if _, gitErr := runGit(ctx, "", args...); gitErr != nil {
			return "", gitErr
		}
	}
	return commit, nil
}
