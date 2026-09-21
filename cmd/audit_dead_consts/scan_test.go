package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// fixtureDir is the directory the in-memory fixture package pretends to live
// in. Nothing is written there: the package exists only in the loader overlay,
// so the walk is exercised on real type-checked source without generated Go
// files landing in the repository.
const fixtureDir = "cmd/audit_dead_consts/fixture"

// repoRoot walks up from the test's working directory to the module root, so
// the overlay can name absolute paths inside the module and the loader
// resolves the module's own import paths.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// scanFixture loads the named fixture files as one package and returns the
// constants the walk found unread, by name.
//
// Every file is handed to the loader through the overlay, so a test can put a
// _test.go beside the source and see what the test variant changes.
func scanFixture(t *testing.T, files map[string]string) []Constant {
	t.Helper()
	return scanFixtureAs(t, files, nil)
}

// scanFixtureAs is [scanFixture] with the targets a package holding a file
// this one excludes is re-read under.
func scanFixtureAs(t *testing.T, files map[string]string, targets []target) []Constant {
	t.Helper()
	report, _ := auditFixture(t, files, targets)
	return report.Findings
}

// auditFixture runs the whole audit over the named fixture files, verbosely,
// and returns the report with what the progress stream said, so a test can
// hold the summary and the platform loads the run made as well as the
// findings.
func auditFixture(t *testing.T, files map[string]string, targets []target) (Report, string) {
	t.Helper()
	root := repoRoot(t)
	overlay := map[string][]byte{}
	for name, source := range files {
		overlay[filepath.Join(root, filepath.FromSlash(fixtureDir), name)] = []byte(source)
	}
	var progress strings.Builder
	report, err := audit(auditConfig{
		dir:      root,
		patterns: []string{"./" + fixtureDir},
		overlay:  overlay,
		targets:  targets,
		verbose:  true,
		out:      &progress,
	})
	if err != nil {
		t.Fatalf("audit fixture: %v", err)
	}
	if len(report.Stale) != 0 {
		t.Fatalf("fixture run reported stale declarations %v, which means the real table excused something here", report.Stale)
	}
	return report, progress.String()
}

// deadNames is the names of the unread constants, sorted so a comparison does
// not depend on the order files were walked in.
func deadNames(found []Constant) []string {
	names := make([]string, 0, len(found))
	for _, constant := range found {
		names = append(names, constant.Name)
	}
	sort.Strings(names)
	return names
}

// assertDead compares the unread constants with the names a case expects.
func assertDead(t *testing.T, found []Constant, want ...string) {
	t.Helper()
	got := deadNames(found)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Fatalf("unread constants = %v, want %v", got, want)
	}
}

// TestScan_MemberOfAGroupWhoseFirstMemberIsRead_IsReported is the whole reason
// this command exists.
//
// staticcheck's unused judges a const group as one unit, so a declaration
// whose first member is read shields every member after it. This case is that
// exact shape: two constants the package returns and one nothing names, in a
// group of three so that the finding's line, its column and its group size
// are three different numbers and the whole record can be held rather than
// one field of it.
func TestScan_MemberOfAGroupWhoseFirstMemberIsRead_IsReported(t *testing.T) {
	found := scanFixture(t, map[string]string{"fixture.go": `package fixture

const (
	usedConst  = "used"
	deadConst  = "dead"
	otherConst = "also used"
)

// Live is what keeps the group's first and last members read.
func Live() string { return usedConst + otherConst }
`})
	want := []Constant{{
		Package:   fixtureDir,
		File:      fixtureDir + "/fixture.go",
		Line:      5,
		Name:      "deadConst",
		GroupSize: 3,
	}}
	if !slices.Equal(found, want) {
		t.Fatalf("findings = %+v, want %+v: the package, the file below the repository root, the line, the name and the size of the group the linter could not see", found, want)
	}
}

// TestScan_ExportedConstant_IsNotReported keeps an exported constant out of
// scope. It may be read from a package these patterns never loaded, so a run
// over them cannot tell a dead one from one it did not look at.
func TestScan_ExportedConstant_IsNotReported(t *testing.T) {
	found := scanFixture(t, map[string]string{"fixture.go": `package fixture

const (
	usedConst = "used"
	// Exported is read from nowhere in this package and is still not judged.
	Exported = "exported"
)

func Live() string { return usedConst }
`})
	assertDead(t, found)
}

