// main_test.go covers the audit_test_names command's classification rules
// and CSV/stderr output contract.
//
// Tests use table-driven cases for the naming heuristics and exercise the
// scanner with temporary Go sources to verify subtest filtering, hidden
// helpers, Benchmark/TestMain exclusion, and the legacy section marks.
package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestClassify_NamingPatterns verifies classify identifies supported test naming patterns and suggestions.
func TestClassify_NamingPatterns(t *testing.T) {
	testCases := []struct {
		name          string
		input         string
		wantPattern   string
		wantSuggested string
	}{
		{name: "three part", input: "TestCreateIssue_ValidInput_ReturnsIssue", wantPattern: Pattern3Part, wantSuggested: "TestCreateIssue_ValidInput_ReturnsIssue"},
		{name: "two part", input: "TestCreateIssue_ReturnsIssue", wantPattern: Pattern2Part, wantSuggested: "TestCreateIssue_ReturnsIssue"},
		{name: "no underscore", input: "TestCreateIssueReturnsIssue", wantPattern: PatternNoUnderscore, wantSuggested: "TestCreate_IssueReturnsIssue"},
		{name: "coverage prefix", input: "TestCovBuildCatalogError", wantPattern: PatternTestCov, wantSuggested: "TestBuild_Catalog_Error"},
		{name: "e2e full workflow", input: "TestFullWorkflow", wantPattern: PatternSkip, wantSuggested: "TestFullWorkflow"},
		{name: "e2e meta workflow", input: "TestMetaToolWorkflow", wantPattern: PatternSkip, wantSuggested: "TestMetaToolWorkflow"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gotPattern, gotSuggested := classify(testCase.input)
			if gotPattern != testCase.wantPattern || gotSuggested != testCase.wantSuggested {
				t.Fatalf("classify(%q) = %q, %q; want %q, %q", testCase.input, gotPattern, gotSuggested, testCase.wantPattern, testCase.wantSuggested)
			}
		})
	}
}

// TestSplitCamelCase_HandlesAcronymsAndShortNames verifies CamelCase splitting preserves meaningful segments.
func TestSplitCamelCase_HandlesAcronymsAndShortNames(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "non test", input: "CreateIssue", want: "CreateIssue"},
		{name: "empty test", input: "Test", want: "Test"},
		{name: "acronym boundary", input: "TestHTTPHandlerReturnsError", want: "TestHTTP_HandlerReturns_Error"},
		{name: "no result suffix", input: "TestBuildCatalogFromSpecs", want: "TestBuild_CatalogFromSpecs"},
		{name: "two words unchanged", input: "TestCatalog", want: "TestCatalog"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := splitCamelCase(testCase.input); got != testCase.want {
				t.Fatalf("splitCamelCase(%q) = %q, want %q", testCase.input, got, testCase.want)
			}
		})
	}
}

// TestRenameCov_TransformsCoveragePrefix verifies coverage-style names are converted to conventional test names.
func TestRenameCov_TransformsCoveragePrefix(t *testing.T) {
	if got := renameCov("TestCovBuildCatalogError"); got != "TestBuild_Catalog_Error" {
		t.Fatalf("renameCov() = %q, want TestBuild_Catalog_Error", got)
	}
	if got := renameCov("TestCov"); got != "TestCov" {
		t.Fatalf("renameCov(TestCov) = %q, want unchanged", got)
	}
}

// TestMergeIntoSegments_GroupsResultWords verifies CamelCase words are grouped into function, scenario, and expected segments.
func TestMergeIntoSegments_GroupsResultWords(t *testing.T) {
	testCases := []struct {
		name  string
		words []string
		want  string
	}{
		{name: "two words", words: []string{"Build", "Catalog"}, want: "Build_Catalog"},
		{name: "result suffix", words: []string{"Build", "Catalog", "Error"}, want: "Build_Catalog_Error"},
		{name: "scenario only", words: []string{"Build", "Catalog", "From", "Specs"}, want: "Build_CatalogFromSpecs"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := mergeIntoSegments(testCase.words); got != testCase.want {
				t.Fatalf("mergeIntoSegments(%v) = %q, want %q", testCase.words, got, testCase.want)
			}
		})
	}
}

