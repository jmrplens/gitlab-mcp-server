package goprogram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// selfPattern is the package this test loads: loading the front end with
// itself keeps the fixture honest (it is real, committed, type-checking Go)
// without adding a testdata tree the rest of the repository would have to know
// about.
const selfPattern = "./cmd/internal/goprogram/..."

// overlayFile is the path an overlaid fixture file takes inside this package.
// go/packages resolves an overlay by absolute path, so the name has to sit in
// the directory of the package the pattern selects.
const overlayFile = "goprogram_overlay_fixture.go"

// repoRoot walks up from the test's working directory to the module root, so a
// pattern and an overlay path can both be written against the repository.
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

// overlayFor puts one source file into this package without writing it to
// disk, which is the mechanism every gate's fixture depends on.
func overlayFor(t *testing.T, source string) map[string][]byte {
	t.Helper()
	path := filepath.Join(repoRoot(t), "cmd", "internal", "goprogram", overlayFile)
	return map[string][]byte{path: []byte(source)}
}

// TestLoad_RealPackage_ReturnsTypedSyntax verifies the happy path: the loader
// returns packages carrying both syntax and type information, which is what
// every caller reads without checking first.
func TestLoad_RealPackage_ReturnsTypedSyntax(t *testing.T) {
	loaded, err := Load(repoRoot(t), []string{selfPattern}, nil)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if len(loaded) == 0 {
		t.Fatal("Load() returned no packages")
	}
	for _, pkg := range loaded {
		if pkg.TypesInfo == nil || pkg.Types == nil {
			t.Errorf("Load() gave %s no type information", pkg.PkgPath)
		}
		if len(pkg.Syntax) == 0 {
			t.Errorf("Load() gave %s no syntax", pkg.PkgPath)
		}
	}
}

// TestLoad_Overlay_TypeChecksSourceThatIsNotOnDisk verifies the parameter three
// of the gates build their fixtures on: a file supplied by the caller is
// type-checked as part of the package, so a fixture written in a test file
// resolves against the real packages it imports.
func TestLoad_Overlay_TypeChecksSourceThatIsNotOnDisk(t *testing.T) {
	const fixture = `package goprogram

// overlayFixtureMarker exists only inside a test's overlay.
const overlayFixtureMarker = "overlaid"
`
	loaded, err := Load(repoRoot(t), []string{selfPattern}, overlayFor(t, fixture))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	// The constant reaching the type checker is the only proof that the
	// overlay was honored rather than silently dropped.
	if !declaresConstant(loaded, "overlayFixtureMarker") {
		t.Error("Load() did not type-check the overlaid file")
	}
}

// TestLoad_BrokenPackage_IsRefused verifies the rule this package exists for. A
// package that does not type-check resolves nothing, so a gate reading it would
// report a clean run over source it never understood.
func TestLoad_BrokenPackage_IsRefused(t *testing.T) {
	const broken = `package goprogram

func brokenOverlayFixture() int { return "not an int" }
`
	loaded, err := Load(repoRoot(t), []string{selfPattern}, overlayFor(t, broken))

	if err == nil {
		t.Fatal("Load() accepted a package that does not type-check")
	}
	if loaded != nil {
		t.Errorf("Load() returned %d package(s) with an error, want none", len(loaded))
	}
	if !strings.Contains(err.Error(), "goprogram") {
		t.Errorf("Load() error = %q, want it to name the package that failed", err)
	}
}

// TestLoad_PatternMatchingNothing_IsRefused verifies the guard against a gate
// pointed at the wrong tree: it would audit nothing and report success. docs/
// is a real directory of this repository holding no Go at all, which is the
// shape a mistyped pattern produces.
func TestLoad_PatternMatchingNothing_IsRefused(t *testing.T) {
	loaded, err := Load(repoRoot(t), []string{"./docs/..."}, nil)

	if err == nil {
		t.Fatalf("Load() error = nil and returned %d package(s), want the empty-match refusal", len(loaded))
	}
	if !strings.Contains(err.Error(), "no packages matched") {
		t.Errorf("Load() error = %q, want it to say nothing matched", err)
	}
}

