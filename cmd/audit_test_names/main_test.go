package main

import (
	"os"
	"path/filepath"
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
