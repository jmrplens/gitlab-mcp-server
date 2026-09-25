package main

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
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
	Findings  int `json:"findings"`
	Excused   int `json:"excused"`
	Stale     int `json:"stale_declarations"`
	Unknown   int `json:"unknown_categories"`
	Unjudged  int `json:"unjudged_files"`
}

// Report is the whole answer.
type Report struct {
	Summary  Summary   `json:"summary"`
	Findings []Finding `json:"findings"`
	// Excused are the findings a declaration answers, kept so a verbose run
	// can show what the table is standing in front of.
	Excused []Finding `json:"excused,omitempty"`
	// Stale are the declarations that excused nothing this run, on the terms
	// every declaration table in this repository is held to: one that has
	// stopped describing the tree is itself a finding.
	Stale []string `json:"stale_declarations,omitempty"`
	// Unknown are the declarations naming a category nobody defined.
	Unknown []string `json:"unknown_categories,omitempty"`
	// Unjudged are the files the load left out that import client-go, whose
	// calls this run could not see.
	Unjudged []string `json:"unjudged_files,omitempty"`
}

// buildReport holds what the scan found against the declaration table.
//
// The stale rule is scoped to the packages the scan looked at: a declaration
// naming a package this run never loaded is neither used nor stale, only out
// of view.
func buildReport(found *scanner, declared map[string]declaration) Report {
	report := Report{Summary: Summary{
		Packages:  len(found.packages),
		Calls:     found.calls,
		Forwarded: found.forwarded,
		Rebound:   found.rebound,
	}}
	sortFindings(found.findings)
	excused := map[string]struct{}{}
	for _, finding := range found.findings {
		key := declarationKey(finding)
		if _, allowed := declared[key]; allowed {
			excused[key] = struct{}{}
			report.Excused = append(report.Excused, finding)
			continue
		}
		report.Findings = append(report.Findings, finding)
	}
	report.Stale = staleDeclarations(declared, excused, found.packages)
	report.Unknown = unknownCategories(declared)
	report.Unjudged = found.unjudged
	sort.Strings(report.Unjudged)
	report.Summary.Findings = len(report.Findings)
	report.Summary.Excused = len(report.Excused)
	report.Summary.Stale = len(report.Stale)
	report.Summary.Unknown = len(report.Unknown)
	report.Summary.Unjudged = len(report.Unjudged)
	return report
}

// ok reports whether the run found nothing to answer for.
func (r Report) ok() bool {
	return len(r.Findings) == 0 && len(r.Stale) == 0 && len(r.Unknown) == 0 && len(r.Unjudged) == 0
}

// write renders the report: each finding on its own line, then the
// declarations and files that need a reader, then one summary line.
func (r Report) write(out io.Writer, verbose bool) {
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
	if verbose || !r.ok() {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "%s: %d calls building or sending a request in %d packages, %d without the caller's context "+
		"(%d forwarded to their own caller, %d rebound after they were built, %d excused by a declaration)\n",
		toolName, r.Summary.Calls, r.Summary.Packages, r.Summary.Findings,
		r.Summary.Forwarded, r.Summary.Rebound, r.Summary.Excused)
}

// sortFindings puts findings in a stable order: by file, then by line, and two
// calls on one line in the order the walk met them.
func sortFindings(found []Finding) {
	slices.SortStableFunc(found, func(a, b Finding) int {
		return cmp.Or(strings.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
}
