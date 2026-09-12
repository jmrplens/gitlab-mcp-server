package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// The exit statuses. Findings and refusals are told apart because a CI step
// reads one as "the suite regressed" and the other as "the audit could not
// run", and only the first is about the code under test.
const (
	exitOK       = 0
	exitFindings = 1
	exitUsage    = 2
)

// options is one invocation.
type options struct {
	// dir is the repository root; empty finds it from the working directory.
	dir string
	// calls is the shard directory, or a parent of one directory per runtime.
	calls string
	// results is the go test -json stream to join.
	results string
	// runtime is the comma-separated runtime selectors.
	runtime string
	// baseline is the shard directory of the old suite.
	baseline string
	// portMap, static, check and report select the gates and the output.
	portMap bool
	static  bool
	check   bool
	report  bool
	// output is the JSON path, empty for stdout.
	output string
	// summary is the Markdown path, "-" for stdout, empty for none.
	summary string
	// oldSuite and newSuite are the port map's two trees.
	oldSuite string
	newSuite string
	// catalogs builds the served catalog per tier; the compiled-in one
	// unless a test hands in another.
	catalogs catalogBuilder
	// staticCatalog lists the actions the static gate accepts; nil builds it
	// from the compiled-in catalog at Ultimate.
	staticCatalog map[string]edition.Tier
	// harnessPath is the harness import path the static gate resolves
	// against.
	harnessPath string
	// staticPatterns are the packages the static gate loads.
	staticPatterns []string
	// ratchet is whether the static ratchet is on.
	ratchet bool
	// exemptions is the ratchet's table.
	exemptions map[string]actionExemption
	// drops is the port map's declared-drops table.
	drops map[string]dropDeclaration
}

func main() {
	opts := options{
		catalogs: buildServedCatalog, harnessPath: harnessImportPath, staticPatterns: staticPatterns,
		ratchet: ratchetEnabled, exemptions: exemptedActions, drops: declaredDrops,
	}
	flag.StringVar(&opts.dir, "dir", "", "repository root (default: found from the working directory)")
	flag.StringVar(&opts.calls, "calls", "", "shard directory written by the e2e suite, or a directory holding one per runtime")
	flag.StringVar(&opts.results, "results", "", "go test -json stream (gotestsum --jsonfile) to join with the calls")
	flag.StringVar(&opts.runtime, "runtime", "", "comma-separated runtimes to report and, with -check, to require: ce, ee, or edition/tier")
	flag.StringVar(&opts.baseline, "baseline", "", "shard directory of the old suite; fails on any credit it reached that -calls does not")
	flag.BoolVar(&opts.portMap, "port-map", false, "check that every old Test function has a Replaces: successor or a declared drop")
	flag.BoolVar(&opts.static, "static", false, "run the push-time gate over the typed action ids of test/e2e/gitlab, without GitLab")
	flag.BoolVar(&opts.check, "check", false, "apply the floors: expected runtimes present, test calls recorded, no package refused or filtered with -run, asserted count at or above its floor")
	flag.BoolVar(&opts.report, "report", false, "print the gap work list as TSV instead of the JSON report")
	flag.StringVar(&opts.output, "o", "", "write the JSON report to this path instead of stdout")
	flag.StringVar(&opts.summary, "summary", "", "write a Markdown summary to this path, or - for stdout, which then carries the summary in place of the JSON")
	flag.StringVar(&opts.oldSuite, "old-suite", "test/e2e/suite", "the old suite the port map reads Test functions from")
	flag.StringVar(&opts.newSuite, "new-suite", gitlabTestDir, "the new suite the port map reads Replaces: lines from")
	flag.Parse()
	os.Exit(run(opts, os.Stdout, os.Stderr))
}

// run is main with its streams and its exit status handed to it.
func run(opts options, stdout, stderr io.Writer) int {
	if opts.calls == "" && !opts.static && !opts.portMap {
		fmt.Fprintln(stderr, "audit_e2e_coverage: nothing to do: give -calls, -static or -port-map")
		return exitUsage
	}
	if opts.dir == "" {
		root, err := mcpsurface.ProjectRoot()
		if err != nil {
			fmt.Fprintln(stderr, "audit_e2e_coverage:", err)
			return exitUsage
		}
		opts.dir = root
	}
	status := exitOK
	var static *staticResult
	if opts.static {
		result, code := runStaticGate(opts, stdout, stderr)
		static = result
		status = max(status, code)
	}
	if opts.portMap {
		status = max(status, runPortMap(opts, stdout, stderr))
	}
	if opts.calls != "" {
		status = max(status, runCoverage(opts, static, stdout, stderr))
	}
	return status
}

