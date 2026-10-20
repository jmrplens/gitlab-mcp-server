package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/tools/go/packages"
)

// auditPlanted audits the named packages of the fixture module under the
// given tags, held to the given declarations, and returns the report.
func auditPlanted(t *testing.T, tags []string, declared map[string]declaration, patterns ...string) Report {
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
	"declared/declared_test.go:15 TestPlanted_ExcusedByADeclaration_Excused srv url:16",
	"flagged/external_test.go:18 TestPlanted_ExternalTestPackage_Reported srv url:19",
	"flagged/flagged_test.go:18 TestPlanted_URLReadAfterTheClose_Reported srv srv:19",
	"flagged/flagged_test.go:27 TestPlanted_SavedURL_Reported srv url:28",
	"flagged/flagged_test.go:36 TestPlanted_ServerFromAHelper_Reported srv addr:37",
	"flagged/flagged_test.go:49 TestPlanted_AddressBuiltInSteps_Reported srv endpoint:50",
	"flagged/flagged_test.go:58 TestPlanted_SavedInAPairedAssignment_Reported srv url:59",
	"flagged/flagged_test.go:72 TestPlanted_ClientBuiltOnTheLiveURL_Reported srv client:77",
	"flagged/flagged_test.go:87 TestPlanted_OneNameReboundAnotherUsed_Reported srv second:90",
	"flagged/flagged_test.go:98 TestPlanted_FunctionBoundToTheURL_Reported srv ping:99",
	"flagged/flagged_test.go:109 TestPlanted_ServerInAField_Reported e.server e:110",
	"flagged/flagged_test.go:120 TestPlanted_EmbeddedServer_Reported f f:121",
	"flagged/flagged_test.go:128 TestPlanted_ClosedInASubtest_Reported srv srv:129",
	"flagged/flagged_test.go:151 TestPlanted_ClosedOnABranchThatGoesOn_Reported logged logged:174",
	"flagged/flagged_test.go:155 TestPlanted_ClosedOnABranchThatGoesOn_Reported builtin builtin:175",
	"flagged/flagged_test.go:159 TestPlanted_ClosedOnABranchThatGoesOn_Reported received received:176",
	"flagged/flagged_test.go:163 TestPlanted_ClosedOnABranchThatGoesOn_Reported assigned assigned:177",
	"flagged/flagged_test.go:167 TestPlanted_ClosedOnABranchThatGoesOn_Reported described described:178",
	"flagged/flagged_test.go:171 TestPlanted_ClosedOnABranchThatGoesOn_Reported noted noted:179",
	"flagged/flagged_test.go:187 TestPlanted_AddressFromAnInvokedLiteral_Reported srv addr:188",
	"flagged/flagged_test.go:199 TestPlanted_DeferredInASubtest_Reported deferred deferred:204",
	"flagged/flagged_test.go:200 TestPlanted_DeferredInASubtest_Reported cleaned cleaned:205",
	"flagged/flagged_test.go:223 TestPlanted_CleanupThatIsNotATests_Reported byMethod byMethod:226",
	"flagged/flagged_test.go:225 TestPlanted_CleanupThatIsNotATests_Reported byField byField:227",
	"flagged/flagged_test.go:237 TestPlanted_ClosedBeforeAFunctionValue_Reported srv srv:240",
	"flagged/flagged_test.go:249 TestPlanted_ReassignedFromItself_Reported srv addr:250",
	"flagged/flagged_test.go:259 TestPlanted_CompoundAssignment_Reported srv base:260",
	"flagged/flagged_test.go:274 TestPlanted_ClosedOnACaseThatGoesOn_Reported switched switched:283",
	"flagged/flagged_test.go:279 TestPlanted_ClosedOnACaseThatGoesOn_Reported selected selected:284",
	"flagged/flagged_test.go:300 TestPlanted_ClosedThroughACopy_Reported other url:301",
	"flagged/flagged_test.go:313 TestPlanted_ClosedThroughAHolder_Reported h.server url:314",
	"flagged/flagged_test.go:322 TestPlanted_ServerAndAddressFromAHelper_Reported f.srv url:323",
	"flagged/flagged_test.go:342 TestPlanted_AddressStoredInAField_Reported byValue cfg:344",
	"flagged/flagged_test.go:343 TestPlanted_AddressStoredInAField_Reported byPointer ptr:345",
	"flagged/flagged_test.go:353 TestPlanted_RangeVariable_Reported srv u:354",
	"flagged/flagged_test.go:369 TestPlanted_AddressAsBytesAndInARequest_Reported asBytes raw:371",
	"flagged/flagged_test.go:370 TestPlanted_AddressAsBytesAndInARequest_Reported asRequest req:372",
	"flagged/flagged_test.go:383 TestPlanted_CleanupInASubtest_Reported named named:388",
	"flagged/flagged_test.go:386 TestPlanted_CleanupInASubtest_Reported renamed renamed:389",
	"flagged/flagged_test.go:401 TestPlanted_ParallelInsideASubtest_Reported srv srv:404",
	"flagged/flagged_test.go:413 TestPlanted_ServerWrappedInAnEmbedding_Reported srv wrapped:414",
	"flagged/flagged_test.go:423 TestPlanted_LiteralDefersItsClose_Reported srv closedAddress:426",
	"flagged/flagged_test.go:435 TestPlanted_DeferredCloseBeforeAParallelSubtest_Reported srv srv:438",
	"flagged/flagged_test.go:453 TestPlanted_ClosedAfterAParallelSubtest_Reported srv saved:451",
	"flagged/flagged_test.go:462 TestPlanted_SubtestDefersBeforeItsParallelSubtest_Reported srv srv:465",
	"flagged/flagged_test.go:481 TestPlanted_ClosedInASubtestAfterAParallelOne_Reported srv srv:478",
	"flagged/flagged_test.go:496 TestPlanted_ClosedOnAStoppingBranchAfterAParallelSubtest_Reported srv srv:493",
	"flagged/flagged_test.go:513 TestPlanted_ReboundOnABranch_Reported srv url:517",
	"flagged/flagged_test.go:528 TestPlanted_ReboundOnTheOtherBranch_Reported srv url:532",
	"flagged/flagged_test.go:542 TestPlanted_ReboundByARangeThatMayNotRun_Reported srv url:547",
	"flagged/flagged_test.go:559 TestPlanted_ClosedThroughAMethodValue_Reported called url:564",
	"flagged/flagged_test.go:562 TestPlanted_ClosedThroughAMethodValue_Reported registered registered:565",
	"flagged/helpers.go:32 closedURL srv url:33",
	"flagged/helpers.go:40 closedLater srv srv:41",
	"flagged/helpers.go:61 deadURL srv srv:62",
	"flagged/helpers.go:68 deadURLLater srv srv:69",
	"flagged/helpers.go:76 deadClient srv client:78",
	"flagged/helpers.go:85 deadDial srv srv:86",
	"flagged/helpers.go:92 deadNamedURL srv url:94",
	"tagged/collectore2e_test.go:15 TestPlanted_BehindThe_collectore2e_Tag_Reported srv url:16",
	"tagged/e2e_test.go:15 TestPlanted_BehindThe_e2e_Tag_Reported srv url:16",
	"tagged/httpe2e_test.go:15 TestPlanted_BehindThe_httpe2e_Tag_Reported srv url:16",
	"tagged/orbitlive_test.go:15 TestPlanted_BehindThe_orbitlive_Tag_Reported srv url:16",
	"tagged/stdioe2e_test.go:15 TestPlanted_BehindThe_stdioe2e_Tag_Reported srv url:16",
}