// TestScan_ConstantReadOnlyByATestFile_IsNotReported is why the load asks for
// the test variants. Forty-odd packages hand their action-ID block to the
// catalog test through an export_test.go, and a rule that skipped test files
// would report the live half of every one of them.
func TestScan_ConstantReadOnlyByATestFile_IsNotReported(t *testing.T) {
	found := scanFixture(t, map[string]string{
		"fixture.go": `package fixture

const (
	usedConst     = "used"
	testOnlyConst = "read only by the test"
)

func Live() string { return usedConst }
`,
		"fixture_test.go": `package fixture

import "testing"

func TestReadsTheConstant(t *testing.T) {
	if testOnlyConst == "" {
		t.Fatal("empty")
	}
}
`,
	})
	assertDead(t, found)
}

// TestScan_TestVariants_AreCountedAsThePackagesTheRepositoryNames holds the
// package count in the summary to what a reader would count. Loading the
// tests hands the walk four packages for one directory: the package, its
// in-package test variant, the external test package and the test main the
// go tool synthesizes. The first two are one package, since the variant is
// the same source with its tests beside it. The external test package is
// counted apart and a constant it declares is reported under its own name,
// because it really is a different package and its declaration key has to
// say so. The synthesized main is nobody's source and is not counted at
// all, which it used to be, once per tested package.
func TestScan_TestVariants_AreCountedAsThePackagesTheRepositoryNames(t *testing.T) {
	source := `package fixture

const usedConst = "used"

func Live() string { return usedConst }
`
	inPackageTest := `package fixture

import "testing"

func TestLive(t *testing.T) {
	if Live() == "" {
		t.Fatal("empty")
	}
}
`
	externalTest := `package fixture_test

import "testing"

const (
	externalUsed = "read by the external test"
	externalDead = "declared by the external test and read by nothing"
)

func TestExternal(t *testing.T) {
	if externalUsed == "" {
		t.Fatal("empty")
	}
}
`
	t.Run("in-package test variant", func(t *testing.T) {
		report, _ := auditFixture(t, map[string]string{"fixture.go": source, "fixture_test.go": inPackageTest}, nil)
		if report.Summary.Packages != 1 {
			t.Fatalf("Summary.Packages = %d, want 1: the test variant is the package with its tests beside it", report.Summary.Packages)
		}
		if len(report.Findings) != 0 || report.Summary.Declared != 1 {
			t.Fatalf("report = %+v, want one declared constant and no finding", report)
		}
	})
	t.Run("external test package", func(t *testing.T) {
		report, _ := auditFixture(t, map[string]string{"fixture.go": source, "fixture_ext_test.go": externalTest}, nil)
		if report.Summary.Packages != 2 {
			t.Fatalf("Summary.Packages = %d, want 2: the external test package is a package of its own", report.Summary.Packages)
		}
		want := []Constant{{Package: fixtureDir + "_test", File: fixtureDir + "/fixture_ext_test.go", Line: 7, Name: "externalDead", GroupSize: 2}}
		if !slices.Equal(report.Findings, want) {
			t.Fatalf("findings = %+v, want %+v: the finding is filed under the external test package's own name", report.Findings, want)
		}
		if report.Summary.Declared != 3 {
			t.Fatalf("Summary.Declared = %d, want 3", report.Summary.Declared)
		}
	})
}

// TestScan_ConstantNamedOnlyInACommentOrAString_IsReported is the difference
// between asking the type checker and grepping. Neither a comment nor a string
// that spells the name is a use of it.
func TestScan_ConstantNamedOnlyInACommentOrAString_IsReported(t *testing.T) {
	found := scanFixture(t, map[string]string{"fixture.go": `package fixture

const (
	usedConst = "used"
	// deadConst is named right here and nowhere that counts.
	deadConst = "dead"
)

// Live mentions deadConst in a string and never reads it.
func Live() string { return usedConst + "deadConst" }
`})
	assertDead(t, found, "deadConst")
}

// TestScan_ConstantReadFromAnotherFileOfThePackage_IsNotReported checks that
// the answer is the package's and not one file's.
func TestScan_ConstantReadFromAnotherFileOfThePackage_IsNotReported(t *testing.T) {
	found := scanFixture(t, map[string]string{
		"fixture.go": `package fixture

const (
	usedConst  = "used"
	otherConst = "read next door"
)

func Live() string { return usedConst }
`,
		"other.go": `package fixture

// NextDoor reads the constant the other file declares.
func NextDoor() string { return otherConst }
`,
	})
	assertDead(t, found)
}

