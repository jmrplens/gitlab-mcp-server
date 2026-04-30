// stats.go collects filesystem-level codebase metrics used by gen_readme to
// auto-generate the Unnecessary Statistics section of the README.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// Marker constants for the stats section of README.md.
const (
	statsStartMarker = "<!-- START STATS -->"
	statsEndMarker   = "<!-- END STATS -->"
	linesPerPage     = 55 // approximate readable lines per A4 page at 12pt

	// tableAlignRight is the Markdown separator row for a two-column table
	// with the second column right-aligned.
	tableAlignRight = "| --- | ---: |\n"
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

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
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
	s.CommitCount = gitRevCount()
	s.ContributorCount = gitContributors()
	return s, nil
}

// scanGoFile reads every line of a .go file and accumulates pattern-based
// counters into s. Returns the total line count.
func scanGoFile(path string, isE2E, isTest bool, s *repoStats) (int, error) {
	f, err := os.Open(filepath.Clean(path)) //#nosec G304 -- path from filepath.Walk within repo
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var lines int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines++
		line := sc.Text()
		trimmed := strings.TrimSpace(line)

		// Function declarations.
		if strings.HasPrefix(trimmed, "func ") {
			name := extractFuncName(trimmed)
			switch {
			case isE2E && strings.HasPrefix(name, "Test"):
				s.E2ETestFuncs++
			case isTest && strings.HasPrefix(name, "Test"):
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

		// Subtests (all test files).
		if (isTest || isE2E) && strings.Contains(line, "t.Run(") {
			s.Subtests++
		}

		// Patterns counted across all files.
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

		// Source-only metrics.
		if !isTest && !isE2E {
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
	}
	return lines, sc.Err()
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

func isTODOComment(trimmed string) bool {
	up := strings.ToUpper(trimmed)
	return strings.HasPrefix(up, "// TODO") ||
		strings.HasPrefix(up, "// FIXME") ||
		strings.HasPrefix(up, "// HACK")
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

func gitRevCount() int {
	bin, err := gitBin()
	if err != nil {
		return 0
	}
	out, err := exec.CommandContext(context.Background(), bin, "rev-list", "--count", "HEAD").Output() //#nosec G204 -- absolute path from LookPath, fixed args
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}

func gitContributors() int {
	bin, err := gitBin()
	if err != nil {
		return 0
	}
	out, err := exec.CommandContext(context.Background(), bin, "log", "--format=%aE").Output() //#nosec G204 -- absolute path from LookPath, fixed args; %aE uses .mailmap
	if err != nil {
		return 0
	}
	emails := make(map[string]bool)
	for line := range strings.SplitSeq(string(out), "\n") {
		if e := strings.TrimSpace(line); e != "" {
			emails[e] = true
		}
	}
	return len(emails)
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
	b.WriteString("| Category | Files | Lines |\n")
	b.WriteString("| --- | ---: | ---: |\n")
	fmt.Fprintf(&b, "| Source (`.go`, non-test) | %s | %s |\n", fmtInt(s.SourceFiles), fmtInt(s.SourceLines))
	fmt.Fprintf(&b, "| Unit tests (`_test.go`) | %s | %s |\n", fmtInt(s.UnitTestFiles), fmtInt(s.UnitTestLines))
	fmt.Fprintf(&b, "| End-to-end tests | %s | %s |\n", fmtInt(s.E2ETestFiles), fmtInt(s.E2ETestLines))
	fmt.Fprintf(&b, "| **Total** | **%s** | **%s** |\n\n", fmtInt(totalFiles), fmtInt(totalLines))

	b.WriteString("### Functions\n\n")
	b.WriteString("| Category | Count |\n")
	b.WriteString(tableAlignRight)
	fmt.Fprintf(&b, "| Source functions | %s |\n", fmtInt(srcFuncs))
	fmt.Fprintf(&b, "| — exported (public) | %s |\n", fmtInt(s.ExportedFuncs))
	fmt.Fprintf(&b, "| — unexported (private) | %s |\n", fmtInt(s.UnexportedFuncs))
	fmt.Fprintf(&b, "| Unit test functions (`TestXxx`) | %s |\n", fmtInt(s.TestFuncs))
	fmt.Fprintf(&b, "| Subtests (`t.Run(...)`) | %s |\n", fmtInt(s.Subtests))
	fmt.Fprintf(&b, "| End-to-end test functions | %s |\n\n", fmtInt(s.E2ETestFuncs))

	b.WriteString("### Ratios worth noting\n\n")
	b.WriteString("| Observation | Value |\n")
	b.WriteString(tableAlignRight)
	fmt.Fprintf(&b, "| Test lines vs source lines | %.2f× more tests than code |\n", testRatio)
	fmt.Fprintf(&b, "| Average source file length | ~%s lines |\n", fmtInt(avgSrc))
	fmt.Fprintf(&b, "| Average test file length | ~%s lines |\n", fmtInt(avgTest))
	fmt.Fprintf(&b, "| Comment lines in source | %s (~%.1f%% of source) |\n", fmtInt(s.CommentLines), commentPct)
	fmt.Fprintf(&b, "| Test functions per source function | %.1f× |\n\n", testPerFunc)

	b.WriteString("### Code patterns\n\n")
	b.WriteString("| Pattern | Count |\n")
	b.WriteString(tableAlignRight)
	fmt.Fprintf(&b, "| `if err != nil` checks | %s |\n", fmtInt(s.ErrChecks))
	fmt.Fprintf(&b, "| `defer` statements | %s |\n", fmtInt(s.DeferStmts))
	fmt.Fprintf(&b, "| `struct` types defined | %s |\n", fmtInt(s.StructTypes))
	fmt.Fprintf(&b, "| `//nolint` suppressions | %s |\n", fmtInt(s.Nolints))
	fmt.Fprintf(&b, "| `TODO` / `FIXME` / `HACK` comments | %s |\n\n", fmtInt(s.TODOs))

	b.WriteString("### Project\n\n")
	b.WriteString("| Metric | Value |\n")
	b.WriteString(tableAlignRight)
	fmt.Fprintf(&b, "| Go packages | %s |\n", fmtInt(s.Packages))
	fmt.Fprintf(&b, "| Direct dependencies (`go.mod`) | %s |\n", fmtInt(s.DirectDeps))
	fmt.Fprintf(&b, "| Indirect dependencies | %s |\n", fmtInt(s.IndirectDeps))
	fmt.Fprintf(&b, "| Git commits | %s |\n", fmtInt(s.CommitCount))
	fmt.Fprintf(&b, "| Unique contributors | %s |\n\n", fmtInt(s.ContributorCount))

	b.WriteString("### Hall of fame\n\n")
	b.WriteString("| Record | File |\n")
	b.WriteString("| --- | --- |\n")
	fmt.Fprintf(&b, "| Longest source file | `%s` — %s lines |\n", s.LargestSrcFile, fmtInt(s.LargestSrcLines))
	fmt.Fprintf(&b, "| Longest test file | `%s` — %s lines |\n\n", s.LargestTestFile, fmtInt(s.LargestTestLines))

	b.WriteString("### Because why not\n\n")
	b.WriteString("| Fact | Value |\n")
	b.WriteString("| --- | --- |\n")
	fmt.Fprintf(&b, "| Source code printed at 55 lines/page | ~%s pages of A4 |\n", fmtInt(s.SourceLines/linesPerPage))
	fmt.Fprintf(&b, "| Source lines mentioning `\"gitlab\"` | %s (impossible to avoid) |\n", fmtInt(s.GitlabLines))
	fmt.Fprintf(&b, "| Longest function name in source | `%s` (%d chars) |\n", s.LongestFuncName, len(s.LongestFuncName))
	fmt.Fprintf(&b, "| Longest test function name | `%s` (%d chars) |\n", s.LongestTestName, len(s.LongestTestName))

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
