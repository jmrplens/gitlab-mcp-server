package main

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// Summary is what a run looked at, so a clean report says what it was clean
// over rather than only that it was clean.
type Summary struct {
	Packages int `json:"packages"`
	// Closes are the closes of an httptest server judged: every one but those
	// that wait for the top-level function's test to end, which nothing in
	// the function can follow.
	Closes int `json:"closes"`
	sourcegate.Counts
}

// Report is the whole answer: what the run looked at, and the verdict the
// declaration table and the load give. Its unjudged files are the ones the
// load left out that import net/http/httptest, whose servers this run could
// not see.
type Report struct {
	Summary Summary `json:"summary"`
	sourcegate.Verdict[Finding]
}

// buildReport holds what the scan found, in the report's order, against the
// declaration table.
func buildReport(found *scanner, declared map[string]sourcegate.Declaration) Report {
	findings := slices.Collect(maps.Values(found.findings))
	sortFindings(findings)
	table := sourcegate.Table{Declared: declared, Categories: categories}
	verdict := sourcegate.Judge(findings, declarationKey, table, found.packages, slices.Collect(maps.Keys(found.unjudged)))
	return Report{
		Summary: Summary{Packages: len(found.packages), Closes: len(found.closes), Counts: verdict.Counts()},
		Verdict: verdict,
	}
}

// Write renders the report: each finding on its own line, then the
// declarations and files that need a reader, then one summary line.
func (r Report) Write(out io.Writer, verbose bool) {
	for _, finding := range r.Findings {
		fmt.Fprintf(out, "%s\n", describe(finding))
	}
	if verbose {
		for _, finding := range r.Excused {
			fmt.Fprintf(out, "%s, excused by its declaration\n", describe(finding))
		}
	}
	for _, key := range r.Stale {
		fmt.Fprintf(out, "%s: declared to use a closed server's address on purpose, and this run found no such use there\n", key)
	}
	for _, key := range r.Unknown {
		fmt.Fprintf(out, "%s: declared with a category that is not one of the defined ones\n", key)
	}
	for _, file := range r.Unjudged {
		fmt.Fprintf(out, "%s: left out of this load by its build constraints and imports net/http/httptest, so no server in it was judged\n", file)
	}
	if verbose || !r.OK() {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "%s: %d closes of an httptest server judged in %d packages; "+
		"%d made while its address is still used, %d excused by a declaration\n",
		toolName, r.Summary.Closes, r.Summary.Packages, r.Summary.Findings, r.Summary.Excused)
}

// describe is one finding as a line of the report.
func describe(finding Finding) string {
	return fmt.Sprintf("%s:%d: %s is closed while its address is still used, through %s at line %d (in %s)",
		finding.File, finding.Line, finding.Server, finding.Use, finding.UseLine, finding.Func)
}

// sortFindings puts findings in a stable order: by file, then by line, then by
// the server's name, which is what tells two closes on one line apart.
func sortFindings(found []Finding) {
	slices.SortFunc(found, func(a, b Finding) int {
		return cmp.Or(strings.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line), strings.Compare(a.Server, b.Server))
	})
}