// TestScan_ConstantInsideAFunction_IsReported covers the declaration the
// compiler lets stand: an unused local variable is a compile error and an
// unused local constant is not, so nothing but this reports one.
func TestScan_ConstantInsideAFunction_IsReported(t *testing.T) {
	found := scanFixture(t, map[string]string{"fixture.go": `package fixture

// Live declares one constant it reads and one it does not.
func Live() string {
	const (
		localUsed = "used"
		localDead = "dead"
	)
	return localUsed
}
`})
	assertDead(t, found, "localDead")
}

// TestScan_BlankConstant_IsNotReported keeps the iota placeholder out. A blank
// is a declaration nothing can ever read, so counting it would report every
// enumeration in the tree.
func TestScan_BlankConstant_IsNotReported(t *testing.T) {
	found := scanFixture(t, map[string]string{"fixture.go": `package fixture

type level int

const (
	_          level = iota
	levelFirst
)

// Live reads the only member that is not blank.
func Live() level { return levelFirst }
`})
	assertDead(t, found)
}

// TestScan_UnreadIotaMember_IsReported holds an enumeration to the same rule
// as any other group: a member nothing names is a member nothing names.
func TestScan_UnreadIotaMember_IsReported(t *testing.T) {
	found := scanFixture(t, map[string]string{"fixture.go": `package fixture

type level int

const (
	levelFirst level = iota
	levelSecond
)

func Live() level { return levelFirst }
`})
	assertDead(t, found, "levelSecond")
}

// TestScan_BlankInAGroup_IsNotCountedInTheGroupsSize keeps the group size the
// report prints to the members a reader could delete: the placeholder is left
// out of it as it is left out of the findings, so a group of a blank and two
// names reads as "1 of 2" rather than "1 of 3".
func TestScan_BlankInAGroup_IsNotCountedInTheGroupsSize(t *testing.T) {
	found := scanFixture(t, map[string]string{"fixture.go": `package fixture

type level int

const (
	_ level = iota
	levelFirst
	levelSecond
)

func Live() level { return levelFirst }
`})
	want := []Constant{{Package: fixtureDir, File: fixtureDir + "/fixture.go", Line: 8, Name: "levelSecond", GroupSize: 2}}
	if !slices.Equal(found, want) {
		t.Fatalf("findings = %+v, want %+v", found, want)
	}
}

// TestScan_ConstantReadByAConstantThatIsRead_IsNotReported follows a use
// through a constant expression, which the type checker records like any
// other.
func TestScan_ConstantReadByAConstantThatIsRead_IsNotReported(t *testing.T) {
	found := scanFixture(t, map[string]string{"fixture.go": `package fixture

const (
	base    = "base"
	derived = base + "/more"
)

func Live() string { return derived }
`})
	assertDead(t, found)
}

// otherPlatform is an operating system this test is not running on, chosen
// among the ones the command re-reads under rather than an exotic one: every
// GOARCH the suite runs on builds for linux and for windows, while plan9, the
// first choice, has no arm64 port and so no packages at all on Apple silicon.
var otherPlatform = func() string {
	if runtime.GOOS == "windows" {
		return "linux"
	}
	return "windows"
}()

// otherArch is an architecture this test is not running on, among the two
// the project ships for, so the pair with the host's operating system is one
// the toolchain builds.
var otherArch = func() string {
	if runtime.GOARCH == "arm64" {
		return "amd64"
	}
	return "arm64"
}()

// constrainedFixture is a package whose only reader of one constant sits
// behind the build constraint given, which this host does not satisfy.
func constrainedFixture(constraint string) map[string]string {
	return map[string]string{
		"fixture.go": `package fixture

const (
	usedConst     = "used"
	platformConst = "read only where the constraint holds"
)

func Live() string { return usedConst }
`,
		"fixture_" + constraint + ".go": `//go:build ` + constraint + `

package fixture

// Elsewhere is the only reader of platformConst, and this file is compiled
// nowhere but where its name and constraint say.
func Elsewhere() string { return platformConst }
`,
	}
}

// platformFixture is [constrainedFixture] for another operating system.
var platformFixture = constrainedFixture(otherPlatform)

