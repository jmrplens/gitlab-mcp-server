package main

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// auditFixture runs the audit over the shared fixture with every context
// judged.
func auditFixture(t *testing.T) Report {
	t.Helper()
	prog := loadFixture(t, caseFixture)
	sel, err := parseContexts(allContexts)
	if err != nil {
		t.Fatalf("parseContexts: %v", err)
	}
	return audit(prog, sel, repoRoot(t))
}

// entries renders the fixture's findings as "package context expression"
// lines, which is what the tests below compare: a line is stable across edits
// to the fixture in a way a file position is not.
//
// toolutil is loaded beside the fixture and has findings of its own, which
// belong to the sweep rather than to this test, so they are filtered out here.
func entries(findings []Finding) []string {
	rendered := make([]string, 0, len(findings))
	for _, finding := range findings {
		if !strings.HasPrefix(finding.Package, fixtureDir) {
			continue
		}
		rendered = append(rendered, fmt.Sprintf("%s %s %s", shortName(finding.Package), finding.Context, finding.Expression))
	}
	sort.Strings(rendered)
	return rendered
}

// shortName drops the fixture directory from a package path so a want list
// reads as the package name alone.
func shortName(pkg string) string {
	if idx := strings.LastIndex(pkg, "/"); idx >= 0 {
		return pkg[idx+1:]
	}
	return pkg
}

