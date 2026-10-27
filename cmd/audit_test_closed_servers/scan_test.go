// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// auditPlanted audits the named packages of the fixture module under the
// given tags, held to the given declarations, and returns the report.
func auditPlanted(t *testing.T, tags []string, declared map[string]sourcegate.Declaration, patterns ...string) Report {
	t.Helper()
	report, err := audit(auditConfig{dir: plantedDir(t), patterns: patterns, tags: tags, declared: declared})
	if err != nil {
		t.Fatalf("audit the planted module: %v", err)
	}
	return report
}

// spell renders a finding the way the expectations below are written: where
// the close is, in which function, what was closed, and through what name and
// on which line its address was used afterwards.
func spell(findings []Finding) []string {
	spelled := make([]string, 0, len(findings))
	for _, f := range findings {
		spelled = append(spelled, fmt.Sprintf("%s:%d %s %s %s:%d", f.File, f.Line, f.Func, f.Server, f.Use, f.UseLine))
	}
	return spelled
}

// plantedFindings is every finding the planted module holds under every tag,
// in the report's order, each written where its fixture says it is.
var plantedFindings = []string{
	"declared/declared_test.go:18 TestPlanted_ExcusedByADeclaration_Excused srv url:19",
	"flagged/external_test.go:21 TestPlanted_ExternalTestPackage_Reported srv url:22",
	"flagged/flagged_test.go:21 TestPlanted_URLReadAfterTheClose_Reported srv srv:22",
	"flagged/flagged_test.go:30 TestPlanted_SavedURL_Reported srv url:31",
	"flagged/flagged_test.go:39 TestPlanted_ServerFromAHelper_Reported srv addr:40",
	"flagged/flagged_test.go:52 TestPlanted_AddressBuiltInSteps_Reported srv endpoint:53",
	"flagged/flagged_test.go:61 TestPlanted_SavedInAPairedAssignment_Reported srv url:62",
	"flagged/flagged_test.go:75 TestPlanted_ClientBuiltOnTheLiveURL_Reported srv client:80",
	"flagged/flagged_test.go:90 TestPlanted_OneNameReboundAnotherUsed_Reported srv second:93",
	"flagged/flagged_test.go:101 TestPlanted_FunctionBoundToTheURL_Reported srv ping:102",
	"flagged/flagged_test.go:112 TestPlanted_ServerInAField_Reported e.server e:113",
	"flagged/flagged_test.go:123 TestPlanted_EmbeddedServer_Reported f f:124",
	"flagged/flagged_test.go:131 TestPlanted_ClosedInASubtest_Reported srv srv:132",
	"flagged/flagged_test.go:154 TestPlanted_ClosedOnABranchThatGoesOn_Reported logged logged:177",
	"flagged/flagged_test.go:158 TestPlanted_ClosedOnABranchThatGoesOn_Reported builtin builtin:178",
	"flagged/flagged_test.go:162 TestPlanted_ClosedOnABranchThatGoesOn_Reported received received:179",
	"flagged/flagged_test.go:166 TestPlanted_ClosedOnABranchThatGoesOn_Reported assigned assigned:180",
	"flagged/flagged_test.go:170 TestPlanted_ClosedOnABranchThatGoesOn_Reported described described:181",
	"flagged/flagged_test.go:174 TestPlanted_ClosedOnABranchThatGoesOn_Reported noted noted:182",
	"flagged/flagged_test.go:190 TestPlanted_AddressFromAnInvokedLiteral_Reported srv addr:191",
	"flagged/flagged_test.go:202 TestPlanted_DeferredInASubtest_Reported deferred deferred:207",
	"flagged/flagged_test.go:203 TestPlanted_DeferredInASubtest_Reported cleaned cleaned:208",
	"flagged/flagged_test.go:226 TestPlanted_CleanupThatIsNotATests_Reported byMethod byMethod:229",
	"flagged/flagged_test.go:228 TestPlanted_CleanupThatIsNotATests_Reported byField byField:230",
	"flagged/flagged_test.go:240 TestPlanted_ClosedBeforeAFunctionValue_Reported srv srv:243",
	"flagged/flagged_test.go:252 TestPlanted_ReassignedFromItself_Reported srv addr:253",
	"flagged/flagged_test.go:262 TestPlanted_CompoundAssignment_Reported srv base:263",
	"flagged/flagged_test.go:277 TestPlanted_ClosedOnACaseThatGoesOn_Reported switched switched:286",
	"flagged/flagged_test.go:282 TestPlanted_ClosedOnACaseThatGoesOn_Reported selected selected:287",
	"flagged/flagged_test.go:303 TestPlanted_ClosedThroughACopy_Reported other url:304",
	"flagged/flagged_test.go:316 TestPlanted_ClosedThroughAHolder_Reported h.server url:317",
	"flagged/flagged_test.go:325 TestPlanted_ServerAndAddressFromAHelper_Reported f.srv url:326",
	"flagged/flagged_test.go:345 TestPlanted_AddressStoredInAField_Reported byValue cfg:347",
	"flagged/flagged_test.go:346 TestPlanted_AddressStoredInAField_Reported byPointer ptr:348",
	"flagged/flagged_test.go:356 TestPlanted_RangeVariable_Reported srv u:357",
	"flagged/flagged_test.go:372 TestPlanted_AddressAsBytesAndInARequest_Reported asBytes raw:374",
	"flagged/flagged_test.go:373 TestPlanted_AddressAsBytesAndInARequest_Reported asRequest req:375",
	"flagged/flagged_test.go:386 TestPlanted_CleanupInASubtest_Reported named named:391",
	"flagged/flagged_test.go:389 TestPlanted_CleanupInASubtest_Reported renamed renamed:392",
	"flagged/flagged_test.go:404 TestPlanted_ParallelInsideASubtest_Reported srv srv:407",
	"flagged/flagged_test.go:416 TestPlanted_ServerWrappedInAnEmbedding_Reported srv wrapped:417",
	"flagged/flagged_test.go:426 TestPlanted_LiteralDefersItsClose_Reported srv closedAddress:429",
	"flagged/flagged_test.go:438 TestPlanted_DeferredCloseBeforeAParallelSubtest_Reported srv srv:441",
	"flagged/flagged_test.go:456 TestPlanted_ClosedAfterAParallelSubtest_Reported srv saved:454",
	"flagged/flagged_test.go:465 TestPlanted_SubtestDefersBeforeItsParallelSubtest_Reported srv srv:468",
	"flagged/flagged_test.go:484 TestPlanted_ClosedInASubtestAfterAParallelOne_Reported srv srv:481",
	"flagged/flagged_test.go:499 TestPlanted_ClosedOnAStoppingBranchAfterAParallelSubtest_Reported srv srv:496",
	"flagged/flagged_test.go:516 TestPlanted_ReboundOnABranch_Reported srv url:520",
	"flagged/flagged_test.go:531 TestPlanted_ReboundOnTheOtherBranch_Reported srv url:535",
	"flagged/flagged_test.go:545 TestPlanted_ReboundByARangeThatMayNotRun_Reported srv url:550",
	"flagged/flagged_test.go:562 TestPlanted_ClosedThroughAMethodValue_Reported called url:567",
	"flagged/flagged_test.go:565 TestPlanted_ClosedThroughAMethodValue_Reported registered registered:568",
	"flagged/helpers.go:35 closedURL srv url:36",
	"flagged/helpers.go:43 closedLater srv srv:44",
	"flagged/helpers.go:64 deadURL srv srv:65",
	"flagged/helpers.go:71 deadURLLater srv srv:72",
	"flagged/helpers.go:79 deadClient srv client:81",
	"flagged/helpers.go:88 deadDial srv srv:89",
	"flagged/helpers.go:95 deadNamedURL srv url:97",
	"tagged/collectore2e_test.go:18 TestPlanted_BehindThe_collectore2e_Tag_Reported srv url:19",
	"tagged/e2e_test.go:18 TestPlanted_BehindThe_e2e_Tag_Reported srv url:19",
	"tagged/httpe2e_test.go:18 TestPlanted_BehindThe_httpe2e_Tag_Reported srv url:19",
	"tagged/orbitlive_test.go:18 TestPlanted_BehindThe_orbitlive_Tag_Reported srv url:19",
	"tagged/stdioe2e_test.go:18 TestPlanted_BehindThe_stdioe2e_Tag_Reported srv url:19",
}

