package main

import (
	"slices"
	"strings"
	"testing"
)

// TestFinding_String_WithAndWithoutAPosition.
func TestFinding_String_WithAndWithoutAPosition(t *testing.T) {
	with := Finding{Rule: "G8", Subject: "ADM-002", Position: "cmd/server/x.go:3", Message: "m"}
	if got := with.String(); got != "G8 ADM-002: cmd/server/x.go:3: m" {
		t.Fatalf("String = %q", got)
	}
	without := Finding{Rule: "G1", Subject: "ROW", Message: "m"}
	if got := without.String(); got != "G1 ROW: m" {
		t.Fatalf("String = %q", got)
	}
}

// TestReport_Write_PrintsWhatAReaderNeeds: the findings, the pending rows,
// what the exemption table answered only when verbose, and the summary line;
// a report with nothing pending prints no pending line.
func TestReport_Write_PrintsWhatAReaderNeeds(t *testing.T) {
	report := Report{
		Summary:  Summary{Rows: 2, Failures: 1, Sites: 3, Packages: 4, Returns: 5, Refusals: 6, Reasons: 7, Settings: 8, Findings: 1, Pending: 1, Exempted: 1},
		Findings: []Finding{{Rule: "G1", Subject: "ROW", Message: "m"}},
		Pending:  []string{"ROW-002"},
		Excused:  []Excuse{{Key: "cmd/server:x", Part: partNames, Category: categoryTransport, Reason: "r"}},
	}
	summary := "audit_tenancy: 2 rows, 1 failures and 3 declared sites over 4 packages " +
		"(5 refusal returns, 6 refusals, 7 reasons and 8 settings read); 1 findings, 1 rows pending, 1 declarations exempted\n"
	tests := []struct {
		name    string
		report  Report
		verbose bool
		want    string
	}{
		{"quiet", report, false, "G1 ROW: m\npending (G2, G3 and G6 deferred until the layer that moves their values): ROW-002\n" + summary},
		{"verbose", report, true, "G1 ROW: m\npending (G2, G3 and G6 deferred until the layer that moves their values): ROW-002\n" +
			"cmd/server:x: not a decision (names, transport): r\n" + summary},
		{"nothing pending", Report{Summary: report.Summary}, false, summary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			tt.report.write(&out, tt.verbose)
			if out.String() != tt.want {
				t.Fatalf("write = %q, want %q", out.String(), tt.want)
			}
		})
	}
}

// TestSortFindings_OrdersByRuleSubjectAndLine: G10 after G9, a subject in
// order, line 100 after line 99, and an exact repeat printed once.
func TestSortFindings_OrdersByRuleSubjectAndLine(t *testing.T) {
	found := []Finding{
		{Rule: "G10", Subject: "a", Message: "m"},
		{Rule: "G9", Subject: "b", Position: "f.go:100", Message: "m"},
		{Rule: "G9", Subject: "b", Position: "f.go:99", Message: "m"},
		{Rule: "G9", Subject: "a", Message: "z"},
		{Rule: "G9", Subject: "a", Message: "y"},
		{Rule: "G9", Subject: "a", Message: "y"},
	}
	got := sortFindings(found)
	var order []string
	for _, f := range got {
		order = append(order, f.Rule+" "+f.Subject+" "+f.Position+" "+f.Message)
	}
	want := []string{"G9 a  y", "G9 a  z", "G9 b f.go:99 m", "G9 b f.go:100 m", "G10 a  m"}
	if !slices.Equal(order, want) {
		t.Fatalf("order = %q, want %q", order, want)
	}
}

// TestSplitPosition_ReadsTheLineAfterTheLastColon, so a path that holds a
// colon of its own keeps it, and a colon at the start leaves an empty file.
func TestSplitPosition_ReadsTheLineAfterTheLastColon(t *testing.T) {
	for position, want := range map[string]struct {
		file string
		line int
	}{
		"f.go:12":       {"f.go", 12},
		"C:/abs/f.go:3": {"C:/abs/f.go", 3},
		":7":            {"", 7},
		"":              {"", 0},
	} {
		t.Run(position, func(t *testing.T) {
			if file, line := splitPosition(position); file != want.file || line != want.line {
				t.Fatalf("splitPosition = %q, %d, want %q, %d", file, line, want.file, want.line)
			}
		})
	}
}
