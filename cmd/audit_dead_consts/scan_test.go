package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"testing"
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
	root := repoRoot(t)
	overlay := map[string][]byte{}
	for name, source := range files {
		overlay[filepath.Join(root, filepath.FromSlash(fixtureDir), name)] = []byte(source)
	}
	report, err := audit(auditConfig{
		dir:      root,
		patterns: []string{"./" + fixtureDir},
		overlay:  overlay,
		targets:  targets,
	})
	if err != nil {
		t.Fatalf("audit fixture: %v", err)
	}
	if len(report.Stale) != 0 {
		t.Fatalf("fixture run reported stale declarations %v, which means the real table excused something here", report.Stale)
	}
	return report.Findings
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
// exact shape: one constant the package returns and one nothing names.
func TestScan_MemberOfAGroupWhoseFirstMemberIsRead_IsReported(t *testing.T) {
	found := scanFixture(t, map[string]string{"fixture.go": `package fixture

const (
	usedConst = "used"
	deadConst = "dead"
)

// Live is what keeps the group's first member read.
func Live() string { return usedConst }
`})
	assertDead(t, found, "deadConst")
	if found[0].GroupSize != 2 {
		t.Fatalf("GroupSize = %d, want 2: the report says whether the linter already had its chance", found[0].GroupSize)
	}
	if found[0].Package != fixtureDir {
		t.Fatalf("Package = %q, want %q", found[0].Package, fixtureDir)
	}
	if found[0].File != fixtureDir+"/fixture.go" {
		t.Fatalf("File = %q, want the fixture below the repository root", found[0].File)
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

// TestImportable_OnlyThePlainPackage_CanBeAskedForAgain guards the second
// load: the go tool resolves none of the decorated spellings, so asking for
// one would fail the run rather than re-read a package.
func TestImportable_OnlyThePlainPackage_CanBeAskedForAgain(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"plain package", "example.com/p", true},
		{"in-package test variant", "example.com/p [example.com/p.test]", false},
		{"external test package", "example.com/p_test [example.com/p.test]", false},
		{"synthesized test main", "example.com/p.test", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := importable(testCase.path); got != testCase.want {
				t.Fatalf("importable(%q) = %v, want %v", testCase.path, got, testCase.want)
			}
		})
	}
}

// TestRelativePath_OutsideTheRoot_KeepsTheAbsolutePath checks the fallback: a
// file the root does not contain is named in full rather than as a climb out
// of the repository.
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
