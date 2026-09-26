package main

import (
	"slices"
	"strings"
	"testing"
)

// sampleFinding is one finding to render, with every field distinct so a
// field printed in another's place would show.
func sampleFinding(file string, line int, fn string) Finding {
	return Finding{
		Package: "internal/tools/x",
		File:    file,
		Line:    line,
		Func:    fn,
		Callee:  "client.GL().Version.GetVersion",
		Reason:  reasonMissing,
	}
}

// TestReport_Write_RendersEverySection holds the whole rendering of a report
// that has something in every section, verbose, in the order a reader meets
// them: the findings, the excused calls, the stale and the unknown
// declarations, the unjudged files, a blank line, the summary. Every figure on
// the summary is one no other figure shares, so one printed in another's
// place shows.
func TestReport_Write_RendersEverySection(t *testing.T) {
	report := Report{
		Summary:  Summary{Packages: 7, Calls: 42, Forwarded: 3, Rebound: 5, Findings: 1, Excused: 2},
		Findings: []Finding{sampleFinding("internal/tools/x/a.go", 12, "Get")},
		Excused: []Finding{
			sampleFinding("internal/tools/x/b.go", 30, "Keep"),
			sampleFinding("internal/tools/x/c.go", 8, "Hold"),
		},
		Stale:    []string{"internal/tools/x:Gone"},
		Unknown:  []string{"internal/tools/x:Keep"},
		Unjudged: []string{"internal/tools/x/hidden.go"},
	}
	var out strings.Builder
	report.write(&out, true)
	want := "internal/tools/x/a.go:12: client.GL().Version.GetVersion " + reasonMissing + " (in Get)\n" +
		"internal/tools/x/b.go:30: client.GL().Version.GetVersion " + reasonMissing + " (in Keep), excused by its declaration\n" +
		"internal/tools/x/c.go:8: client.GL().Version.GetVersion " + reasonMissing + " (in Hold), excused by its declaration\n" +
		"internal/tools/x:Gone: declared to reach client-go without the caller's context, and this run found no such call there\n" +
		"internal/tools/x:Keep: declared with a category that is not one of the defined ones\n" +
		"internal/tools/x/hidden.go: left out of this load by its build constraints and imports client-go, so no call in it was judged\n" +
		"\n" +
		"audit_sdk_context: 42 calls building or sending a request in 7 packages " +
		"(3 clean by forwarding to their own caller, 5 by rebinding after they were built); " +
		"1 without the caller's context, 2 excused by a declaration\n"
	if out.String() != want {
		t.Fatalf("write =\n%s\nwant\n%s", out.String(), want)
	}
}

// TestReport_Write_QuietRunHidesTheExcused: without -v an excused call is not
// printed, because it is not something to act on, and a clean run is the
// summary line alone.
func TestReport_Write_QuietRunHidesTheExcused(t *testing.T) {
	report := Report{
		Summary: Summary{Packages: 4, Calls: 6, Excused: 1},
		Excused: []Finding{sampleFinding("internal/tools/x/b.go", 30, "Keep")},
	}
	var out strings.Builder
	report.write(&out, false)
	want := "audit_sdk_context: 6 calls building or sending a request in 4 packages " +
		"(0 clean by forwarding to their own caller, 0 by rebinding after they were built); " +
		"0 without the caller's context, 1 excused by a declaration\n"
	if out.String() != want {
		t.Fatalf("write = %q, want the summary line alone, %q", out.String(), want)
	}
}

