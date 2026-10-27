// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// toolName is how every message this command prints names its source.
const toolName = "fuzz_targets"

// osExit is a seam over os.Exit, so a test can observe the exit code main
// passes on without ending the test process.
var osExit = os.Exit

func main() {
	osExit(runMain(os.Args[1:], os.Stdout, os.Stderr, goList))
}

// runMain finds the fuzz targets of the module -dir names and prints the
// matrix as one `matrix=<json>` line for $GITHUB_OUTPUT, and each target and
// their count on stderr. It returns 2 for a usage error, 1 when the targets
// could not be found or planned or there are none, and 0 otherwise.
func runMain(args []string, stdout, stderr io.Writer, list lister) int {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", ".", "module root the fuzz targets are looked for in")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "%s: takes no arguments, and was given %q\n", toolName, flags.Args())
		return 2
	}
	targets, err := findTargets(context.Background(), *dir, list)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	if len(targets) == 0 {
		fmt.Fprintf(stderr, "%s: no fuzz target found in %s, so the workflow would fuzz nothing and pass\n", toolName, *dir)
		return 1
	}
	planned, err := buildMatrix(targets)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	fmt.Fprintf(stdout, "matrix=%s\n", cmdutil.Must(json.Marshal(planned)))
	packages := map[string]bool{}
	for _, each := range targets {
		packages[each.pkg] = true
		fmt.Fprintf(stderr, "%s: %s %s\n", toolName, each.pkg, each.name)
	}
	fmt.Fprintf(stderr, "%s: %d fuzz target(s) in %d package(s)\n", toolName, len(targets), len(packages))
	return 0
}
