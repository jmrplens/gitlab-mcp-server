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
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
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

// TestClassify_EverySuggestion_IsANonEmptyName verifies the property the
// rename loop relies on rather than guards: classify answers every name it is
// given with a name. collectRenames writes the suggestion straight into a
// declaration, so an empty one would blank a function, and it carries no check
// for that because none of classify's branches can return one: the rewrites
// prepend "Test" and the rest fall back to the name itself.
func TestClassify_EverySuggestion_IsANonEmptyName(t *testing.T) {
	names := []string{
		"Test", "TestCov", "TestCatalog", "TestFooBar", "TestFoo_Bar",
		"TestFoo_Bar_Baz", "TestCovAlphaBeta", "TestCovX", "TestHTTPHandlerReturnsError",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			if _, suggested := classify(name); suggested == "" {
				t.Errorf("classify(%q) suggested an empty name", name)
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
		// The last rune being the uppercase one is the case where the word
		// boundary has to be decided with no following rune to look at.
		{name: "trailing uppercase", input: "TestFooB", want: "TestFoo_B"},
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

// TestScanDir_RecursesAndClassifiesTestFunctions verifies scanDir reads nested
// test files and skips non-test helpers. TestMain_Flags_Parse is in the fixture
// on purpose: the framework entry point is exactly TestMain, and a name that
// merely starts with those letters is an ordinary test, which is what both
// generators have always counted and this auditor used to skip.
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
func TestMain_Flags_Parse(t *testing.T) {}
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

	// The whole entry is compared rather than its pattern alone: the file and
	// the suggested name are assignments with no branch of their own, so a
	// converter that read either from the wrong neighbor would be reported by
	// neither gate and by no assertion that looks at one field.
	file := filepath.ToSlash(filepath.Join(nested, "sample_test.go"))
	want := []testEntry{
		{File: file, CurrentName: "TestCreateIssueReturnsIssue", Pattern: PatternNoUnderscore, SuggestedName: "TestCreate_IssueReturnsIssue"},
		{File: file, CurrentName: "TestCreateIssue_ReturnsIssue", Pattern: Pattern2Part, SuggestedName: "TestCreateIssue_ReturnsIssue"},
		{File: file, CurrentName: "TestCovBuildCatalogError", Pattern: PatternTestCov, SuggestedName: "TestBuild_Catalog_Error"},
		{File: file, CurrentName: "TestMain_Flags_Parse", Pattern: Pattern3Part, SuggestedName: "TestMain_Flags_Parse"},
	}
	if got := scanDir(root); !reflect.DeepEqual(got, want) {
		t.Fatalf("scanDir() = %+v\nwant %+v", got, want)
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

// summaryFixture holds one to four test functions of each bucket, so that no
// two counts in the stderr summary agree and a count printed under the wrong
// label cannot pass for the right one.
const summaryFixture = `package sample

import "testing"

func TestAlpha_Beta_Gamma(t *testing.T) {}
func TestCreateIssue_ReturnsIssue(t *testing.T) {}
func TestDelta_Epsilon(t *testing.T) {}
func TestCovBuildCatalogError(t *testing.T) {}
func TestCovMuNu(t *testing.T) {}
func TestCovXiOmicron(t *testing.T) {}
func TestCreateIssueReturnsIssue(t *testing.T) {}
func TestZetaEta(t *testing.T) {}
func TestThetaIota(t *testing.T) {}
func TestKappaLambda(t *testing.T) {}
`

// TestRun_WritesCSVAndSummary verifies the run entry point walks the supplied
// directories, emits the whole CSV report to stdout, and prints a
// classification summary to stderr.
//
// Every column of every row is compared, and the summary is read back as
// counts: the file and the suggested name are written by plain assignments and
// each count by one Fprintf, so a column filled from the wrong field and a
// count printed under the wrong label are defects no mutation and no condition
// can report, and were invisible here until this compared the whole report.
func TestRun_WritesCSVAndSummary(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample_test.go"), []byte(summaryFixture), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := run([]string{root}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	file := filepath.ToSlash(filepath.Join(root, "sample_test.go"))
	wantRecords := [][]string{
		{"file", "current_name", "pattern", "suggested_name"},
		{file, "TestAlpha_Beta_Gamma", Pattern3Part, "TestAlpha_Beta_Gamma"},
		{file, "TestCreateIssue_ReturnsIssue", Pattern2Part, "TestCreateIssue_ReturnsIssue"},
		{file, "TestDelta_Epsilon", Pattern2Part, "TestDelta_Epsilon"},
		{file, "TestCovBuildCatalogError", PatternTestCov, "TestBuild_Catalog_Error"},
		{file, "TestCovMuNu", PatternTestCov, "TestMu_Nu"},
		{file, "TestCovXiOmicron", PatternTestCov, "TestXi_Omicron"},
		{file, "TestCreateIssueReturnsIssue", PatternNoUnderscore, "TestCreate_IssueReturnsIssue"},
		{file, "TestZetaEta", PatternNoUnderscore, "TestZeta_Eta"},
		{file, "TestThetaIota", PatternNoUnderscore, "TestTheta_Iota"},
		{file, "TestKappaLambda", PatternNoUnderscore, "TestKappa_Lambda"},
	}
	if got := readCSVRecords(t, stdout.Bytes()); !reflect.DeepEqual(got, wantRecords) {
		t.Errorf("CSV = %q\nwant %q", got, wantRecords)
	}

	// The map is compared whole, so a bucket nothing classified into ("other"
	// and "skip", which no classification produces) is asserted absent too.
	wantCounts := map[string]int{
		"Total test functions": 10,
		Pattern3Part:           1,
		Pattern2Part:           2,
		PatternTestCov:         3,
		PatternNoUnderscore:    4,
	}
	if got := summaryCounts(stderr.String()); !reflect.DeepEqual(got, wantCounts) {
		t.Errorf("summary counts = %v, want %v\nstderr:\n%s", got, wantCounts, stderr.String())
	}
}

// summaryCounts reads the numbers back out of the audit's stderr summary, so
// the counts can be asserted without pinning the column padding they are
// printed with.
func summaryCounts(stderr string) map[string]int {
	counts := map[string]int{}
	for line := range strings.SplitSeq(stderr, "\n") {
		label, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if !found {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			continue
		}
		counts[label] = n
	}
	return counts
}

// TestRunMain_Flags_SelectTheModeAndTheExitCode verifies the entry point's own
// work: which of the three modes each flag selects, the usage line when the
// command line names no directory, and the code each outcome exits with. main
// is one line over this function, so nothing else can observe the dispatch,
// and a -dry-run given on its own has to reach the rename mode, which is what
// tells the two flags apart from a pair that must both be set.
func TestRunMain_Flags_SelectTheModeAndTheExitCode(t *testing.T) {
	const cleanTest = "package sample\n\nimport \"testing\"\n\nfunc TestOne_Two(t *testing.T) {}\n"
	testCases := []struct {
		name       string
		files      []fileSpec
		args       func(root string) []string
		failStdout bool
		wantCode   int
		wantStdout []string
		notStdout  []string
		wantStderr []string
		wantFile   string
	}{
		{
			name:       "no directory named",
			args:       func(string) []string { return nil },
			wantCode:   1,
			wantStderr: []string{"usage: go run ./cmd/audit_test_names/ [flags] <dir>..."},
		},
		{
			name:       "a flag the command does not define",
			args:       func(root string) []string { return []string{"-nope", root} },
			wantCode:   2,
			wantStderr: []string{"flag provided but not defined", "-check-files"},
		},
		{
			name:       "help is not a failure",
			args:       func(string) []string { return []string{"-h"} },
			wantCode:   0,
			wantStderr: []string{"-apply", "-dry-run", "-check-files"},
		},
		{
			name:       "no mode flag reports",
			files:      []fileSpec{{"sample_test.go", legacyNamesFixture}},
			args:       func(root string) []string { return []string{root} },
			wantCode:   0,
			wantStdout: []string{"current_name", "TestCovBuildCatalogError"},
			wantFile:   legacyNamesFixture,
		},
		{
			name:       "a report that cannot be written",
			files:      []fileSpec{{"sample_test.go", legacyNamesFixture}},
			args:       func(root string) []string { return []string{root} },
			failStdout: true,
			wantCode:   1,
			wantStderr: []string{"flush csv: boom"},
		},
		{
			name:       "check-files on a clean tree",
			files:      []fileSpec{{"kind.go", "package kind\n"}, {"kind_test.go", cleanTest}},
			args:       func(root string) []string { return []string{"-check-files", root} },
			wantCode:   0,
			wantStdout: []string{"every test file is named after a module it tests"},
		},
		{
			name:       "check-files on a violating tree",
			files:      []fileSpec{{"kind.go", "package kind\n"}, {"theme_test.go", cleanTest}},
			args:       func(root string) []string { return []string{"-check-files", root} },
			wantCode:   1,
			wantStdout: []string{"theme_test.go", "1 file(s) violate the convention"},
		},
		{
			name:       "dry-run on its own selects the rename mode",
			files:      []fileSpec{{"sample_test.go", legacyNamesFixture}},
			args:       func(root string) []string { return []string{"-dry-run", root} },
			wantCode:   0,
			wantStdout: []string{"TestCovBuildCatalogError -> TestBuild_Catalog_Error"},
			notStdout:  []string{"current_name"},
			wantStderr: []string{"Rename Summary (dry-run)"},
			wantFile:   legacyNamesFixture,
		},
		{
			name:       "apply rewrites the file",
			files:      []fileSpec{{"sample_test.go", legacyNamesFixture}},
			args:       func(root string) []string { return []string{"-apply", root} },
			wantCode:   0,
			wantStdout: []string{"TestCovBuildCatalogError -> TestBuild_Catalog_Error"},
			wantStderr: []string{"Rename Summary (applied)"},
			wantFile:   legacyNamesRewritten,
		},
		{
			name:       "apply on a directory that is not there",
			args:       func(root string) []string { return []string{"-apply", filepath.Join(root, "absent")} },
			wantCode:   1,
			wantStderr: []string{"walk "},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFixtureDir(t, root, nil, tc.files)

			var buffered, stderr bytes.Buffer
			var stdout io.Writer = &buffered
			if tc.failStdout {
				stdout = failingWriter{}
			}

			if got := runMain(tc.args(root), stdout, &stderr); got != tc.wantCode {
				t.Errorf("runMain() = %d, want %d\nstdout:\n%s\nstderr:\n%s", got, tc.wantCode, buffered.String(), stderr.String())
			}
			assertMentions(t, buffered.String(), tc.wantStdout, tc.notStdout)
			assertMentions(t, stderr.String(), tc.wantStderr, nil)
			if tc.wantFile != "" {
				if got := readFile(t, filepath.Join(root, "sample_test.go")); got != tc.wantFile {
					t.Errorf("file = \n%s\nwant\n%s", got, tc.wantFile)
				}
			}
		})
	}
}

// TestMain_HandsTheExitCodeToOsExit verifies main forwards whatever code
// runMain returned rather than a constant: a command line naming a clean tree
// exits 0 and one naming no directory at all exits 1, through the same one
// line.
func TestMain_HandsTheExitCodeToOsExit(t *testing.T) {
	root := t.TempDir()
	testCases := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{name: "a named directory audits clean", args: []string{toolName, root}, wantCode: 0},
		{name: "no directory is a usage failure", args: []string{toolName}, wantCode: 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
			if err != nil {
				t.Fatalf("OpenFile(%s) error = %v", os.DevNull, err)
			}
			oldArgs, oldStdout, oldStderr := os.Args, os.Stdout, os.Stderr
			os.Args, os.Stdout, os.Stderr = tc.args, devNull, devNull
			got := -1
			osExit = func(code int) { got = code }
			t.Cleanup(func() {
				os.Args, os.Stdout, os.Stderr = oldArgs, oldStdout, oldStderr
				osExit = os.Exit
				devNull.Close()
			})

			main()

			if got != tc.wantCode {
				t.Errorf("main() handed os.Exit %d, want %d", got, tc.wantCode)
			}
		})
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
			if !runApply([]string{root}, &stdout, &stderr, tc.dryRun) {
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
			wantStderrPre: func(root string) string { return "walk " + filepath.Join(root, "absent") + ": " },
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
		{
			// A directory that failed must not end the run: the verdict is
			// already false and every later root still has to be judged, which
			// the file it scanned is what shows.
			name:          "a later directory is still walked",
			files:         []fileSpec{{"clean_test.go", "package sample\n\nimport \"testing\"\n\nfunc TestOne_Two(t *testing.T) {}\n"}},
			dirs:          func(root string) []string { return []string{filepath.Join(root, "absent"), root} },
			wantStderrPre: func(root string) string { return "walk " + filepath.Join(root, "absent") + ": " },
			wantSummary:   "\n=== Rename Summary (applied) ===\nFiles scanned: 1\nRenames: 0\n",
		},
		{
			// The same within one directory: the file that would not parse is
			// reported and the walk carries on to the next, which the count of
			// two scanned files is what shows.
			name: "a later file is still judged",
			files: []fileSpec{
				{"a_broken_test.go", "package sample\n\nfunc (\n"},
				{"b_clean_test.go", "package sample\n\nimport \"testing\"\n\nfunc TestOne_Two(t *testing.T) {}\n"},
			},
			wantStderrPre: func(root string) string { return "parse " + filepath.Join(root, "a_broken_test.go") + ": " },
			wantSummary:   "\n=== Rename Summary (applied) ===\nFiles scanned: 2\nRenames: 0\n",
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

// TestApplyFile_UnprovokableFailures_ReportAndFail verifies the three
// failures a real tree cannot produce, because by then the file has parsed
// and every replacement is one identifier for another: the second read
// failing, the rewritten source no longer parsing, and the write being
// refused. Each names the file on stderr, counts no rename and fails the
// run, so -apply exits non-zero instead of reporting a rewrite it did not
// make.
func TestApplyFile_UnprovokableFailures_ReportAndFail(t *testing.T) {
	sentinel := errors.New("boom")
	testCases := []struct {
		name    string
		install func(t *testing.T)
		want    string
	}{
		{
			name: "read fails",
			install: func(t *testing.T) {
				t.Helper()
				original := readSource
				readSource = func(string) ([]byte, error) { return nil, sentinel }
				t.Cleanup(func() { readSource = original })
			},
			want: "read ",
		},
		{
			name: "rewritten source does not parse",
			install: func(t *testing.T) {
				t.Helper()
				original := parseRewritten
				parseRewritten = func(string, []byte) error { return sentinel }
				t.Cleanup(func() { parseRewritten = original })
			},
			want: "ABORT ",
		},
		{
			name: "write is refused",
			install: func(t *testing.T) {
				t.Helper()
				original := writeSource
				writeSource = func(string, []byte, os.FileMode) error { return sentinel }
				t.Cleanup(func() { writeSource = original })
			},
			want: "write ",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "sample_test.go")
			if err := os.WriteFile(path, []byte(legacyNamesFixture), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			tc.install(t)

			var stdout, stderr bytes.Buffer
			applied, ok := applyFile(path, &stdout, &stderr, false)
			if applied != 0 || ok {
				t.Errorf("applyFile() = (%d, %t), want (0, false)", applied, ok)
			}
			if got := stderr.String(); !strings.Contains(got, tc.want) || !strings.Contains(got, sentinel.Error()) {
				t.Errorf("stderr = %q, want it to mention %q and %q", got, tc.want, sentinel.Error())
			}
		})
	}
}

// TestApplyFile_SourceChangedBetweenTheTwoReads_ReportsOnlyRealReplacements
// verifies the guard on the gap between the parse and the read. applyFile
// takes the names to rename from a parse of the path and then reads the path
// again, so an editor saving in between hands the loop names that no longer
// occur. A rename is counted, reported and written only once it has actually
// replaced something, and a file where nothing matched is left alone instead
// of being overwritten with the snapshot the second read returned.
func TestApplyFile_SourceChangedBetweenTheTwoReads_ReportsOnlyRealReplacements(t *testing.T) {
	testCases := []struct {
		name      string
		reread    string
		want      int
		wantWrite string
		wantOut   []string
		notOut    []string
	}{
		{
			name: "no rename still matches",
			reread: `package sample

import "testing"

func TestCatalog(t *testing.T) {}
`,
			want:   0,
			notOut: []string{"TestCreate_IssueReturnsIssue", "TestBuild_Catalog_Error"},
		},
		{
			name: "one of two still matches",
			reread: `package sample

import "testing"

func TestCovBuildCatalogError(t *testing.T) {}
`,
			want: 1,
			wantWrite: `package sample

import "testing"

func TestBuild_Catalog_Error(t *testing.T) {}
`,
			wantOut: []string{"TestBuild_Catalog_Error"},
			notOut:  []string{"TestCreate_IssueReturnsIssue"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "sample_test.go")
			if err := os.WriteFile(path, []byte(legacyNamesFixture), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			originalRead := readSource
			readSource = func(string) ([]byte, error) { return []byte(tc.reread), nil }
			t.Cleanup(func() { readSource = originalRead })
			wrote := recordWrites(t)

			var stdout, stderr bytes.Buffer
			applied, ok := applyFile(path, &stdout, &stderr, false)
			if applied != tc.want || !ok {
				t.Errorf("applyFile() = (%d, %t), want (%d, true)", applied, ok, tc.want)
			}
			if tc.want == 0 {
				if wrote.calls != 0 {
					t.Errorf("writeSource called %d times, want the file left alone", wrote.calls)
				}
			} else {
				wrote.assert(t, path, tc.wantWrite)
			}
			assertMentions(t, stdout.String(), tc.wantOut, tc.notOut)
			if stderr.String() != "" {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

// writeRecord is what applyFile handed writeSource, kept so the content, the
// path and the mode can be asserted after the call.
type writeRecord struct {
	calls int
	path  string
	data  string
	mode  os.FileMode
}

// recordWrites replaces writeSource with one that records its arguments and
// succeeds, restoring the original when the test ends.
func recordWrites(t *testing.T) *writeRecord {
	t.Helper()
	rec := &writeRecord{}
	original := writeSource
	writeSource = func(path string, data []byte, mode os.FileMode) error {
		rec.calls++
		rec.path, rec.data, rec.mode = path, string(data), mode
		return nil
	}
	t.Cleanup(func() { writeSource = original })
	return rec
}

// assert verifies the one write went to path with the content and the mode the
// command promises. The path and the mode reach the write as plain arguments,
// so a rewrite sent to the wrong file or left world-readable is something only
// an assertion here can report.
func (r *writeRecord) assert(t *testing.T, path, want string) {
	t.Helper()
	if r.calls != 1 {
		t.Errorf("writeSource called %d times, want once", r.calls)
	}
	if r.data != want {
		t.Errorf("written = %q, want %q", r.data, want)
	}
	if r.path != path {
		t.Errorf("wrote to %q, want %q", r.path, path)
	}
	if r.mode != 0o600 {
		t.Errorf("wrote with mode %#o, want %#o", r.mode, 0o600)
	}
}

// assertMentions reports every name in wanted that got is missing and every
// name in unwanted that it contains.
func assertMentions(t *testing.T, got string, wanted, unwanted []string) {
	t.Helper()
	for _, want := range wanted {
		if !strings.Contains(got, want) {
			t.Errorf("output = %q, want it to mention %q", got, want)
		}
	}
	for _, name := range unwanted {
		if strings.Contains(got, name) {
			t.Errorf("output = %q, want no mention of %q", got, name)
		}
	}
}

// TestParseGoSourceText_Sources_ReportWhatTheParserSays verifies the seam's
// own body: valid Go passes and invalid Go comes back as an error naming the
// file, which is what the ABORT message quotes.
func TestParseGoSourceText_Sources_ReportWhatTheParserSays(t *testing.T) {
	if err := parseGoSourceText("sample_test.go", []byte(legacyNamesFixture)); err != nil {
		t.Errorf("parseGoSourceText() error = %v, want nil", err)
	}
	err := parseGoSourceText("broken_test.go", []byte("package sample\n\nfunc (\n"))
	if err == nil || !strings.Contains(err.Error(), "broken_test.go") {
		t.Errorf("parseGoSourceText() error = %v, want one naming broken_test.go", err)
	}
}

// TestCollectRenames_SkipsCollisionsAndReservesTargets verifies a legacy
// name whose suggestion already exists is skipped with a message, a target
// claimed by an earlier rename is not claimed twice, and names that are
// compliant in either the two-part or the three-part form, self-suggesting,
// TestMain, lowercase helpers or not functions are left alone.
func TestCollectRenames_SkipsCollisionsAndReservesTargets(t *testing.T) {
	source := `package sample

import "testing"

var fixture = 1

func TestFooBar(t *testing.T) {}
func TestFoo_Bar(t *testing.T) {}
func TestCovAlphaBeta(t *testing.T) {}
func TestAlphaBeta(t *testing.T) {}
func TestCatalog(t *testing.T) {}
func TestGamma_Delta_Epsilon(t *testing.T) {}
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

// TestCollectRenames_ADeclarationWithNoName_IsPassedOver verifies the guard
// both of collectRenames' loops carry. Go's parser gives every function
// declaration a name, so only a caller handing in an assembled tree can
// produce one without: the guard is what keeps that from dereferencing nil,
// and it is asserted here because a tree can never reach it.
func TestCollectRenames_ADeclarationWithNoName_IsPassedOver(t *testing.T) {
	const source = "package sample\n\nimport \"testing\"\n\nfunc TestCovAlphaBeta(t *testing.T) {}\n"
	node, err := parser.ParseFile(token.NewFileSet(), "sample_test.go", source, 0)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	node.Decls = append([]ast.Decl{&ast.FuncDecl{}}, node.Decls...)

	var stderr bytes.Buffer
	got := collectRenames(node, "sample_test.go", &stderr)

	want := map[string]string{"TestCovAlphaBeta": "TestAlpha_Beta"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collectRenames() = %v, want %v", got, want)
	}
	if stderr.String() != "" {
		t.Errorf("stderr = %q, want empty", stderr.String())
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
