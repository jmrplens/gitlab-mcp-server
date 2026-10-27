// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// Finding is one place a text reaches a person without the catalog, or the
// catalog holds something no render can reach.
type Finding struct {
	// Rule is the rule the finding breaks, I1 to I5.
	Rule    string `json:"rule"`
	Package string `json:"package"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	// Func is the declaration the finding sits in: a function as its file
	// spells it, the variable a package-level initializer belongs to, the
	// catalog's english variable for an entry, or a key constant.
	Func   string `json:"func"`
	Reason string `json:"reason"`
}

// Summary is what a run looked at, so a clean report says what it was clean
// over rather than only that it was clean.
type Summary struct {
	Packages int `json:"packages"`
	// Renders are the calls of T and New outside the catalog package.
	Renders int `json:"renders"`
	// Keys are the entries of the English catalog.
	Keys int `json:"keys"`
	// WholeTree says whether the run was over the default patterns, the only
	// run that asks whether a key is dead.
	WholeTree bool `json:"whole_tree"`
	sourcegate.Counts
}

// Report is the whole answer: what the run looked at, and the verdict the
// declaration table and the load give. Its unjudged files are the ones the
// load left out that import the catalog, which this run could not see.
type Report struct {
	Summary Summary `json:"summary"`
	sourcegate.Verdict[Finding]
}

// buildReport holds what the scan found against the declaration table, and
// orders the result.
//
// Only a conversion to the Key type is held to the table; every other finding
// is reported as it stands. The stale rule is scoped to the packages the scan
// looked at: a declaration naming a package this run never loaded is neither
// used nor stale, only out of view.
func buildReport(found *scanner, declared map[string]sourcegate.Declaration) Report {
	table := sourcegate.Table{Declared: declared, Categories: categories}
	verdict := sourcegate.Judge(found.conversions, declarationKey, table, found.packages, found.unjudged)
	verdict.Findings = append(verdict.Findings, found.findings...)
	sortFindings(verdict.Findings)
	sortFindings(verdict.Excused)
	return Report{
		Summary: Summary{
			Packages:  len(found.packages),
			Renders:   found.renders,
			Keys:      len(found.cat.templates),
			WholeTree: found.wholeTree,
			Counts:    verdict.Counts(),
		},
		Verdict: verdict,
	}
}

// Write renders the report: each finding on its own line, then the
// declarations and files that need a reader, then one summary line.
func (r Report) Write(out io.Writer, verbose bool) {
	for _, finding := range r.Findings {
		fmt.Fprintf(out, "%s:%d: %s %s (in %s)\n", finding.File, finding.Line, finding.Rule, finding.Reason, finding.Func)
	}
	if verbose {
		for _, finding := range r.Excused {
			fmt.Fprintf(out, "%s:%d: %s %s (in %s), excused by its declaration\n", finding.File, finding.Line, finding.Rule, finding.Reason, finding.Func)
		}
	}
	for _, key := range r.Stale {
		fmt.Fprintf(out, "%s: declared to convert a value to the catalog's Key type, and this run found no such conversion there\n", key)
	}
	for _, key := range r.Unknown {
		fmt.Fprintf(out, "%s: declared with a category that is not one of the defined ones\n", key)
	}
	for _, file := range r.Unjudged {
		fmt.Fprintf(out, "%s: left out of this load by its build constraints and imports the message catalog, so nothing in it was judged\n", file)
	}
	if verbose || !r.OK() {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "%s: %d renders of the message catalog in %d packages, against %d English templates; %d findings, %d excused by a declaration",
		toolName, r.Summary.Renders, r.Summary.Packages, r.Summary.Keys, r.Summary.Findings, r.Summary.Excused)
	if !r.Summary.WholeTree {
		fmt.Fprint(out, "; a key nothing references is judged only over the default patterns")
	}
	fmt.Fprintln(out)
}

// sortFindings puts findings in a stable order: by file and line, then by rule
// and reason, so two findings on one line read the same in every run.
func sortFindings(found []Finding) {
	slices.SortFunc(found, func(a, b Finding) int {
		return cmp.Or(
			strings.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			strings.Compare(a.Rule, b.Rule),
			strings.Compare(a.Reason, b.Reason),
		)
	})
}
