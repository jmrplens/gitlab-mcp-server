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
	// Closes are the closes of an httptest server judged: every one but those
	// that wait for the top-level function's test to end, which nothing in
	// the function can follow.
	Closes   int `json:"closes"`
	Findings int `json:"findings"`
	Excused  int `json:"excused"`
	Stale    int `json:"stale_declarations"`
	Unknown  int `json:"unknown_categories"`
	Unjudged int `json:"unjudged_files"`
}

// Report is the whole answer.
type Report struct {
	Summary  Summary   `json:"summary"`
	Findings []Finding `json:"findings"`
	// Excused are the findings a declaration answers, kept so a verbose run
	// can show what the table is standing in front of.
	Excused []Finding `json:"excused,omitempty"`
	// Stale are the declarations that excused nothing this run.
	Stale []string `json:"stale_declarations,omitempty"`
	// Unknown are the declarations naming a category nobody defined.
	Unknown []string `json:"unknown_categories,omitempty"`
	// Unjudged are the files the load left out that import
	// net/http/httptest, whose servers this run could not see.
	Unjudged []string `json:"unjudged_files,omitempty"`
}

// buildReport holds what the scan found against the declaration table.
func buildReport(found *scanner, declared map[string]declaration) Report {
	report := Report{Summary: Summary{Packages: len(found.packages), Closes: len(found.closes)}}
	findings := make([]Finding, 0, len(found.findings))
	for _, finding := range found.findings {
		findings = append(findings, finding)
	}
	sortFindings(findings)
	excused := map[string]struct{}{}
	for _, finding := range findings {
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
	for file := range found.unjudged {
		report.Unjudged = append(report.Unjudged, file)
	}
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
	if verbose || !r.ok() {
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
