// Command audit_test_names scans all Go test files and classifies test
// function names by their naming pattern. It outputs a CSV report with
// columns: file, current_name, pattern, suggested_name.
//
// Usage:
//
//	go run ./cmd/audit_test_names/ <dir>...

package main

import (
	"encoding/csv"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// Pattern classifications for test function names.
const (
	Pattern3Part        = "3-part"
	Pattern2Part        = "2-part"
	PatternNoUnderscore = "no-underscore"
	PatternTestCov      = "TestCov"
	PatternOther        = "other"
	PatternSkip         = "skip"
)

// testEntry holds the audit result for a single test function.
type testEntry struct {
	File          string
	CurrentName   string
	Pattern       string
	SuggestedName string
}

var (
	// covPattern matches TestCov* prefixed tests.
	covPattern = regexp.MustCompile(`^TestCov[A-Z]`)

	// e2eWorkflow matches top-level E2E workflow tests that should be skipped.
	e2eWorkflow = regexp.MustCompile(`^Test(FullWorkflow|MetaToolWorkflow)$`)
)

// main audits test function naming convention compliance across the project.
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/audit_test_names/ <dir>...")
		os.Exit(1)
	}

	entries := make([]testEntry, 0, len(os.Args[1:])*10)
	for _, dir := range os.Args[1:] {
		entries = append(entries, scanDir(dir)...)
	}

	w := csv.NewWriter(os.Stdout)
	_ = w.Write([]string{"file", "current_name", "pattern", "suggested_name"})
	for _, e := range entries {
		_ = w.Write([]string{e.File, e.CurrentName, e.Pattern, e.SuggestedName})
	}
	w.Flush()

	// Print summary to stderr.
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.Pattern]++
	}
	fmt.Fprintf(os.Stderr, "\n=== Test Naming Audit Summary ===\n")
	fmt.Fprintf(os.Stderr, "Total test functions: %d\n", len(entries))
	for _, p := range []string{Pattern3Part, Pattern2Part, PatternNoUnderscore, PatternTestCov, PatternOther, PatternSkip} {
		if c, ok := counts[p]; ok {
			fmt.Fprintf(os.Stderr, "  %-16s %d\n", p+":", c)
		}
	}
}

// scanDir recursively scans a directory for test files and classifies test names.
func scanDir(dir string) []testEntry {
	cleanDir := filepath.Clean(dir)
	entries, err := os.ReadDir(cleanDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "readdir %s: %v\n", cleanDir, err)
		return nil
	}

	var results []testEntry
	for _, e := range entries {
		path := filepath.Join(cleanDir, e.Name())
		if e.IsDir() {
			results = append(results, scanDir(path)...)
			continue
		}
		if strings.HasSuffix(e.Name(), "_test.go") {
			results = append(results, scanFile(path)...)
		}
	}
	return results
}

// scanFile parses a single test file and classifies each Test* function.
func scanFile(path string) []testEntry {
	cleanPath := filepath.Clean(path)
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, cleanPath, nil, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", cleanPath, err)
		return nil
	}

	// Use forward-slash paths for consistent CSV output.
	relPath := filepath.ToSlash(cleanPath)

	var results []testEntry
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		name := fn.Name.Name

		// Only Test* functions (exported, starts with Test).
		if !strings.HasPrefix(name, "Test") {
			continue
		}
		// Skip lowercase test helpers (e.g., testCreateProject).
		if len(name) > 4 && unicode.IsLower(rune(name[4])) {
			continue
		}
		// Skip Benchmark*, Fuzz*, Example*.
		if strings.HasPrefix(name, "TestMain") {
			continue
		}

		entry := testEntry{
			File:        relPath,
			CurrentName: name,
		}

		entry.Pattern, entry.SuggestedName = classify(name)
		results = append(results, entry)
	}
	return results
}

// classify determines the naming pattern and suggests a corrected name.
func classify(name string) (pattern, suggested string) {
	// Skip E2E workflow entry points.
	if e2eWorkflow.MatchString(name) {
		return PatternSkip, name
	}

	// TestCov* prefix tests.
	if covPattern.MatchString(name) {
		suggested = renameCov(name)
		return PatternTestCov, suggested
	}

	parts := strings.Split(name, "_")
	switch {
	case len(parts) >= 3:
		return Pattern3Part, name
	case len(parts) == 2:
		return Pattern2Part, name
	default:
		// Single part — no underscores at all.
		suggested = splitCamelCase(name)
		return PatternNoUnderscore, suggested
	}
}

// renameCov transforms TestCovFuncScenario into TestFunc_Scenario.
func renameCov(name string) string {
	// Remove "TestCov" prefix, keep the rest.
	rest := strings.TrimPrefix(name, "TestCov")
	if rest == "" {
		return name
	}
	// Split the remaining CamelCase into parts and form Test_Part1_Part2.
	return splitCamelCase("Test" + rest)
}

// splitCamelCase splits a TestCamelCase name into Test_Part1_Part2 form.
// It identifies word boundaries at uppercase letters that follow lowercase letters.
func splitCamelCase(name string) string {
	if !strings.HasPrefix(name, "Test") {
		return name
	}

	// Work on the part after "Test".
	rest := name[4:]
	if rest == "" {
		return name
	}

	var parts []string
	current := strings.Builder{}

	runes := []rune(rest)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			// Word boundary: uppercase after lowercase, or uppercase before lowercase
			// (handles acronyms like "HTTPHandler" → "HTTP", "Handler").
			prevLower := unicode.IsLower(runes[i-1])
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if prevLower || (nextLower && !prevLower && current.Len() > 0) {
				parts = append(parts, current.String())
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	if len(parts) <= 1 {
		return name
	}

	// Merge parts into segments separated by underscores.
	// Try to create meaningful 2-3 segments from the words.
	return "Test" + mergeIntoSegments(parts)
}

// mergeIntoSegments takes CamelCase words and groups them into 2-3 underscore-separated
// segments for the TestFunc_Scenario_Expected pattern.
func mergeIntoSegments(words []string) string {
	if len(words) <= 2 {
		return strings.Join(words, "_")
	}

	// Heuristic: look for common "result" suffixes to identify the Expected part.
	resultWords := map[string]bool{
		"Success": true, "Error": true, "Returns": true, "Panics": true,
		"Fails": true, "Creates": true, "Updates": true, "Deletes": true,
		"Empty": true, "Nil": true, "Valid": true, "Invalid": true,
		"NoPanic": true, "Contains": true, "Match": true, "Matches": true,
	}

	// Check if the last word is a result indicator.
	last := words[len(words)-1]
	if resultWords[last] {
		// func = first word, scenario = middle, expected = last.
		funcPart := words[0]
		scenarioPart := strings.Join(words[1:len(words)-1], "")
		return funcPart + "_" + scenarioPart + "_" + last
	}

	// No clear result word — split at roughly the boundary between func and scenario.
	// First word is the function name, rest is the scenario.
	funcPart := words[0]
	scenarioPart := strings.Join(words[1:], "")
	return funcPart + "_" + scenarioPart
}
