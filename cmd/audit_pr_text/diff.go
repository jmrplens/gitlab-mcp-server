// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/committext"
)

// excludedPaths are the paths whose added lines are not judged, because they
// are not this branch's prose. docs/development/gitlab-api-live.json is
// machine-written from a booted GitLab and holds the parameter descriptions
// it reported, one of which already contains an em dash, so a re-take of that
// record must not fail this gate. The rest are generated documents that copy
// package doc comments verbatim, so the roughly two thousand em dashes
// already in the tree reach them the moment a branch regenerates one:
// docs/development/testing/testing.md carries the one in
// internal/gatewaycompat/doc.go today. Failing a branch for regenerating an
// artifact it did not write is how a gate teaches people to route around it.
// README.md is deliberately not here, being mostly hand-written; an em dash
// reaching its generated block would be a finding about the generator. A
// declaration that no longer names a file in the tree is a finding of its
// own, on the terms every declaration table in this repository is held to.
var excludedPaths = []string{
	"docs/development/gitlab-api-live.json",
	"docs/development/testing/testing.md",
	"docs/development/testing/e2e-coverage.md",
	"llms.txt",
	"llms-full.txt",
}

// hunkStart is the number of the first line a hunk header gives the new
// file, the first plus sign and the digits after it.
var hunkStart = regexp.MustCompile(`\+\d+`)

// gitCommand builds a git invocation in dir. It is a seam so a test can see
// its answer without starting git.
//
// The program is resolved to an absolute path first, which is how every other
// command here runs a tool it did not build (cmd/gen_orbit_record): the
// lookup is the one exec would do, decided here rather than by whatever PATH
// holds when the command starts. A git the lookup does not find is the
// command's Err, which Start returns wrapping the lookup's own error.
var gitCommand = func(ctx context.Context, dir string, args ...string) *exec.Cmd {
	bin, err := exec.LookPath("git")
	if err != nil {
		return &exec.Cmd{Err: fmt.Errorf("find git: %w", err)}
	}
	// #nosec G204 G702 -- the program is the absolute path just resolved, with this command's own git arguments, the base among them
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	return cmd
}

// finding is an added line carrying an em dash: the file, its number in the
// new file, and its text.
type finding struct {
	path string
	line int
	text string
}

// runDiff judges the lines the branch at dir adds on top of its merge base
// with the base args or env name, and returns the exit code.
func runDiff(ctx context.Context, dir string, args []string, env func(string) string, stdout, stderr io.Writer) int {
	if len(args) > 1 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	// make passes its EM_DASH_BASE as the argument, empty when unset, so an
	// empty argument is no argument.
	base := env("EM_DASH_BASE")
	for _, arg := range args {
		if arg != "" {
			base = arg
		}
	}
	if base == "" {
		base = "origin/main"
	}

	if !checkExclusions(dir, stderr) {
		return 1
	}
	if err := gitCommand(ctx, dir, "rev-parse", "--verify", "--quiet", base+"^{commit}").Run(); err != nil {
		// Only a git that ran and exited non-zero has said the base is not
		// there. A git that never started said nothing about it, and sending
		// the reader to fetch a base would hide the reason it did not start.
		if _, ran := errors.AsType[*exec.ExitError](err); !ran {
			fmt.Fprintf(stderr, "ERROR: git rev-parse %s: %v\n", base, err)
			return 1
		}
		fmt.Fprintf(stderr, "ERROR: cannot resolve the base commit '%s'.\n", base)
		fmt.Fprintln(stderr, "Fetch it (git fetch origin main), or name another: make check-em-dash EM_DASH_BASE=<ref>.")
		return 1
	}

	// Explicit prefixes and no external diff driver: a developer with
	// diff.noprefix, diff.mnemonicPrefix or a textconv filter configured would
	// otherwise hand this parser a shape it cannot read, and a gate that reads
	// nothing passes everything. Rename detection is stated for the same
	// reason: under diff.renames=false a moved file reads as one deleted and
	// one added, and every em dash it already carried would be reported as
	// written here.
	gitArgs := []string{
		"-c", "core.quotePath=false", "diff", "--no-color", "--no-ext-diff", "--no-textconv",
		"--find-renames", "-U0", "--src-prefix=a/", "--dst-prefix=b/", base + "...HEAD", "--", ":(top)",
	}
	for _, path := range excludedPaths {
		gitArgs = append(gitArgs, ":(top,exclude)"+path)
	}
	cmd := gitCommand(ctx, dir, gitArgs...)
	var gitErr bytes.Buffer
	cmd.Stderr = &gitErr
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: git diff %s...HEAD: %v: %s\n", base, err, strings.TrimSpace(gitErr.String()))
		return 1
	}

	findings := parseDiff(out)
	if len(findings) == 0 {
		fmt.Fprintf(stdout, "OK: no line added between %s and HEAD carries an em dash.\n", base)
		return 0
	}
	annotate := env("GITHUB_ACTIONS") != ""
	for _, f := range findings {
		fmt.Fprintf(stdout, "%s:%d: %s\n", f.path, f.line, f.text)
		// GitHub renders an annotation against the file and line it names,
		// so a finding lands on the diff the author is looking at and not
		// only in a log.
		if annotate {
			fmt.Fprintf(stdout, "::error file=%s,line=%d::this line adds an em dash (U+2014); rewrite it\n", f.path, f.line)
		}
	}
	fmt.Fprintf(stderr, `
FAIL: %d added line(s) carry an em dash (U+2014).

Nothing written here uses one: not code, not a comment, not documentation, not
a commit message. Rewrite each line above. A comma, a colon, a full stop or a
pair of parentheses says the same thing, and the sentence usually reads better
cut in two.

Only the lines this branch adds are judged, so prose that was already there is
not yours to fix. Re-check with:

    make check-em-dash
`, len(findings))
	return 1
}

