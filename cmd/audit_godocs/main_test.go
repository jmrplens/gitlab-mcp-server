package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var errUnexpectedSuccess = errors.New("expected command to fail")

// TestAuditPackage_DetectsPackageCommentProblems verifies package-level
// documentation checks for missing, malformed, and duplicate package comments.
//
// The test builds temporary packages with file comments attached to package
// clauses, then audits them directly without invoking go list. It protects the
// Godoc rule that each package must have one canonical package comment.
func TestAuditPackage_DetectsPackageCommentProblems(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		files      map[string]string
		categories []string
	}{
		{
			name: "missing package doc",
			files: map[string]string{
				"sample.go": "package sample\n",
			},
			categories: []string{categoryPackageDocMissing},
		},
		{
			name: "malformed package doc",
			files: map[string]string{
				"sample.go": "// sample.go describes a file, not the package.\npackage sample\n",
			},
			categories: []string{categoryPackageDocForm},
		},
		{
			name: "multiple package docs",
			files: map[string]string{
				"doc.go":    "// Package sample provides a fixture.\npackage sample\n",
				"sample.go": "// sample.go should not be package documentation.\npackage sample\n",
			},
			categories: []string{categoryPackageDocMultiple, categoryPackageDocForm},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pkg := writePackageFixture(t, "sample", tc.files)

			findings, err := auditPackage(pkg, false)
			if err != nil {
				t.Fatalf("auditPackage() error = %v", err)
			}
			for _, category := range tc.categories {
				if !hasCategory(findings, category) {
					t.Fatalf("missing category %q in %#v", category, findings)
				}
			}
		})
	}
}

// TestAuditPackage_AcceptsCommandPackageDoc verifies that main packages use the
// `Command` documentation form instead of the regular `Package` form.
//
// The fixture represents a command under `cmd/`. The audit should accept the
// package comment and report no package documentation findings.
func TestAuditPackage_AcceptsCommandPackageDoc(t *testing.T) {
	t.Parallel()

	pkg := writePackageFixture(t, "main", map[string]string{
		"main.go": "// Command widget audits widgets.\npackage main\n",
	})
	findings, err := auditPackage(pkg, false)
	if err != nil {
		t.Fatalf("auditPackage() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("auditPackage() findings = %#v, want none", findings)
	}
}

// TestAuditPackage_DetectsExportedSymbolDocumentation verifies checks for
// exported functions, types, constants, and variables.
//
// The fixture intentionally mixes missing and malformed comments. The audit
// must report each exported symbol category so package cleanup can be planned
// without relying on golangci-lint output parsing.
func TestAuditPackage_DetectsExportedSymbolDocumentation(t *testing.T) {
	t.Parallel()

	pkg := writePackageFixture(t, "sample", map[string]string{
		"doc.go": "// Package sample provides a fixture.\npackage sample\n",
		"sample.go": `package sample

const MissingConst = "missing"

// Defaults for sample.
const DefaultMode = "auto"

var MissingVar = "missing"

// Values used by sample.
var DefaultName = "sample"

func MissingFunc() {}

// BadType describes a type with a valid comment.
type BadType struct{}

// Does something without starting with the method name.
func (BadType) Run() {}
`,
	})

	findings, err := auditPackage(pkg, false)
	if err != nil {
		t.Fatalf("auditPackage() error = %v", err)
	}
	for _, category := range []string{categoryConstMissing, categoryConstForm, categoryVarMissing, categoryVarForm, categoryFuncMissing, categoryMethodForm} {
		if !hasCategory(findings, category) {
			t.Fatalf("missing category %q in %#v", category, findings)
		}
	}
}

// TestAuditPackage_AcceptsGroupedConstAndVarDocumentation verifies that
// descriptive comments on grouped exported values are accepted.
//
// Go doc comments allow a grouped const or var declaration to have a group-level
// sentence that describes the set without starting with any one identifier. The
// audit should follow that convention while still requiring ungrouped exported
// values to start with their declared name.
func TestAuditPackage_AcceptsGroupedConstAndVarDocumentation(t *testing.T) {
	t.Parallel()

	pkg := writePackageFixture(t, "sample", map[string]string{
		"doc.go": "// Package sample provides a fixture.\npackage sample\n",
		"sample.go": `package sample

// States accepted by the sample workflow.
const (
	StateOpen = "open"
	StateClosed = "closed"
)

// Shared errors returned by sample operations.
var (
	ErrMissing = errors.New("missing")
	ErrInvalid = errors.New("invalid")
)
`,
	})

	findings, err := auditPackage(pkg, false)
	if err != nil {
		t.Fatalf("auditPackage() error = %v", err)
	}
	if hasCategory(findings, categoryConstForm) || hasCategory(findings, categoryVarForm) {
		t.Fatalf("grouped const/var comments should be accepted: %#v", findings)
	}
}

// TestAuditPackage_IncludeTestsDetectsTestDocs verifies the optional test
// documentation audit for Test, Benchmark, Fuzz, and Example functions.
//
// The fixture places undocumented test functions in a `_test.go` file. The
// audit should ignore them by default and report them when includeTests is true.
func TestAuditPackage_IncludeTestsDetectsTestDocs(t *testing.T) {
	t.Parallel()

	pkg := writePackageFixture(t, "sample", map[string]string{
		"doc.go":    "// Package sample provides a fixture.\npackage sample\n",
		"sample.go": "package sample\n",
		"sample_test.go": `package sample

func TestWidget(t *testing.T) {}
func BenchmarkWidget(b *testing.B) {}
func FuzzWidget(f *testing.F) {}

// ExampleWidget demonstrates widget output.
func ExampleWidget() {
}
`,
	})

	withoutTests, err := auditPackage(pkg, false)
	if err != nil {
		t.Fatalf("auditPackage(includeTests=false) error = %v", err)
	}
	if hasCategory(withoutTests, categoryTestMissing) {
		t.Fatalf("test docs should be ignored by default: %#v", withoutTests)
	}

	withTests, err := auditPackage(pkg, true)
	if err != nil {
		t.Fatalf("auditPackage(includeTests=true) error = %v", err)
	}
	for _, category := range []string{categoryTestMissing, categoryBenchmarkMissing, categoryFuzzMissing, categoryExampleOutput} {
		if !hasCategory(withTests, category) {
			t.Fatalf("missing category %q in %#v", category, withTests)
		}
	}
}

// TestRun_UnsupportedFormat_ReturnsError verifies that the command rejects
// unknown report formats.
//
// The test invokes run through the test seam with an unsupported format. It
// confirms CLI validation fails before any repository scan occurs.
func TestRun_UnsupportedFormat_ReturnsError(t *testing.T) {
	t.Parallel()

	_, err := runForTest([]string{"--format=xml"})
	if err == nil {
		t.Fatal(errUnexpectedSuccess)
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("runForTest() error = %q, want unsupported format", err)
	}
}

func writePackageFixture(t *testing.T, packageName string, files map[string]string) packageInfo {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	return packageInfo{Dir: dir, ImportPath: "example.com/" + packageName, Name: packageName}
}

func hasCategory(findings []finding, category string) bool {
	for _, finding := range findings {
		if finding.Category == category {
			return true
		}
	}
	return false
}

func runForTest(args []string) (string, error) {
	var out bytes.Buffer
	err := run(args, &out)
	return out.String(), err
}
