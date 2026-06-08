package main

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
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
		if records[0][i] != col {
			t.Errorf("CSV header[%d] = %q, want %q", i, records[0][i], col)
		}
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

func readCSVRecords(t *testing.T, data []byte) [][]string {
	t.Helper()
	r := csv.NewReader(bytes.NewReader(data))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV read error: %v", err)
	}
	return records
}
