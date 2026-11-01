// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
)

// valid is the line a check that passes prints.
const valid = "Agent Plugins and legacy Open Plugins manifests are valid"

// exitProcess is [os.Exit] behind a seam, so the code [run] decided is a
// value a test can read rather than the end of the test binary.
var exitProcess = os.Exit

func main() {
	exitProcess(run(".", os.Stdout, os.Stderr))
}

// run checks the manifests of the repository at root and returns the exit
// code: 0 when they pass, 1 when any does not.
func run(root string, stdout, stderr io.Writer) int {
	problems := checkManifests(root)
	for _, problem := range problems {
		fmt.Fprintln(stderr, "ERROR: "+problem)
	}
	if len(problems) > 0 {
		return 1
	}
	fmt.Fprintln(stdout, valid)
	return 0
}
