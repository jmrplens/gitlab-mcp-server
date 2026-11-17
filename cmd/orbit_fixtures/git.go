// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// gitFunc runs git with args in dir and returns its standard output. A test
// replaces it to stand in for the clones and pushes a run asks for.
type gitFunc func(ctx context.Context, dir string, args ...string) ([]byte, error)

// runGit is the real [gitFunc]: the git on the PATH, with its standard error
// in the error it returns when it fails. Nothing it is handed is shown in a
// message but the command's own words: the token reaches git as a
// configuration value, which git does not echo.
func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	//#nosec G204 -- git from the PATH, with arguments this command builds
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir
	var stderr bytes.Buffer
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", gitVerb(args), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// gitVerb is the subcommand args name, the first argument that is not an
// option or the value of -c, so a message never repeats a -c value, which is
// where the token goes.
func gitVerb(args []string) string {
	optionValue := false
	for _, arg := range args {
		if optionValue {
			optionValue = false
			continue
		}
		if arg == "-c" || arg == "-C" {
			optionValue = true
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return "(no subcommand)"
}

// trackedFiles lists the files HEAD holds under dir, relative to it, sorted
// with the LICENSE first: jmrp-oss requires an OSI license on every project,
// and a commit that adds the fixture lists it first. The list is HEAD's and
// not the index's, so a file staged and not committed is left out, as its
// content is by [committedBytes].
func trackedFiles(ctx context.Context, git gitFunc, dir string) ([]string, error) {
	listed, err := git(ctx, dir, "ls-tree", "-r", "-z", "--name-only", "HEAD")
	if err != nil {
		return nil, err
	}
	var files []string
	for name := range strings.SplitSeq(string(listed), "\x00") {
		if name != "" {
			files = append(files, name)
		}
	}
	slices.SortFunc(files, func(a, b string) int {
		if (a == "LICENSE") != (b == "LICENSE") {
			if a == "LICENSE" {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	return files, nil
}

// committedBytes is the content of the file rel under dir as HEAD holds it,
// never as the working tree does: an edit nobody has committed must not reach
// a public project.
func committedBytes(ctx context.Context, git gitFunc, dir, rel string) ([]byte, error) {
	return git(ctx, dir, "show", "HEAD:./"+rel)
}
