package main

import (
	"fmt"
	"io"
	"os"
)

// toolName is how every message this command prints names its source.
const toolName = "measure_conditions"

// usage is printed when the subcommand is missing or unknown.
const usage = `usage: measure_conditions <plan|summarize> [flags]
  plan        validate a dispatch and print the matrix the workflow runs
  summarize   write the job summary from the records the runs left
`

// osExit is a seam over os.Exit, so a test can observe the exit code main
// passes on without ending the test process.
var osExit = os.Exit

func main() {
	osExit(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

// runMain dispatches a subcommand and returns the process exit code: 2 for a
// usage error, otherwise whatever the subcommand returns.
func runMain(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "plan":
		return runPlan(args[1:], stdout, stderr, goList)
	case "summarize":
		return runSummarize(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s: unknown subcommand %q\n%s", toolName, args[0], usage)
		return 2
	}
}