// TestAudit_PlantedModule_ReportsEveryPlantedShapeAndNothingElse is the rule
// held whole: every shape under flagged, declared and tagged is reported where
// its fixture says, nothing under clean is, the file a constraint hides is
// named as unjudged, and the figures say what the run was over.
func TestAudit_PlantedModule_ReportsEveryPlantedShapeAndNothingElse(t *testing.T) {
	report := auditPlanted(t, analysisTags, map[string]declaration{}, "./...")
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
		report := auditPlanted(t, nil, map[string]declaration{}, "./tagged/...")
		if len(report.Findings) != 0 || report.Summary.Closes != 0 {
			t.Errorf("an untagged load found %v in %d closes, want nothing", spell(report.Findings), report.Summary.Closes)
		}
	})
	for _, tag := range analysisTags {
		t.Run(tag, func(t *testing.T) {
			report := auditPlanted(t, []string{tag}, map[string]declaration{}, "./tagged/...")
			want := fmt.Sprintf("tagged/%s_test.go:15 TestPlanted_BehindThe_%s_Tag_Reported srv url:16", tag, tag)
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
	declared := map[string]declaration{
		"example.com/planted/declared:TestPlanted_ExcusedByADeclaration_Excused": {category: categoryOnlyAsText, reason: "compared as text"},
		"example.com/planted/declared:TestPlanted_Gone":                          {category: categoryOnlyAsText, reason: "no longer there"},
		"example.com/planted/elsewhere:TestPlanted_OutOfView":                    {category: categoryOnlyAsText, reason: "not loaded"},
		"example.com/planted/declared:TestPlanted_Invented":                      {category: "invented", reason: "nobody defined it"},
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
	if report.ok() {
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

// TestIsTestMain_BelievesTheSuffixOnlyBesideAVariant verifies the synthesized
// test executable is told apart from a package whose directory is named like
// one.
func TestIsTestMain_BelievesTheSuffixOnlyBesideAVariant(t *testing.T) {
	underTest := packagesUnderTest([]*packages.Package{
		{PkgPath: "example.com/p [example.com/p.test]", ForTest: "example.com/p"},
		{PkgPath: "example.com/p"},
	})
	for _, tt := range []struct {
		name string
		pkg  *packages.Package
		want bool
	}{
		{name: "the executable built for a package with tests", pkg: &packages.Package{Name: "main", PkgPath: "example.com/p.test"}, want: true},
		{name: "a main package in a directory named like one", pkg: &packages.Package{Name: "main", PkgPath: "example.com/q.test"}},
		{name: "a library in a directory named like one", pkg: &packages.Package{Name: "p", PkgPath: "example.com/p.test"}},
		{name: "a main package", pkg: &packages.Package{Name: "main", PkgPath: "example.com/cmd"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTestMain(tt.pkg, underTest); got != tt.want {
				t.Errorf("isTestMain(%s) = %v, want %v", tt.pkg.PkgPath, got, tt.want)
			}
		})
	}
}

// TestNaming_SpellsPackagesAndFilesAsTheRepositoryDoes covers the names a
// finding carries: a variant's decoration taken off, the module path trimmed,
// and a file below the root made relative, one outside it or one that cannot
// be made relative left whole.
func TestNaming_SpellsPackagesAndFilesAsTheRepositoryDoes(t *testing.T) {
	for _, tt := range []struct{ name, got, want string }{
		{"a test variant", variantName("example.com/p [example.com/p.test]"), "example.com/p"},
		{"a plain package", variantName("example.com/p"), "example.com/p"},
		{"a package of this module", trimModulePath("github.com/jmrplens/gitlab-mcp-server/v3/internal/oauth"), "internal/oauth"},
		{"a package of another module", trimModulePath("example.com/p"), "example.com/p"},
		{"a file below the root", relativePath(filepath.Join("/repo", "a", "b.go"), "/repo"), "a/b.go"},
		{"a file outside the root", relativePath(filepath.Join("/elsewhere", "b.go"), "/repo"), "/elsewhere/b.go"},
		{"a relative file against an absolute root", relativePath(filepath.Join("a", "b.go"), "/repo"), "a/b.go"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

// TestFuncDeclName_SpellsTheReceiverAsTheFileDoes covers every receiver form
// a declaration key has to name: none, a value, a pointer, a parenthesized
// one, a generic type with one parameter and with two, and a receiver that is
// not a type name, which keeps the method's name alone.
func TestFuncDeclName_SpellsTheReceiverAsTheFileDoes(t *testing.T) {
	const source = `package p

func Plain() {}
func (T) Value() {}
func (t *T) Pointer() {}
func ((T)) Parenthesized() {}
func (g G[A]) OneParameter() {}
func (g *G[A, B]) TwoParameters() {}
func (x pkg.T) Qualified() {}
`
	file, err := parser.ParseFile(token.NewFileSet(), "p.go", source, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var got []string
	for _, decl := range file.Decls {
		got = append(got, funcDeclName(decl.(*ast.FuncDecl)))
	}
	want := []string{"Plain", "T.Value", "T.Pointer", "T.Parenthesized", "G.OneParameter", "G.TwoParameters", "Qualified"}
	if !slices.Equal(got, want) {
		t.Errorf("names = %v, want %v", got, want)
	}
}
