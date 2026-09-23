package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionids"
)

// toolName is how the report names itself.
const toolName = "audit_action_ids"

// defaultPatterns is the tree that publishes action IDs. Every surface
// projects from what these packages declare, so nothing outside them can put
// an ID in front of a model.
var defaultPatterns = []string{"./internal/tools/..."}

// defaultSuitePatterns is the e2e suite that quotes what that tree serves: the
// one corpus that reads the server's hints back to it, and so the one whose
// quotation of a tool name breaks the day the hint is fixed. It has to lie
// under suiteDir, or a run naming it would load it as served source.
var defaultSuitePatterns = []string{"./test/e2e/gitlab/..."}

// defaultJSONPath is where the work list lands. plan/ is the repository's
// uncommitted working directory, which is what a work list wants: the layer
// that reads this file is the one that empties it.
const defaultJSONPath = "plan/action-ids.json"

func main() {
	dir := flag.String("dir", ".", "repository root the patterns are resolved against")
	jsonPath := flag.String("json", defaultJSONPath, "write the work list here; empty writes none")
	verbose := flag.Bool("v", false, "also print what fails nothing: the alias references of a clean run, what was judged by kind, the prose sites nothing could fold and the calls read per assertion helper")
	check := flag.Bool("check", false, "exit non-zero when a published ID is not a canonical catalog ID, names a registered alias, sits at a site the type checker could not fold, or is excused by a declaration that excuses nothing; when a hint, or a substring the e2e suite asserts a served text carries, names a tool; or when an assertion helper declaration matches no call")
	fixHintNames := flag.Bool("fix-hints", false, "rewrite each gitlab_* tool name a folded hint spells to the canonical ID of the action that tool projects, then report what moved")
	fixHintTests := flag.Bool("fix-hints-tests", false, "with -fix-hints, rewrite the test files too, so an assertion pinning a hint moves with the hint")
	flag.Parse()

	os.Exit(run(auditConfig{
		dir:          *dir,
		patterns:     flag.Args(),
		jsonPath:     *jsonPath,
		verbose:      *verbose,
		check:        *check,
		fixHints:     *fixHintNames,
		fixHintTests: *fixHintTests,
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
	// fixHints rewrites rather than reports, which is why it is not a mode of
	// -check: the gate answers whether the tree is clean and this changes the
	// tree, and a flag that could do either would be one somebody runs in CI.
	fixHints bool
	// fixHintTests extends the rewrite to the test files of the same packages,
	// and has to run in the same pass as the production rewrite rather than
	// after it. What decides whether a literal belongs to a hint is the hint
	// text the walk folded, and once the production hint has been rewritten no
	// test literal spelling the old tool name is part of any hint any more, so
	// a second run finds nothing. Off by default so a reviewer can read the
	// production half of the diff on its own.
	fixHintTests bool
}

// buildCatalogIDs is how the run reaches the canonical catalog, swapped in
// tests. Building it needs no network and no credentials, so the branch that
// reports its failure is reachable only by replacing it.
var buildCatalogIDs = actionids.Build

// run builds the catalog, walks the source, reports, and returns the process
// exit code.
//
// Without -check it is 1 only for a run that could not be made: a catalog that
// would not build, source that did not type-check, a work list that could not
// be written. With -check a finding fails it too, on the terms [Report.Clean]
// states.
//
// The failure is written to stderr rather than left to the exit code, and it
// repeats the counts the report already printed, because a CI log is read at
// the end: the one line that says why the job stopped should name the rule
// rather than send a reader back up through a thousand lines of report.
//
// The served tree and the suite are two loads, each made only when a pattern
// asks for it, and -fix-hints never makes the second: it rewrites what the
// server writes, and a fixer that moved the suite would move the test to
// agree with the server rather than hold the server to it.
func run(cfg auditConfig, stdout, stderr io.Writer) int {
	ids, err := buildCatalogIDs()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	served, suite := splitPatterns(cfg.patterns)
	var sites []site
	if len(served) > 0 {
		sites, err = collectSites(cfg.dir, served, cfg.overlay)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
			return 1
		}
	}
	if cfg.fixHints {
		fixed, fixErr := fixHints(cfg.dir, sites, ids, cfg.fixHintTests)
		if fixErr != nil {
			fmt.Fprintf(stderr, "%s: %v\n", toolName, fixErr)
			return 1
		}
		writeHintFixReport(stdout, fixed)
		return 0
	}
	var read suiteRead
	if len(suite) > 0 {
		read, err = collectAssertionSites(cfg.dir, suite, cfg.overlay)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
			return 1
		}
	}
	report := classify(append(sites, read.sites...), ids, namesWhole(served, defaultPatterns))
	if len(suite) > 0 {
		report.judgeHelpers(read, namesWhole(suite, defaultSuitePatterns))
	}
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
			"\nERROR: %d published ID(s) resolve to no action, %d name a registered alias rather than a catalog ID, %d site(s) could not be folded, %d declaration(s) excuse nothing, %d hint(s) name a tool rather than an action, %d e2e assertion(s) name a tool rather than an action, %d assertion helper declaration(s) match no call\n",
			report.Summary.Findings, report.Summary.AliasHits, report.Summary.Unresolved, report.Summary.Stale, report.Hints.Findings,
			report.Assertions.Findings, len(report.StaleHelpers))
		return 1
	}
	return 0
}

// splitPatterns sorts the positional patterns between the two loads: the
// served tree and the e2e suite.
//
// A run with no arguments audits both whole. A run naming patterns loads only
// what they name, so `./internal/tools/issues` stays the quick check it was
// and `./test/e2e/gitlab/ee` audits one suite package; an explicit
// `./internal/tools/...` therefore reads no suite at all, and says so by
// printing no assertion section. What holds a table to its tree is that the
// run covered the tree, which [namesWhole] decides: the bare run does for
// both, and a run naming one whole tree itself does for that one.
func splitPatterns(patterns []string) (served, suite []string) {
	if len(patterns) == 0 {
		return defaultPatterns, defaultSuitePatterns
	}
	for _, pattern := range patterns {
		if isSuitePattern(pattern) {
			suite = append(suite, pattern)
			continue
		}
		served = append(served, pattern)
	}
	return served, suite
}

// isSuitePattern reports whether a pattern names part of the e2e suite, in the
// spelling a caller types it: with or without the leading ./, and with the
// platform's separator.
func isSuitePattern(pattern string) bool {
	return strings.HasPrefix(normalizePattern(pattern), suiteDir)
}

// namesWhole reports whether patterns name exactly the whole of a tree, which
// is the only run that can hold a declaration table to that tree: over part of
// it every entry the part does not use excuses nothing, and reporting them
// would be an answer about the patterns rather than about the table.
//
// The comparison reads the spelling isSuitePattern reads. Compared literally,
// `test/e2e/gitlab/...` typed without the leading ./ loaded the whole suite
// and left the helper table unjudged, which is a clean report over a run that
// never asked the question.
func namesWhole(patterns, whole []string) bool {
	return slices.EqualFunc(patterns, whole, func(given, want string) bool {
		return normalizePattern(given) == normalizePattern(want)
	})
}

// normalizePattern is a pattern in the one spelling the comparisons read:
// slash-separated, without a leading ./.
func normalizePattern(pattern string) string {
	return strings.TrimPrefix(filepath.ToSlash(pattern), "./")
}
