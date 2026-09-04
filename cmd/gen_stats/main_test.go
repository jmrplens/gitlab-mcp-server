// main_test.go verifies the repository statistics collector used by
// cmd/gen_stats.
//
// The collector reads the git index, so the end-to-end tests build a small
// repository in a temporary directory, stage a handful of Go files whose
// every line and declaration is known, and check the counts and the rendered
// README section against those numbers. The real repository is never written.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// fakeReadme carries the managed markers around a stale body plus text on
// either side that must survive regeneration.
const fakeReadme = "# Fake\n\n<!-- START STATS -->\n\nstale\n\n<!-- END STATS -->\n\nTail.\n"

// fakeSource is the single non-test file of the fixture repository: one
// struct type and one non-struct type, one exported and one unexported
// function, a defer, an error check, a nolint directive, a TODO comment, five
// comment lines, and two lines mentioning gitlab.
const fakeSource = `// Package a is fake.
package a

import "errors"

// Thing is a struct.
type Thing struct{}

// Kind is not a struct.
type Kind int

// Exported does work with GitLab.
func Exported() error {
	defer func() {}()
	err := helper()
	if err != nil {
		return err
	}
	return nil
}

func helper() error { //nolint:unparam // fixture
	// TODO: finish
	return errors.New("gitlab")
}
`

// fakeUnitTest is the fixture's unit test file: one test with one real
// subtest, one error check, and a raw string holding a fake test whose
// t.Run must not be counted.
const fakeUnitTest = `package a

import "testing"

func TestExported_Default_ReturnsNil(t *testing.T) {
	t.Run("case", func(t *testing.T) {
		err := Exported()
		if err != nil {
			t.Fatal(err)
		}
	})
}

const fixture = ` + "`" + `
func TestFake(t *testing.T) {
	t.Run("ghost", nil)
}
` + "`" + `
`

// fakeE2ETest is the fixture's build-tagged end-to-end test with one defer.
const fakeE2ETest = `//go:build e2e

package suite

import "testing"

func TestFlow_Default_Passes(t *testing.T) {
	defer t.Log("done")
}
`

// fakeGoMod declares two direct and two indirect dependencies across a block
// and two single-line requires, with a comment line inside the block.
const fakeGoMod = `module example.com/fake

go 1.27.0

require (
	github.com/a/b v1.0.0
	github.com/c/d v1.0.0 // indirect
	// a comment inside the block
)

require github.com/e/f v1.0.0

require github.com/g/h v1.0.0 // indirect
`

// TestCollectStats_NonZeroCounts verifies collectStats returns non-zero
// counts when run against the real repository root.
func TestCollectStats_NonZeroCounts(t *testing.T) {
	// The counts come from the git index, so a copy of the tree without its
	// .git, a source archive or a mirror synced without it, has nothing to
	// count, and that is not what this test is about.
	if out, err := exec.CommandContext(t.Context(), "git", "rev-parse", "--is-inside-work-tree").Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
		t.Skip("not inside a git work tree; the counts come from the index")
	}
	stats, err := collectStats(".")
	if err != nil {
		t.Fatalf("collectStats: %v", err)
	}
	if stats.SourceFiles == 0 {
		t.Error("SourceFiles = 0, want > 0")
	}
	if stats.SourceLines == 0 {
		t.Error("SourceLines = 0, want > 0")
	}
	if stats.Packages == 0 {
		t.Error("Packages = 0, want > 0")
	}
}

