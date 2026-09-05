package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/doc"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
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
		{
			name: "package doc outside doc.go",
			files: map[string]string{
				"sample.go": "// Package sample provides a fixture.\npackage sample\n",
			},
			categories: []string{categoryPackageDocLocation},
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
		"doc.go":  "// Command widget audits widgets.\npackage main\n",
		"main.go": "package main\n",
	})
	findings, err := auditPackage(pkg, false)
	if err != nil {
		t.Fatalf("auditPackage() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("auditPackage() findings = %#v, want none", findings)
	}
}

// TestAuditPackage_CommandPackageDocForm verifies a main package whose
// comment does not open with "Command " is reported, and that the expected
// opening named in the finding is the command form rather than "Package
// main". A subdirectory beside the source is ignored by the parser.
func TestAuditPackage_CommandPackageDocForm(t *testing.T) {
	t.Parallel()

	pkg := writePackageFixture(t, "main", map[string]string{
		"doc.go": "// widget audits widgets.\npackage main\n",
	})
	if err := os.MkdirAll(filepath.Join(pkg.Dir, "testdata"), 0o750); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	findings, err := auditPackage(pkg, false)
	if err != nil {
		t.Fatalf("auditPackage() error = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("auditPackage() = %#v, want exactly one finding", findings)
	}
	if findings[0].Category != categoryPackageDocForm {
		t.Errorf("category = %q, want %q", findings[0].Category, categoryPackageDocForm)
	}
	if want := `package comment must start with "Command "; got "widget audits widgets."`; findings[0].Detail != want {
		t.Errorf("detail = %q, want %q", findings[0].Detail, want)
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
		t.Run(category, func(t *testing.T) {
			t.Parallel()
			if !hasCategory(findings, category) {
				t.Fatalf("missing category %q in %#v", category, findings)
			}
		})
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
		t.Run(category, func(t *testing.T) {
			t.Parallel()
			if !hasCategory(withTests, category) {
				t.Fatalf("missing category %q in %#v", category, withTests)
			}
		})
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

// writeAuditFile writes content to name (a slash-separated path relative to
// dir), creating parent directories as needed.
func writeAuditFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

// writeModuleFixture writes a two-package Go module into a temporary
// directory and returns its root. Package a has no package comment, one
// undocumented exported function and one undocumented test; internal/b is
// documented except for one function, so --ignore-internal changes both the
// audited package count and the finding list.
func writeModuleFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeAuditFile(t, dir, "go.mod", "module example.com/fixture\n\ngo 1.27\n")
	writeAuditFile(t, dir, "a/a.go", "package a\n\nfunc Missing() {}\n")
	writeAuditFile(t, dir, "a/a_test.go", "package a\n\nimport \"testing\"\n\nfunc TestUndocumented(t *testing.T) {}\n")
	writeAuditFile(t, dir, "internal/b/doc.go", "// Package b is documented.\npackage b\n")
	writeAuditFile(t, dir, "internal/b/b.go", "package b\n\nfunc AlsoMissing() {}\n")
	return dir
}

// findingA is the func_missing finding for package a's exported function.
var findingA = finding{
	Category: categoryFuncMissing, ImportPath: "example.com/fixture/a", Package: "a",
	Name: "Missing", Detail: "missing func documentation",
}

// findingPkgA is the missing package comment for package a.
var findingPkgA = finding{
	Category: categoryPackageDocMissing, ImportPath: "example.com/fixture/a", Package: "a",
	Name: "a", Detail: "missing package documentation",
}

// findingTestA is the undocumented test in package a, reported only with
// --include-tests. Its File is the path go list handed the audit: absolute,
// because relativePath only shortens a path that is already relative to the
// working directory.
func findingTestA(dir string) finding {
	return finding{
		Category: categoryTestMissing, ImportPath: "example.com/fixture/a", Package: "a",
		File: filepath.ToSlash(filepath.Join(dir, "a", "a_test.go")),
		Name: "TestUndocumented", Detail: "missing test documentation",
	}
}

// findingB is the func_missing finding inside the internal package.
var findingB = finding{
	Category: categoryFuncMissing, ImportPath: "example.com/fixture/internal/b", Package: "b",
	Name: "AlsoMissing", Detail: "missing func documentation",
}

// TestRun_AuditsModuleAndReportsFindings verifies the audit command end to
// end against a real module: go list discovers both packages, the JSON
// report carries every finding in sorted order with the counts derived from
// them, --include-tests adds the test finding, and --ignore-internal drops
// the internal package from both the count and the findings.
func TestRun_AuditsModuleAndReportsFindings(t *testing.T) {
	testCases := []struct {
		name         string
		args         []string
		wantPackages int
		wantFindings func(dir string) []finding
	}{
		{
			name:         "default audits source symbols only",
			args:         []string{"--format=json"},
			wantPackages: 2,
			wantFindings: func(string) []finding { return []finding{findingA, findingPkgA, findingB} },
		},
		{
			name:         "include-tests adds test documentation findings",
			args:         []string{"--format=json", "--include-tests"},
			wantPackages: 2,
			wantFindings: func(dir string) []finding {
				return []finding{findingA, findingPkgA, findingTestA(dir), findingB}
			},
		},
		{
			name:         "ignore-internal skips internal packages",
			args:         []string{"--format=json", "--ignore-internal"},
			wantPackages: 1,
			wantFindings: func(string) []finding { return []finding{findingA, findingPkgA} },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeModuleFixture(t)
			t.Chdir(dir)
			wantFindings := tc.wantFindings(dir)

			out, err := runForTest(tc.args)
			if err != nil {
				t.Fatalf("run(%v) error = %v", tc.args, err)
			}

			var got report
			if unmarshalErr := json.Unmarshal([]byte(out), &got); unmarshalErr != nil {
				t.Fatalf("decode report: %v\n%s", unmarshalErr, out)
			}
			if got.Packages != tc.wantPackages {
				t.Errorf("Packages = %d, want %d", got.Packages, tc.wantPackages)
			}
			if !reflect.DeepEqual(got.Findings, wantFindings) {
				t.Errorf("Findings = %+v\nwant %+v", got.Findings, wantFindings)
			}
			wantCategories := countBy(wantFindings, func(f finding) string { return f.Category })
			if !reflect.DeepEqual(got.ByCategory, wantCategories) {
				t.Errorf("ByCategory = %v, want %v", got.ByCategory, wantCategories)
			}
			wantPerPackage := countBy(wantFindings, func(f finding) string { return f.ImportPath })
			if !reflect.DeepEqual(got.ByPackage, wantPerPackage) {
				t.Errorf("ByPackage = %v, want %v", got.ByPackage, wantPerPackage)
			}
			if _, parseErr := time.Parse(time.RFC3339, got.GeneratedAt); parseErr != nil {
				t.Errorf("GeneratedAt = %q, want RFC3339: %v", got.GeneratedAt, parseErr)
			}
			if !strings.HasSuffix(out, "}\n") {
				t.Errorf("JSON report does not end with a newline: %q", out)
			}
		})
	}
}

// TestRun_OutputPathWritesReportToFile verifies --output redirects the
// report to a file and leaves stdout untouched.
func TestRun_OutputPathWritesReportToFile(t *testing.T) {
	dir := writeModuleFixture(t)
	t.Chdir(dir)
	path := filepath.Join(dir, "report.md")

	out, err := runForTest([]string{"--output=" + path})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty when --output is set", out)
	}

	data, err := os.ReadFile(path) //#nosec G304 -- test fixture path from t.TempDir.
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	rendered := string(data)
	if !strings.HasPrefix(rendered, "# Godoc Audit Report\n") {
		t.Errorf("report does not start with the markdown title:\n%s", rendered)
	}
	wantRows := []string{
		"| Packages audited | 2 |",
		"| Findings | 3 |",
		"| func_missing | example.com/fixture/a | - | Missing | missing func documentation |",
	}
	for _, want := range wantRows {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(rendered, want) {
				t.Errorf("report missing %q:\n%s", want, rendered)
			}
		})
	}
}