// TestScanDir_RecursesAndClassifiesTestFunctions verifies scanDir reads nested test files and skips non-test helpers.
func TestScanDir_RecursesAndClassifiesTestFunctions(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	fixture := `package sample

import "testing"

func TestCreateIssueReturnsIssue(t *testing.T) {}
func TestCreateIssue_ReturnsIssue(t *testing.T) {}
func TestCovBuildCatalogError(t *testing.T) {}
func TestMain(m *testing.M) {}
func Testhelper(t *testing.T) {}
func BenchmarkCreateIssue(b *testing.B) {}
`
	if err := os.WriteFile(filepath.Join(nested, "sample_test.go"), []byte(fixture), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "sample.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(non-test) error = %v", err)
	}

	entries := scanDir(root)
	if len(entries) != 3 {
		t.Fatalf("scanDir() len = %d, want 3 entries: %+v", len(entries), entries)
	}
	patterns := map[string]string{}
	for _, entry := range entries {
		patterns[entry.CurrentName] = entry.Pattern
	}
	if patterns["TestCreateIssueReturnsIssue"] != PatternNoUnderscore {
		t.Fatalf("patterns = %+v, want TestCreateIssueReturnsIssue as no-underscore", patterns)
	}
	if patterns["TestCreateIssue_ReturnsIssue"] != Pattern2Part {
		t.Fatalf("patterns = %+v, want TestCreateIssue_ReturnsIssue as 2-part", patterns)
	}
	if patterns["TestCovBuildCatalogError"] != PatternTestCov {
		t.Fatalf("patterns = %+v, want TestCovBuildCatalogError as TestCov", patterns)
	}
}

// TestScanDir_InvalidPathsReturnNoEntries verifies scanner failures are reported without panics.
func TestScanDir_InvalidPathsReturnNoEntries(t *testing.T) {
	root := t.TempDir()
	if entries := scanDir(filepath.Join(root, "missing")); entries != nil {
		t.Fatalf("scanDir(missing) = %+v, want nil", entries)
	}
	if entries := scanFile(filepath.Join(root, "missing_test.go")); entries != nil {
		t.Fatalf("scanFile(missing) = %+v, want nil", entries)
	}
}