// TestCollectStats_FakeRepository_CountsEveryField verifies every counter
// against a staged fixture whose contents are known: file, line, function,
// subtest, pattern, comment, and dependency counts, plus the hall-of-fame
// records, with an untracked file present to prove the index is the source.
func TestCollectStats_FakeRepository_CountsEveryField(t *testing.T) {
	root := writeFakeRepository(t, fakeRepositoryFiles())
	writeFile(t, root, "internal/a/untracked.go", "package a\n\nfunc Ghost() {}\n")

	stats, err := collectStats(root)
	if err != nil {
		t.Fatalf("collectStats() error = %v", err)
	}
	want := &repoStats{
		SourceFiles:      1,
		UnitTestFiles:    1,
		E2ETestFiles:     1,
		SourceLines:      strings.Count(fakeSource, "\n"),
		UnitTestLines:    strings.Count(fakeUnitTest, "\n"),
		E2ETestLines:     strings.Count(fakeE2ETest, "\n"),
		ExportedFuncs:    1,
		UnexportedFuncs:  1,
		TestFuncs:        1,
		E2ETestFuncs:     1,
		Subtests:         1,
		CommentLines:     5,
		DeferStmts:       2,
		ErrChecks:        2,
		Nolints:          1,
		TODOs:            1,
		StructTypes:      1,
		GitlabLines:      2,
		LongestFuncName:  "Exported",
		LongestTestName:  "TestExported_Default_ReturnsNil",
		LargestSrcFile:   "internal/a/a.go",
		LargestSrcLines:  strings.Count(fakeSource, "\n"),
		LargestTestFile:  "internal/a/a_test.go",
		LargestTestLines: strings.Count(fakeUnitTest, "\n"),
		Packages:         2,
		DirectDeps:       2,
		IndirectDeps:     2,
	}
	if *stats != *want {
		t.Fatalf("collectStats() =\n%+v\nwant\n%+v", *stats, *want)
	}
}

// TestCollectStats_TrackedFileShapes_SkipOrFail verifies how the collector
// treats index entries that no longer match the working tree: a deleted file
// is skipped with a warning, a file the scanner cannot read fails, a file the
// parser rejects fails, and a directory that is not a repository fails.
func TestCollectStats_TrackedFileShapes_SkipOrFail(t *testing.T) {
	tests := []struct {
		name         string
		mutate       func(t *testing.T, root string)
		skipGit      bool
		wantErr      string
		wantE2EFiles int
	}{
		{
			name: "deleted on disk is skipped",
			mutate: func(t *testing.T, root string) {
				t.Helper()
				if err := os.Remove(filepath.Join(root, "test", "e2e", "suite", "flow_test.go")); err != nil {
					t.Fatalf("remove: %v", err)
				}
			},
		},
		{
			name: "replaced by a directory fails the scan",
			mutate: func(t *testing.T, root string) {
				t.Helper()
				path := filepath.Join(root, "internal", "a", "a.go")
				if err := os.Remove(path); err != nil {
					t.Fatalf("remove: %v", err)
				}
				if err := os.Mkdir(path, 0o750); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
			},
			wantErr:      "scanning",
			wantE2EFiles: 1,
		},
		{
			name: "unparseable file fails the declaration pass",
			mutate: func(t *testing.T, root string) {
				t.Helper()
				writeFile(t, root, "internal/a/a.go", "package a\n\nfunc (")
			},
			wantErr:      "parsing",
			wantE2EFiles: 1,
		},
		{
			name:         "not a repository",
			mutate:       func(*testing.T, string) {},
			skipGit:      true,
			wantErr:      "git ls-files",
			wantE2EFiles: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, fakeRepositoryFiles())
			if !tt.skipGit {
				stageAll(t, root)
			}
			tt.mutate(t, root)

			stats, err := collectStats(root)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("collectStats() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("collectStats() error = %v", err)
			}
			if stats.E2ETestFiles != tt.wantE2EFiles {
				t.Fatalf("E2ETestFiles = %d, want %d", stats.E2ETestFiles, tt.wantE2EFiles)
			}
		})
	}
}