// runStaticGate runs -static and prints what it found.
func runStaticGate(opts options, stdout, stderr io.Writer) (result *staticResult, status int) {
	catalog := opts.staticCatalog
	if catalog == nil {
		built, err := opts.catalogs(edition.Ultimate)
		if err != nil {
			fmt.Fprintln(stderr, "audit_e2e_coverage: static:", err)
			return nil, exitUsage
		}
		catalog = map[string]edition.Tier{}
		for id, action := range built.actions {
			catalog[id] = action.tier
		}
	}
	result, err := runStatic(staticConfig{
		dir: opts.dir, harnessPath: opts.harnessPath, patterns: opts.staticPatterns,
		catalog: catalog, ratchet: opts.ratchet, exemptions: opts.exemptions,
	})
	if err != nil {
		fmt.Fprintln(stderr, "audit_e2e_coverage: static:", err)
		return nil, exitUsage
	}
	printStatic(opts, result, stdout, stderr)
	if result.failed() {
		return result, exitFindings
	}
	return result, exitOK
}

// printStatic prints what the static gate found: the notes, the findings and
// the summary line.
func printStatic(opts options, result *staticResult, stdout, stderr io.Writer) {
	if result.Skipped {
		fmt.Fprintf(stdout, "static: %s does not exist yet, nothing to check\n", gitlabTestDir)
		return
	}
	for _, note := range result.NonConstant {
		fmt.Fprintf(stdout, "static: note: %s: %s\n", note.Pos, note.Text)
	}
	if !opts.ratchet {
		for _, name := range result.DeadExports {
			fmt.Fprintf(stdout, "static: note: harness export %s is used by nothing yet\n", name)
		}
	}
	for _, finding := range result.Findings {
		fmt.Fprintln(stderr, "static:", finding)
	}
	fmt.Fprintf(stdout, "static: %d id sites in %d packages, %d non-constant sites, %d unused harness exports, %d findings\n",
		len(result.Sites), len(result.Packages), len(result.NonConstant), len(result.DeadExports), len(result.Findings))
}

// runPortMap runs -port-map and prints what is unresolved.
func runPortMap(opts options, stdout, stderr io.Writer) int {
	oldDir := filepath.Join(opts.dir, opts.oldSuite)
	oldTests, err := testFunctions(oldDir)
	if err != nil {
		fmt.Fprintln(stderr, "audit_e2e_coverage: port map:", err)
		return exitUsage
	}
	if len(oldTests) == 0 {
		fmt.Fprintf(stderr, "audit_e2e_coverage: port map: no Test function under %s\n", oldDir)
		return exitUsage
	}
	replaces, err := replacesLines(filepath.Join(opts.dir, opts.newSuite))
	if err != nil {
		fmt.Fprintln(stderr, "audit_e2e_coverage: port map:", err)
		return exitUsage
	}
	m := resolvePortMap(oldTests, replaces, opts.drops)
	for _, finding := range m.Findings {
		fmt.Fprintln(stderr, "port map:", finding)
	}
	for _, name := range m.Unresolved {
		fmt.Fprintf(stdout, "port map: %s has no Replaces: successor and no declared drop\n", name)
	}
	fmt.Fprintf(stdout, "port map: %d old tests, %d replaced, %d dropped, %d unresolved, %d findings\n",
		len(m.Old), len(m.Replaced), len(m.Dropped), len(m.Unresolved), len(m.Findings))
	if !m.complete() {
		return exitFindings
	}
	return exitOK
}

// runCoverage reads the shards, classifies every runtime and writes the
// report.
func runCoverage(opts options, static *staticResult, stdout, stderr io.Writer) int {
	runtimes, err := readRuntimes(opts.calls)
	if err != nil {
		fmt.Fprintln(stderr, "audit_e2e_coverage:", err)
		return exitUsage
	}
	selectors := splitSelectors(opts.runtime)
	status := exitOK
	if missing := missingRuntimes(runtimes, selectors); len(missing) > 0 {
		for _, selector := range missing {
			fmt.Fprintf(stderr, "audit_e2e_coverage: no runtime under %s matches %s\n", opts.calls, selector)
		}
		if opts.check {
			status = exitFindings
		}
	}
	reports, code := classifyRuntimes(opts, selectRuntimes(runtimes, selectors), selectors, static, stderr)
	status = max(status, code)
	if writeErr := writeOutputs(opts, reports, stdout); writeErr != nil {
		fmt.Fprintln(stderr, "audit_e2e_coverage:", writeErr)
		return exitUsage
	}
	return status
}

