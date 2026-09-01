// Command gen_stats auto-generates the repository statistics section in
// README.md. It classifies every tracked Go file, counts
// dependencies, and replaces the content between the
// <!-- START STATS --> / <!-- END STATS --> markers.
//
// Every figure is a pure function of the tracked file set. Nothing is derived
// from git history: a commit count changes with the very commit that would
// refresh it, so the section could never be both committed and current, and a
// CI shallow clone would compute a different number anyway. Files come from the
// git index rather than a directory walk for the same reason — see
// [listTrackedGoFiles]. That is what makes `--check` usable as a gate.
//
// Usage:
//
//	go run ./cmd/gen_stats/
//
// With --check it verifies the section is current without writing.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/docgen"
)

// Marker constants for the stats section of README.md, plus generator paths.
const (
	statsStartMarker = "<!-- START STATS -->"
	statsEndMarker   = "<!-- END STATS -->"
	readmePath       = "README.md"
	repoRoot         = "."
	linesPerPage     = 55 // approximate readable lines per A4 page at 12pt

	// scannerBufSize is the initial and maximum bufio.Scanner token size.
	// 512 KB handles the largest generated Go files without reallocating.
	scannerBufSize = 512 * 1024
)

// main regenerates the README stats section and exits non-zero on failure.
// With --check it verifies the section is current without writing.
func main() {
	check := flag.Bool("check", false, "verify README stats section is current without writing")
	flag.Parse()
	if err := run(*check); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// run collects repository statistics and replaces the README stats section.
// When check is true, it only verifies the section is current and returns an
// error if it is stale.
func run(check bool) error {
	stats, err := collectStats(repoRoot)
	if err != nil {
		return fmt.Errorf("collecting stats: %w", err)
	}
	rendered := renderStats(stats)
	if check {
		data, readErr := os.ReadFile(readmePath) //#nosec G304 -- path is a hardcoded constant
		if readErr != nil {
			return fmt.Errorf("read %s: %w", readmePath, readErr)
		}
		text := string(data)
		updated, replaceErr := docgen.ComputeReplacedSection(text, statsStartMarker, statsEndMarker, rendered)
		if replaceErr != nil {
			return fmt.Errorf("replace stats section in %s: %w", readmePath, replaceErr)
		}
		if updated != text {
			return fmt.Errorf("%s stats section is stale; run go run ./cmd/gen_stats/", readmePath)
		}
		return nil
	}
	if replaceErr := docgen.ReplaceSection(readmePath, statsStartMarker, statsEndMarker, rendered); replaceErr != nil {
		return fmt.Errorf("write stats section in %s: %w", readmePath, replaceErr)
	}
	fmt.Printf("Updated %s stats section\n", readmePath)
	return nil
}

// repoStats accumulates filesystem-level measurements of the Go codebase.
//
// File counts, line counts, and function counts are populated by walking
// root with [collectStats]. Hall-of-fame fields track the longest names
// and largest files. Project meta fields (Packages, DirectDeps,
// IndirectDeps) are filled after the walk
// from go.mod and git, so callers should not assume they are valid until
// [collectStats] returns.
type repoStats struct {
	// File counts
	SourceFiles   int
	UnitTestFiles int
	E2ETestFiles  int

	// Line counts
	SourceLines   int
	UnitTestLines int
	E2ETestLines  int

	// Function counts
	ExportedFuncs   int
	UnexportedFuncs int
	TestFuncs       int
	E2ETestFuncs    int
	Subtests        int

	// Source-only metrics
	CommentLines int
	DeferStmts   int
	ErrChecks    int
	Nolints      int
	TODOs        int
	StructTypes  int
	GitlabLines  int // lines in source containing "gitlab" (case-insensitive)

	// Hall of fame
	LongestFuncName  string
	LongestTestName  string
	LargestSrcFile   string
	LargestSrcLines  int
	LargestTestFile  string
	LargestTestLines int

	// Project meta (filled after the walk)
	Packages     int
	DirectDeps   int
	IndirectDeps int
}

// collectStats classifies every tracked .go file under root and returns a
// populated repoStats. root should be the repository root directory.
func collectStats(root string) (*repoStats, error) {
	s := &repoStats{}
	dirs := make(map[string]bool)

	// WalkDir is used instead of Walk: it receives fs.DirEntry directly from
	// the OS directory read, avoiding the extra os.Lstat call Walk performs
	// for every entry.
	files, err := listTrackedGoFiles(root)
	if err != nil {
		return nil, err
	}
	for _, rel := range files {
		path := filepath.Join(root, rel)
		isE2E := strings.Contains(rel, "/e2e/")
		isTest := strings.HasSuffix(rel, "_test.go")

		dirs[filepath.Dir(rel)] = true

		lines, scanErr := scanGoFile(path, isE2E, isTest, s)
		if scanErr != nil {
			return nil, fmt.Errorf("scanning %s: %w", path, scanErr)
		}
		if declErr := scanGoDecls(path, isE2E, isTest, s); declErr != nil {
			return nil, declErr
		}

		switch {
		case isE2E:
			s.E2ETestFiles++
			s.E2ETestLines += lines
		case isTest:
			s.UnitTestFiles++
			s.UnitTestLines += lines
			if lines > s.LargestTestLines {
				s.LargestTestLines = lines
				s.LargestTestFile = rel
			}
		default:
			s.SourceFiles++
			s.SourceLines += lines
			if lines > s.LargestSrcLines {
				s.LargestSrcLines = lines
				s.LargestSrcFile = rel
			}
		}
	}

	s.Packages = len(dirs)
	s.DirectDeps, s.IndirectDeps = parseDeps(filepath.Join(root, "go.mod"))

	return s, nil
}

// listTrackedGoFiles returns every .go file git tracks under root, as
// slash-separated paths relative to root.
//
// The file set comes from the index rather than from a directory walk so that
// the generated section describes the repository and nothing else. A walk also
// counts whatever happens to sit in a developer's working tree — build output,
// scratch packages, a test file an over-broad .gitignore rule excluded — and
// then `--check` disagrees between that machine and CI for reasons no diff
// explains. The index is identical in a shallow clone, so this stays safe under
// the `fetch-depth: 1` checkout CI uses.
func listTrackedGoFiles(root string) ([]string, error) {
	bin, err := exec.LookPath("git") //#nosec G204 -- resolves to an absolute path; no user input involved
	if err != nil {
		return nil, fmt.Errorf("locating git: %w", err)
	}
	out, err := exec.CommandContext(context.Background(), bin, "-C", root, "ls-files", "-z", "--", "*.go").Output() //#nosec G204 -- absolute path from LookPath, fixed args
	if err != nil {
		return nil, fmt.Errorf("git ls-files in %s: %w", root, err)
	}
	var files []string
	var missing int
	for f := range strings.SplitSeq(string(out), "\x00") {
		if f == "" {
			continue
		}
		// The index still lists a file that has been deleted on disk but not
		// yet staged. Skipping it keeps the tool usable mid-refactor instead of
		// failing on a dirty tree; a fresh checkout, which is what CI measures,
		// has nothing to skip. Only a genuinely absent file is skipped — a
		// permission or I/O error must surface rather than silently drop a
		// tracked file from the published counts.
		if _, statErr := os.Stat(filepath.Join(root, f)); statErr != nil {
			if os.IsNotExist(statErr) {
				missing++
				continue
			}
			return nil, fmt.Errorf("stat tracked file %s: %w", f, statErr)
		}
		files = append(files, f)
	}
	warnIndexDrift(root, bin, missing)
	return files, nil
}

// warnIndexDrift reports Go files whose tracked state does not match the
// working tree, because the counts are taken from the index.
//
// Regenerating before staging is the one way to produce a section that CI then
// rejects: a new .go file that is not yet added is invisible here but present
// in the commit CI checks out. Silently emitting the wrong numbers turns that
// into a confusing red build, so say it out loud instead.
func warnIndexDrift(root, gitBin string, missing int) {
	if missing > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d tracked .go file(s) are deleted on disk but not staged; they are not counted\n", missing)
	}
	out, err := exec.CommandContext(context.Background(), gitBin, "-C", root, "ls-files", "-z", "--others", "--exclude-standard", "--", "*.go").Output() //#nosec G204 -- absolute path from LookPath, fixed args
	if err != nil {
		return
	}
	var untracked []string
	for f := range strings.SplitSeq(string(out), "\x00") {
		if f != "" {
			untracked = append(untracked, f)
		}
	}
	if len(untracked) > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d untracked .go file(s) are not counted; stage them and rerun, or CI will disagree:\n", len(untracked))
		for _, f := range untracked {
			fmt.Fprintf(os.Stderr, "  %s\n", f)
		}
	}
}

