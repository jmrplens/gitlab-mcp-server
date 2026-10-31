// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
)

// toolName is how the findings and the usage name the command.
const toolName = "release_checks"

// usage is the command line of both checks, each written out as the check
// itself prints it.
const usage = `usage: release_checks order TAG   (the tags the repository holds on stdin)
       release_checks elf-interp BINARY [BINARY...]
`

// exitProcess is [os.Exit] behind a seam, so the code [runMain] decided is a
// value a test can read rather than the end of the test binary.
var exitProcess = os.Exit

func main() {
	exitProcess(runMain(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// runMain runs the check its first argument names and returns the process
// exit code: 2 for no check or an unknown one, and otherwise what the check
// decides.
func runMain(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "order":
		return runOrder(args[1:], stdin, stdout, stderr)
	case "elf-interp":
		return runELFInterp(args[1:], stdout, stderr)
	case "-h", "-help", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "%s: unknown check %q\n%s", toolName, args[0], usage)
	return 2
}