// TestRun_ErrorPaths verifies every way the audit command fails: an
// unparseable flag, a stray positional argument, a directory that is not a
// module, an unwritable output path, and --fail-on-findings on a module that
// has findings.
func TestRun_ErrorPaths(t *testing.T) {
	testCases := []struct {
		name    string
		module  bool
		args    func(dir string) []string
		wantErr string
	}{
		{
			name:    "unknown flag",
			module:  true,
			args:    func(string) []string { return []string{"--nope"} },
			wantErr: "flag provided but not defined: -nope",
		},
		{
			name:    "positional argument",
			module:  true,
			args:    func(string) []string { return []string{"extra", "args"} },
			wantErr: "unexpected positional arguments: extra args",
		},
		{
			name:    "not a module",
			args:    func(string) []string { return nil },
			wantErr: "go list: ",
		},
		{
			name:   "unwritable output path",
			module: true,
			args: func(dir string) []string {
				return []string{"--output=" + filepath.Join(dir, "missing", "report.md")}
			},
			wantErr: "write report: ",
		},
		{
			name:    "fail-on-findings",
			module:  true,
			args:    func(string) []string { return []string{"--fail-on-findings"} },
			wantErr: "godoc audit found 3 issue(s)",
		},
		{
			name:   "unparseable package aborts the walk",
			module: true,
			args: func(dir string) []string {
				writeAuditFile(t, dir, "a/broken.go", "package a\n\nfunc (\n")
				return nil
			},
			wantErr: "parse ",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.module {
				dir = writeModuleFixture(t)
			}
			t.Chdir(dir)

			_, err := runForTest(tc.args(dir))
			if err == nil {
				t.Fatal(errUnexpectedSuccess)
			}
			if !strings.HasPrefix(err.Error(), tc.wantErr) {
				t.Fatalf("run() error = %q, want prefix %q", err, tc.wantErr)
			}
		})
	}
}