// scanGoFile reads every line of a .go file and accumulates pattern-based
// counters into s. Returns the total line count.
func scanGoFile(path string, isE2E, isTest bool, s *repoStats) (int, error) {
	f, err := os.Open(filepath.Clean(path)) //#nosec G304 -- path from filepath.WalkDir within repo
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var lines int
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, scannerBufSize), scannerBufSize)

	// Lines inside multi-line raw strings are data, not code: test fixtures
	// (e.g. the goroutine-audit fixture sources) embed entire fake Go files
	// whose "func Test..." lines would otherwise inflate every counter. A
	// line with an odd number of backticks toggles the state; pairs on one
	// line open and close within it.
	inRawString := false
	for sc.Scan() {
		lines++
		line := sc.Text()
		if inRawString {
			if strings.Count(line, "`")%2 == 1 {
				inRawString = false
			}
			continue
		}
		scanGoLine(line, isE2E, isTest, s)
		if strings.Count(line, "`")%2 == 1 {
			inRawString = true
		}
	}
	return lines, sc.Err()
}

func scanGoLine(line string, isE2E, isTest bool, s *repoStats) {
	trimmed := strings.TrimSpace(line)
	if (isTest || isE2E) && strings.Contains(line, "t.Run(") {
		s.Subtests++
	}
	if strings.HasPrefix(trimmed, "defer ") {
		s.DeferStmts++
	}
	if strings.Contains(line, "if err != nil") {
		s.ErrChecks++
	}
	if strings.Contains(line, "//nolint") {
		s.Nolints++
	}
	if isTODOComment(trimmed) {
		s.TODOs++
	}
	if !isTest && !isE2E {
		updateSourceLineStats(line, trimmed, s)
	}
}

