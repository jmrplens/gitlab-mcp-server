// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docwalk"
)

// documentationRoots are the trees, relative to the repository, whose
// Markdown and MDX files are checked: the repository itself, and the two
// dot-directories that hold tracked documentation, which a walk of the
// repository leaves out with every other dot-directory: .github (the
// instructions, agents, skills, templates and release notes) and .claude
// (the agents). A walk of .claude leaves out the worktrees the
// parallel-agent tooling puts there, each another checkout.
var documentationRoots = []string{".", ".github", ".claude"}

// exitProcess is [os.Exit] behind a seam, so the code [run] decided is a
// value a test can read rather than the end of the test binary.
var exitProcess = os.Exit

func main() {
	exitProcess(run(".", os.Stdout, os.Stderr))
}

// run checks the links of the documentation of the repository at root and
// returns the exit code: 0 when every link resolves, 1 when any does not or
// the documentation cannot be read.
func run(root string, stdout, stderr io.Writer) int {
	files, err := documents(root)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: list the documentation: %v\n", err)
		return 1
	}
	c := newChecker(root)
	for _, file := range files {
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(file))) //#nosec G304 -- a page of the repository's documentation, found by the walk
		if readErr != nil {
			fmt.Fprintf(stderr, "ERROR: read %s: %v\n", file, readErr)
			return 1
		}
		c.checkFile(file, string(data))
	}
	if len(c.issues) > 0 {
		fmt.Fprintln(stderr, "Broken local documentation links:")
		for _, i := range c.issues {
			fmt.Fprintf(stderr, "- %s:%d -> %s (%s)\n", i.file, i.line, i.target, i.reason)
		}
		return 1
	}
	fmt.Fprintf(stdout, "Checked %d Markdown/MDX files; local links are valid, including %d anchors.\n", len(files), c.checked)
	return 0
}

// documents lists the documentation of the repository at root, relative to
// it with slashes, sorted as git sorts paths and each once.
func documents(root string) ([]string, error) {
	roots := make([]string, 0, len(documentationRoots))
	for _, rel := range documentationRoots {
		roots = append(roots, filepath.Join(root, rel))
	}
	found, err := docwalk.Files(roots)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(found))
	for _, path := range found {
		rel, _ := filepath.Rel(root, path)
		files = append(files, filepath.ToSlash(rel))
	}
	slices.Sort(files)
	return slices.Compact(files), nil
}
