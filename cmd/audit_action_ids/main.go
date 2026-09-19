package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// toolName is how the report names itself.
const toolName = "audit_action_ids"

// defaultPatterns is the tree that publishes action IDs. Every surface
// projects from what these packages declare, so nothing outside them can put
// an ID in front of a model.
var defaultPatterns = []string{"./internal/tools/..."}

// defaultJSONPath is where the work list lands. plan/ is the repository's
// uncommitted working directory, which is what a work list wants: the layer
// that reads this file is the one that empties it.
const defaultJSONPath = "plan/action-ids.json"

func main() {
	dir := flag.String("dir", ".", "repository root the patterns are resolved against")
	jsonPath := flag.String("json", defaultJSONPath, "write the work list here; empty writes none")
	verbose := flag.Bool("v", false, "also print the alias references and the sites that could not be folded")
	flag.Parse()

	os.Exit(run(*dir, flag.Args(), *jsonPath, *verbose, os.Stdout, os.Stderr))
}

// run builds the oracle, walks the source, reports, and returns the process
// exit code.
//
// It is 1 only for a run that could not be made: a catalog that would not
// build, source that did not type-check, a work list that could not be
// written. A finding does not fail this command, and the reason is in doc.go:
// the findings are spread over packages no single change touches, so a gate
// that failed today would fail on code the change introducing it never went
// near.
func run(dir string, patterns []string, jsonPath string, verbose bool, stdout, stderr io.Writer) int {
	ids, err := buildOracle()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	sites, err := collectSites(dir, patternsOrDefault(patterns), nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	report := classify(sites, ids)
	writeReport(stdout, report, verbose)
	if jsonPath == "" {
		return 0
	}
	if writeErr := writeJSON(jsonPath, report); writeErr != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, writeErr)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s\n", jsonPath)
	return 0
}

// patternsOrDefault is what a run with no arguments audits: the whole tree
// that publishes action IDs.
func patternsOrDefault(patterns []string) []string {
	if len(patterns) == 0 {
		return defaultPatterns
	}
	return patterns
}
