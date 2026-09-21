package main

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionids"
)

// The hint fixer: the one rewrite the report's tool-name rule has an exact
// answer for.
//
// A hint naming gitlab_project_get names a tool that only one surface of three
// registers, and the individual surface publishes exactly one tool per action,
// so the catalog can say what should have been written: project.get. That is a
// substitution, and at 785 sites across 137 packages it is the shape the
// repository's own rule about scripts allows one for.
//
// What makes it safe is not the substitution but what it is applied to. The
// same token is CORRECT in an individual tool's Description ("See also:
// gitlab_project_get"), which sits in the same file and often the same
// declaration block as the hint. So a literal is rewritten only when its text
// is part of a hint the walk actually folded: a Description is not, and cannot
// be reached by this pass however often it spells the same tool.

// writeSource is how a rewritten file reaches the disk, swapped in tests.
//
// The seam exists for one branch that real input cannot reach: the file was
// listed, read and parsed a moment earlier, so the write fails only for
// something outside this run's control. It is the one failure this pass must
// not absorb, because a rewrite that carried on past it would report a clean
// run over a tree it had half finished, and the next thing a reader does is
// trust that report instead of the diff.
var writeSource = os.WriteFile

// hintFix is one rewritten token, reported so a reviewer reads what moved
// rather than only how much.
type hintFix struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Tool string `json:"tool"`
	ID   string `json:"action_id"`
}

// hintFixReport is what one fixing run did and what it could not do.
type hintFixReport struct {
	// Files is how many files were rewritten, Fixes every token replaced.
	Files int       `json:"files_rewritten"`
	Fixes []hintFix `json:"fixes"`
	// Unresolved is a tool name in a hint that the catalog does not publish,
	// which the fixer leaves alone: it has no answer for it, and writing a
	// guess into served prose is worse than leaving the finding.
	Unresolved []hintFix `json:"unresolved,omitempty"`
}

// fixHints rewrites the tool names in every hint literal under the audited
// packages and returns what it did.
//
// It works from the hint values the walk folded, so a literal is a candidate
// only when its text is part of one. Whole files are parsed rather than
// searched, because the unit being decided about is the string literal and a
// text scan cannot tell one from the identifier beside it; and each rewrite is
// applied to the file's own bytes at the literal's offsets rather than through
// a printer, so nothing else in the file moves.
func fixHints(dir string, sites []site, ids *actionids.IDs, includeTests bool) (hintFixReport, error) {
	report := hintFixReport{}
	packages, values := hintValuesByPackage(sites)
	for _, pkg := range packages {
		if err := fixHintsInPackage(dir, pkg, values[pkg], ids, includeTests, &report); err != nil {
			return report, err
		}
	}
	return report, nil
}

// fixHintsInPackage rewrites one package: its production files first, and its
// test files after, under the same rule.
//
// The order is not cosmetic. What decides whether a literal belongs to a hint
// is the hint text the walk folded, and the production rewrite is what makes
// that text stop naming the tool, so a test pass run afterwards would find
// nothing to match. They have to be one pass over one set of values.
func fixHintsInPackage(dir, pkg string, values []string, ids *actionids.IDs, includeTests bool, report *hintFixReport) error {
	admit := hintProse(values)
	production, tests, err := goFilesOf(filepath.Join(dir, filepath.FromSlash(pkg)))
	if err != nil {
		return err
	}
	rewritten := 0
	for _, file := range production {
		fixes, fixErr := fixOneFile(dir, file, admit, ids, report)
		if fixErr != nil {
			return fixErr
		}
		rewritten += fixes
	}
	if !includeTests || rewritten == 0 {
		return nil
	}
	for _, file := range tests {
		if _, fixErr := fixOneFile(dir, file, admit, ids, report); fixErr != nil {
			return fixErr
		}
	}
	return nil
}

// fixOneFile rewrites one file into the report and says how many names moved.
func fixOneFile(dir, file string, admit candidate, ids *actionids.IDs, report *hintFixReport) (int, error) {
	changed, fixes, unresolved, err := fixHintsInFile(dir, file, admit, ids)
	if err != nil {
		return 0, err
	}
	report.Fixes = append(report.Fixes, fixes...)
	report.Unresolved = append(report.Unresolved, unresolved...)
	if changed {
		report.Files++
	}
	return len(fixes), nil
}

