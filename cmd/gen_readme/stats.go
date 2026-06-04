package main

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/docgen"
)

// Marker constants for the stats section of README.md.
const (
	statsStartMarker = "<!-- START STATS -->"
	statsEndMarker   = "<!-- END STATS -->"
	linesPerPage     = 55 // approximate readable lines per A4 page at 12pt

	// scannerBufSize is the initial and maximum bufio.Scanner token size.
	// 512 KB handles the largest generated Go files without reallocating.
	scannerBufSize = 512 * 1024
)

// repoStats accumulates filesystem-level measurements of the Go codebase.
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
	Packages         int
	DirectDeps       int
	IndirectDeps     int
	CommitCount      int
	ContributorCount int
}

// collectStats walks root, classifies every .go file, and returns a populated
// repoStats. root should be the repository root directory.
func collectStats(root string) (*repoStats, error) {
	s := &repoStats{}
	dirs := make(map[string]bool)

	// WalkDir is used instead of Walk: it receives fs.DirEntry directly from
	// the OS directory read, avoiding the extra os.Lstat call Walk performs
	// for every entry.
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		rel := filepath.ToSlash(path)
		isE2E := strings.Contains(rel, "/e2e/")
		isTest := strings.HasSuffix(path, "_test.go")

		dirs[filepath.Dir(path)] = true

		lines, scanErr := scanGoFile(path, isE2E, isTest, s)
		if scanErr != nil {
			return fmt.Errorf("scanning %s: %w", path, scanErr)
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
				s.LargestTestFile = strings.TrimPrefix(rel, "./")
			}
		default:
			s.SourceFiles++
			s.SourceLines += lines
			if lines > s.LargestSrcLines {
				s.LargestSrcLines = lines
				s.LargestSrcFile = strings.TrimPrefix(rel, "./")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.Packages = len(dirs)
	s.DirectDeps, s.IndirectDeps = parseDeps(filepath.Join(root, "go.mod"))

	var gitErr error
	if s.CommitCount, gitErr = gitRevCount(); gitErr != nil {
		fmt.Fprintf(os.Stderr, "warning: git rev count: %v\n", gitErr)
	}
	if s.ContributorCount, gitErr = gitContributors(); gitErr != nil {
		fmt.Fprintf(os.Stderr, "warning: git contributors: %v\n", gitErr)
	}
	return s, nil
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

	for sc.Scan() {
		lines++
		scanGoLine(sc.Text(), isE2E, isTest, s)
	}
	return lines, sc.Err()
}

func scanGoLine(line string, isE2E, isTest bool, s *repoStats) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "func ") {
		updateFunctionStats(extractFuncName(trimmed), isE2E, isTest, s)
	}
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
	if strings.HasPrefix(trimmed, "type ") && strings.Contains(trimmed, "struct") {
		s.StructTypes++
	}
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

// extractFuncName returns the identifier from a trimmed "func ..." line.
// Handles methods (func (r *T) Name(...)) and plain functions (func Name(...)).
func extractFuncName(trimmed string) string {
	rest := strings.TrimPrefix(trimmed, "func ")
	if strings.HasPrefix(rest, "(") {
		depth := 0
		for i, c := range rest {
			switch c {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					after := strings.TrimSpace(rest[i+1:])
					if idx := strings.IndexByte(after, '('); idx > 0 {
						return after[:idx]
					}
					return ""
				}
			}
		}
		return ""
	}
	if idx := strings.IndexByte(rest, '('); idx > 0 {
		return rest[:idx]
	}
	return ""
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

// gitBin resolves the absolute path of the git executable so downstream calls
// use a fixed path instead of relying on PATH lookup at runtime.
func gitBin() (string, error) {
	return exec.LookPath("git") //#nosec G204 -- resolves to an absolute path; no user input involved
}

func gitRevCount() (int, error) {
	bin, err := gitBin()
	if err != nil {
		return 0, err
	}
	out, err := exec.CommandContext(context.Background(), bin, "rev-list", "--count", "HEAD").Output() //#nosec G204 -- absolute path from LookPath, fixed args
	if err != nil {
		return 0, fmt.Errorf("rev-list: %w", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("parsing rev-list output %q: %w", strings.TrimSpace(string(out)), err)
	}
	return n, nil
}

func gitContributors() (int, error) {
	bin, err := gitBin()
	if err != nil {
		return 0, err
	}
	out, err := exec.CommandContext(context.Background(), bin, "log", "--format=%aE").Output() //#nosec G204 -- absolute path from LookPath, fixed args; %aE uses .mailmap
	if err != nil {
		return 0, fmt.Errorf("git log: %w", err)
	}
	emails := make(map[string]bool)
	for line := range strings.SplitSeq(string(out), "\n") {
		if e := strings.TrimSpace(line); e != "" {
			emails[e] = true
		}
	}
	return len(emails), nil
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
			{"— exported (public)", fmtInt(s.ExportedFuncs)},
			{"— unexported (private)", fmtInt(s.UnexportedFuncs)},
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
			{"Test lines vs source lines", fmt.Sprintf("%.2f× more tests than code", testRatio)},
			{"Average source file length", "~" + fmtInt(avgSrc) + " lines"},
			{"Average test file length", "~" + fmtInt(avgTest) + " lines"},
			{"Comment lines in source", fmt.Sprintf("%s (~%.1f%% of source)", fmtInt(s.CommentLines), commentPct)},
			{"Test functions per source function", fmt.Sprintf("%.1f×", testPerFunc)},
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
			{"Git commits", fmtInt(s.CommitCount)},
			{"Unique contributors", fmtInt(s.ContributorCount)},
		},
	))
	b.WriteByte('\n')

	b.WriteString("### Hall of fame\n\n")
	b.WriteString(docgen.RenderMarkdownTable(
		[]string{"Record", "File"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignLeft},
		[][]string{
			{"Longest source file", fmt.Sprintf("`%s` — %s lines", s.LargestSrcFile, fmtInt(s.LargestSrcLines))},
			{"Longest test file", fmt.Sprintf("`%s` — %s lines", s.LargestTestFile, fmtInt(s.LargestTestLines))},
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