// scanGoDecls counts declarations by parsing the file, not by reading its
// lines.
//
// The line scanner cannot do this correctly and never could. It skips
// multi-line raw strings by counting backtick parity, which is the right
// instinct — a fixture embedding a fake Go file must not inflate the
// counters — but the wrong mechanism: a single line holding an odd number of
// backticks flips it into "inside a raw string" and everything until the next
// such line disappears. In this repository that is table-driven tests whose
// case data contains Markdown or Go snippets, and it was swallowing 225 real
// test declarations, which is why these figures disagreed with
// cmd/gen_testing_docs, whose totals come from go/ast.
//
// Only declarations move here. The remaining line heuristics — defer, error
// checks, nolint directives, TODO comments, comment and "gitlab" lines — are
// textual by nature and genuinely want the raw-string skip, so they stay
// where they are.
func scanGoDecls(path string, isE2E, isTest bool, s *repoStats) error {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			updateFunctionStats(d.Name.Name, isE2E, isTest, s)
		case *ast.GenDecl:
			if isTest || isE2E || d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if _, isStruct := ts.Type.(*ast.StructType); isStruct {
					s.StructTypes++
				}
			}
		}
	}
	return nil
}

func updateFunctionStats(name string, isE2E, isTest bool, s *repoStats) {
	switch {
	case isE2E && isTestFunctionName(name):
		s.E2ETestFuncs++
	case isTest && isTestFunctionName(name):
		s.TestFuncs++
		if len(name) > len(s.LongestTestName) {
			s.LongestTestName = name
		}
	case !isTest && !isE2E:
		if name != "" && unicode.IsUpper(rune(name[0])) {
			s.ExportedFuncs++
		} else {
			s.UnexportedFuncs++
		}
		if len(name) > len(s.LongestFuncName) {
			s.LongestFuncName = name
		}
	}
}