// candidate decides which string literals a pass rewrites. The production pass
// and the test pass ask different questions of the same file shape, and the
// difference is the whole reason both are safe.
type candidate func(text string) bool

// hintProse admits a literal that is a piece of prose belonging to some hint
// the walk folded, which is the production rule.
func hintProse(values []string) candidate {
	return func(text string) bool { return partOfAHint(text, values) }
}

// A test file is held to the SAME prose rule as production, and the narrower
// rule that suggests itself does not work. Admitting any test literal that
// spells a tool name the package's own hints had just stopped spelling looks
// exact and is not: a test asserts an individual tool's name as often as it
// asserts a hint, in the same file and with the same literal, and measured on
// this tree that rule turned 98 failing assertions into 351. The suite caught
// it, which is the argument for running it rather than for keeping the rule.
// What the prose rule leaves behind is the assertion written as the bare
// token, and that one is a judgement each time: whether the sentence moved or
// the name did is not something the text says.

// hintValuesByPackage groups every hint text the walk folded under the package
// it was written in, and returns the packages in order so a run rewrites the
// same files in the same order twice.
//
// Package scope rather than file scope, because a hint is commonly a const
// shared by a dozen handlers and the site the walk recorded is the call rather
// than the declaration, which is in whichever file of the package its author
// put it.
//
// One pass, and the reason is the rule rather than the cost. This was a pair of
// functions, one choosing the packages and one collecting their values, and
// both applied the same four-term filter. The second hid the first: a site the
// package filter should have rejected was let through by a flipped operator,
// collected no values, and was skipped one line later for having none, so
// mutation testing reported four survivors no test could ever have killed.
// With one filter each term decides something a caller can see.
func hintValuesByPackage(sites []site) (packages []string, values map[string][]string) {
	values = map[string][]string{}
	for _, at := range sites {
		if !isHintKind(at.Kind) || !at.Resolved || at.Package == "" || at.Value == "" {
			continue
		}
		if _, seen := values[at.Package]; !seen {
			packages = append(packages, at.Package)
		}
		values[at.Package] = append(values[at.Package], at.Value)
	}
	sort.Strings(packages)
	return packages, values
}

// goFilesOf splits one directory's Go source into the production files and
// the test files, each sorted, so a run rewrites the same files in the same
// order twice.
//
// One read rather than one per half. The two halves are answered from the same
// listing, so they cannot disagree about what is in the directory, and there is
// one place a directory that cannot be read is reported rather than two, the
// second of which could only fire if the directory vanished between them.
func goFilesOf(dir string) (production, tests []string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", dir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			tests = append(tests, filepath.Join(dir, name))
			continue
		}
		production = append(production, filepath.Join(dir, name))
	}
	sort.Strings(production)
	sort.Strings(tests)
	return production, tests, nil
}

// replacement is one token rewritten inside one literal, kept as byte offsets
// into the file so the edits can be applied from the end backwards.
type replacement struct {
	start int
	end   int
	with  string
}