// TestScan_ConstantReadOnlyByAnotherPlatformsFile_IsNotReported is why the
// packages a load left files out of are read again. Reporting this constant
// would fail a build over code doing its job on another operating system.
func TestScan_ConstantReadOnlyByAnotherPlatformsFile_IsNotReported(t *testing.T) {
	assertDead(t, scanFixtureAs(t, platformFixture, []target{{otherPlatform, runtime.GOARCH}}))
}

// TestScan_ConstantReadOnlyByAnotherArchitecturesFile_IsNotReported is the
// same for a file constrained to an architecture: a reload that set only the
// operating system kept the host's architecture and never saw it.
func TestScan_ConstantReadOnlyByAnotherArchitecturesFile_IsNotReported(t *testing.T) {
	assertDead(t, scanFixtureAs(t, constrainedFixture(otherArch), []target{{runtime.GOOS, otherArch}}))
}

// TestScan_PlatformNotReadAgain_ReportsTheConstantAsDead is the same fixture
// with the second load withheld, so the pair says what the second load buys
// rather than only that the first passes.
func TestScan_PlatformNotReadAgain_ReportsTheConstantAsDead(t *testing.T) {
	assertDead(t, scanFixtureAs(t, platformFixture, nil), "platformConst")
}

// TestScan_ArchitectureNotReadAgain_ReportsTheConstantAsDead is the
// architecture half of that pair: with the host's operating system alone in
// the list, the arm64 or amd64 file is never loaded and its read never seen.
func TestScan_ArchitectureNotReadAgain_ReportsTheConstantAsDead(t *testing.T) {
	assertDead(t, scanFixtureAs(t, constrainedFixture(otherArch), []target{{runtime.GOOS, runtime.GOARCH}}), "platformConst")
}

// TestScan_ConstantReadOnlyByAnotherPlatformsTestFile_IsNotReported holds the
// second load to the same terms as the first: it asks for the test variants
// too, since a constant read only by the Windows half of a package's tests
// is read, and a reload of the production files alone would report it.
func TestScan_ConstantReadOnlyByAnotherPlatformsTestFile_IsNotReported(t *testing.T) {
	files := map[string]string{
		"fixture.go": `package fixture

const (
	usedConst     = "used"
	platformConst = "read only by the other platform's test"
)

func Live() string { return usedConst }
`,
		"fixture_" + otherPlatform + "_test.go": `//go:build ` + otherPlatform + `

package fixture

import "testing"

func TestElsewhere(t *testing.T) {
	if platformConst == "" {
		t.Fatal("empty")
	}
}
`,
	}
	assertDead(t, scanFixtureAs(t, files, []target{{otherPlatform, runtime.GOARCH}}))
}

// TestScan_PackageReadTwice_DeclaresEachConstantOnce checks that the second
// load is a union with the first rather than a second count: a constant is
// keyed by where it sits in the tree, which both loads agree on, so the
// summary says two constants were declared and not four.
func TestScan_PackageReadTwice_DeclaresEachConstantOnce(t *testing.T) {
	report, progress := auditFixture(t, platformFixture, []target{{otherPlatform, runtime.GOARCH}})
	if want := "re-reading 1 package(s) as " + otherPlatform + "/" + runtime.GOARCH; !strings.Contains(progress, want) {
		t.Fatalf("progress = %q, want %q: the fixture holds a file this platform left out", progress, want)
	}
	if report.Summary.Declared != 2 {
		t.Fatalf("Summary.Declared = %d, want 2: the two loads saw the same two constants", report.Summary.Declared)
	}
	if report.Summary.Packages != 1 {
		t.Fatalf("Summary.Packages = %d, want 1", report.Summary.Packages)
	}
}

// TestScan_OnlyAnAssemblyFileLeftOut_IsNotReadAgain keeps the extra loads
// measured: a file the platform left out is a reason to re-read only when it
// is Go, because assembly reads no constant and the type checker records
// nothing from it, so a package whose sole excluded file is a `.s` for
// another platform is loaded once.
func TestScan_OnlyAnAssemblyFileLeftOut_IsNotReadAgain(t *testing.T) {
	files := map[string]string{
		"fixture.go": `package fixture

const usedConst = "used"

func Live() string { return usedConst }
`,
		"fixture_" + otherPlatform + ".s": `//go:build ` + otherPlatform + `

TEXT ·Nothing(SB),0,$0
	RET
`,
	}
	report, progress := auditFixture(t, files, []target{{otherPlatform, runtime.GOARCH}})
	if progress != "" {
		t.Fatalf("progress = %q, want nothing: an excluded assembly file is not a reason to load the package again", progress)
	}
	if len(report.Findings) != 0 || report.Summary.Declared != 1 {
		t.Fatalf("report = %+v, want one declared constant and no finding", report)
	}
}