// TestLoad_DirectoryOutsideAModule_IsReported verifies that a root the go
// command cannot work in is reported rather than read as an empty tree.
func TestLoad_DirectoryOutsideAModule_IsReported(t *testing.T) {
	loaded, err := Load(filepath.Join(t.TempDir(), "absent"), []string{"./..."}, nil)

	if err == nil {
		t.Fatalf("Load() error = nil and returned %d package(s), want a load failure", len(loaded))
	}
	if !strings.Contains(err.Error(), "load packages") {
		t.Errorf("Load() error = %q, want it to say the load failed", err)
	}
}

// TestLoadWith_Tests_IncludesTheTestVariant verifies the one thing the options
// variant adds for a gate over test files: with Tests set, the package comes
// back type-checked with its _test.go files in it, which [Load] never does.
// The proof is this very file appearing in a loaded package's syntax.
func TestLoadWith_Tests_IncludesTheTestVariant(t *testing.T) {
	loaded, err := LoadWith(repoRoot(t), []string{selfPattern}, Options{Tests: true})
	if err != nil {
		t.Fatalf("LoadWith() error = %v, want nil", err)
	}
	if !loadedFile(loaded, "load_test.go") {
		t.Error("LoadWith(Tests: true) loaded no package holding load_test.go")
	}
}

// TestLoad_Default_ExcludesTestFiles pins the other half of that contract: the
// four earlier gates load production source only, and the options variant
// must not have changed what [Load] hands them.
func TestLoad_Default_ExcludesTestFiles(t *testing.T) {
	loaded, err := Load(repoRoot(t), []string{selfPattern}, nil)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if loadedFile(loaded, "load_test.go") {
		t.Error("Load() loaded a package holding load_test.go, which only a Tests load should")
	}
}

// TestLoadWith_BuildTags_SelectsConstrainedSource verifies that a build tag
// reaches the loader: a file behind a constraint is absent under the default
// load and present when the tag is asked for. The e2e packages exist only
// behind their tag, so a load that dropped it would type-check nothing and
// report a clean run over an empty tree.
func TestLoadWith_BuildTags_SelectsConstrainedSource(t *testing.T) {
	const constrained = `//go:build goprogramfixture

package goprogram

// constrainedFixtureMarker exists only behind the goprogramfixture tag.
const constrainedFixtureMarker = "constrained"
`
	overlay := overlayFor(t, constrained)
	cases := []struct {
		name string
		tags []string
		want bool
	}{
		{name: "without the tag the file is excluded", tags: nil, want: false},
		{name: "with the tag the file is type-checked", tags: []string{"goprogramfixture"}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loaded, err := LoadWith(repoRoot(t), []string{selfPattern}, Options{BuildTags: tc.tags, Overlay: overlay})
			if err != nil {
				t.Fatalf("LoadWith() error = %v, want nil", err)
			}
			if got := declaresConstant(loaded, "constrainedFixtureMarker"); got != tc.want {
				t.Errorf("constrained file type-checked = %t, want %t", got, tc.want)
			}
		})
	}
}

// loadedFile reports whether any loaded package carries a file of that base
// name in its syntax, which is how a test tells a test variant from the plain
// package.
func loadedFile(loaded []*packages.Package, base string) bool {
	for _, pkg := range loaded {
		for _, file := range pkg.CompiledGoFiles {
			if filepath.Base(file) == base {
				return true
			}
		}
	}
	return false
}

// declaresConstant reports whether the type checker saw a package-level
// constant of that name in any loaded package.
func declaresConstant(loaded []*packages.Package, name string) bool {
	for _, pkg := range loaded {
		if pkg.Types != nil && pkg.Types.Scope().Lookup(name) != nil {
			return true
		}
	}
	return false
}

// TestPackageLoadError_CleanPackage_ReturnsNil verifies the happy path of the
// per-package check, which is what lets a clean package through untouched.
func TestPackageLoadError_CleanPackage_ReturnsNil(t *testing.T) {
	if err := packageLoadError(&packages.Package{PkgPath: "example.com/clean"}); err != nil {
		t.Errorf("packageLoadError() = %v, want nil", err)
	}
}
