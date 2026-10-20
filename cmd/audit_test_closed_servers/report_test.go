package main

import (
	"slices"
	"strings"
	"testing"
)

// finding is a Finding with only what the report tests read.
func finding(file string, line int, server string) Finding {
	return Finding{Package: "p", File: file, Line: line, Func: "TestX", Server: server, Use: "url", UseLine: line + 1}
}

// TestReport_Write_RendersEverySection holds the whole output of a report
// that has something in every section: the findings, the excused ones only
// when verbose, the stale and unknown declarations, the unjudged files, a
// blank line, and the summary.
func TestReport_Write_RendersEverySection(t *testing.T) {
	report := Report{
		Summary:  Summary{Packages: 3, Closes: 5, Findings: 1, Excused: 1},
		Findings: []Finding{finding("a_test.go", 7, "srv")},
		Excused:  []Finding{finding("b_test.go", 9, "other")},
		Stale:    []string{"p:TestGone"},
		Unknown:  []string{"p:TestInvented"},
		Unjudged: []string{"c_windows_test.go"},
	}
	const findingLine = "a_test.go:7: srv is closed while its address is still used, through url at line 8 (in TestX)\n"
	const excusedLine = "b_test.go:9: other is closed while its address is still used, through url at line 10 (in TestX), excused by its declaration\n"
	const rest = "p:TestGone: declared to use a closed server's address on purpose, and this run found no such use there\n" +
		"p:TestInvented: declared with a category that is not one of the defined ones\n" +
		"c_windows_test.go: left out of this load by its build constraints and imports net/http/httptest, so no server in it was judged\n" +
		"\n" +
		"audit_test_closed_servers: 5 closes of an httptest server judged in 3 packages; " +
		"1 made while its address is still used, 1 excused by a declaration\n"
	for _, tt := range []struct {
		name    string
		verbose bool
		want    string
	}{
		{name: "quiet", want: findingLine + rest},
		{name: "verbose", verbose: true, want: findingLine + excusedLine + rest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			report.write(&out, tt.verbose)
			if out.String() != tt.want {
				t.Errorf("write =\n%s\nwant\n%s", out.String(), tt.want)
			}
		})
	}
}

// TestReport_Ok_FailsOnEachKindOfProblem verifies every section but the
// excused one fails the gate on its own, and an empty report passes it.
func TestReport_Ok_FailsOnEachKindOfProblem(t *testing.T) {
	for _, tt := range []struct {
		name   string
		report Report
		want   bool
	}{
		{name: "nothing", want: true},
		{name: "only excused", report: Report{Excused: []Finding{finding("a_test.go", 1, "srv")}}, want: true},
		{name: "a finding", report: Report{Findings: []Finding{finding("a_test.go", 1, "srv")}}},
		{name: "a stale declaration", report: Report{Stale: []string{"p:F"}}},
		{name: "an unknown category", report: Report{Unknown: []string{"p:F"}}},
		{name: "an unjudged file", report: Report{Unjudged: []string{"a_test.go"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.report.ok(); got != tt.want {
				t.Errorf("ok() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSortFindings_OrdersByFileThenLineThenServer verifies a report reads the
// same on every run, two closes on one line included.
func TestSortFindings_OrdersByFileThenLineThenServer(t *testing.T) {
	found := []Finding{
		finding("b_test.go", 1, "srv"),
		finding("a_test.go", 9, "srv"),
		finding("a_test.go", 2, "second"),
		finding("a_test.go", 2, "first"),
	}
	sortFindings(found)
	var got []string
	for _, f := range found {
		got = append(got, f.File+":"+f.Server)
	}
	if want := []string{"a_test.go:first", "a_test.go:second", "a_test.go:srv", "b_test.go:srv"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}