// diff reports what one list has that the other does not, in both directions.
func diff(got, want []string) (missing, extra []string) {
	inWant := map[string]int{}
	for _, line := range want {
		inWant[line]++
	}
	for _, line := range got {
		if inWant[line] > 0 {
			inWant[line]--
			continue
		}
		extra = append(extra, line)
	}
	for line, count := range inWant {
		for range count {
			missing = append(missing, line)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}

// wantFindings is every value the fixture leaves unescaped in a construct that
// value can change the shape of.
var wantFindings = []string{
	"mdcase heading item.Title",
	"mdcase link-destination item.URL",
	"mdcase link-destination item.URL",
	"mdcase link-label item.Title",
	"mdcase link-label name",
	"mdcase list-item item.Labels[0]",
	"mdcase table-cell \"opened by \" + item.Title",
	"mdcase table-cell (*(&item)).Title",
	"mdcase table-cell *item.Author",
	"mdcase table-cell []string{…}",
	"mdcase table-cell cells",
	"mdcase table-cell item.Extra.(string)",
	"mdcase table-cell item.State",
	"mdcase table-cell item.Title",
	"mdcase table-cell item.Title + \" (open)\"",
	"mdcase table-cell item.Title",
	"mdcase table-cell item.Title[:8]",
	"mdcase table-cell map[string]string{…}",
	"mdcase table-cell name",
	"mdcase table-cell pair.Right",
	"mdcase table-cell repeat(item.Title, depth)",
	"mdcase table-cell strings.Join([]string{…}, \", \")",
	"mdcase table-cell title",
	"mdexempt table-cell result.Title",
}

// wantUnresolved is every value the fixture writes in a shape the audit
// answers with neither verdict.
var wantUnresolved = []string{
	"mdcase table-cell fmt.Sprintf(dynamicTemplate(), item.Title)",
	"mdcase table-cell left",
	"mdcase table-cell mutableTitle",
	"mdcase table-cell namedResult(item)",
	"mdcase table-cell pair.Left",
	"mdcase table-cell render(item)",
	"mdcase table-cell rest[0]",
	"mdcase table-cell right",
	"mdcase table-cell title",
	"mdcase table-cell value",
}

// TestAudit_Fixture_ReportsEveryUnescapedValue is the audit's own regression
// test: the fixture holds one of each shape the classifier has to answer for,
// and this pins what it answered.
func TestAudit_Fixture_ReportsEveryUnescapedValue(t *testing.T) {
	report := auditFixture(t)

	missing, extra := diff(entries(report.Findings), wantFindings)
	for _, line := range missing {
		t.Errorf("finding not reported: %s", line)
	}
	for _, line := range extra {
		t.Errorf("unexpected finding: %s", line)
	}
}

// TestAudit_Fixture_SeparatesTheValuesItCannotFollow checks that a value the
// audit cannot follow lands in its own bucket rather than among the failures
// or among the values it called safe.
func TestAudit_Fixture_SeparatesTheValuesItCannotFollow(t *testing.T) {
	report := auditFixture(t)

	missing, extra := diff(entries(report.Unresolved), wantUnresolved)
	for _, line := range missing {
		t.Errorf("unresolved value not reported: %s", line)
	}
	for _, line := range extra {
		t.Errorf("unexpected unresolved value: %s", line)
	}
}

// TestAudit_Fixture_LeavesSafeValuesAlone checks that the package written the
// way the rule asks produces no finding at all, which is what makes the gate
// worth keeping.
func TestAudit_Fixture_LeavesSafeValuesAlone(t *testing.T) {
	report := auditFixture(t)

	for _, finding := range append(append([]Finding{}, report.Findings...), report.Unresolved...) {
		if strings.HasSuffix(finding.Package, "/mdsafe") {
			t.Errorf("reported a safe value: %s %s (%s)", finding.Context, finding.Expression, finding.Reason)
		}
	}
	if report.Summary.Safe == 0 {
		t.Error("the sweep counted no safe value at all")
	}
}

// TestAudit_Fixture_ExcusesDeclaredValuesAndReportsStaleDirectives checks both
// halves of the exemption mechanism: a declared value leaves the failures, and
// a directive that excuses nothing is itself reported.
func TestAudit_Fixture_ExcusesDeclaredValuesAndReportsStaleDirectives(t *testing.T) {
	report := auditFixture(t)

	if got := entries(report.Excused); len(got) != 1 || got[0] != "mdexempt table-cell result.ID" {
		t.Errorf("excused = %v, want the declared catalog ID alone", got)
	}
	if len(report.StaleDirectives) != 1 {
		t.Fatalf("stale directives = %d, want the one that excuses nothing", len(report.StaleDirectives))
	}
	if report.StaleDirectives[0].Expression != "result.Retired" {
		t.Errorf("stale directive names %q, want result.Retired", report.StaleDirectives[0].Expression)
	}
	if report.StaleDirectives[0].Line == 0 || !strings.HasSuffix(report.StaleDirectives[0].File, "mdexempt.go") {
		t.Errorf("stale directive at %s:%d does not point at the source that declared it",
			report.StaleDirectives[0].File, report.StaleDirectives[0].Line)
	}
}

// TestAudit_Fixture_NamesTheCallSiteThatPassesARawValue checks the rule that
// keeps a shared row helper quiet: the helper is judged by what its callers
// pass, and the reason says which call site made it fail.
func TestAudit_Fixture_NamesTheCallSiteThatPassesARawValue(t *testing.T) {
	report := auditFixture(t)

	for _, finding := range report.Findings {
		if finding.Expression != "title" {
			continue
		}
		if !strings.Contains(finding.Reason, "call site of FormatRow") {
			t.Errorf("reason %q does not name the call site the raw value came from", finding.Reason)
		}
		if finding.Func != "FormatRow" {
			t.Errorf("finding names %q, want the formatter holding the hole", finding.Func)
		}
		return
	}
	t.Error("the shared row helper was not reported at all")
}

// TestAudit_Fixture_SummarisesWhatItSaw checks the counts a report ends with,
// which are what the walk through the findings is planned from.
func TestAudit_Fixture_SummarisesWhatItSaw(t *testing.T) {
	report := auditFixture(t)
	summary := report.Summary

	if summary.Findings != len(report.Findings) || summary.Unresolved != len(report.Unresolved) ||
		summary.Excused != len(report.Excused) || summary.Stale != len(report.StaleDirectives) {
		t.Errorf("summary counts %+v disagree with the lists they count", summary)
	}
	if summary.Packages < 2 {
		t.Errorf("summary counts %d packages with findings, want at least mdcase and mdexempt", summary.Packages)
	}
	if summary.Holes <= summary.Findings || summary.Sinks == 0 {
		t.Errorf("summary judged %d value(s) in %d sink(s), which cannot hold %d findings",
			summary.Holes, summary.Sinks, summary.Findings)
	}
	if summary.ByContext["table-cell"] == 0 || summary.ByPackage[fixtureDir+"/mdcase"] == 0 {
		t.Errorf("summary breakdowns %v / %v are missing the packages that failed", summary.ByContext, summary.ByPackage)
	}
	if summary.Contexts != "table-cell, heading, list-item, link-label, link-destination" {
		t.Errorf("summary names contexts %q, want every structural one", summary.Contexts)
	}
}

// TestAudit_Fixture_NarrowedToOneContext_JudgesOnlyThat checks that staging the
// sweep by context leaves the other contexts unjudged rather than passed.
func TestAudit_Fixture_NarrowedToOneContext_JudgesOnlyThat(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	sel, err := parseContexts("heading")
	if err != nil {
		t.Fatalf("parseContexts: %v", err)
	}

	report := audit(prog, sel, repoRoot(t))

	for _, finding := range report.Findings {
		if finding.Context != "heading" {
			t.Errorf("a narrowed run reported a %s finding: %s", finding.Context, finding.Expression)
		}
	}
	if len(report.Findings) == 0 {
		t.Error("a narrowed run reported nothing at all")
	}
}

// TestAudit_Fixture_IsDeterministic checks that two runs produce the same work
// list, since a gate whose output moves cannot be reviewed in a diff.
func TestAudit_Fixture_IsDeterministic(t *testing.T) {
	first := auditFixture(t)
	second := auditFixture(t)

	if got, want := strings.Join(entries(first.Findings), "\n"), strings.Join(entries(second.Findings), "\n"); got != want {
		t.Errorf("two runs disagree:\n%s\n---\n%s", got, want)
	}
}

// TestFindingLess_Cases_OrdersByPackageFileLineExpression checks the order two
// runs have to agree on, field by field.
func TestFindingLess_Cases_OrdersByPackageFileLineExpression(t *testing.T) {
	base := Finding{Package: "b", File: "b.go", Line: 2, Expression: "b"}
	cases := []struct {
		name  string
		other Finding
		want  bool
	}{
		{name: "earlier package", other: Finding{Package: "a", File: "z.go", Line: 9, Expression: "z"}, want: true},
		{name: "later package", other: Finding{Package: "c"}, want: false},
		{name: "earlier file", other: Finding{Package: "b", File: "a.go", Line: 9, Expression: "z"}, want: true},
		{name: "later file", other: Finding{Package: "b", File: "c.go"}, want: false},
		{name: "earlier line", other: Finding{Package: "b", File: "b.go", Line: 1, Expression: "z"}, want: true},
		{name: "later line", other: Finding{Package: "b", File: "b.go", Line: 3}, want: false},
		{name: "earlier expression", other: Finding{Package: "b", File: "b.go", Line: 2, Expression: "a"}, want: true},
		{name: "later expression", other: Finding{Package: "b", File: "b.go", Line: 2, Expression: "c"}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := findingLess(tc.other, base); got != tc.want {
				t.Errorf("findingLess(%+v, %+v) = %v, want %v", tc.other, base, got, tc.want)
			}
		})
	}
}

// TestSortDirectives_TwoDirectives_OrdersByFileThenLine checks that stale
// exemptions are listed in source order.
func TestSortDirectives_TwoDirectives_OrdersByFileThenLine(t *testing.T) {
	directives := []Directive{
		{File: "b.go", Line: 1},
		{File: "a.go", Line: 9},
		{File: "a.go", Line: 2},
	}

	sortDirectives(directives)

	var order []string
	for _, directive := range directives {
		order = append(order, fmt.Sprintf("%s:%d", directive.File, directive.Line))
	}
	if got, want := strings.Join(order, " "), "a.go:2 a.go:9 b.go:1"; got != want {
		t.Errorf("directives ordered %s, want %s", got, want)
	}
}