// TestRun_WritesCSVAndSummary verifies the run entry point walks the supplied
// directories, emits the expected CSV header/rows to stdout, and prints a
// classification summary to stderr.
//
// The test stages a directory tree containing a mix of compliant and
// non-compliant test names plus a non-test file, then asserts that the CSV
// output contains the expected rows and the stderr summary references the
// audited counts.
func TestRun_WritesCSVAndSummary(t *testing.T) {
	root := t.TempDir()
	fixture := `package sample

import "testing"

func TestCreateIssue_ReturnsIssue(t *testing.T) {}
func TestCovBuildCatalogError(t *testing.T) {}
func TestCreateIssueReturnsIssue(t *testing.T) {}
`
	if err := os.WriteFile(filepath.Join(root, "sample_test.go"), []byte(fixture), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := run([]string{root}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	records := readCSVRecords(t, stdout.Bytes())
	if len(records) == 0 {
		t.Fatalf("run() emitted no CSV records; stderr:\n%s", stderr.String())
	}
	wantHeader := []string{"file", "current_name", "pattern", "suggested_name"}
	if len(records[0]) != len(wantHeader) {
		t.Fatalf("CSV header = %v, want %v", records[0], wantHeader)
	}
	for i, col := range wantHeader {
		t.Run(col, func(t *testing.T) {
			if records[0][i] != col {
				t.Errorf("CSV header[%d] = %q, want %q", i, records[0][i], col)
			}
		})
	}
	patterns := map[string]string{}
	for _, rec := range records[1:] {
		if len(rec) >= 3 {
			patterns[rec[1]] = rec[2]
		}
	}
	if patterns["TestCreateIssue_ReturnsIssue"] != Pattern2Part {
		t.Fatalf("patterns = %+v, want TestCreateIssue_ReturnsIssue as 2-part", patterns)
	}
	if patterns["TestCovBuildCatalogError"] != PatternTestCov {
		t.Fatalf("patterns = %+v, want TestCovBuildCatalogError as TestCov", patterns)
	}
	if patterns["TestCreateIssueReturnsIssue"] != PatternNoUnderscore {
		t.Fatalf("patterns = %+v, want TestCreateIssueReturnsIssue as no-underscore", patterns)
	}

	if !strings.Contains(stderr.String(), "Total test functions: 3") {
		t.Fatalf("stderr = %q, want total count line", stderr.String())
	}
	if !strings.Contains(stderr.String(), Pattern2Part+":") {
		t.Fatalf("stderr = %q, want %s summary", stderr.String(), Pattern2Part)
	}
	if !strings.Contains(stderr.String(), PatternTestCov+":") {
		t.Fatalf("stderr = %q, want %s summary", stderr.String(), PatternTestCov)
	}
}

// TestRun_EmptyInputStillEmitsHeaderAndSummary verifies the run function emits
// the CSV header and a zero-count summary even when given no directories.
//
// The expected stderr reports zero total test functions; the CSV must still
// contain the header row so downstream consumers can parse the output.
func TestRun_EmptyInputStillEmitsHeaderAndSummary(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(nil, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	records := readCSVRecords(t, stdout.Bytes())
	if len(records) != 1 {
		t.Fatalf("run() with no dirs emitted %d records, want 1 (header)", len(records))
	}
	if !strings.Contains(stderr.String(), "Total test functions: 0") {
		t.Fatalf("stderr = %q, want zero-count summary", stderr.String())
	}
}

// TestRun_WriterFailures_ReportWhichStageFailed verifies the CSV writer's
// error is surfaced from the stage that observed it: the final flush for a
// small report, and a row write once the buffered rows exceed the writer's
// buffer.
func TestRun_WriterFailures_ReportWhichStageFailed(t *testing.T) {
	var many strings.Builder
	many.WriteString("package sample\n\nimport \"testing\"\n\n")
	for i := range 80 {
		fmt.Fprintf(&many, "func TestVeryLongFunctionName%02d_Scenario_ReturnsExpected(t *testing.T) {}\n", i)
	}

	testCases := []struct {
		name    string
		fixture string
		wantErr string
	}{
		{name: "small report fails at flush", fixture: "package sample\n\nimport \"testing\"\n\nfunc TestOne_Two(t *testing.T) {}\n", wantErr: "flush csv: boom"},
		{name: "large report fails on a row write", fixture: many.String(), wantErr: "write csv row: boom"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "sample_test.go"), []byte(tc.fixture), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			var stderr bytes.Buffer
			err := run([]string{root}, failingWriter{}, &stderr)
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("run() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// legacyNamesFixture mixes two renamable legacy names with a compliant
// two-part name, a name whose suggestion equals itself, TestMain and a
// lowercase helper, plus a comment that references a renamed function.
const legacyNamesFixture = `package sample

import "testing"

// TestCreateIssueReturnsIssue is referenced here and renamed with the function.
func TestCreateIssueReturnsIssue(t *testing.T) {}
func TestCovBuildCatalogError(t *testing.T) {}
func TestCreateIssue_ReturnsIssue(t *testing.T) {}
func TestCatalog(t *testing.T) {}
func TestMain(m *testing.M) {}
func Testhelper(t *testing.T) {}
`

// legacyNamesRewritten is legacyNamesFixture after -apply: both legacy
// names are replaced wherever they appear, everything else is untouched.
const legacyNamesRewritten = `package sample

import "testing"

// TestCreate_IssueReturnsIssue is referenced here and renamed with the function.
func TestCreate_IssueReturnsIssue(t *testing.T) {}
func TestBuild_Catalog_Error(t *testing.T) {}
func TestCreateIssue_ReturnsIssue(t *testing.T) {}
func TestCatalog(t *testing.T) {}
func TestMain(m *testing.M) {}
func Testhelper(t *testing.T) {}
`

// TestRunApply_DryRunAndApply_RewriteLegacyNames verifies the rename
// workflow end to end: the per-rename stdout lines, the stderr summary
// naming the mode, the file left untouched under -dry-run and rewritten
// exactly under -apply, with nested directories walked and non-Go files
// ignored.
func TestRunApply_DryRunAndApply_RewriteLegacyNames(t *testing.T) {
	testCases := []struct {
		name        string
		dryRun      bool
		wantMode    string
		wantContent string
	}{
		{name: "dry-run", dryRun: true, wantMode: "dry-run", wantContent: legacyNamesFixture},
		{name: "apply", dryRun: false, wantMode: "applied", wantContent: legacyNamesRewritten},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			nested := filepath.Join(root, "nested")
			if err := os.MkdirAll(nested, 0o750); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			path := filepath.Join(nested, "sample_test.go")
			if err := os.WriteFile(path, []byte(legacyNamesFixture), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			if err := os.WriteFile(filepath.Join(nested, "sample.go"), []byte("package sample\n"), 0o600); err != nil {
				t.Fatalf("WriteFile(non-test) error = %v", err)
			}
			if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("func TestCreateIssueReturnsIssue\n"), 0o600); err != nil {
				t.Fatalf("WriteFile(README) error = %v", err)
			}

			var stdout, stderr bytes.Buffer
			if ok := runApply([]string{root}, &stdout, &stderr, tc.dryRun); !ok {
				t.Fatalf("runApply() = false, want true; stderr:\n%s", stderr.String())
			}

			slashed := filepath.ToSlash(path)
			gotLines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
			sort.Strings(gotLines)
			wantLines := []string{
				slashed + ": TestCovBuildCatalogError -> TestBuild_Catalog_Error",
				slashed + ": TestCreateIssueReturnsIssue -> TestCreate_IssueReturnsIssue",
			}
			if !reflect.DeepEqual(gotLines, wantLines) {
				t.Errorf("stdout lines = %q, want %q", gotLines, wantLines)
			}
			if got, want := stderr.String(), "\n=== Rename Summary ("+tc.wantMode+") ===\nFiles scanned: 1\nRenames: 2\n"; got != want {
				t.Errorf("stderr = %q, want %q", got, want)
			}
			if got := readFile(t, path); got != tc.wantContent {
				t.Errorf("file after %s = \n%s\nwant\n%s", tc.name, got, tc.wantContent)
			}
		})
	}
}

// TestRunApply_Failures_ReturnFalse verifies a missing directory and an
// unparseable test file each fail the run while the summary is still
// printed, and a file with nothing to rename is neither a rename nor a
// failure.
func TestRunApply_Failures_ReturnFalse(t *testing.T) {
	testCases := []struct {
		name          string
		files         []fileSpec
		dirs          func(root string) []string
		wantOK        bool
		wantStderrPre func(root string) string
		wantSummary   string
	}{
		{
			name:          "missing directory",
			dirs:          func(root string) []string { return []string{filepath.Join(root, "absent")} },
			wantStderrPre: func(root string) string { return "readdir " + filepath.Join(root, "absent") + ": " },
			wantSummary:   "\n=== Rename Summary (applied) ===\nFiles scanned: 0\nRenames: 0\n",
		},
		{
			name:          "unparseable test file",
			files:         []fileSpec{{"broken_test.go", "package sample\n\nfunc (\n"}},
			wantStderrPre: func(root string) string { return "parse " + filepath.Join(root, "broken_test.go") + ": " },
			wantSummary:   "\n=== Rename Summary (applied) ===\nFiles scanned: 1\nRenames: 0\n",
		},
		{
			name:          "compliant file is untouched",
			files:         []fileSpec{{"clean_test.go", "package sample\n\nimport \"testing\"\n\nfunc TestOne_Two(t *testing.T) {}\n"}},
			wantOK:        true,
			wantStderrPre: func(string) string { return "" },
			wantSummary:   "\n=== Rename Summary (applied) ===\nFiles scanned: 1\nRenames: 0\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFixtureDir(t, root, nil, tc.files)
			dirs := []string{root}
			if tc.dirs != nil {
				dirs = tc.dirs(root)
			}

			var stdout, stderr bytes.Buffer
			ok := runApply(dirs, &stdout, &stderr, false)
			if ok != tc.wantOK {
				t.Errorf("runApply() = %t, want %t", ok, tc.wantOK)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			prefix := tc.wantStderrPre(root)
			if got := stderr.String(); !strings.HasPrefix(got, prefix) || !strings.HasSuffix(got, tc.wantSummary) {
				t.Errorf("stderr = %q, want prefix %q and suffix %q", got, prefix, tc.wantSummary)
			}
		})
	}
}

// TestCollectRenames_SkipsCollisionsAndReservesTargets verifies a legacy
// name whose suggestion already exists is skipped with a message, a target
// claimed by an earlier rename is not claimed twice, and names that are
// compliant, self-suggesting, TestMain, lowercase helpers or not functions
// are left alone.
func TestCollectRenames_SkipsCollisionsAndReservesTargets(t *testing.T) {
	source := `package sample

import "testing"

var fixture = 1

func TestFooBar(t *testing.T) {}
func TestFoo_Bar(t *testing.T) {}
func TestCovAlphaBeta(t *testing.T) {}
func TestAlphaBeta(t *testing.T) {}
func TestCatalog(t *testing.T) {}
func TestMain(m *testing.M) {}
func Testhelper(t *testing.T) {}
`
	node, err := parser.ParseFile(token.NewFileSet(), "sample_test.go", source, 0)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	var stderr bytes.Buffer
	got := collectRenames(node, "sample_test.go", &stderr)

	want := map[string]string{"TestCovAlphaBeta": "TestAlpha_Beta"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collectRenames() = %v, want %v", got, want)
	}
	wantStderr := "  skip TestFooBar -> TestFoo_Bar in sample_test.go: target name already exists\n" +
		"  skip TestAlphaBeta -> TestAlpha_Beta in sample_test.go: target name already exists\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr = %q, want %q", stderr.String(), wantStderr)
	}
}

// failingWriter fails every write so the CSV stage that observes the
// failure can be asserted.
type failingWriter struct{}

// Write always fails with errBoom.
func (failingWriter) Write([]byte) (int, error) { return 0, errBoom }

var errBoom = errors.New("boom")

// readFile returns the content of path or fails the test.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //#nosec G304 -- test fixture path from t.TempDir.
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}

func readCSVRecords(t *testing.T, data []byte) [][]string {
	t.Helper()
	r := csv.NewReader(bytes.NewReader(data))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV read error: %v", err)
	}
	return records
}