// TestScan_LocalConstantSharingAName_IsKeyedApartFromThePackageLevelOne holds
// the declaration table to the constant's identity: a declaration that
// excuses the package-level constant leaves a function-local one of the same
// name reported, and the finding says which function it sits in.
func TestScan_LocalConstantSharingAName_IsKeyedApartFromThePackageLevelOne(t *testing.T) {
	original := unreadOnPurpose
	t.Cleanup(func() { unreadOnPurpose = original })
	unreadOnPurpose = map[string]string{fixtureDir + ":shared": "the package-level one is kept on purpose in this test"}

	found := scanFixtureAs(t, map[string]string{"fixture.go": `package fixture

const shared = "package level, declared unread on purpose"

type walker struct{}

func (w *walker) Walk() {
	const shared = "local, not declared, and unread"
}

func Live() {}
`}, nil)
	assertDead(t, found, "shared")
	if got := found[0].Func; got != "walker.Walk" {
		t.Fatalf("Func = %q, want the enclosing method %q", got, "walker.Walk")
	}
	if got := declarationKey(found[0]); got != fixtureDir+":walker.Walk.shared" {
		t.Fatalf("declarationKey = %q, want the function between package and name", got)
	}
}

// TestScan_LocalConstantInAGenericMethod_IsKeyedByTheReceiversTypeName
// spells a generic receiver the way the declaration table does: the type
// parameters are the receiver's and not part of its name, whether there is
// one of them or two, and whether the receiver is a pointer.
func TestScan_LocalConstantInAGenericMethod_IsKeyedByTheReceiversTypeName(t *testing.T) {
	found := scanFixture(t, map[string]string{"fixture.go": `package fixture

type box[T any] struct{ value T }

func (b box[T]) One() {
	const oneLocal = "unread"
}

type pair[K comparable, V any] struct{ key K }

func (p *pair[K, V]) Two() {
	const twoLocal = "unread"
}

func Live() {}
`})
	want := []Constant{
		{Package: fixtureDir, File: fixtureDir + "/fixture.go", Line: 6, Name: "oneLocal", Func: "box.One", GroupSize: 1},
		{Package: fixtureDir, File: fixtureDir + "/fixture.go", Line: 12, Name: "twoLocal", Func: "pair.Two", GroupSize: 1},
	}
	if !slices.Equal(found, want) {
		t.Fatalf("findings = %+v, want %+v", found, want)
	}
}

// TestScan_LocalConstantInAParenthesizedReceiver_IsKeyedByTheReceiversTypeName
// holds the key to what the type checker accepts: `(w (walker))`,
// `(w *(walker))` and `(w (*walker))` all declare methods of walker, and a
// key that dropped the receiver there would leave a declaration written for
// `walker.Walk` matching nothing.
func TestScan_LocalConstantInAParenthesizedReceiver_IsKeyedByTheReceiversTypeName(t *testing.T) {
	found := scanFixture(t, map[string]string{"fixture.go": `package fixture

type walker struct{}

func (w (walker)) Walk() {
	const walkLocal = "unread"
}

func (w *(walker)) Ptr() {
	const ptrLocal = "unread"
}

func (w (*walker)) PtrParen() {
	const ptrParenLocal = "unread"
}

func Live() {}
`})
	want := []Constant{
		{Package: fixtureDir, File: fixtureDir + "/fixture.go", Line: 6, Name: "walkLocal", Func: "walker.Walk", GroupSize: 1},
		{Package: fixtureDir, File: fixtureDir + "/fixture.go", Line: 10, Name: "ptrLocal", Func: "walker.Ptr", GroupSize: 1},
		{Package: fixtureDir, File: fixtureDir + "/fixture.go", Line: 14, Name: "ptrParenLocal", Func: "walker.PtrParen", GroupSize: 1},
	}
	if !slices.Equal(found, want) {
		t.Fatalf("findings = %+v, want %+v", found, want)
	}
}

