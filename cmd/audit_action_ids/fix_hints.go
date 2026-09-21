package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path"
	"path/filepath"
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
	for _, pkg := range packagesWithHints(sites) {
		values := hintValues(sites, pkg)
		if len(values) == 0 {
			continue
		}
		files, err := goFilesOf(filepath.Join(dir, filepath.FromSlash(pkg)), includeTests)
		if err != nil {
			return report, err
		}
		for _, file := range files {
			changed, fixes, unresolved, fixErr := fixHintsInFile(dir, file, values, ids)
			if fixErr != nil {
				return report, fixErr
			}
			report.Fixes = append(report.Fixes, fixes...)
			report.Unresolved = append(report.Unresolved, unresolved...)
			if changed {
				report.Files++
			}
		}
	}
	return report, nil
}

// packagesWithHints is every audited package that folded at least one hint,
// in order, so a run rewrites the same files in the same order twice.
func packagesWithHints(sites []site) []string {
	seen := map[string]struct{}{}
	var packages []string
	for _, at := range sites {
		if !isHintKind(at.Kind) || !at.Resolved || at.Package == "" {
			continue
		}
		if _, recorded := seen[at.Package]; recorded {
			continue
		}
		seen[at.Package] = struct{}{}
		packages = append(packages, at.Package)
	}
	sort.Strings(packages)
	return packages
}

// hintValues is every hint text one package folded.
//
// Package scope rather than file scope, because a hint is commonly a const
// shared by a dozen handlers and the site the walk recorded is the call rather
// than the declaration, which is in whichever file of the package its author
// put it.
func hintValues(sites []site, pkg string) []string {
	var values []string
	for _, at := range sites {
		if at.Package != pkg || !isHintKind(at.Kind) || !at.Resolved || at.Value == "" {
			continue
		}
		values = append(values, at.Value)
	}
	return values
}

// goFilesOf is the Go source of one directory, sorted, without descending.
func goFilesOf(dir string, includeTests bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if !includeTests && strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	return files, nil
}

// replacement is one token rewritten inside one literal, kept as byte offsets
// into the file so the edits can be applied from the end backwards.
type replacement struct {
	start int
	end   int
	with  string
}

// fixHintsInFile rewrites one file and says what it changed.
func fixHintsInFile(root, file string, values []string, ids *actionids.IDs) (bool, []hintFix, []hintFix, error) {
	source, err := os.ReadFile(file) //#nosec G304 -- a Go file of a package this run audits
	if err != nil {
		return false, nil, nil, fmt.Errorf("read %s: %w", file, err)
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, source, parser.ParseComments)
	if err != nil {
		return false, nil, nil, fmt.Errorf("parse %s: %w", file, err)
	}

	shown := relativePath(file, root)
	var edits []replacement
	var fixes, unresolved []hintFix
	ast.Inspect(parsed, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, unquoteErr := strconv.Unquote(lit.Value)
		if unquoteErr != nil || !partOfAHint(text, values) {
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
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	rewritten := make([]byte, 0, len(source))
	written := 0
	for _, edit := range edits {
		rewritten = append(rewritten, source[written:edit.start]...)
		rewritten = append(rewritten, edit.with...)
		written = edit.end
	}
	rewritten = append(rewritten, source[written:]...)
	if writeErr := os.WriteFile(file, rewritten, 0o600); writeErr != nil {
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