// TestAuditPackage_ParseFailures verifies a package directory that cannot be
// read and a source file that cannot be parsed both fail the audit rather
// than being reported as clean.
func TestAuditPackage_ParseFailures(t *testing.T) {
	t.Parallel()

	t.Run("unreadable directory", func(t *testing.T) {
		t.Parallel()
		missing := filepath.Join(t.TempDir(), "missing")
		_, err := auditPackage(packageInfo{Dir: missing, ImportPath: "example.com/x", Name: "x"}, false)
		if err == nil || !strings.HasPrefix(err.Error(), "read package dir ") {
			t.Fatalf("auditPackage() error = %v, want read package dir error", err)
		}
	})

	t.Run("unparseable source", func(t *testing.T) {
		t.Parallel()
		pkg := writePackageFixture(t, "sample", map[string]string{"broken.go": "package sample\n\nfunc (\n"})
		_, err := auditPackage(pkg, false)
		if err == nil || !strings.HasPrefix(err.Error(), "parse ") {
			t.Fatalf("auditPackage() error = %v, want parse error", err)
		}
	})
}

// TestAuditPackage_NoMatchingSourceFiles_ReturnsNoFindings verifies a
// directory whose Go files all belong to another package produces no
// findings at all, not a missing-package-comment finding.
func TestAuditPackage_NoMatchingSourceFiles_ReturnsNoFindings(t *testing.T) {
	t.Parallel()

	pkg := writePackageFixture(t, "sample", map[string]string{"other.go": "package other\n"})
	findings, err := auditPackage(pkg, false)
	if err != nil {
		t.Fatalf("auditPackage() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("auditPackage() = %#v, want no findings", findings)
	}
}

// TestCheckTestFunctionDoc_FormAndOutputRules verifies the per-function test
// documentation rules: the comment must start with the function name for
// every test kind, an Example needs an Output or Unordered output line, and
// plain helpers and methods in a test file are not audited at all.
func TestCheckTestFunctionDoc_FormAndOutputRules(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		source     string
		categories []string
	}{
		{
			name:       "test comment with the wrong prefix",
			source:     "// Checks the widget.\nfunc TestWidget(t *testing.T) {}\n",
			categories: []string{categoryTestForm},
		},
		{
			name:       "benchmark comment with the wrong prefix",
			source:     "// Measures the widget.\nfunc BenchmarkWidget(b *testing.B) {}\n",
			categories: []string{categoryBenchmarkForm},
		},
		{
			name:       "fuzz comment with the wrong prefix",
			source:     "// Fuzzes the widget.\nfunc FuzzWidget(f *testing.F) {}\n",
			categories: []string{categoryFuzzForm},
		},
		{
			name:       "example comment with the wrong prefix and no output",
			source:     "// Shows the widget.\nfunc ExampleWidget() {}\n",
			categories: []string{categoryExampleForm, categoryExampleOutput},
		},
		{
			name:       "example missing only its output line",
			source:     "// ExampleWidget shows the widget.\nfunc ExampleWidget() {}\n",
			categories: []string{categoryExampleOutput},
		},
		{
			name:       "example with an Output line",
			source:     "// ExampleWidget shows the widget.\n//\n// Output:\n// widget\nfunc ExampleWidget() {}\n",
			categories: nil,
		},
		{
			name:       "example with an Unordered output line",
			source:     "// ExampleWidget shows the widget.\n//\n// Unordered output:\n// widget\nfunc ExampleWidget() {}\n",
			categories: nil,
		},
		{
			name:       "TestMain is audited like a test",
			source:     "func TestMain(m *testing.M) {}\n",
			categories: []string{categoryTestMissing},
		},
		{
			name:       "helpers and methods are not audited",
			source:     "func helper() {}\n\ntype widget struct{}\n\nfunc (widget) Run() {}\n",
			categories: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pkg := writePackageFixture(t, "sample", map[string]string{
				"doc.go":         "// Package sample provides a fixture.\npackage sample\n",
				"sample.go":      "package sample\n",
				"sample_test.go": "package sample\n\n" + tc.source,
			})

			findings, err := auditPackage(pkg, true)
			if err != nil {
				t.Fatalf("auditPackage() error = %v", err)
			}
			got := []string{}
			for _, f := range findings {
				got = append(got, f.Category)
			}
			sort.Strings(got)
			want := append([]string{}, tc.categories...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("categories = %v, want %v (findings %+v)", got, want, findings)
			}
		})
	}
}