// TestAudit_PlantedModule_ReportsEveryPlantedShapeAndNothingElse is the rule
// held whole: every shape under flagged, declared and tagged is reported where
// its fixture says, nothing under clean is, the file a constraint hides is
// named as unjudged, and the figures say what the run was over.
func TestAudit_PlantedModule_ReportsEveryPlantedShapeAndNothingElse(t *testing.T) {
	report := auditPlanted(t, analysisTags, map[string]sourcegate.Declaration{}, "./...")
	if got := spell(report.Findings); !slices.Equal(got, plantedFindings) {
		t.Errorf("findings =\n%q\nwant\n%q", got, plantedFindings)
	}
	if want := []string{"hidden/hidden_test.go"}; !slices.Equal(report.Unjudged, want) {
		t.Errorf("unjudged = %v, want %v", report.Unjudged, want)
	}
	want := Summary{Packages: 6, Closes: 110, Findings: len(plantedFindings), Unjudged: 1}
	if report.Summary != want {
		t.Errorf("summary = %+v, want %+v", report.Summary, want)
	}
	if !slices.ContainsFunc(report.Findings, func(f Finding) bool { return f.Package == "example.com/planted/flagged_test" }) {
		t.Error("no finding names the external test package, so its variant was not read as a package of its own")
	}
}

