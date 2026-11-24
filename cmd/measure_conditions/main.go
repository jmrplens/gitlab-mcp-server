// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// toolName is how every message this command prints names its source.
const toolName = "measure_conditions"

// usage is printed when the subcommand is missing or unknown.
const usage = `usage: measure_conditions <plan|measure|summarize|tree> [flags]
  plan        validate a dispatch and print the matrix the workflow runs
  measure     measure the packages of one job and leave their records
  summarize   write the job summary from the records the runs left
  tree        measure every package below cmd/ and internal/ here, and summarize
`

// osExit is a seam over os.Exit, so a test can observe the exit code main
// passes on without ending the test process.
var osExit = os.Exit

func main() {
	ctx, stop := recipe.Signals(context.Background())
	status := runMain(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	osExit(status)
}

// runMain dispatches a subcommand and returns the process exit code: 2 for a
// usage error, otherwise whatever the subcommand returns.
func runMain(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "plan":
		return runPlan(ctx, args[1:], stdout, stderr, goList)
	case "measure":
		return runMeasure(ctx, args[1:], stdout, stderr, goList, runRecipe)
	case "summarize":
		return runSummarize(args[1:], stdout, stderr)
	case "tree":
		return runTree(ctx, args[1:], stdout, stderr, goList, runRecipe)
	default:
		fmt.Fprintf(stderr, "%s: unknown subcommand %q\n%s", toolName, args[0], usage)
		return 2
	}
}