// checkExclusions reports whether every entry of [excludedPaths] names a
// file in the tree at dir, saying which do not.
func checkExclusions(dir string, stderr io.Writer) bool {
	stale := false
	for _, path := range excludedPaths {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(path))); err != nil {
			fmt.Fprintf(stderr, "ERROR: the exclusion list names %s, which is not in the tree.\n", path)
			stale = true
		}
	}
	if stale {
		fmt.Fprintln(stderr, "Remove the entry from excludedPaths in cmd/audit_pr_text/diff.go, or restore the file.")
	}
	return !stale
}

// parseDiff reads a unified diff written with zero context lines and returns
// the added lines that carry an em dash, in the order of the diff.
//
// A file header only ever precedes the first hunk of its file, so the hunk
// state is what tells it from an added line whose own text starts with "++ ".
// Without that, such a line reads as a header, sets the path to nothing, and
// every added line after it in the file is skipped: the gate would fail open
// and say nothing, which is the one way for a gate to be worse than absent.
func parseDiff(diff []byte) []finding {
	var (
		findings []finding
		path     string
		line     int
		inHunk   bool
	)
	for raw := range bytes.Lines(diff) {
		text := strings.TrimSuffix(string(raw), "\n")
		if strings.HasPrefix(text, "diff --git ") {
			path, inHunk = "", false
			continue
		}
		if !inHunk && strings.HasPrefix(text, "+++ ") {
			path = newFilePath(strings.TrimPrefix(text, "+++ "))
			continue
		}
		if strings.HasPrefix(text, "@@ ") {
			line, _ = strconv.Atoi(strings.TrimPrefix(hunkStart.FindString(text), "+"))
			inHunk = true
			continue
		}
		if strings.HasPrefix(text, "+") {
			if path != "" && strings.Contains(text, committext.EmDash) {
				findings = append(findings, finding{path: path, line: line, text: text[1:]})
			}
			line++
			continue
		}
		if strings.HasPrefix(text, " ") {
			line++
		}
	}
	return findings
}

// newFilePath reads the path of a "+++ " header: nothing for a deleted file,
// the path without its b/ prefix otherwise. git quotes a path that carries a
// control character, a quote or a backslash even under core.quotePath=false;
// a quoted path is decoded, and one that does not decode is kept as git wrote
// it, so its findings are still reported.
func newFilePath(header string) string {
	if header == "/dev/null" {
		return ""
	}
	if unquoted, err := strconv.Unquote(header); err == nil {
		header = unquoted
	}
	return strings.TrimPrefix(header, "b/")
}
