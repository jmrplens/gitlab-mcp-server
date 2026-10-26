// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// Seams over the filesystem calls a run makes, so a test can drive the
// failures a filesystem the tests own never produces for a root user: a file
// that cannot be stated, read or rewritten.
var (
	lstat     = os.Lstat
	readFile  = os.ReadFile
	writeFile = os.WriteFile
)

// listFiles returns the slash-separated paths, relative to dir, of the files
// of the repository at dir: the ones in git's index and the untracked ones
// git would add, which leaves out what .gitignore and the repository's other
// exclude files name and the files of any checkout nested in the tree. The
// list is sorted, since git prints the untracked files before the others.
//
// A path in the index whose file the working tree no longer has, and a path
// that is a symbolic link or a nested checkout's directory, are listed too;
// the caller skips what is not a regular file.
//
// git is resolved to an absolute path first, which is how every other command
// here runs a tool it did not build (cmd/gen_orbit_record): the lookup is the
// one exec would do, decided here rather than by whatever PATH holds when the
// command starts.
func listFiles(ctx context.Context, dir string) ([]string, error) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("find git: %w", err)
	}
	// #nosec G204 G702 -- the program is the absolute path just resolved, with fixed arguments and the directory this command was given
	cmd := exec.CommandContext(ctx, bin, "-C", dir, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--deduplicate")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var names []string
	for name := range strings.SplitSeq(string(out), "\x00") {
		if name != "" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}