// TestListTrackedGoFiles_StatFailure_ReturnsError verifies a tracked path that
// fails to stat for a reason other than absence is reported rather than
// dropped, here by turning its parent directory into a regular file.
func TestListTrackedGoFiles_StatFailure_ReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a file in place of a directory reports not-found on Windows")
	}
	root := writeFakeRepository(t, fakeRepositoryFiles())
	dir := filepath.Join(root, "internal", "a")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove dir: %v", err)
	}
	if err := os.WriteFile(dir, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write file in place of dir: %v", err)
	}

	_, err := listTrackedGoFiles(root)
	if err == nil || !strings.Contains(err.Error(), "stat tracked file") {
		t.Fatalf("listTrackedGoFiles() error = %v, want stat error", err)
	}
}

// TestWarnIndexDrift_NotARepository_StaysQuiet verifies the drift warning
// gives up silently when git cannot list untracked files, since the warning
// is advisory and the caller has already obtained the tracked set.
func TestWarnIndexDrift_NotARepository_StaysQuiet(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	warnIndexDrift(t.TempDir(), gitBin, 0)
}

// TestListTrackedGoFiles_NoGitOnPath_ReturnsError verifies the collector
// reports a missing git binary instead of guessing at the file set.
func TestListTrackedGoFiles_NoGitOnPath_ReturnsError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := listTrackedGoFiles(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "locating git") {
		t.Fatalf("listTrackedGoFiles() error = %v, want locating git error", err)
	}
}

// TestRun_FakeRepository_ChecksAndWrites verifies the command end to end on a
// staged fixture: --check rejects the stale section, a write regenerates it
// while keeping the surrounding text, and --check then accepts it.
func TestRun_FakeRepository_ChecksAndWrites(t *testing.T) {
	root := writeFakeRepository(t, fakeRepositoryFiles())
	t.Chdir(root)

	if err := run(true); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("run(check) on stale section error = %v, want stale error", err)
	}
	if err := run(false); err != nil {
		t.Fatalf("run(write) error = %v", err)
	}
	if err := run(true); err != nil {
		t.Fatalf("run(check) after write error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, readmePath))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	readme := string(data)
	if !strings.HasPrefix(readme, "# Fake\n\n"+statsStartMarker+"\n\n### File counts") || !strings.HasSuffix(readme, statsEndMarker+"\n\nTail.\n") {
		t.Fatalf("surrounding text not preserved:\n%s", readme)
	}
	if strings.Contains(readme, "stale") {
		t.Fatalf("stale body survived:\n%s", readme)
	}
	srcLines := strings.Count(fakeSource, "\n")
	unitLines := strings.Count(fakeUnitTest, "\n")
	e2eLines := strings.Count(fakeE2ETest, "\n")
	rows := []struct {
		label string
		want  []string
	}{
		{label: "Source (`.go`, non-test)", want: []string{"1", strconv.Itoa(srcLines)}},
		{label: "Unit tests (`_test.go`)", want: []string{"1", strconv.Itoa(unitLines)}},
		{label: "End-to-end tests", want: []string{"1", strconv.Itoa(e2eLines)}},
		{label: "**Total**", want: []string{"**3**", "**" + strconv.Itoa(srcLines+unitLines+e2eLines) + "**"}},
		{label: "Go packages", want: []string{"2"}},
		{label: "Direct dependencies (`go.mod`)", want: []string{"2"}},
		{label: "Indirect dependencies", want: []string{"2"}},
	}
	for _, row := range rows {
		t.Run(row.label, func(t *testing.T) {
			assertRow(t, readme, row.label, row.want)
		})
	}
}

// TestRun_ReadmeShapes_ReturnEachError verifies the entry point reports a
// missing README in both modes, a README without markers, and a directory
// that is not a repository.
func TestRun_ReadmeShapes_ReturnEachError(t *testing.T) {
	tests := []struct {
		name    string
		readme  string
		skipGit bool
		check   bool
		wantErr string
	}{
		{name: "not a repository", readme: fakeReadme, skipGit: true, check: true, wantErr: "collecting stats"},
		{name: "missing readme in check mode", check: true, wantErr: "read README.md"},
		{name: "missing readme in write mode", wantErr: "write stats section"},
		{name: "readme without markers", readme: "# Fake\n\nno markers\n", check: true, wantErr: "replace stats section"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := fakeRepositoryFiles()
			delete(files, readmePath)
			if tt.readme != "" {
				files[readmePath] = tt.readme
			}
			root := t.TempDir()
			writeFiles(t, root, files)
			if !tt.skipGit {
				stageAll(t, root)
			}
			t.Chdir(root)

			err := run(tt.check)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("run(%v) error = %v, want %q", tt.check, err, tt.wantErr)
			}
		})
	}
}

