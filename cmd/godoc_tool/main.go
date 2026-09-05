package main

import (
	"flag"
	"fmt"
	"os"
)

// dryRun controls whether the fix subcommand writes files (false) or only
// prints what would change (true). Set by the fix subcommand's --dry-run flag.
var dryRun bool

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: godoc_tool <audit|fix> [options]")
		fmt.Fprintln(os.Stderr, "  audit   report missing or malformed Go doc comments")
		fmt.Fprintln(os.Stderr, "  fix     generate and insert godoc-compliant comments")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "audit":
		if err := run(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "fix":
		fs := flag.NewFlagSet("fix", flag.ExitOnError)
		fs.BoolVar(&dryRun, "dry-run", false, "print what would change without writing files")
		var movePackageDoc bool
		fs.BoolVar(&movePackageDoc, "move-package-doc", false, "move each package comment into a doc.go of its own instead of documenting symbols")
		fs.Parse(os.Args[2:]) //nolint:errcheck // ExitOnError handles parse failures
		if fs.NArg() == 0 {
			fmt.Fprintln(os.Stderr, "fix: at least one file or directory path is required")
			os.Exit(2)
		}
		process := processPath
		if movePackageDoc {
			process = movePackageDocs
		}
		var failed bool
		for _, p := range fs.Args() {
			if err := process(p); err != nil {
				fmt.Fprintln(os.Stderr, err)
				failed = true
			}
		}
		if failed {
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q (valid: audit, fix)\n", os.Args[1])
		os.Exit(2)
	}
}