// TestFuncDeclName_ReceiverShapesTheTypeCheckerRefuses_FallBackToTheBareName
// pins the fallback for the two shapes no accepted package can carry: a
// receiver list with nothing in it, and a receiver whose base is not a type
// name. The loader refuses both before the walk sees them, so this is the
// one place they are exercised, and what is held is that the walk spells
// the bare name rather than dereferencing an empty list.
func TestFuncDeclName_ReceiverShapesTheTypeCheckerRefuses_FallBackToTheBareName(t *testing.T) {
	cases := []struct {
		name string
		recv *ast.FieldList
	}{
		{"no receiver", nil},
		{"empty receiver list", &ast.FieldList{}},
		{"qualified receiver", &ast.FieldList{List: []*ast.Field{{
			Type: &ast.SelectorExpr{X: ast.NewIdent("other"), Sel: ast.NewIdent("Type")},
		}}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fn := &ast.FuncDecl{Name: ast.NewIdent("Method"), Recv: testCase.recv}
			if got := funcDeclName(fn); got != "Method" {
				t.Fatalf("funcDeclName = %q, want the bare name", got)
			}
		})
	}
}

// TestObserveFile_NameTheTypeCheckerDidNotDefine_IsNotRecorded pins what the
// walk trusts: a constant is recorded from the type checker's definition of
// its name and never from the syntax alone, so a file whose names carry no
// definition declares nothing here. The loader never hands the walk such a
// file, since a package that did not type-check is refused before it; the
// same file under the checker's own record declares its one constant.
func TestObserveFile_NameTheTypeCheckerDidNotDefine_IsNotRecorded(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", "package fixture\n\nconst alone = \"declared\"\n", 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	checked := &types.Info{Defs: map[*ast.Ident]types.Object{}}
	if _, checkErr := (&types.Config{}).Check("fixture", fset, []*ast.File{file}, checked); checkErr != nil {
		t.Fatalf("type-check: %v", checkErr)
	}
	cases := []struct {
		name string
		info *types.Info
		want int
	}{
		{"the type checker's record", checked, 1},
		{"no record of the name", &types.Info{Defs: map[*ast.Ident]types.Object{}}, 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			found := newScanner(repoRoot(t))
			found.observeFile(&packages.Package{PkgPath: "example.com/fixture", Fset: fset, TypesInfo: testCase.info}, file)
			if len(found.declared) != testCase.want {
				t.Fatalf("declared = %v, want %d recorded", found.declared, testCase.want)
			}
		})
	}
}

// TestConstNames_SpecThatIsNotAValueSpec_BindsNothing pins the other
// defensive branch of the walk: the parser puts nothing but value specs
// under a const keyword, so a spec of another kind reaches the walk from no
// file it parsed, and one that did would bind no names rather than abort
// the scan.
func TestConstNames_SpecThatIsNotAValueSpec_BindsNothing(t *testing.T) {
	decl := &ast.GenDecl{Tok: token.CONST, Specs: []ast.Spec{
		&ast.TypeSpec{Name: ast.NewIdent("notAValue")},
		&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent("_"), ast.NewIdent("first"), ast.NewIdent("second")}},
	}}
	names := constNames(decl)
	got := make([]string, 0, len(names))
	for _, name := range names {
		got = append(got, name.Name)
	}
	if want := []string{"first", "second"}; !slices.Equal(got, want) {
		t.Fatalf("constNames = %v, want %v: the type spec binds nothing and the blank is left out", got, want)
	}
}