func updateSourceLineStats(line, trimmed string, s *repoStats) {
	if strings.HasPrefix(trimmed, "//") {
		s.CommentLines++
	}
	if strings.Contains(strings.ToLower(line), "gitlab") {
		s.GitlabLines++
	}
}

// isTestFunctionName reports whether name follows Go's Test* entry-point
// rules: starts with "Test" and the next rune is uppercase (or there is no
// next rune). It excludes "TestMain", which is the framework entry point
// for _test.go packages and is not itself a test. This matches the
// behavior of cmd/gen_testing_docs so the two generators report the
// same counts.
func isTestFunctionName(name string) bool {
	if name == "TestMain" {
		return false
	}
	if !strings.HasPrefix(name, "Test") {
		return false
	}
	if len(name) == len("Test") {
		return true
	}
	return unicode.IsUpper(rune(name[len("Test")]))
}

// isTODOComment reports whether trimmed is a task-annotation comment.
// It requires a word boundary after the marker so that identifiers like
// "TodoOutput" or "toDomainOutput" are not mistaken for task annotations.
func isTODOComment(trimmed string) bool {
	if !strings.HasPrefix(trimmed, "//") {
		return false
	}
	keyword := strings.ToUpper(strings.TrimLeft(trimmed[2:], " \t"))
	for _, marker := range []string{"TODO", "FIXME", "HACK"} {
		if strings.HasPrefix(keyword, marker) {
			rest := keyword[len(marker):]
			if rest == "" || (!unicode.IsLetter(rune(rest[0])) && rest[0] != '_') {
				return true
			}
		}
	}
	return false
}

// parseDeps counts direct and indirect dependencies declared in go.mod.
func parseDeps(path string) (direct, indirect int) {
	data, err := os.ReadFile(filepath.Clean(path)) //#nosec G304 -- path is a compile-time constant
	if err != nil {
		return 0, 0
	}
	inRequire := false
	for raw := range strings.SplitSeq(string(data), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "require (":
			inRequire = true
		case line == ")" && inRequire:
			inRequire = false
		case inRequire && line != "" && !strings.HasPrefix(line, "//"):
			classifyDep(line, &direct, &indirect)
		case strings.HasPrefix(line, "require ") && !strings.HasPrefix(line, "require ("):
			classifyDep(line, &direct, &indirect)
		}
	}
	return direct, indirect
}

// classifyDep increments indirect if line ends with "// indirect", direct otherwise.
func classifyDep(line string, direct, indirect *int) {
	if strings.HasSuffix(line, "// indirect") {
		*indirect++
	} else {
		*direct++
	}
}

