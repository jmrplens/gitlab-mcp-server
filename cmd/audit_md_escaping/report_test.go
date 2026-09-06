package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleReport is a report with one of everything a run can produce, so the
// writers are exercised on every section they know how to print.
func sampleReport() Report {
	report := Report{
		Findings: []Finding{
			{
				Package: "internal/tools/issues", File: "internal/tools/issues/markdown.go", Line: 42,
				Func: "formatIssue", Context: "table-cell", Verb: "%s", Expression: "issue.Title",
				Wants: "toolutil.EscapeMdTableCell", Reason: "a field of a value filled from a GitLab response",
			},
			{
				Package: "internal/tools/issues", File: "internal/tools/issues/markdown.go", Line: 43,
				Context: "heading", Verb: "%s", Expression: "issue.Title",
				Wants: "toolutil.EscapeMdHeading", Reason: "a field of a value filled from a GitLab response",
			},
			{
				Package: "internal/tools/users", File: "internal/tools/users/markdown.go", Line: 7,
				Func: "formatUser", Context: "list-item", Verb: "%s", Expression: "user.Name",
				Wants: "toolutil.EscapeMdTableCell", Reason: "a field of a value filled from a GitLab response",
			},
		},
		Unresolved: []Finding{{
			Package: "internal/tools/dynamic", File: "internal/tools/dynamic/register.go", Line: 9,
			Func: "formatFind", Context: "table-cell", Verb: "%s", Expression: "render(result)",
			Wants: "toolutil.EscapeMdTableCell", Reason: "the call is of a function value rather than of a named function",
		}},
		Excused: []Finding{{
			Package: "internal/tools/dynamic", File: "internal/tools/dynamic/register.go", Line: 11,
			Func: "formatFind", Context: "table-cell", Verb: "%s", Expression: "result.ID",
			Wants: "toolutil.EscapeMdTableCell", Reason: "a field of a value filled from a GitLab response",
		}},
		StaleDirectives: []Directive{{
			Package: "internal/tools/dynamic", File: "internal/tools/dynamic/register.go", Line: 3,
			Expression: "result.Retired", Reason: "nothing interpolates this any more",
		}},
	}
	report.Summary.Contexts = "table-cell, heading"
	finish(&report)
	return report
}

// TestWriteReport_Sample_GroupsFindingsByPackage checks the report a person
// reads: one section per package, the helper each finding wants, and the
// counts the walk is planned from.
func TestWriteReport_Sample_GroupsFindingsByPackage(t *testing.T) {
	var out strings.Builder

	writeReport(&out, sampleReport(), false)

	text := out.String()
	for _, want := range []string{
		"=== internal/tools/issues ===",
		"=== internal/tools/users ===",
		"internal/tools/issues/markdown.go:42 formatIssue table-cell %s issue.Title",
		"wants toolutil.EscapeMdTableCell: a field of a value filled from a GitLab response",
		"internal/tools/issues/markdown.go:43 - heading %s issue.Title",
		"=== directives that excuse nothing ===",
		"result.Retired no longer reaches a Markdown construct unescaped",
		"audit_md_escaping: 3 value(s) unescaped in 2 package(s); 1 excused; 1 unresolved; 1 stale directive(s)",
		"by context: heading 1, list-item 1, table-cell 1",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(text, want) {
				t.Errorf("report does not contain %q:\n%s", want, text)
			}
		})
	}
	for _, unwanted := range []string{"excused by directive:", "unresolved:"} {
		t.Run(unwanted, func(t *testing.T) {
			if strings.Contains(text, unwanted) {
				t.Errorf("the quiet report printed the %q section:\n%s", unwanted, text)
			}
		})
	}
}

// TestWriteReport_Verbose_AddsTheBucketsThatAreNotFailures checks that the two
// lists a gate does not fail on are printed when asked for, since a value the
// audit could not follow is its own blind spot rather than a clean answer.
func TestWriteReport_Verbose_AddsTheBucketsThatAreNotFailures(t *testing.T) {
	var out strings.Builder

	writeReport(&out, sampleReport(), true)

	text := out.String()
	for _, want := range []string{
		"=== excused by directive: internal/tools/dynamic ===",
		"=== unresolved: internal/tools/dynamic ===",
		"render(result)",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(text, want) {
				t.Errorf("verbose report does not contain %q:\n%s", want, text)
			}
		})
	}
}