// TestScanGoFile_MissingFile_ReturnsError verifies an unreadable path is
// reported by the line scanner.
func TestScanGoFile_MissingFile_ReturnsError(t *testing.T) {
	var s repoStats
	if _, err := scanGoFile(filepath.Join(t.TempDir(), "absent.go"), false, false, &s); err == nil {
		t.Fatal("scanGoFile() error = nil, want error")
	}
}

// TestScanGoLine_LineShapes_UpdateEachCounter verifies each textual pattern
// increments exactly its own counter, and that comment and gitlab lines are
// counted for source files only.
func TestScanGoLine_LineShapes_UpdateEachCounter(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		isTest bool
		isE2E  bool
		want   repoStats
	}{
		{name: "subtest in unit test", line: "\tt.Run(\"x\", func(t *testing.T) {", isTest: true, want: repoStats{Subtests: 1}},
		{name: "subtest in e2e test", line: "\tt.Run(\"x\", nil)", isE2E: true, want: repoStats{Subtests: 1}},
		{name: "subtest text in source is ignored", line: "\tt.Run(\"x\", nil)"},
		{name: "defer", line: "\tdefer f.Close()", want: repoStats{DeferStmts: 1}},
		{name: "error check", line: "\tif err != nil {", want: repoStats{ErrChecks: 1}},
		{name: "nolint", line: "\tx := y //nolint:gosec // reason", want: repoStats{Nolints: 1}},
		{name: "todo comment", line: "\t// TODO: later", want: repoStats{TODOs: 1, CommentLines: 1}},
		{name: "comment mentioning gitlab", line: "// GitLab client", want: repoStats{CommentLines: 1, GitlabLines: 1}},
		{name: "gitlab code in test file", line: "\turl := \"https://gitlab.com\"", isTest: true},
		{name: "plain code", line: "\tx := 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s repoStats
			scanGoLine(tt.line, tt.isE2E, tt.isTest, &s)
			if s != tt.want {
				t.Fatalf("scanGoLine(%q) = %+v, want %+v", tt.line, s, tt.want)
			}
		})
	}
}

// TestUpdateFunctionStats_NameShapes_ClassifyEachBucket verifies function
// names land in the e2e, unit-test, exported, or unexported bucket by file
// kind, longest-name records are kept per bucket, and an empty name counts as
// unexported.
func TestUpdateFunctionStats_NameShapes_ClassifyEachBucket(t *testing.T) {
	tests := []struct {
		name   string
		fn     string
		isTest bool
		isE2E  bool
		want   repoStats
	}{
		{name: "e2e test", fn: "TestFlow", isE2E: true, want: repoStats{E2ETestFuncs: 1}},
		{name: "e2e helper is ignored", fn: "helper", isE2E: true},
		{name: "unit test", fn: "TestThing_Works", isTest: true, want: repoStats{TestFuncs: 1, LongestTestName: "TestThing_Works"}},
		{name: "unit helper is ignored", fn: "TestMain", isTest: true},
		{name: "exported", fn: "Exported", want: repoStats{ExportedFuncs: 1, LongestFuncName: "Exported"}},
		{name: "unexported", fn: "internal", want: repoStats{UnexportedFuncs: 1, LongestFuncName: "internal"}},
		{name: "empty name", fn: "", want: repoStats{UnexportedFuncs: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s repoStats
			updateFunctionStats(tt.fn, tt.isE2E, tt.isTest, &s)
			if s != tt.want {
				t.Fatalf("updateFunctionStats(%q) = %+v, want %+v", tt.fn, s, tt.want)
			}
		})
	}
}

