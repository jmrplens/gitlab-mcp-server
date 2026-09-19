package main

import (
	"strings"
	"testing"
)

// scannedSet is the set of packages a report was built over, in the shape
// [buildReport] takes it.
func scannedSet(packages ...string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, pkg := range packages {
		set[pkg] = struct{}{}
	}
	return set
}

// TestBuildReport_DeclaredUnread_IsExcusedRatherThanReported checks the escape
// hatch: a constant the table names is kept out of the findings and keeps its
// declaration from reading as stale.
func TestBuildReport_DeclaredUnread_IsExcusedRatherThanReported(t *testing.T) {
	found := []Constant{
		{Package: "internal/tools/dynamic", Name: "aliasSourceCatalog", File: "a.go", Line: 1, GroupSize: 5},
		{Package: "internal/tools/dynamic", Name: "aliasSourceStandalone", File: "a.go", Line: 2, GroupSize: 5},
	}
	report := buildReport(found, 10, scannedSet("internal/tools/dynamic"))
	if len(report.Findings) != 0 {
		t.Fatalf("findings = %v, want none: both constants are declared unread on purpose", report.Findings)
	}
	if len(report.Stale) != 0 {
		t.Fatalf("stale = %v, want none: both declarations excused a constant", report.Stale)
	}
	if !report.ok() {
		t.Fatal("ok() = false, want true")
	}
}

// TestBuildReport_DeclarationThatExcusedNothing_IsItsOwnFinding holds the
// table to the terms every declaration table here is held to.
func TestBuildReport_DeclarationThatExcusedNothing_IsItsOwnFinding(t *testing.T) {
	report := buildReport(nil, 10, scannedSet("internal/tools/dynamic"))
	if len(report.Stale) != len(unreadOnPurpose) {
		t.Fatalf("stale = %v, want every entry of the table: the run looked at the package and found the constants read", report.Stale)
	}
	if report.ok() {
		t.Fatal("ok() = true, want false: a stale declaration is a finding")
	}
	if report.Summary.Stale != len(report.Stale) {
		t.Fatalf("Summary.Stale = %d, want %d", report.Summary.Stale, len(report.Stale))
	}
}

// TestBuildReport_DeclarationOutsideTheRun_IsNeitherUsedNorStale is what lets
// the command be pointed at one package. Without the scope, every narrowed run
// would condemn the whole table.
func TestBuildReport_DeclarationOutsideTheRun_IsNeitherUsedNorStale(t *testing.T) {
	report := buildReport(nil, 3, scannedSet("internal/tools/issues"))
	if len(report.Stale) != 0 {
		t.Fatalf("stale = %v, want none: the run never loaded the packages the table names", report.Stale)
	}
	if !report.ok() {
		t.Fatal("ok() = false, want true")
	}
}

// TestBuildReport_UnreadConstant_IsAFindingAndCountedByItsGroup checks both
// halves a reader acts on: the finding itself, and whether the linter had
// already had its chance at it.
func TestBuildReport_UnreadConstant_IsAFindingAndCountedByItsGroup(t *testing.T) {
	found := []Constant{
		{Package: "internal/tools/issues", Name: "alone", File: "a.go", Line: 3, GroupSize: 1},
		{Package: "internal/tools/issues", Name: "hidden", File: "a.go", Line: 9, GroupSize: 4},
	}
	report := buildReport(found, 20, scannedSet("internal/tools/issues"))
	if report.Summary.Findings != 2 {
		t.Fatalf("Summary.Findings = %d, want 2", report.Summary.Findings)
	}
	if report.Summary.InGroup != 1 {
		t.Fatalf("Summary.InGroup = %d, want 1: only the second shares a declaration", report.Summary.InGroup)
	}
	if report.Summary.Declared != 20 || report.Summary.Packages != 1 {
		t.Fatalf("Summary = %+v, want 20 declared in 1 package", report.Summary)
	}
}

// TestReportWrite_Findings_NameTheFileLineAndWhetherTheLinterCouldSeeIt checks
// what a reader is handed.
func TestReportWrite_Findings_NameTheFileLineAndWhetherTheLinterCouldSeeIt(t *testing.T) {
	found := []Constant{
		{Package: "internal/tools/issues", Name: "alone", File: "internal/tools/issues/a.go", Line: 3, GroupSize: 1},
		{Package: "internal/tools/issues", Name: "hidden", File: "internal/tools/issues/a.go", Line: 9, GroupSize: 4},
	}
	report := buildReport(found, 20, scannedSet("internal/tools/issues"))
	var out strings.Builder
	report.write(&out, false)
	text := out.String()
	for _, want := range []string{
		"internal/tools/issues/a.go:3: alone is never read (declared on its own)",
		"internal/tools/issues/a.go:9: hidden is never read (1 of 4 in its const declaration)",
		"2 never read (1 of those in a group the linter cannot see)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("report does not contain %q:\n%s", want, text)
		}
	}
}

// TestReportWrite_StaleDeclaration_SaysWhatWentWrongWithIt keeps the second
// kind of finding legible: the reader has to know whether to delete the entry
// or the constant.
func TestReportWrite_StaleDeclaration_SaysWhatWentWrongWithIt(t *testing.T) {
	report := buildReport(nil, 1, scannedSet("internal/tools/dynamic"))
	var out strings.Builder
	report.write(&out, false)
	text := out.String()
	if !strings.Contains(text, "declared unread on purpose, and this run found it read or found it gone") {
		t.Fatalf("report does not explain a stale declaration:\n%s", text)
	}
	for key := range unreadOnPurpose {
		if !strings.Contains(text, key) {
			t.Fatalf("report does not name the stale declaration %q:\n%s", key, text)
		}
	}
}

// TestReportWrite_CleanRun_SaysWhatItWasCleanOver: a gate that says only "ok"
// cannot be told from one that looked at nothing.
func TestReportWrite_CleanRun_SaysWhatItWasCleanOver(t *testing.T) {
	report := buildReport(nil, 5545, scannedSet("internal/tools/issues"))
	var out strings.Builder
	report.write(&out, false)
	text := out.String()
	if !strings.Contains(text, "5545 unexported constants in 1 packages, 0 never read") {
		t.Fatalf("clean report does not say what it covered:\n%s", text)
	}
}

// TestSortConstants_FileThenLine_IsTheOrderFindingsAreRead keeps two runs over
// one tree printing the same report.
func TestSortConstants_FileThenLine_IsTheOrderFindingsAreRead(t *testing.T) {
	found := []Constant{
		{File: "b.go", Line: 1, Name: "second"},
		{File: "a.go", Line: 9, Name: "later"},
		{File: "a.go", Line: 2, Name: "first"},
	}
	sortConstants(found)
	want := []string{"first", "later", "second"}
	for index, name := range want {
		if found[index].Name != name {
			t.Fatalf("order = %v, want %v", deadNames(found), want)
		}
	}
}

// TestGroupNote_Sizes_TellTheTwoClassesApart.
func TestGroupNote_Sizes_TellTheTwoClassesApart(t *testing.T) {
	cases := []struct {
		name string
		size int
		want string
	}{
		{"on its own", 1, "declared on its own"},
		{"no group recorded", 0, "declared on its own"},
		{"inside a group", 6, "1 of 6 in its const declaration"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := groupNote(testCase.size); got != testCase.want {
				t.Fatalf("groupNote(%d) = %q, want %q", testCase.size, got, testCase.want)
			}
		})
	}
}
