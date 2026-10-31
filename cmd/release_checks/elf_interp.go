// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"debug/elf"
	"fmt"
	"io"
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/releaseassets"
)

// elfInterpUsage is the command line of the elf-interp check.
const elfInterpUsage = "usage: release_checks elf-interp BINARY [BINARY...]\n"

// loader is the interpreter a Linux release binary of one machine has to
// name.
type loader struct {
	// name is the machine as a finding names it.
	name string
	// path is the loader's path, the one .goreleaser.yml passes with -I.
	path string
}

// loaders are the glibc loaders every Linux channel promises, by the ELF
// machine of the binary that names them. They are the paths the per-target
// overrides in .goreleaser.yml pass with -I, written here rather than read
// from it because the check runs in the credentialed release job, where
// rule 2 of cmd/audit_supply_chain allows no YAML module; a test holds the
// two lists to each other.
var loaders = map[elf.Machine]loader{
	elf.EM_X86_64:  {name: "x86-64", path: "/lib64/ld-linux-x86-64.so.2"},
	elf.EM_AARCH64: {name: "aarch64", path: "/lib/ld-linux-aarch64.so.1"},
}

// runELFInterp holds each binary named to the loader of its machine. It
// names every binary that fails on stderr and returns 1, or, when none does,
// names each on stdout with the loader it names and returns 0. No binary
// named is a usage error, 2.
func runELFInterp(paths []string, stdout, stderr io.Writer) int {
	if len(paths) == 0 {
		fmt.Fprint(stderr, elfInterpUsage)
		return 2
	}
	named := make([]string, len(paths))
	failed := false
	for i, path := range paths {
		interp, err := checkInterpreter(path)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
			failed = true
		}
		named[i] = interp
	}
	if failed {
		return 1
	}
	for i, path := range paths {
		fmt.Fprintf(stdout, "%s: %s names %s, the loader of its machine\n", toolName, path, named[i])
	}
	return 0
}

// checkInterpreter reads the binary at path and returns the loader its
// PT_INTERP names when that is the loader its machine is promised. Otherwise
// it says what is wrong: the file cannot be read, is not a 64-bit
// little-endian ELF file (with the reason the reader gives), is for a
// machine the release does not build, or names another loader or none.
func checkInterpreter(path string) (string, error) {
	data, err := os.ReadFile(path) //#nosec G304,G703 -- a release binary the operator named on the command line, read and never written
	if err != nil {
		return "", fmt.Errorf("%s cannot be read: %w", path, err)
	}
	img, err := releaseassets.ReadELF(data)
	if err != nil {
		return "", fmt.Errorf("%s %w", path, err)
	}
	want, ok := loaders[img.Machine]
	if !ok {
		return "", fmt.Errorf("%s is for ELF machine %#x, which the release does not build", path, uint32(img.Machine))
	}
	if img.Interpreter == "" {
		return "", fmt.Errorf("%s is an %s binary naming no interpreter, want %s", path, want.name, want.path)
	}
	if img.Interpreter != want.path {
		return "", fmt.Errorf("%s is an %s binary naming interpreter %s, want %s", path, want.name, img.Interpreter, want.path)
	}
	return img.Interpreter, nil
}