// renderStats builds the Markdown tables for the <!-- START STATS --> section.
func renderStats(s *repoStats) string {
	totalFiles := s.SourceFiles + s.UnitTestFiles + s.E2ETestFiles
	totalLines := s.SourceLines + s.UnitTestLines + s.E2ETestLines

	testRatio := 0.0
	if s.SourceLines > 0 {
		testRatio = float64(s.UnitTestLines) / float64(s.SourceLines)
	}
	srcFuncs := s.ExportedFuncs + s.UnexportedFuncs
	testPerFunc := 0.0
	if srcFuncs > 0 {
		testPerFunc = float64(s.TestFuncs) / float64(srcFuncs)
	}
	avgSrc := 0
	if s.SourceFiles > 0 {
		avgSrc = s.SourceLines / s.SourceFiles
	}
	avgTest := 0
	if s.UnitTestFiles > 0 {
		avgTest = s.UnitTestLines / s.UnitTestFiles
	}
	commentPct := 0.0
	if s.SourceLines > 0 {
		commentPct = float64(s.CommentLines) / float64(s.SourceLines) * 100
	}

	var b strings.Builder

	b.WriteString("### File counts\n\n")
	b.WriteString(docgen.RenderMarkdownTable(
		[]string{"Category", "Files", "Lines"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight},
		[][]string{
			{"Source (`.go`, non-test)", fmtInt(s.SourceFiles), fmtInt(s.SourceLines)},
			{"Unit tests (`_test.go`)", fmtInt(s.UnitTestFiles), fmtInt(s.UnitTestLines)},
			{"End-to-end tests", fmtInt(s.E2ETestFiles), fmtInt(s.E2ETestLines)},
			{"**Total**", "**" + fmtInt(totalFiles) + "**", "**" + fmtInt(totalLines) + "**"},
		},
	))
	b.WriteByte('\n')

	b.WriteString("### Functions\n\n")
	b.WriteString(docgen.RenderMarkdownTable(
		[]string{"Category", "Count"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignRight},
		[][]string{
			{"Source functions", fmtInt(srcFuncs)},
			{". Exported (public)", fmtInt(s.ExportedFuncs)},
			{". Unexported (private)", fmtInt(s.UnexportedFuncs)},
			{"Unit test functions (`TestXxx`)", fmtInt(s.TestFuncs)},
			{"Subtests (`t.Run(...)`)", fmtInt(s.Subtests)},
			{"End-to-end test functions", fmtInt(s.E2ETestFuncs)},
		},
	))
	b.WriteByte('\n')

	b.WriteString("### Ratios worth noting\n\n")
	b.WriteString(docgen.RenderMarkdownTable(
		[]string{"Observation", "Value"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignRight},
		[][]string{
			{"Test lines vs source lines", fmt.Sprintf("%.2f\u00d7 more tests than code", testRatio)},
			{"Average source file length", "~" + fmtInt(avgSrc) + " lines"},
			{"Average test file length", "~" + fmtInt(avgTest) + " lines"},
			{"Comment lines in source", fmt.Sprintf("%s (~%.1f%% of source)", fmtInt(s.CommentLines), commentPct)},
			{"Test functions per source function", fmt.Sprintf("%.1f\u00d7", testPerFunc)},
		},
	))
	b.WriteByte('\n')

	b.WriteString("### Code patterns\n\n")
	b.WriteString(docgen.RenderMarkdownTable(
		[]string{"Pattern", "Count"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignRight},
		[][]string{
			{"`if err != nil` checks", fmtInt(s.ErrChecks)},
			{"`defer` statements", fmtInt(s.DeferStmts)},
			{"`struct` types defined", fmtInt(s.StructTypes)},
			{"`//nolint` suppressions", fmtInt(s.Nolints)},
			{"`TODO` / `FIXME` / `HACK` comments", fmtInt(s.TODOs)},
		},
	))
	b.WriteByte('\n')

	b.WriteString("### Project\n\n")
	b.WriteString(docgen.RenderMarkdownTable(
		[]string{"Metric", "Value"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignRight},
		[][]string{
			{"Go packages", fmtInt(s.Packages)},
			{"Direct dependencies (`go.mod`)", fmtInt(s.DirectDeps)},
			{"Indirect dependencies", fmtInt(s.IndirectDeps)},
		},
	))
	b.WriteByte('\n')

	b.WriteString("### Hall of fame\n\n")
	b.WriteString(docgen.RenderMarkdownTable(
		[]string{"Record", "File"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignLeft},
		[][]string{
			{"Longest source file", fmt.Sprintf("`%s`. %s lines", s.LargestSrcFile, fmtInt(s.LargestSrcLines))},
			{"Longest test file", fmt.Sprintf("`%s`. %s lines", s.LargestTestFile, fmtInt(s.LargestTestLines))},
		},
	))
	b.WriteByte('\n')

	b.WriteString("### Because why not\n\n")
	b.WriteString(docgen.RenderMarkdownTable(
		[]string{"Fact", "Value"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignLeft},
		[][]string{
			{"Source code printed at 55 lines/page", "~" + fmtInt(s.SourceLines/linesPerPage) + " pages of A4"},
			{"Source lines mentioning `\"gitlab\"`", fmtInt(s.GitlabLines) + " (impossible to avoid)"},
			{"Longest function name in source", fmt.Sprintf("`%s` (%d chars)", s.LongestFuncName, len(s.LongestFuncName))},
			{"Longest test function name", fmt.Sprintf("`%s` (%d chars)", s.LongestTestName, len(s.LongestTestName))},
		},
	))

	return b.String()
}

// fmtInt formats n with comma thousands separators.
func fmtInt(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var buf []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, c)
	}
	return string(buf)
}
