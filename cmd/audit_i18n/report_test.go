// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// reportWith is a report holding the given findings and figures, for the
// writer's own cases.
func reportWith(verdict sourcegate.Verdict[Finding], summary Summary) Report {
	summary.Counts = verdict.Counts()
	return Report{Summary: summary, Verdict: verdict}
}

// finding is one finding for the writer's cases.
func finding(file string, line int, rule, fn, reason string) Finding {
	return Finding{Rule: rule, Package: "p", File: file, Line: line, Func: fn, Reason: reason}
}

// TestReport_Write_RendersEveryPartOnce holds the whole text of a report: the
// findings, what the declarations excused when verbose, each line the table
// and the load need a reader for, a blank line before the summary when
// anything stands above it, and a summary that says what the run was over,
// with the note a narrowed run carries.
func TestReport_Write_RendersEveryPartOnce(t *testing.T) {
	failing := reportWith(sourcegate.Verdict[Finding]{
		Findings: []Finding{finding("a.go", 3, "I4", "F", "uses i18n.Verbatim")},
		Excused:  []Finding{finding("b.go", 7, "I2", "G", "converts s")},
		Stale:    []string{"p:Stale"},
		Unknown:  []string{"p:Odd"},
		Unjudged: []string{"c.go"},
	}, Summary{Packages: 2, Renders: 5, Keys: 4, WholeTree: true})
	clean := reportWith(sourcegate.Verdict[Finding]{}, Summary{Packages: 1, Renders: 0, Keys: 1})

	cases := []struct {
		name    string
		report  Report
		verbose bool
		want    string
	}{
		{
			name: "a failing run", report: failing,
			want: "a.go:3: I4 uses i18n.Verbatim (in F)\n" +
				"p:Stale: declared to convert a value to the catalog's Key type, and this run found no such conversion there\n" +
				"p:Odd: declared with a category that is not one of the defined ones\n" +
				"c.go: left out of this load by its build constraints and imports the message catalog, so nothing in it was judged\n" +
				"\n" +
				"audit_i18n: 5 renders of the message catalog in 2 packages, against 4 English templates; 1 findings, 1 excused by a declaration\n",
		},
		{
			name: "a failing run, verbose", report: failing, verbose: true,
			want: "a.go:3: I4 uses i18n.Verbatim (in F)\n" +
				"b.go:7: I2 converts s (in G), excused by its declaration\n" +
				"p:Stale: declared to convert a value to the catalog's Key type, and this run found no such conversion there\n" +
				"p:Odd: declared with a category that is not one of the defined ones\n" +
				"c.go: left out of this load by its build constraints and imports the message catalog, so nothing in it was judged\n" +
				"\n" +
				"audit_i18n: 5 renders of the message catalog in 2 packages, against 4 English templates; 1 findings, 1 excused by a declaration\n",
		},
		{
			name: "a clean narrowed run", report: clean,
			want: "audit_i18n: 0 renders of the message catalog in 1 packages, against 1 English templates; 0 findings, 0 excused by a declaration; " +
				"a key nothing references is judged only over the default patterns\n",
		},
		{
			name: "a clean narrowed run, verbose", report: clean, verbose: true,
			want: "\naudit_i18n: 0 renders of the message catalog in 1 packages, against 1 English templates; 0 findings, 0 excused by a declaration; " +
				"a key nothing references is judged only over the default patterns\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			tc.report.Write(&out, tc.verbose)
			if got := out.String(); got != tc.want {
				t.Errorf("Write =\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

// TestSortFindings_Cases_OrdersByFileLineRuleAndReason holds each key of the
// order, the later ones only where the earlier tie.
func TestSortFindings_Cases_OrdersByFileLineRuleAndReason(t *testing.T) {
	found := []Finding{
		finding("b.go", 1, "I1", "F", "a"),
		finding("a.go", 9, "I1", "F", "a"),
		finding("a.go", 2, "I2", "F", "a"),
		finding("a.go", 2, "I1", "F", "z"),
		finding("a.go", 2, "I1", "F", "b"),
	}
	sortFindings(found)
	got := make([]string, len(found))
	for i, f := range found {
		got[i] = f.File + ":" + f.Rule + ":" + f.Reason + ":" + strconv.Itoa(f.Line)
	}
	want := []string{"a.go:I1:b:2", "a.go:I1:z:2", "a.go:I2:a:2", "a.go:I1:a:9", "b.go:I1:a:1"}
	if !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}