// TestWriteReport_Clean_SaysSoWithoutASection checks the shape of a run with
// nothing to report.
func TestWriteReport_Clean_SaysSoWithoutASection(t *testing.T) {
	var out strings.Builder
	report := Report{Summary: Summary{Contexts: "table-cell"}}
	finish(&report)

	writeReport(&out, report, true)

	text := out.String()
	if strings.Contains(text, "===") {
		t.Errorf("a clean run printed a section:\n%s", text)
	}
	if !strings.Contains(text, "0 value(s) unescaped in 0 package(s)") {
		t.Errorf("a clean run does not say so:\n%s", text)
	}
	if strings.Contains(text, "by context:") {
		t.Errorf("a clean run printed an empty breakdown:\n%s", text)
	}
}

// TestByCount_Breakdown_OrdersByCountThenName checks that a breakdown opens
// with where the work is, and is stable when two counts tie.
func TestByCount_Breakdown_OrdersByCountThenName(t *testing.T) {
	got := byCount(map[string]int{"heading": 2, "table-cell": 9, "list-item": 2, "link-label": 1})

	if want := "table-cell 9, heading 2, list-item 2, link-label 1"; got != want {
		t.Errorf("byCount = %q, want %q", got, want)
	}
}

// TestWriteJSON_Report_RoundTrips checks that the work list on disk is the
// report, since the walk through the findings reads that file rather than the
// terminal.
func TestWriteJSON_Report_RoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backlog.json")
	report := sampleReport()

	if err := writeJSON(path, report); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var decoded Report
	if decodeErr := json.Unmarshal(data, &decoded); decodeErr != nil {
		t.Fatalf("decode: %v", decodeErr)
	}
	if len(decoded.Findings) != len(report.Findings) || decoded.Summary.Findings != report.Summary.Findings {
		t.Errorf("the work list read back as %+v, want the report that was written", decoded.Summary)
	}
	if decoded.StaleDirectives[0].Expression != "result.Retired" {
		t.Errorf("the stale directive did not survive the round trip: %+v", decoded.StaleDirectives)
	}
}

// TestWriteJSON_Failures_AreReported checks both ways writing the work list can
// fail, because a gate that could not write one must say so rather than report
// a clean run.
func TestWriteJSON_Failures_AreReported(t *testing.T) {
	dir := t.TempDir()
	notADirectory := filepath.Join(dir, "file")
	if err := os.WriteFile(notADirectory, []byte("x"), 0o600); err != nil {
		t.Fatalf("prepare: %v", err)
	}

	cases := []struct {
		name   string
		path   string
		report any
		want   string
	}{
		{name: "a path that cannot be written", path: filepath.Join(notADirectory, "backlog.json"), report: Report{}, want: "write"},
		{name: "a report that cannot be encoded", path: filepath.Join(dir, "backlog.json"), report: make(chan int), want: "marshal"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := writeJSON(tc.path, tc.report)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("writeJSON error = %v, want one saying %q", err, tc.want)
			}
		})
	}
}

// TestRelativePath_Paths_RenderBelowTheRoot checks that a finding reads the
// same wherever the audit was run from, and that a file outside the tree keeps
// the only name it has.
func TestRelativePath_Paths_RenderBelowTheRoot(t *testing.T) {
	cases := []struct {
		name string
		path string
		root string
		want string
	}{
		{name: "inside the tree", path: "/repo/internal/tools/x.go", root: "/repo", want: "internal/tools/x.go"},
		{name: "the root itself", path: "/repo", root: "/repo", want: "."},
		{name: "outside the tree", path: "/elsewhere/x.go", root: "/repo", want: "/elsewhere/x.go"},
		{name: "no root at all", path: "/repo/x.go", root: "", want: "/repo/x.go"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := relativePath(tc.path, tc.root); got != tc.want {
				t.Errorf("relativePath(%q, %q) = %q, want %q", tc.path, tc.root, got, tc.want)
			}
		})
	}
}

// TestOrDash_Values_KeepsTheColumn checks that a finding with no enclosing
// formatter still prints a column where one is expected.
func TestOrDash_Values_KeepsTheColumn(t *testing.T) {
	if got := orDash(""); got != "-" {
		t.Errorf("orDash(\"\") = %q, want a dash", got)
	}
	if got := orDash("formatIssue"); got != "formatIssue" {
		t.Errorf("orDash = %q, want the name it was given", got)
	}
}
