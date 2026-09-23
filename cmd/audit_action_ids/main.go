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
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// toolName is how the report names itself.
const toolName = "audit_action_ids"

// defaultPatterns is the tree that publishes action IDs. Every surface
// projects from what these packages declare, so nothing outside them can put
// an ID in front of a model.
//
// internal/toolutil is part of it although it declares no action, because
// the walk can follow a value only into a package it loaded: a hint handed to
// a toolutil helper under a hint-named parameter (NewTemplateRenderer's
// listHint, NewDiscussionRenderer's hints, ExecGraphQLDestroyNote's hint) is
// followed out to the domain that wrote it from the helper's own signature,
// and with toolutil unloaded that parameter was never met. The tree read 1330
// hints and reported none; loading toolutil read 1355 and found eleven that
// named a tool the dynamic surface does not register.
var defaultPatterns = []string{"./internal/tools/...", "./internal/toolutil"}

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
	root, err := absolutePath(cfg.dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	served, suite := splitPatterns(root, cfg.patterns)
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
	report.ServedJudged = len(served) > 0
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
// both, and a run whose patterns load exactly one whole tree does for that
// one, which for the suite includes a run naming a wildcard that encloses it.
//
// Each pattern is first read as the relative pattern it names
// ([relativePattern]), and that spelling is what both the sorting and the
// load are handed, so the two cannot disagree about which tree a pattern
// names.
//
// A wildcard pattern whose prefix encloses the suite, ./... or ./test/...,
// goes to the served load as it was given and brings the whole suite into the
// suite load besides ([enclosesSuite]). Sorted by prefix alone it went to the
// served load only, which reads no test file and sets no e2e tag, so it found
// the suite's three packages holding a doc.go each, printed no assertion
// section and exited 0: the silently clean run the absolute and import path
// spellings used to give.
func splitPatterns(root string, patterns []string) (served, suite []string) {
	if len(patterns) == 0 {
		return defaultPatterns, defaultSuitePatterns
	}
	wholeSuite := false
	for _, given := range patterns {
		pattern := relativePattern(root, given)
		if isSuitePattern(pattern) {
			suite = append(suite, pattern)
			continue
		}
		served = append(served, pattern)
		wholeSuite = wholeSuite || enclosesSuite(pattern)
	}
	if wholeSuite {
		suite = append(suite, defaultSuitePatterns...)
	}
	return served, suite
}

// enclosesSuite reports whether a pattern ending in the ... wildcard matches
// every package under suiteDir, which go list decides by the prefix in front
// of the wildcard: ./... has none, ./test/... has test/, and both are a
// prefix of test/e2e/. A pattern without the wildcard names one package, and
// one outside the suite encloses none of it.
func enclosesSuite(pattern string) bool {
	prefix, wildcard := strings.CutSuffix(normalizePattern(pattern), "...")
	return wildcard && strings.HasPrefix(suiteDir, prefix)
}

// relativePattern reads a pattern given in either of the two other spellings
// go list loads, an absolute path below root or an import path below this
// module, as the relative pattern it names, and returns any other pattern as
// it was given.
//
// Both spellings used to be sorted as they were typed, which the suite's
// prefix never matches, so a run naming the whole suite either way was handed
// to the served load. That load reads no test file and sets no e2e tag, so it
// found the three runtime packages holding nothing but a doc.go each, judged
// nothing, printed no assertion section and exited 0. A relative pattern
// passes through untouched, because filepath.Rel refuses to relate it to an
// absolute root, and so does an absolute path outside the root, which names
// nothing of either tree.
func relativePattern(root, pattern string) string {
	if rel, err := filepath.Rel(root, pattern); err == nil && !escapesRoot(rel) {
		return "./" + filepath.ToSlash(rel)
	}
	if rest, underModule := strings.CutPrefix(pattern, goprogram.ModulePath+"/"); underModule {
		return "./" + rest
	}
	return pattern
}

// escapesRoot reports whether a path filepath.Rel related to the root leaves
// it: the parent itself, or anything below the parent. A name that merely
// begins with two dots stays inside, and the one that matters is the
// wildcard: <root>/... relates as ..., which a test for the prefix ".." read
// as the parent, so the whole module named by its absolute path was handed to
// the load as typed and never read as the tree enclosing the suite.
func escapesRoot(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// isSuitePattern reports whether a pattern names part of the e2e suite, read
// slash-separated, so .\test\e2e\gitlab\ee on Windows is the same pattern as
// ./test/e2e/gitlab/ee. A relative pattern without the leading ./ is sorted
// the same way and goes no further: go list reads it as an import path, which
// matches no package of this module, so the load it is handed to refuses the
// run.
func isSuitePattern(pattern string) bool {
	return strings.HasPrefix(normalizePattern(pattern), suiteDir)
}

// namesWhole reports whether patterns name exactly the whole of a tree, which
// is the only run that can hold a declaration table to that tree: over part of
// it every entry the part does not use excuses nothing, and reporting them
// would be an answer about the patterns rather than about the table.
//
// The comparison reads the spelling isSuitePattern reads, because go list
// takes a pattern in the platform's spelling. Compared literally,
// .\test\e2e\gitlab\... on Windows loaded the whole suite and left the helper
// table unjudged, which is a clean report over a run that never asked the
// question. An absolute path or an import path reaches it already read as the
// relative pattern it names ([relativePattern]). A relative pattern without
// the leading ./ never does: go list reads it as an import path, which
// matches no package of this module, and the load refuses the run first.
//
// What is compared is what the patterns load rather than how they are
// listed ([outermostPatterns]), because go list loads a package once whatever
// names it. A wildcard that encloses the suite brings the whole of it into the
// suite load beside any suite package named with it, so ./... with
// ./test/e2e/gitlab/ee loads the suite and nothing else, and compared as
// listed it said it had not.
func namesWhole(patterns, whole []string) bool {
	return slices.Equal(outermostPatterns(patterns), outermostPatterns(whole))
}

// outermostPatterns is a list of patterns in the spelling the comparisons
// read, sorted, without the ones another pattern of the list already loads: a
// repeat, and a pattern a wildcard of the list encloses ([enclosesPattern]).
func outermostPatterns(patterns []string) []string {
	normalized := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		normalized = append(normalized, normalizePattern(pattern))
	}
	slices.Sort(normalized)
	normalized = slices.Compact(normalized)
	outermost := make([]string, 0, len(normalized))
	for _, pattern := range normalized {
		enclosed := slices.ContainsFunc(normalized, func(wildcard string) bool {
			return enclosesPattern(wildcard, pattern)
		})
		if !enclosed {
			outermost = append(outermost, pattern)
		}
	}
	return outermost
}

// enclosesPattern reports whether a wildcard pattern loads every package
// another pattern names, both in the spelling the comparisons read. go list
// matches a pattern ending in /... by the prefix in front of the wildcard and
// by the directory it names, so test/e2e/gitlab/... loads test/e2e/gitlab,
// test/e2e/gitlab/ee and test/e2e/gitlab/ee/... alike. A pattern never
// encloses itself, so a list with its repeats removed keeps every wildcard.
func enclosesPattern(wildcard, pattern string) bool {
	prefix, isWildcard := strings.CutSuffix(wildcard, "...")
	if !isWildcard || wildcard == pattern {
		return false
	}
	return strings.HasPrefix(pattern, prefix) || pattern+"/" == prefix
}

// normalizePattern is a pattern in the one spelling the comparisons read:
// slash-separated, without a leading ./, which is the form suiteDir is written
// in.
func normalizePattern(pattern string) string {
	return strings.TrimPrefix(filepath.ToSlash(pattern), "./")
}