// classifyRuntimes builds one report per selected runtime, with the results
// join, the baseline comparison and the check applied as asked.
func classifyRuntimes(opts options, runtimes []*runtimeRecords, selectors []string, static *staticResult, stderr io.Writer) (reports []*report, status int) {
	var (
		results  testResults
		baseline []*runtimeRecords
		err      error
	)
	if opts.results != "" {
		if results, err = readResults(opts.results); err != nil {
			fmt.Fprintln(stderr, "audit_e2e_coverage:", err)
			return nil, exitUsage
		}
	}
	if opts.baseline != "" {
		if baseline, err = readRuntimes(opts.baseline); err != nil {
			fmt.Fprintln(stderr, "audit_e2e_coverage: baseline:", err)
			return nil, exitUsage
		}
	}
	status = exitOK
	// An empty list rather than null when no runtime was selected, so a
	// reader of the JSON sees a list either way.
	reports = []*report{}
	for _, rt := range runtimes {
		rep, code, classifyErr := classifyRuntime(opts, rt, selectors, results, baseline, static)
		if classifyErr != nil {
			fmt.Fprintln(stderr, "audit_e2e_coverage:", classifyErr)
			return nil, exitUsage
		}
		status = max(status, code)
		reports = append(reports, rep)
	}
	return reports, status
}

// classifyRuntime builds one runtime's report.
func classifyRuntime(opts options, rt *runtimeRecords, selectors []string, results testResults, baseline []*runtimeRecords, static *staticResult) (rep *report, status int, err error) {
	catalog, err := opts.catalogs(rt.tier)
	if err != nil {
		return nil, exitUsage, err
	}
	var join *resultsJoin
	if results != nil {
		joined := joinResults(rt, results)
		join = &joined
	}
	c := classify(rt, catalog)
	if static != nil && !static.Skipped {
		c.applyStatic(static)
	}
	rep = buildReport(c)
	rep.Results = join
	status = exitOK
	if baseline != nil {
		rep.Baseline, err = compareWithBaseline(opts, c, rt, baseline)
		if err != nil {
			return nil, exitUsage, err
		}
		if len(rep.Baseline.Lost) > 0 {
			status = exitFindings
		}
	}
	if opts.check {
		rep.Check = checkRuntime(rep, selectors)
		if !rep.Check.Passed {
			status = exitFindings
		}
	}
	return rep, status, nil
}

// errBaselineRuntime is a baseline directory holding no run of the runtime
// being compared, which is a comparison against nothing.
var errBaselineRuntime = errors.New("the baseline holds no run of this runtime")

// compareWithBaseline finds the baseline run of the same runtime and compares
// the two.
func compareWithBaseline(opts options, c *classification, rt *runtimeRecords, baseline []*runtimeRecords) (*baselineResult, error) {
	for _, old := range baseline {
		if old.key != rt.key {
			continue
		}
		catalog, err := opts.catalogs(old.tier)
		if err != nil {
			return nil, err
		}
		return compareBaseline(c, classify(old, catalog), old.dir), nil
	}
	return nil, fmt.Errorf("%s: %w (%s)", opts.baseline, errBaselineRuntime, rt.key)
}

// writeOutputs writes the JSON report, the TSV work list and the Markdown
// summary wherever the options put them.
func writeOutputs(opts options, reports []*report, stdout io.Writer) error {
	if opts.report {
		for _, rep := range reports {
			writeGapTSV(stdout, rep)
		}
	}
	if err := writeReportJSON(opts, reports, stdout); err != nil {
		return err
	}
	return writeSummary(opts, reports, stdout)
}

// writeReportJSON writes the JSON report to -o, or to stdout when nothing
// else claimed stdout: neither -report nor -summary - was given.
//
// The JSON is what a reader parses and the other two are what a reader
// reads, and one stream cannot be both. A summary on stdout after a JSON
// document is a document nothing parses, so the JSON stays on stdout only
// while it is the only thing there; a caller who wants both names a file
// for the JSON with -o.
func writeReportJSON(opts options, reports []*report, stdout io.Writer) error {
	if opts.output == "" {
		if opts.report || opts.summary == summaryToStdout {
			return nil
		}
		return writeJSON(stdout, reports)
	}
	file, err := os.Create(filepath.Clean(opts.output))
	if err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	defer func() { _ = file.Close() }()
	if writeErr := writeJSON(file, reports); writeErr != nil {
		return writeErr
	}
	for _, rep := range reports {
		fmt.Fprintf(stdout, "%s: L1 %d/%d, L2 %d, L3 %d, written to %s\n",
			rep.Runtime, rep.Summary.L1, rep.Summary.CatalogActions, rep.Summary.L2, rep.Summary.L3, opts.output)
	}
	return nil
}

// summaryToStdout is the -summary value that puts the Markdown on stdout.
const summaryToStdout = "-"

// writeSummary writes the Markdown summary to -summary.
//
// The file is appended to rather than replaced: the path a CI step passes is
// GITHUB_STEP_SUMMARY, which every step of a job writes its own section to,
// and truncating it would drop what the steps before this one said.
func writeSummary(opts options, reports []*report, stdout io.Writer) error {
	switch opts.summary {
	case "":
		return nil
	case summaryToStdout:
		writeMarkdownSummary(stdout, reports)
		return nil
	}
	// #nosec G304 -- the path is the one the caller named on the command line.
	file, err := os.OpenFile(filepath.Clean(opts.summary), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	defer func() { _ = file.Close() }()
	writeMarkdownSummary(file, reports)
	return nil
}