// TestReport_Ok_FailsOnEachKindOfFinding: every section but the excused calls
// fails the gate on its own.
func TestReport_Ok_FailsOnEachKindOfFinding(t *testing.T) {
	tests := []struct {
		name   string
		report Report
		want   bool
	}{
		{"nothing", Report{}, true},
		{"only excused", Report{Excused: []Finding{{}}}, true},
		{"a finding", Report{Findings: []Finding{{}}}, false},
		{"a stale declaration", Report{Stale: []string{"k"}}, false},
		{"an unknown category", Report{Unknown: []string{"k"}}, false},
		{"an unjudged file", Report{Unjudged: []string{"f"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.report.ok(); got != tt.want {
				t.Fatalf("ok() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBuildReport_HoldsTheScanAgainstTheTable: a finding in a declared
// function is excused and counted apart, the rest are findings in file and
// line order, the counters are carried over, and a declaration naming a
// package the run did not load is out of view rather than stale, although a
// category nobody defined is reported wherever its declaration points. Every
// counter on the summary comes out at a figure no other one shares, so one
// counted in another's place shows.
func TestBuildReport_HoldsTheScanAgainstTheTable(t *testing.T) {
	found := newScanner("/repo", nil)
	for _, pkg := range []string{"internal/tools/x", "internal/tools/p1", "internal/tools/p2", "internal/tools/p3", "internal/tools/p4", "internal/tools/p5"} {
		found.packages[pkg] = struct{}{}
	}
	found.calls, found.forwarded, found.rebound = 20, 8, 7
	found.findings = []Finding{
		sampleFinding("internal/tools/x/b.go", 3, "Later"),
		sampleFinding("internal/tools/x/a.go", 20, "Second"),
		sampleFinding("internal/tools/x/a.go", 5, "First"),
		sampleFinding("internal/tools/x/a.go", 5, "SameLine"),
		sampleFinding("internal/tools/x/c.go", 1, "Keep"),
	}
	found.unjudged = []string{"z.go", "a.go", "m.go", "b.go", "y.go"}
	report := buildReport(found, map[string]declaration{
		"internal/tools/x:Keep":   {category: categoryOutlivesTheCall, reason: "r"},
		"internal/tools/x:Gone":   {category: categoryOutlivesTheCall, reason: "r"},
		"internal/tools/y:Absent": {category: categoryOutlivesTheCall, reason: "r"},
		"internal/tools/x:Later":  {category: "made-up", reason: "r"},
		"internal/tools/q:A":      {category: "invented", reason: "r"},
		"internal/tools/q:B":      {category: "", reason: "r"},
		"internal/tools/q:C":      {category: "also-invented", reason: "r"},
	})
	var order []string
	for _, finding := range report.Findings {
		order = append(order, finding.Func)
	}
	if want := []string{"First", "SameLine", "Second"}; !slices.Equal(order, want) {
		t.Fatalf("findings = %v, want %v", order, want)
	}
	if len(report.Excused) != 2 || report.Excused[0].Func != "Later" || report.Excused[1].Func != "Keep" {
		t.Fatalf("excused = %+v, want Later and Keep", report.Excused)
	}
	if !slices.Equal(report.Stale, []string{"internal/tools/x:Gone"}) {
		t.Fatalf("stale = %v, want only the declaration of a loaded package that excused nothing", report.Stale)
	}
	if want := []string{"internal/tools/q:A", "internal/tools/q:B", "internal/tools/q:C", "internal/tools/x:Later"}; !slices.Equal(report.Unknown, want) {
		t.Fatalf("unknown = %v, want %v", report.Unknown, want)
	}
	if want := []string{"a.go", "b.go", "m.go", "y.go", "z.go"}; !slices.Equal(report.Unjudged, want) {
		t.Fatalf("unjudged = %v, want them sorted, %v", report.Unjudged, want)
	}
	want := Summary{Packages: 6, Calls: 20, Forwarded: 8, Rebound: 7, Findings: 3, Excused: 2, Stale: 1, Unknown: 4, Unjudged: 5}
	if report.Summary != want {
		t.Fatalf("summary = %+v, want %+v", report.Summary, want)
	}
}

// TestScan_ADeclaredFunction_IsExcusedAndAStaleOneReported runs the table
// through a real scan: the declared function's call is excused, and a
// declaration for a function in the loaded package that has no such call is
// reported stale.
func TestScan_ADeclaredFunction_IsExcusedAndAStaleOneReported(t *testing.T) {
	report := auditFixture(t, map[string]string{"fixture.go": fixtureHeader + `
func Detached(c *gl.Client) {
	_, _, _ = c.Version.GetVersion()
}

func Bound(ctx context.Context, c *gl.Client) {
	_, _, _ = c.Version.GetVersion(gl.WithContext(ctx))
}
`}, map[string]declaration{
		fixtureDir + ":Detached": {category: categoryOutlivesTheCall, reason: "the fixture's request outlives its call"},
		fixtureDir + ":Bound":    {category: categoryOutlivesTheCall, reason: "passes the context, so this excuses nothing"},
	})
	if len(report.Findings) != 0 || len(report.Excused) != 1 || report.Excused[0].Func != "Detached" {
		t.Fatalf("findings = %+v, excused = %+v, want the one call excused", report.Findings, report.Excused)
	}
	if !slices.Equal(report.Stale, []string{fixtureDir + ":Bound"}) {
		t.Fatalf("stale = %v, want the declaration that excused nothing", report.Stale)
	}
}