// TestUpdateFunctionStats_ShorterNames_KeepLongestRecord verifies a later,
// shorter name does not displace the recorded longest name in either bucket.
func TestUpdateFunctionStats_ShorterNames_KeepLongestRecord(t *testing.T) {
	var s repoStats
	updateFunctionStats("TestLongerName_Case", false, true, &s)
	updateFunctionStats("TestShort", false, true, &s)
	updateFunctionStats("LongerExported", false, false, &s)
	updateFunctionStats("short", false, false, &s)
	if s.LongestTestName != "TestLongerName_Case" || s.LongestFuncName != "LongerExported" {
		t.Fatalf("longest names = %q / %q, want TestLongerName_Case / LongerExported", s.LongestTestName, s.LongestFuncName)
	}
}

// TestIsTODOComment_MarkerShapes_RequireWordBoundary verifies the three task
// markers are recognized only as comments and only at a word boundary, so
// identifiers that merely start with a marker are not counted.
func TestIsTODOComment_MarkerShapes_RequireWordBoundary(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{line: "// TODO: later", want: true},
		{line: "//TODO", want: true},
		{line: "// fixme(x): later", want: true},
		{line: "//\tHACK - workaround", want: true},
		{line: "// TodoOutput is a struct", want: false},
		{line: "// TODO_LATER", want: false},
		{line: "// nothing here", want: false},
		{line: "x := TODO", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			if got := isTODOComment(tt.line); got != tt.want {
				t.Fatalf("isTODOComment(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

// TestParseDeps_GoModShapes_CountsDirectAndIndirect verifies block and
// single-line requires are classified by the indirect marker, comment lines
// inside the block are ignored, and a missing go.mod yields zeros.
func TestParseDeps_GoModShapes_CountsDirectAndIndirect(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", fakeGoMod)
	tests := []struct {
		name         string
		path         string
		wantDirect   int
		wantIndirect int
	}{
		{name: "fixture", path: filepath.Join(dir, "go.mod"), wantDirect: 2, wantIndirect: 2},
		{name: "missing", path: filepath.Join(dir, "absent.mod")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			direct, indirect := parseDeps(tt.path)
			if direct != tt.wantDirect || indirect != tt.wantIndirect {
				t.Fatalf("parseDeps() = %d direct, %d indirect; want %d, %d", direct, indirect, tt.wantDirect, tt.wantIndirect)
			}
		})
	}
}

// TestRenderStats_PopulatedStats_RendersEveryTable verifies every table row
// derives from the collected numbers: totals, ratios, averages, percentages,
// pattern counts, project meta, and the hall-of-fame records.
func TestRenderStats_PopulatedStats_RendersEveryTable(t *testing.T) {
	out := renderStats(&repoStats{
		SourceFiles: 10, UnitTestFiles: 5, E2ETestFiles: 2,
		SourceLines: 1000, UnitTestLines: 2500, E2ETestLines: 300,
		ExportedFuncs: 30, UnexportedFuncs: 70, TestFuncs: 150, E2ETestFuncs: 12, Subtests: 400,
		CommentLines: 250, DeferStmts: 40, ErrChecks: 90, Nolints: 3, TODOs: 1, StructTypes: 55, GitlabLines: 42,
		LongestFuncName: "RegisterEverything", LongestTestName: "TestRegisterEverything_Works",
		LargestSrcFile: "internal/big.go", LargestSrcLines: 1200, LargestTestFile: "internal/big_test.go", LargestTestLines: 3400,
		Packages: 12, DirectDeps: 7, IndirectDeps: 90,
	})
	rows := []struct {
		label string
		want  []string
	}{
		{label: "Source (`.go`, non-test)", want: []string{"10", "1,000"}},
		{label: "Unit tests (`_test.go`)", want: []string{"5", "2,500"}},
		{label: "End-to-end tests", want: []string{"2", "300"}},
		{label: "**Total**", want: []string{"**17**", "**3,800**"}},
		{label: "Source functions", want: []string{"100"}},
		{label: ". Exported (public)", want: []string{"30"}},
		{label: ". Unexported (private)", want: []string{"70"}},
		{label: "Unit test functions (`TestXxx`)", want: []string{"150"}},
		{label: "Subtests (`t.Run(...)`)", want: []string{"400"}},
		{label: "End-to-end test functions", want: []string{"12"}},
		{label: "Test lines vs source lines", want: []string{"2.50× more tests than code"}},
		{label: "Average source file length", want: []string{"~100 lines"}},
		{label: "Average test file length", want: []string{"~500 lines"}},
		{label: "Comment lines in source", want: []string{"250 (~25.0% of source)"}},
		{label: "Test functions per source function", want: []string{"1.5×"}},
		{label: "`if err != nil` checks", want: []string{"90"}},
		{label: "`defer` statements", want: []string{"40"}},
		{label: "`struct` types defined", want: []string{"55"}},
		{label: "`//nolint` suppressions", want: []string{"3"}},
		{label: "`TODO` / `FIXME` / `HACK` comments", want: []string{"1"}},
		{label: "Go packages", want: []string{"12"}},
		{label: "Direct dependencies (`go.mod`)", want: []string{"7"}},
		{label: "Indirect dependencies", want: []string{"90"}},
		{label: "Longest source file", want: []string{"`internal/big.go`. 1,200 lines"}},
		{label: "Longest test file", want: []string{"`internal/big_test.go`. 3,400 lines"}},
		{label: "Source code printed at 55 lines/page", want: []string{"~18 pages of A4"}},
		{label: "Source lines mentioning `\"gitlab\"`", want: []string{"42 (impossible to avoid)"}},
		{label: "Longest function name in source", want: []string{"`RegisterEverything` (18 chars)"}},
		{label: "Longest test function name", want: []string{"`TestRegisterEverything_Works` (28 chars)"}},
	}
	for _, row := range rows {
		t.Run(row.label, func(t *testing.T) {
			assertRow(t, out, row.label, row.want)
		})
	}
}

// TestRenderStats_ZeroStats_GuardsDivisions verifies an empty repository
// renders zero ratios and averages instead of dividing by zero.
func TestRenderStats_ZeroStats_GuardsDivisions(t *testing.T) {
	out := renderStats(&repoStats{})
	rows := []struct {
		label string
		want  []string
	}{
		{label: "Test lines vs source lines", want: []string{"0.00× more tests than code"}},
		{label: "Average source file length", want: []string{"~0 lines"}},
		{label: "Average test file length", want: []string{"~0 lines"}},
		{label: "Comment lines in source", want: []string{"0 (~0.0% of source)"}},
		{label: "Test functions per source function", want: []string{"0.0×"}},
		{label: "Longest function name in source", want: []string{"`` (0 chars)"}},
	}
	for _, row := range rows {
		t.Run(row.label, func(t *testing.T) {
			assertRow(t, out, row.label, row.want)
		})
	}
}

// TestFmtInt_AddsThousandsSeparators verifies the integer formatter inserts
// comma separators at the expected positions.
func TestFmtInt_AddsThousandsSeparators(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{in: 0, want: "0"},
		{in: 999, want: "999"},
		{in: 1000, want: "1,000"},
		{in: 1234567, want: "1,234,567"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := fmtInt(tt.in); got != tt.want {
				t.Fatalf("fmtInt(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestIsTestFunctionName_ExcludesTestMain verifies TestMain is not counted as a
// test function while regular Test* entry points are.
func TestIsTestFunctionName_ExcludesTestMain(t *testing.T) {
	if isTestFunctionName("TestMain") {
		t.Error("isTestFunctionName(TestMain) = true, want false")
	}
	if !isTestFunctionName("TestFoo") {
		t.Error("isTestFunctionName(TestFoo) = false, want true")
	}
	if !isTestFunctionName("Test") {
		t.Error("isTestFunctionName(Test) = false, want true")
	}
	if isTestFunctionName("Testable") {
		t.Error("isTestFunctionName(Testable) = true, want false (lowercase-ish suffix)")
	}
}

// TestScanGoFile_SkipsRawStringFixtures verifies that a fake test embedded in
// a multi-line raw string does not reach the line-level counters. A fixture
// source holding a whole fake Go file must not inflate subtest, defer or
// error-check totals, which is what the backtick-parity skip is for.
//
// Declaration counting is no longer the line scanner's job — see
// [TestScanGoDecls_CountsDeclarationsNotText] — so TestFuncs stays zero here.
func TestScanGoFile_SkipsRawStringFixtures(t *testing.T) {
	dir := t.TempDir()
	src := "package p\n\nconst fixture = `\nfunc TestFake(t *testing.T) {\n\tt.Run(\"sub\", nil)\n}\n`\n\nfunc TestReal(t *testing.T) {}\n"
	path := filepath.Join(dir, "x_test.go")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	var s repoStats
	if _, err := scanGoFile(path, false, true, &s); err != nil {
		t.Fatalf("scanGoFile: %v", err)
	}
	if s.Subtests != 0 {
		t.Errorf("Subtests = %d, want 0 — the t.Run inside the raw string is data, not code", s.Subtests)
	}
	if s.TestFuncs != 0 {
		t.Errorf("TestFuncs = %d, want 0 — the line scanner no longer counts declarations", s.TestFuncs)
	}
}

// TestScanGoDecls_CountsDeclarationsNotText verifies that declarations are
// counted by parsing, which is what makes the fixture case correct without a
// heuristic.
//
// The line scanner skipped raw strings by counting backtick parity, and a
// single line holding an odd number of backticks flipped it into "inside a
// raw string" until the next such line — swallowing 225 real test
// declarations across this repository and putting these totals permanently at
// odds with cmd/gen_testing_docs. A parser has no such failure mode: the fake
// below is a string constant, and the real declaration after it is a
// declaration regardless of how many backticks preceded it.
func TestScanGoDecls_CountsDeclarationsNotText(t *testing.T) {
	tests := []struct {
		name          string
		src           string
		isTest        bool
		isE2E         bool
		wantTestFuncs int
		wantE2EFuncs  int
		wantExported  int
		wantStructs   int
	}{
		{
			name:          "fake test inside a raw string is not a declaration",
			src:           "package p\n\nconst fixture = `\nfunc TestFake(t *testing.T) {}\n`\n\nfunc TestReal(t *testing.T) {}\n",
			isTest:        true,
			wantTestFuncs: 1,
		},
		{
			name: "an odd backtick count does not hide what follows",
			src: "package p\n\nfunc TestBefore(t *testing.T) {}\n\n" +
				"var doc = \"a ` lone backtick in an interpreted string\"\n\n" +
				"func TestAfter(t *testing.T) {}\n",
			isTest:        true,
			wantTestFuncs: 2,
		},
		{
			name:          "TestMain is not a test",
			src:           "package p\n\nfunc TestMain(m *testing.M) {}\n\nfunc TestReal(t *testing.T) {}\n",
			isTest:        true,
			wantTestFuncs: 1,
		},
		{
			name:         "e2e files count separately",
			src:          "package p\n\nfunc TestFlow(t *testing.T) {}\n",
			isE2E:        true,
			wantE2EFuncs: 1,
		},
		{
			name:         "source declarations, structs included",
			src:          "package p\n\ntype Thing struct{}\n\ntype Alias = int\n\nfunc Exported() {}\n\nfunc unexported() {}\n",
			wantExported: 1,
			wantStructs:  1,
		},
		{
			name:        "test file types are not counted",
			src:         "package p\n\ntype fixture struct{}\n\nvar x = 1\n",
			isTest:      true,
			wantStructs: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			name := "x.go"
			if tt.isTest || tt.isE2E {
				name = "x_test.go"
			}
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte(tt.src), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			var s repoStats
			if err := scanGoDecls(path, tt.isE2E, tt.isTest, &s); err != nil {
				t.Fatalf("scanGoDecls: %v", err)
			}
			if s.TestFuncs != tt.wantTestFuncs {
				t.Errorf("TestFuncs = %d, want %d", s.TestFuncs, tt.wantTestFuncs)
			}
			if s.E2ETestFuncs != tt.wantE2EFuncs {
				t.Errorf("E2ETestFuncs = %d, want %d", s.E2ETestFuncs, tt.wantE2EFuncs)
			}
			if s.ExportedFuncs != tt.wantExported {
				t.Errorf("ExportedFuncs = %d, want %d", s.ExportedFuncs, tt.wantExported)
			}
			if s.StructTypes != tt.wantStructs {
				t.Errorf("StructTypes = %d, want %d", s.StructTypes, tt.wantStructs)
			}
		})
	}
}

// TestScanGoDecls_UnparseableFile_Reports verifies that a file the parser
// cannot read surfaces as an error rather than silently contributing zero.
// A generator that under-reports without saying so is what this whole change
// is correcting.
func TestScanGoDecls_UnparseableFile_Reports(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.go")
	if err := os.WriteFile(path, []byte("package p\n\nfunc ("), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	var s repoStats
	if err := scanGoDecls(path, false, false, &s); err == nil {
		t.Error("expected an error for an unparseable file")
	}
}

// fakeRepositoryFiles returns the fixture repository: a README with the
// managed markers, a go.mod with four dependencies, one source file, one
// unit test file, and one end-to-end test file.
func fakeRepositoryFiles() map[string]string {
	return map[string]string{
		readmePath:                    fakeReadme,
		"go.mod":                      fakeGoMod,
		"internal/a/a.go":             fakeSource,
		"internal/a/a_test.go":        fakeUnitTest,
		"test/e2e/suite/flow_test.go": fakeE2ETest,
	}
}

// writeFakeRepository writes files into a fresh temporary directory,
// initializes a git repository there, and stages everything.
func writeFakeRepository(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, files)
	stageAll(t, root)
	return root
}

// stageAll initializes a repository at root and adds every file to its index.
func stageAll(t *testing.T, root string) {
	t.Helper()
	runGit(t, root, "init", "-q")
	runGit(t, root, "add", "-A")
}

// runGit runs one git subcommand against root and fails the test on error.
func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", root}, args...)...) //#nosec G204 -- fixed git subcommands against a temporary directory this test created
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// writeFiles writes every entry of files (slash-separated paths relative to
// root) creating parent directories as needed.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		writeFile(t, root, rel, content)
	}
}

// writeFile writes one file under root, creating parent directories.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(rel), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// assertRow checks the cells after the label of the first Markdown table row
// in rendered whose first cell equals label.
func assertRow(t *testing.T, rendered, label string, want []string) {
	t.Helper()
	for line := range strings.Lines(rendered) {
		cells := splitTableRow(line)
		if len(cells) == 0 || cells[0] != label {
			continue
		}
		got := cells[1:]
		if len(got) != len(want) {
			t.Fatalf("row %q has cells %v, want %v", label, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("row %q cell %d = %q, want %q (row %v)", label, i+1, got[i], want[i], got)
			}
		}
		return
	}
	t.Fatalf("row %q not found in:\n%s", label, rendered)
}

// splitTableRow splits one Markdown table line into trimmed cells, or returns
// nil when the line is not a table row.
func splitTableRow(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return nil
	}
	parts := strings.Split(strings.Trim(line, "|"), "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells
}