// TestCheckValueDoc_GroupedNamesAndUnexportedValues verifies that a grouped
// declaration carrying several exported names is exempt from the
// starts-with-the-name rule and that an unexported value is not audited.
func TestCheckValueDoc_GroupedNamesAndUnexportedValues(t *testing.T) {
	t.Parallel()

	pkg := writePackageFixture(t, "sample", map[string]string{
		"doc.go": "// Package sample provides a fixture.\npackage sample\n",
		"sample.go": `package sample

const (
	Alpha = "a"
	Beta  = "b"
)

var unexported = "x"
`,
	})

	findings, err := auditPackage(pkg, false)
	if err != nil {
		t.Fatalf("auditPackage() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("auditPackage() = %#v, want no findings", findings)
	}
}

// TestRenderReport_MarkdownJSONAndUnsupportedFormat verifies the renderer
// emits the exact markdown document, a newline-terminated JSON document that
// round-trips, and an error for any other format.
func TestRenderReport_MarkdownJSONAndUnsupportedFormat(t *testing.T) {
	t.Parallel()

	sample := report{
		GeneratedAt:  "2026-01-02T03:04:05Z",
		IncludeTests: true,
		Packages:     2,
		Findings: []finding{
			{Category: categoryFuncMissing, ImportPath: "example.com/a", Package: "a", File: "a.go", Name: "Foo", Detail: "missing func documentation"},
			{Category: categoryFuncMissing, ImportPath: "example.com/a", Package: "a", Name: "Bar", Detail: "missing func documentation"},
			{Category: categoryTypeForm, ImportPath: "example.com/b", Package: "b", File: "b.go", Name: "Baz", Detail: `type comment must start with "Baz"; got "A | pipe"`},
		},
		ByCategory: map[string]int{categoryFuncMissing: 2, categoryTypeForm: 1},
		ByPackage:  map[string]int{"example.com/a": 2, "example.com/b": 1},
	}

	t.Run("markdown", func(t *testing.T) {
		t.Parallel()
		got, err := renderReport(sample, formatMarkdown)
		if err != nil {
			t.Fatalf("renderReport() error = %v", err)
		}
		want := "# Godoc Audit Report\n\n" +
			"## Summary\n\n" +
			"| Metric | Value |\n| --- | ---: |\n" +
			"| Packages audited | 2 |\n" +
			"| Findings | 3 |\n" +
			"| Include test functions | true |\n\n" +
			"## Findings By Category\n\n" +
			"| Name | Count |\n| --- | ---: |\n" +
			"| func_missing | 2 |\n" +
			"| type_form | 1 |\n\n" +
			"## Top Packages\n\n" +
			"| Name | Count |\n| --- | ---: |\n" +
			"| example.com/a | 2 |\n" +
			"| example.com/b | 1 |\n\n" +
			"## Findings\n\n" +
			"| Category | Package | File | Name | Detail |\n" +
			"| --- | --- | --- | --- | --- |\n" +
			"| func_missing | example.com/a | a.go | Foo | missing func documentation |\n" +
			"| func_missing | example.com/a | - | Bar | missing func documentation |\n" +
			"| type_form | example.com/b | b.go | Baz | type comment must start with \"Baz\"; got \"A \\| pipe\" |\n"
		if string(got) != want {
			t.Fatalf("renderReport(markdown) =\n%s\nwant\n%s", got, want)
		}
	})

	t.Run("json", func(t *testing.T) {
		t.Parallel()
		got, err := renderReport(sample, formatJSON)
		if err != nil {
			t.Fatalf("renderReport() error = %v", err)
		}
		var round report
		if unmarshalErr := json.Unmarshal(got, &round); unmarshalErr != nil {
			t.Fatalf("decode: %v", unmarshalErr)
		}
		if !reflect.DeepEqual(round, sample) {
			t.Errorf("round-tripped report = %+v, want %+v", round, sample)
		}
		if !bytes.HasSuffix(got, []byte("}\n")) {
			t.Errorf("JSON report does not end with a newline: %q", got)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		t.Parallel()
		if _, err := renderReport(sample, "xml"); err == nil || !strings.Contains(err.Error(), `unsupported format "xml"`) {
			t.Fatalf("renderReport(xml) error = %v, want unsupported format", err)
		}
	})
}

// TestRenderMarkdown_EmptyReportStatesNoFindings verifies a clean audit
// renders the empty-table and no-findings wording instead of headers with no
// rows beneath them.
func TestRenderMarkdown_EmptyReportStatesNoFindings(t *testing.T) {
	t.Parallel()

	got := renderMarkdown(report{GeneratedAt: "2026-01-02T03:04:05Z", Packages: 7})
	want := "# Godoc Audit Report\n\n" +
		"## Summary\n\n" +
		"| Metric | Value |\n| --- | ---: |\n" +
		"| Packages audited | 7 |\n" +
		"| Findings | 0 |\n" +
		"| Include test functions | false |\n\n" +
		"## Findings By Category\n\nNo entries.\n\n" +
		"## Top Packages\n\nNo entries.\n\n" +
		"## Findings\n\nNo findings.\n"
	if got != want {
		t.Fatalf("renderMarkdown() =\n%s\nwant\n%s", got, want)
	}
}

// TestWriteCountTable_SortsByCountAndTruncatesToLimit verifies rows are
// ordered by descending count then by name, and that a positive limit keeps
// only the leading rows.
func TestWriteCountTable_SortsByCountAndTruncatesToLimit(t *testing.T) {
	t.Parallel()

	counts := map[string]int{"b": 1, "a": 1, "c": 5}
	testCases := []struct {
		name  string
		limit int
		want  string
	}{
		{
			name:  "unlimited",
			limit: 0,
			want:  "## Title\n\n| Name | Count |\n| --- | ---: |\n| c | 5 |\n| a | 1 |\n| b | 1 |\n\n",
		},
		{
			name:  "limited",
			limit: 2,
			want:  "## Title\n\n| Name | Count |\n| --- | ---: |\n| c | 5 |\n| a | 1 |\n\n",
		},
		{
			name:  "limit above the row count keeps every row",
			limit: 25,
			want:  "## Title\n\n| Name | Count |\n| --- | ---: |\n| c | 5 |\n| a | 1 |\n| b | 1 |\n\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			writeCountTable(&b, "## Title", counts, tc.limit)
			if b.String() != tc.want {
				t.Fatalf("writeCountTable() = %q, want %q", b.String(), tc.want)
			}
		})
	}
}

// TestMd_EscapesPipesAndBlanks verifies the Markdown cell formatter renders
// an empty value as a dash and escapes table separators.
func TestMd_EscapesPipesAndBlanks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: "-"},
		{name: "plain", value: "text", want: "text"},
		{name: "pipe", value: "a|b|c", want: "a\\|b\\|c"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := md(tc.value); got != tc.want {
				t.Errorf("md(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestSortFindings_OrdersByPathFileCategoryName verifies the report's stable
// ordering, exercising each tie-break in turn.
func TestSortFindings_OrdersByPathFileCategoryName(t *testing.T) {
	t.Parallel()

	findings := []finding{
		{ImportPath: "b", File: "a.go", Category: "func_missing", Name: "A"},
		{ImportPath: "a", File: "b.go", Category: "func_missing", Name: "A"},
		{ImportPath: "a", File: "a.go", Category: "type_missing", Name: "A"},
		{ImportPath: "a", File: "a.go", Category: "func_missing", Name: "B"},
		{ImportPath: "a", File: "a.go", Category: "func_missing", Name: "A"},
	}
	sortFindings(findings)
	want := []finding{
		{ImportPath: "a", File: "a.go", Category: "func_missing", Name: "A"},
		{ImportPath: "a", File: "a.go", Category: "func_missing", Name: "B"},
		{ImportPath: "a", File: "a.go", Category: "type_missing", Name: "A"},
		{ImportPath: "a", File: "b.go", Category: "func_missing", Name: "A"},
		{ImportPath: "b", File: "a.go", Category: "func_missing", Name: "A"},
	}
	if !reflect.DeepEqual(findings, want) {
		t.Fatalf("sortFindings() = %+v\nwant %+v", findings, want)
	}
}

// TestCountBy_TalliesByKey verifies the aggregation helper counts findings
// under the key the caller extracts, and returns an empty map for none.
func TestCountBy_TalliesByKey(t *testing.T) {
	t.Parallel()

	category := func(f finding) string { return f.Category }
	findings := []finding{
		{Category: "a", ImportPath: "p"},
		{Category: "a", ImportPath: "q"},
		{Category: "b", ImportPath: "p"},
	}
	if got, want := countBy(findings, category), (map[string]int{"a": 2, "b": 1}); !reflect.DeepEqual(got, want) {
		t.Errorf("countBy() = %v, want %v", got, want)
	}
	if got, want := countBy(nil, category), (map[string]int{}); !reflect.DeepEqual(got, want) {
		t.Errorf("countBy(nil) = %v, want %v", got, want)
	}
}

// TestRelativePath_CleansRelativePathsAndKeepsAbsoluteOnes verifies the
// report's path column: a relative path is cleaned and slash-separated,
// an empty path stays empty, and an absolute path is kept as-is, because
// filepath.Rel refuses to relate an absolute path to the relative base "."
// the helper passes it.
func TestRelativePath_CleansRelativePathsAndKeepsAbsoluteOnes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		want string
	}{
		{name: "empty", path: "", want: ""},
		{name: "relative", path: filepath.FromSlash("pkg/./file.go"), want: "pkg/file.go"},
		{name: "dot-prefixed relative", path: filepath.FromSlash("./pkg/file.go"), want: "pkg/file.go"},
		{name: "absolute", path: filepath.FromSlash("/tmp/elsewhere/file.go"), want: "/tmp/elsewhere/file.go"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := relativePath(tc.path); got != tc.want {
				t.Errorf("relativePath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestGoExecutable_PointsAtTheRunningToolchain verifies the resolved Go tool
// path is an existing file inside the active GOROOT, which is what keeps the
// audit off the PATH.
func TestGoExecutable_PointsAtTheRunningToolchain(t *testing.T) {
	t.Parallel()

	got := goExecutable()
	if !filepath.IsAbs(got) {
		t.Fatalf("goExecutable() = %q, want an absolute path", got)
	}
	if want := filepath.Join(runtime.GOROOT(), "bin"); filepath.Dir(got) != want { //nolint:staticcheck // The audited helper resolves the same GOROOT.
		t.Fatalf("goExecutable() = %q, want it under %q", got, want)
	}
	info, err := os.Stat(got)
	if err != nil || info.IsDir() {
		t.Fatalf("goExecutable() = %q, want an existing file (stat error %v)", got, err)
	}
}

// TestGoExecutable_WindowsGOOS_AppendsExeSuffix verifies the Windows branch of
// goExecutable, which appends the .exe suffix. It drives the runtimeGOOS seam
// because the tests run on a non-Windows host that never takes the branch on
// its own. The test is serial so its global override never overlaps the
// parallel TestGoExecutable_PointsAtTheRunningToolchain.
func TestGoExecutable_WindowsGOOS_AppendsExeSuffix(t *testing.T) {
	original := runtimeGOOS
	runtimeGOOS = "windows"
	t.Cleanup(func() { runtimeGOOS = original })

	if got := filepath.Base(goExecutable()); got != "go.exe" {
		t.Fatalf("goExecutable() base = %q, want go.exe", got)
	}
}

// TestListPackages_MalformedGoListRows verifies the three rows a real
// toolchain never emits: a blank interior line is skipped, and a row missing
// either tab is rejected. It drives the goListOutput seam to feed listPackages
// output the toolchain would never produce.
func TestListPackages_MalformedGoListRows(t *testing.T) {
	testCases := []struct {
		name    string
		output  string
		wantErr string
		wantLen int
	}{
		{name: "blank interior line is skipped", output: "d\tp\tn\n\nd2\tp2\tn2\n", wantLen: 2},
		{name: "row without a tab is rejected", output: "no-tabs-here\n", wantErr: "unexpected go list row"},
		{name: "row with a single tab is rejected", output: "dir\tonly-one-field\n", wantErr: "unexpected go list row"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			original := goListOutput
			goListOutput = func(context.Context) ([]byte, error) { return []byte(tc.output), nil }
			t.Cleanup(func() { goListOutput = original })

			pkgs, err := listPackages()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("listPackages() error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("listPackages() error = %v", err)
			}
			if len(pkgs) != tc.wantLen {
				t.Errorf("listPackages() len = %d, want %d", len(pkgs), tc.wantLen)
			}
		})
	}
}

// TestAuditPackage_DocBuildError_IsReported verifies auditPackage surfaces a
// doc.NewFromFiles failure rather than reporting a clean package. The failure
// is unreachable with real input (NewFromFiles only errors on an invalid
// option type, which the audit never passes), so it drives the newDocFromFiles
// seam.
func TestAuditPackage_DocBuildError_IsReported(t *testing.T) {
	original := newDocFromFiles
	newDocFromFiles = func(*token.FileSet, []*ast.File, string, ...any) (*doc.Package, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { newDocFromFiles = original })

	pkg := writePackageFixture(t, "sample", map[string]string{
		"doc.go":    "// Package sample provides a fixture.\npackage sample\n",
		"sample.go": "package sample\n\nfunc Missing() {}\n",
	})
	if _, err := auditPackage(pkg, false); err == nil || !strings.HasPrefix(err.Error(), "build doc package ") {
		t.Fatalf("auditPackage() error = %v, want build doc package error", err)
	}
}

// TestRun_JSONMarshalError_IsReported verifies run reports a JSON marshal
// failure from renderReport rather than writing a truncated report. A real
// report always marshals, so it drives the marshalIndent seam, and empties the
// package list through the goListOutput seam so run reaches the render step.
func TestRun_JSONMarshalError_IsReported(t *testing.T) {
	origList := goListOutput
	goListOutput = func(context.Context) ([]byte, error) { return []byte(""), nil }
	t.Cleanup(func() { goListOutput = origList })
	origMarshal := marshalIndent
	marshalIndent = func(any, string, string) ([]byte, error) { return nil, errors.New("boom") }
	t.Cleanup(func() { marshalIndent = origMarshal })

	if _, err := runForTest([]string{"--format=json"}); err == nil || !strings.HasPrefix(err.Error(), "marshal json") {
		t.Fatalf("run() error = %v, want marshal json error", err)
	}
}

// errWriter is an io.Writer whose Write always fails, used to exercise the
// report's stdout write-error branch.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

// TestRun_StdoutWriteError_IsReported verifies run reports a failure to write
// the report to stdout. It feeds an empty package list through the goListOutput
// seam so run reaches the write, and a writer that always fails as the
// destination.
func TestRun_StdoutWriteError_IsReported(t *testing.T) {
	original := goListOutput
	goListOutput = func(context.Context) ([]byte, error) { return []byte(""), nil }
	t.Cleanup(func() { goListOutput = original })

	if err := run(nil, errWriter{}); err == nil || !strings.HasPrefix(err.Error(), "write stdout") {
		t.Fatalf("run() error = %v, want write stdout error", err)
	}
}

// TestCheckValueDoc_UnexportedAndSingleValidDoc verifies the two remaining
// value-doc paths: a group whose names are all unexported is not audited at
// all, and a single exported name whose comment already starts with it yields
// no finding. go/doc filters unexported names before the audit sees them, so
// the helper is exercised directly.
func TestCheckValueDoc_UnexportedAndSingleValidDoc(t *testing.T) {
	t.Parallel()

	pkg := packageInfo{ImportPath: "example.com/x", Name: "x"}
	testCases := []struct {
		name  string
		names []string
		doc   string
	}{
		{name: "all names unexported are skipped", names: []string{"lower", "alsoLower"}, doc: ""},
		{name: "single valid comment yields no finding", names: []string{"Answer"}, doc: "Answer is the answer."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var findings []finding
			checkValueDoc(pkg, categoryConstMissing, categoryConstForm, "const", tc.names, tc.doc, &findings)
			if len(findings) != 0 {
				t.Fatalf("checkValueDoc() findings = %+v, want none", findings)
			}
		})
	}
}

// TestAuditPackage_TypeAssociatedSymbolsAreChecked verifies the audit walks
// the symbols go/doc groups under a type: a constructor function, a typed
// constant, and a typed variable. Each carries a valid comment, so the audit
// reports nothing while the type-associated loop bodies still run.
func TestAuditPackage_TypeAssociatedSymbolsAreChecked(t *testing.T) {
	t.Parallel()

	pkg := writePackageFixture(t, "sample", map[string]string{
		"doc.go": "// Package sample provides a fixture.\npackage sample\n",
		"sample.go": `package sample

// Widget builds things.
type Widget struct{}

// NewWidget builds a Widget.
func NewWidget() Widget { return Widget{} }

// Mode selects behavior.
type Mode int

// ModeOn enables the mode.
const ModeOn Mode = 1

// Registry holds widgets.
type Registry struct{}

// DefaultRegistry is the default registry.
var DefaultRegistry Registry
`,
	})

	findings, err := auditPackage(pkg, false)
	if err != nil {
		t.Fatalf("auditPackage() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("auditPackage() = %#v, want no findings", findings)
	}
}
