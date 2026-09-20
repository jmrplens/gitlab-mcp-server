package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionids"
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
	verbose := flag.Bool("v", false, "also print the alias references of a clean run and what was judged by kind")
	check := flag.Bool("check", false, "exit non-zero when a published ID is not a canonical catalog ID, names a registered alias, sits at a site the type checker could not fold, or is excused by a declaration that excuses nothing")
	flag.Parse()

	os.Exit(run(auditConfig{
		dir:      *dir,
		patterns: flag.Args(),
		jsonPath: *jsonPath,
		verbose:  *verbose,
		check:    *check,
	}, os.Stdout, os.Stderr))
}

// auditConfig is one configured run: where to look, what to look at, where the
// work list goes, how much the report says, and whether a finding fails the
// process.
//
// A struct rather than six parameters, which is what the shape had grown to.
type auditConfig struct {
	dir      string
	patterns []string
	// overlay supplies source that is not on disk, which is how a test drives
	// the whole command over a fixture tree, so the gate is exercised failing
	// rather than only passing. Production passes nil.
	overlay  map[string][]byte
	jsonPath string
	verbose  bool
	check    bool
}

// run builds the catalog, walks the source, reports, and returns the process
// exit code.
//
// Without -check it is 1 only for a run that could not be made: a catalog that
// would not build, source that did not type-check, a work list that could not
// be written. With -check a finding fails it too, on the four terms
// [Report.Clean] states.
//
// The failure is written to stderr rather than left to the exit code, and it
// repeats the counts the report already printed, because a CI log is read at
// the end: the one line that says why the job stopped should name the rule
// rather than send a reader back up through a thousand lines of report.
func run(cfg auditConfig, stdout, stderr io.Writer) int {
	ids, err := actionids.Build()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	audited := patternsOrDefault(cfg.patterns)
	sites, err := collectSites(cfg.dir, audited, cfg.overlay)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	report := classify(sites, ids, slices.Equal(audited, defaultPatterns))
	writeReport(stdout, report, cfg.verbose)
	if cfg.jsonPath != "" {
		if writeErr := writeJSON(cfg.jsonPath, report); writeErr != nil {
			fmt.Fprintf(stderr, "%s: %v\n", toolName, writeErr)
			return 1
		}
		fmt.Fprintf(stdout, "wrote %s\n", cfg.jsonPath)
	}
	if cfg.check && !report.Clean() {
		fmt.Fprintf(stderr,
			"\nERROR: %d published ID(s) resolve to no action, %d name a registered alias rather than a catalog ID, %d site(s) could not be folded, %d declaration(s) excuse nothing\n",
			report.Summary.Findings, report.Summary.AliasHits, report.Summary.Unresolved, report.Summary.Stale)
		return 1
	}
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
