package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// dryRun controls whether the fix subcommand writes files (false) or only
// prints what would change (true). Set by the fix subcommand's --dry-run flag.
var dryRun bool

// osExit is a seam over os.Exit, so a test can observe the exit code runMain
// returns without terminating the test process.
var osExit = os.Exit

func main() {
	osExit(runMain(os.Args, os.Stdout, os.Stderr))
}

// runMain dispatches the audit and fix subcommands and returns the process
// exit code. It takes the arguments and output streams explicitly, the way run
// does, so a test can drive every branch without touching the real process.
func runMain(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "usage: godoc_tool <audit|fix> [options]")
		fmt.Fprintln(stderr, "  audit   report missing or malformed Go doc comments")
		fmt.Fprintln(stderr, "  fix     generate and insert godoc-compliant comments")
		return 2
	}

	switch args[1] {
	case "audit":
		if err := run(args[2:], stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	case "fix":
		fs := flag.NewFlagSet("fix", flag.ExitOnError)
		fs.SetOutput(stderr)
		fs.BoolVar(&dryRun, "dry-run", false, "print what would change without writing files")
		var movePackageDoc bool
		fs.BoolVar(&movePackageDoc, "move-package-doc", false, "move each package comment into a doc.go of its own instead of documenting symbols")
		fs.Parse(args[2:]) //nolint:errcheck // ExitOnError handles parse failures
		if fs.NArg() == 0 {
			fmt.Fprintln(stderr, "fix: at least one file or directory path is required")
			return 2
		}
		process := processPath
		if movePackageDoc {
			process = movePackageDocs
		}
		var failed bool
		for _, p := range fs.Args() {
			if err := process(p); err != nil {
				fmt.Fprintln(stderr, err)
				failed = true
			}
		}
		if failed {
			return 1
		}
	default:
		fmt.Fprintf(stderr, "unknown subcommand %q (valid: audit, fix)\n", args[1])
		return 2
	}
	return 0
}