// TestAudit_DeclarationForAConstantThatIsRead_IsReportedStale is the second
// claim of the declaration table, driven through the scan rather than handed
// to the report: the table names the fixture package the way the repository
// names it, the run loads that package and finds the constant read, and the
// entry is reported stale under -check. It is the one place the package
// names the scan records are held against the names the table is written
// in, so a scan that kept the module path or the test-variant decoration
// would pass over every entry as out of view.
func TestAudit_DeclarationForAConstantThatIsRead_IsReportedStale(t *testing.T) {
	original := unreadOnPurpose
	t.Cleanup(func() { unreadOnPurpose = original })
	unreadOnPurpose = map[string]string{fixtureDir + ":usedConst": "declared unread on purpose in this test, and read by Live"}

	root := repoRoot(t)
	overlay := map[string][]byte{
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte(`package fixture

const usedConst = "used"

func Live() string { return usedConst }
`),
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture_test.go"): []byte(`package fixture

import "testing"

func TestLive(t *testing.T) {
	if Live() == "" {
		t.Fatal("empty")
	}
}
`),
	}
	var stdout, stderr strings.Builder
	code := run(auditConfig{
		dir:      root,
		patterns: []string{"./" + fixtureDir},
		overlay:  overlay,
		out:      &stdout,
	}, true, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1: a stale declaration fails the gate", code)
	}
	if want := fixtureDir + ":usedConst: declared unread on purpose, and this run found it read or found it gone\n"; !strings.Contains(stdout.String(), want) {
		t.Fatalf("stdout does not name the stale declaration %q:\n%s", want, stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty: a stale declaration is a finding, not a broken run", stderr.String())
	}
}

// TestAudit_PlatformLoadThatFails_StopsTheRun: a platform whose load cannot be
// made leaves that platform's uses unrecorded, which would turn into findings
// about live code, so the run ends rather than reporting.
func TestAudit_PlatformLoadThatFails_StopsTheRun(t *testing.T) {
	root := repoRoot(t)
	overlay := map[string][]byte{}
	for name, source := range platformFixture {
		overlay[filepath.Join(root, filepath.FromSlash(fixtureDir), name)] = []byte(source)
	}
	_, err := audit(auditConfig{
		dir:      root,
		patterns: []string{"./" + fixtureDir},
		overlay:  overlay,
		targets:  []target{{"notanoperatingsystem", runtime.GOARCH}},
	})
	if err == nil {
		t.Fatal("audit error = nil, want the platform load failure")
	}
}

// TestVariantName_TestDecorations_ResolveToOnePackage checks the one piece of
// string handling the loader forces on this command.
func TestVariantName_TestDecorations_ResolveToOnePackage(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"plain package", "example.com/p", "example.com/p"},
		{"in-package test variant", "example.com/p [example.com/p.test]", "example.com/p"},
		{"external test package", "example.com/p_test [example.com/p.test]", "example.com/p_test"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := variantName(testCase.path); got != testCase.want {
				t.Fatalf("variantName(%q) = %q, want %q", testCase.path, got, testCase.want)
			}
		})
	}
}

// TestIsTestMain_OnlyTheSynthesizedMain_IsPassedOver keeps the skip to the
// one package it is for: the external test package ends in "_test" and is
// source of this repository, and a package whose last element merely
// contains "test" is an ordinary package.
func TestIsTestMain_OnlyTheSynthesizedMain_IsPassedOver(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"plain package", "example.com/p", false},
		{"external test package", "example.com/p_test", false},
		{"package named after tests", "example.com/testutil", false},
		{"synthesized test main", "example.com/p.test", true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := isTestMain(testCase.path); got != testCase.want {
				t.Fatalf("isTestMain(%q) = %v, want %v", testCase.path, got, testCase.want)
			}
		})
	}
}

// TestRelativePath_OutsideTheRoot_KeepsTheAbsolutePath checks the fallback: a
// file the root does not contain is named in full rather than as a climb out
// of the repository, and a file named relative to nowhere, which cannot be
// made relative to an absolute root at all, is named as it came.
func TestRelativePath_OutsideTheRoot_KeepsTheAbsolutePath(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "repo", "root")
	inside := filepath.Join(root, "internal", "tools", "file.go")
	if got := relativePath(inside, root); got != "internal/tools/file.go" {
		t.Fatalf("relativePath(inside) = %q, want internal/tools/file.go", got)
	}
	outside := filepath.Join(string(filepath.Separator), "elsewhere", "file.go")
	if got := relativePath(outside, root); got != "/elsewhere/file.go" {
		t.Fatalf("relativePath(outside) = %q, want the path unchanged", got)
	}
	relative := filepath.Join("already", "relative", "file.go")
	if got := relativePath(relative, root); got != "already/relative/file.go" {
		t.Fatalf("relativePath(relative) = %q, want the path as it came", got)
	}
}

// TestTrimModulePath_ThisModule_IsNamedTheWayTheRepositoryDoes checks that a
// finding names a package the way a reader would type it.
func TestTrimModulePath_ThisModule_IsNamedTheWayTheRepositoryDoes(t *testing.T) {
	if got := trimModulePath("github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"); got != "internal/tools" {
		t.Fatalf("trimModulePath = %q, want internal/tools", got)
	}
	if got := trimModulePath("example.com/other/pkg"); got != "example.com/other/pkg" {
		t.Fatalf("trimModulePath of another module = %q, want it unchanged", got)
	}
}