// fixHintsInFile rewrites one file and says what it changed.
func fixHintsInFile(root, file string, admit candidate, ids *actionids.IDs) (changed bool, fixes, unresolved []hintFix, err error) {
	source, readErr := os.ReadFile(file) //#nosec G304 -- a Go file of a package this run audits
	if readErr != nil {
		return false, nil, nil, fmt.Errorf("read %s: %w", file, readErr)
	}
	fset := token.NewFileSet()
	parsed, parseErr := parser.ParseFile(fset, file, source, parser.ParseComments)
	if parseErr != nil {
		return false, nil, nil, fmt.Errorf("parse %s: %w", file, parseErr)
	}

	shown := relativePath(file, root)
	var edits []replacement
	ast.Inspect(parsed, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		// The unquote answers for a literal the parser accepted, so its error
		// is a branch no source reaches; the empty string it would leave is
		// refused by admit anyway, which is why discarding it changes nothing
		// rather than hiding something.
		text, _ := strconv.Unquote(lit.Value)
		if !admit(text) {
			return true
		}
		line := fset.Position(lit.Pos()).Line
		start := fset.Position(lit.Pos()).Offset
		for _, match := range toolToken.FindAllStringIndex(lit.Value, -1) {
			tool := lit.Value[match[0]:match[1]]
			id, known := ids.ToolID(tool)
			if !known {
				unresolved = append(unresolved, hintFix{File: shown, Line: line, Tool: tool})
				continue
			}
			edits = append(edits, replacement{start: start + match[0], end: start + match[1], with: id})
			fixes = append(fixes, hintFix{File: shown, Line: line, Tool: tool, ID: id})
		}
		return true
	})
	if len(edits) == 0 {
		return false, nil, unresolved, nil
	}
	// Ascending, into a fresh buffer: an in-place splice would have every edit
	// after the first one reading offsets into an array the previous edit had
	// already moved, and the descending order that hides that is a property of
	// the loop rather than of the data.
	//
	// Ordered by a comparison rather than by a "less", because no two edits
	// share a start: they are distinct positions in one file, so < and <= are
	// the same function here and mutation testing reports a survivor no test
	// could kill.
	slices.SortFunc(edits, func(left, right replacement) int { return cmp.Compare(left.start, right.start) })
	rewritten := make([]byte, 0, len(source))
	written := 0
	for _, edit := range edits {
		rewritten = append(rewritten, source[written:edit.start]...)
		rewritten = append(rewritten, edit.with...)
		written = edit.end
	}
	rewritten = append(rewritten, source[written:]...)
	if writeErr := writeSource(file, rewritten, 0o600); writeErr != nil {
		return false, nil, nil, fmt.Errorf("write %s: %w", file, writeErr)
	}
	return true, fixes, unresolved, nil
}

// partOfAHint reports whether a literal's text is a piece of prose belonging
// to some hint the walk folded.
//
// Containment rather than equality, because a hint is often assembled from
// pieces: a format string, a const prefix, a suffix beside it. Each piece is
// contained in the folded value, and the piece carrying the tool name is the
// one this pass is for.
//
// The prose test is what makes containment safe, and it was found by running
// without it. A tool name is also written as a literal that is nothing but the
// name (`toolProjectDelete = "gitlab_project_delete"`, and the name argument
// every spec builder takes), and such a literal is contained in every hint
// that mentions that tool, so the first run rewrote 48 lines of
// internal/tools/projects/action_specs.go and renamed the tools themselves. A
// hint always reads as a sentence and a name never does, so a literal with no
// space in it is not one this pass has anything to say about. Where a hint is
// assembled as prose plus the name constant, the token stays and the finding
// stays with it, which is the right answer: rewriting the constant there would
// rename the tool to fix the sentence.
func partOfAHint(text string, values []string) bool {
	if strings.TrimSpace(text) == "" || !strings.Contains(text, " ") {
		return false
	}
	for _, value := range values {
		if strings.Contains(value, text) {
			return true
		}
	}
	return false
}

// writeHintFixReport says what a fixing run moved, per package, and what it
// left alone.
//
// Per package rather than per site, because 785 lines is a list a reviewer
// scrolls past: what a reader wants before reading the diff is which packages
// moved and whether anything was left behind.
func writeHintFixReport(out io.Writer, report hintFixReport) {
	fmt.Fprintf(out, "%s: rewrote %d tool name(s) in %d file(s)\n", toolName, len(report.Fixes), report.Files)
	byPackage := map[string]int{}
	for _, fix := range report.Fixes {
		byPackage[path.Dir(fix.File)]++
	}
	for _, pkg := range sortedKeys(byPackage) {
		fmt.Fprintf(out, "  %-56s %d\n", pkg, byPackage[pkg])
	}
	if len(report.Unresolved) == 0 {
		return
	}
	fmt.Fprintf(out, "  %d tool name(s) left alone: the catalog publishes no such tool\n", len(report.Unresolved))
	for _, left := range report.Unresolved {
		fmt.Fprintf(out, "    %s:%d %s\n", left.File, left.Line, left.Tool)
	}
}

// sortedKeys is the keys of a count in order, so two runs print the same
// report.
func sortedKeys(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
