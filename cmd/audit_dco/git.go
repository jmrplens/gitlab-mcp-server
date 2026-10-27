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

// logFormat is what git log prints for each commit: the hash, the parents,
// the author's name and email, the subject, and the values of the
// Signed-off-by trailers git finds in the message, unfolded, a NUL between
// two fields and between two trailers, and a record separator after the
// commit, which git follows with the newline it ends every commit's output
// with.
const logFormat = "%H%x00%P%x00%an%x00%ae%x00%s%x00%(trailers:key=Signed-off-by,valueonly,unfold,separator=%x00)%x1e"

// The separator logFormat prints between fields, the end of every record (the
// record separator logFormat prints and the newline git adds), and how many
// fields come before the trailers. A commit without trailers still has the
// field after them, empty.
//
// A record ends where the record separator is followed by a newline, and no
// field holds a newline: git keeps none in a name or an email, joins the
// lines of the subject with spaces, and unfolds each trailer onto one line.
// So a message carrying either separator byte shifts nothing: the byte stays
// in its field and is read as the value git reads.
const (
	fieldSeparator = "\x00"
	recordEnd      = "\x1e\n"
	fixedFields    = 5
)

// runGit runs git in dir and returns what it printed on stdout. A failure
// carries what git said on stderr, which is the only place it says why.
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
	// #nosec G204 G702 -- the program is the absolute path just resolved, with arguments this command builds from resolved commit hashes and its own flags
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// resolveCommit returns the full hash of the commit rev names in dir. The
// caller refuses a revision git would read as an option before asking.
func resolveCommit(ctx context.Context, dir, rev string) (string, error) {
	out, err := runGit(ctx, dir, "rev-parse", "--verify", rev+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// listCommits returns the commits reachable from head and not from base,
// oldest first, read from git log and held to the hashes git rev-list lists
// for the same range.
func listCommits(ctx context.Context, dir, base, head string) ([]commit, error) {
	span := base + ".." + head
	hashes, err := runGit(ctx, dir, "rev-list", "--reverse", span, "--")
	if err != nil {
		return nil, err
	}
	out, err := runGit(ctx, dir, "log", "--no-show-signature", "--reverse", "--format="+logFormat, span, "--")
	if err != nil {
		return nil, err
	}
	return readLog(strings.Fields(hashes), out)
}

// readLog reads what git log printed in logFormat and holds it to the hashes
// git rev-list listed for the same range. The records cannot be split by a
// message (see recordEnd), so the second list is a guard on the reading
// itself: should git ever print a newline inside a field, the run stops
// rather than judging a record that belongs to no commit, or moving a
// sign-off onto another one.
func readLog(hashes []string, out string) ([]commit, error) {
	commits, err := parseLog(out)
	if err != nil {
		return nil, err
	}
	if mismatch := matchHashes(hashes, commits); mismatch != nil {
		return nil, mismatch
	}
	return commits, nil
}

// parseLog reads the records git log printed in logFormat. Text after the
// last record end, and a record with no more fields than the ones before the
// trailers, are not what git prints for a commit, and stop the read.
func parseLog(out string) ([]commit, error) {
	var commits []commit
	for rest := out; rest != ""; {
		record, after, ended := strings.Cut(rest, recordEnd)
		if !ended {
			return nil, fmt.Errorf("git log printed %q after its last record, where every record ends with a record separator and a newline", rest)
		}
		rest = after
		fields := strings.Split(record, fieldSeparator)
		if len(fields) <= fixedFields {
			return nil, fmt.Errorf("git log printed a record of %d field(s) where every commit has at least %d: %q", len(fields), fixedFields+1, record)
		}
		var signoffs []string
		for _, value := range fields[fixedFields:] {
			if value != "" {
				signoffs = append(signoffs, value)
			}
		}
		commits = append(commits, commit{
			hash:     fields[0],
			parents:  len(strings.Fields(fields[1])),
			author:   identity{name: fields[2], email: fields[3]},
			subject:  fields[4],
			signoffs: signoffs,
		})
	}
	return commits, nil
}

// matchHashes holds the commits git log printed to the hashes git rev-list
// listed for the same range: as many, and the same ones in the same order.
func matchHashes(hashes []string, commits []commit) error {
	if len(hashes) != len(commits) {
		return fmt.Errorf("git rev-list lists %d commit(s) and git log printed %d, so the records were not read as git printed them", len(hashes), len(commits))
	}
	for i, hash := range hashes {
		if commits[i].hash != hash {
			return fmt.Errorf("commit %d is %s to git rev-list and %s to git log, so the records were not read as git printed them", i+1, hash, commits[i].hash)
		}
	}
	return nil
}

// markCleanMerges marks each merge of two parents that brings no change of
// its own: one whose tree is the tree git's own merge of its parents makes,
// which git show --remerge-diff prints no difference for. That is the merge
// GitHub's Update branch button writes. A merge that resolves a conflict,
// changes anything while merging or drops a side's change prints one, and a
// merge of more than two parents is one git does not remerge; both stay
// unmarked, so they are judged like any other commit. Every setting that
// could make git print nothing for a change is overridden: an external diff
// program or a textconv filter the configuration names, a diff limited to the
// directory git runs in, and submodules left out of it.
func markCleanMerges(ctx context.Context, dir string, commits []commit) error {
	for i := range commits {
		if commits[i].parents != 2 {
			continue
		}
		diff, err := runGit(ctx, dir, "show", "--no-show-signature", "--no-color", "--no-ext-diff", "--no-textconv", "--no-relative", "--ignore-submodules=none", "--remerge-diff", "--format=", commits[i].hash, "--")
		if err != nil {
			return err
		}
		commits[i].cleanMerge = diff == ""
	}
	return nil
}
