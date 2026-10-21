package main

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// Summary is what a run looked at, so a clean report says what it was clean
// over rather than only that it was clean.
type Summary struct {
	Packages int `json:"packages"`
	// Calls are the calls judged: those that reached a request option
	// parameter, and the request constructors of go-retryablehttp.
	Calls int `json:"calls"`
	// Forwarded and Rebound are the clean calls that rest on the two rules
	// that are not an option in the call itself.
	Forwarded int `json:"forwarded"`
	Rebound   int `json:"rebound"`
	sourcegate.Counts
}

// Report is the whole answer: what the run looked at, and the verdict the
// declaration table and the load give. Its unjudged files are the ones the
// load left out that import client-go, whose calls this run could not see.
type Report struct {
	Summary Summary `json:"summary"`
	sourcegate.Verdict[Finding]
}

// buildReport holds what the scan found, in the report's order, against the
// declaration table.
//
// The stale rule is scoped to the packages the scan looked at: a declaration
// naming a package this run never loaded is neither used nor stale, only out
// of view.
func buildReport(found *scanner, declared map[string]sourcegate.Declaration) Report {
	sortFindings(found.findings)
	table := sourcegate.Table{Declared: declared, Categories: categories}
	verdict := sourcegate.Judge(found.findings, declarationKey, table, found.packages, found.unjudged)
	return Report{
		Summary: Summary{
			Packages:  len(found.packages),
			Calls:     found.calls,
			Forwarded: found.forwarded,
			Rebound:   found.rebound,
			Counts:    verdict.Counts(),
		},
		Verdict: verdict,
	}
}

// Write renders the report: each finding on its own line, then the
// declarations and files that need a reader, then one summary line.
func (r Report) Write(out io.Writer, verbose bool) {
	for _, finding := range r.Findings {
		fmt.Fprintf(out, "%s:%d: %s %s (in %s)\n", finding.File, finding.Line, finding.Callee, finding.Reason, finding.Func)
	}
	if verbose {
		for _, finding := range r.Excused {
			fmt.Fprintf(out, "%s:%d: %s %s (in %s), excused by its declaration\n", finding.File, finding.Line, finding.Callee, finding.Reason, finding.Func)
		}
	}
	for _, key := range r.Stale {
		fmt.Fprintf(out, "%s: declared to reach client-go without the caller's context, and this run found no such call there\n", key)
	}
	for _, key := range r.Unknown {
		fmt.Fprintf(out, "%s: declared with a category that is not one of the defined ones\n", key)
	}
	for _, file := range r.Unjudged {
		fmt.Fprintf(out, "%s: left out of this load by its build constraints and imports client-go, so no call in it was judged\n", file)
	}
	if verbose || !r.OK() {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "%s: %d calls building or sending a request in %d packages "+
		"(%d clean by forwarding to their own caller, %d by rebinding after they were built); "+
		"%d without the caller's context, %d excused by a declaration\n",
		toolName, r.Summary.Calls, r.Summary.Packages,
		r.Summary.Forwarded, r.Summary.Rebound,
		r.Summary.Findings, r.Summary.Excused)
}

// sortFindings puts findings in a stable order: by file, then by line, and two
// calls on one line in the order the walk met them.
func sortFindings(found []Finding) {
	slices.SortStableFunc(found, func(a, b Finding) int {
		return cmp.Or(strings.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
}