// TestAudit_EachTag_RevealsItsOwnFile verifies every tag the gate sets is the
// one its suite's files are behind: loaded under one tag alone, the tagged
// package shows that tag's finding and no other, and under none it shows
// nothing.
func TestAudit_EachTag_RevealsItsOwnFile(t *testing.T) {
	t.Run("no tag", func(t *testing.T) {
		report := auditPlanted(t, nil, map[string]sourcegate.Declaration{}, "./tagged/...")
		if len(report.Findings) != 0 || report.Summary.Closes != 0 {
			t.Errorf("an untagged load found %v in %d closes, want nothing", spell(report.Findings), report.Summary.Closes)
		}
	})
	for _, tag := range analysisTags {
		t.Run(tag, func(t *testing.T) {
			report := auditPlanted(t, []string{tag}, map[string]sourcegate.Declaration{}, "./tagged/...")
			want := fmt.Sprintf("tagged/%s_test.go:18 TestPlanted_BehindThe_%s_Tag_Reported srv url:19", tag, tag)
			if got := spell(report.Findings); !slices.Equal(got, []string{want}) {
				t.Errorf("findings = %q, want only %q", got, want)
			}
		})
	}
}

// TestAudit_Declarations_ExcuseStaleAndUnknownAreEachReported verifies the
// table is held as every declaration table here is: an entry that names a
// finding excuses it, one that names nothing in a loaded package is stale,
// one for a package the run did not load is out of view, and a category
// nobody defined is reported whatever it excuses.
func TestAudit_Declarations_ExcuseStaleAndUnknownAreEachReported(t *testing.T) {
	declared := map[string]sourcegate.Declaration{
		"example.com/planted/declared:TestPlanted_ExcusedByADeclaration_Excused": {Category: categoryOnlyAsText, Reason: "compared as text"},
		"example.com/planted/declared:TestPlanted_Gone":                          {Category: categoryOnlyAsText, Reason: "no longer there"},
		"example.com/planted/elsewhere:TestPlanted_OutOfView":                    {Category: categoryOnlyAsText, Reason: "not loaded"},
		"example.com/planted/declared:TestPlanted_Invented":                      {Category: "invented", Reason: "nobody defined it"},
	}
	report := auditPlanted(t, analysisTags, declared, "./declared/...")
	if len(report.Findings) != 0 {
		t.Errorf("findings = %v, want the one there excused", spell(report.Findings))
	}
	if got, want := spell(report.Excused), []string{plantedFindings[0]}; !slices.Equal(got, want) {
		t.Errorf("excused = %q, want %q", got, want)
	}
	if want := []string{"example.com/planted/declared:TestPlanted_Gone", "example.com/planted/declared:TestPlanted_Invented"}; !slices.Equal(report.Stale, want) {
		t.Errorf("stale = %v, want %v", report.Stale, want)
	}
	if want := []string{"example.com/planted/declared:TestPlanted_Invented"}; !slices.Equal(report.Unknown, want) {
		t.Errorf("unknown = %v, want %v", report.Unknown, want)
	}
	if report.OK() {
		t.Error("a report with stale and unknown declarations is ok, want it to fail the gate")
	}
}

// TestNoteUnjudged_ReadsTheImportsOfTheGoFilesLeftOut verifies which of the
// files a load left out are named: a Go file that imports net/http/httptest,
// under any name, and one that cannot be read, but neither a file that is not
// Go nor a Go file without the import. The files are served from the overlay
// the way a test's own source is.
func TestNoteUnjudged_ReadsTheImportsOfTheGoFilesLeftOut(t *testing.T) {
	root := t.TempDir()
	path := func(name string) string { return filepath.Join(root, name) }
	overlay := map[string][]byte{
		path("quiet_test.go"):   []byte("package p\n\nimport \"testing\"\n"),
		path("aliased_test.go"): []byte("package p\n\nimport served \"net/http/httptest\"\n\nvar _ = served.NewServer\n"),
		path("broken_test.go"):  []byte("package p\n\nimport (\n"),
	}
	found := newScanner(root, overlay)
	found.noteUnjudged(&packages.Package{IgnoredFiles: []string{
		path("notes.txt"), path("quiet_test.go"), path("aliased_test.go"), path("broken_test.go"),
	}})
	var got []string
	for file := range found.unjudged {
		got = append(got, file)
	}
	slices.Sort(got)
	if want := []string{"aliased_test.go", "broken_test.go"}; !slices.Equal(got, want) {
		t.Errorf("unjudged = %v, want %v", got, want)
	}
}
