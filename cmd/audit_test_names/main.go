// Command audit_test_names scans all Go test files and classifies test
// function names by their naming pattern. It outputs a CSV report with
// columns: file, current_name, pattern, suggested_name.
//
// With -apply it renames test functions in place to match the suggested
// names. With -dry-run it prints what would change without writing.
//
// Usage:
//
//	go run ./cmd/audit_test_names/ <dir>...
//	go run ./cmd/audit_test_names/ -apply <dir>...
//	go run ./cmd/audit_test_names/ -apply -dry-run <dir>...
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
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
//
// File is the slash-separated path of the source file. CurrentName is the
// function name as written. Pattern is one of the Pattern* constants
// classifying the naming convention. SuggestedName is the recommended
// replacement — for compliant names it equals CurrentName.
type testEntry struct {
	File          string
	CurrentName   string
	Pattern       string
	SuggestedName string
}

// Test name regular expressions used to classify known naming patterns.
var (
	// covPattern matches TestCov* prefixed tests.
	covPattern = regexp.MustCompile(`^TestCov[A-Z]`)
)

// main audits test function naming convention compliance across the project.
func main() {
	apply := flag.Bool("apply", false, "rename test functions in place to match the suggested names")
	dryRun := flag.Bool("dry-run", false, "print what would be renamed without writing files (use with -apply)")
	checkFiles := flag.Bool("check-files", false, "audit test FILE names against the module-naming convention and exit non-zero on violations")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/audit_test_names/ [flags] <dir>...")
		os.Exit(1)
	}

	if *checkFiles {
		if !runFileCheck(args, os.Stdout) {
			os.Exit(1)
		}
		return
	}

	if *apply || *dryRun {
		if !runApply(args, os.Stdout, os.Stderr, *dryRun) {
			os.Exit(1)
		}
		return
	}
	if err := run(args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run executes the audit workflow against the supplied directories. It writes
// CSV rows to stdout and a human-readable summary to stderr.
func run(args []string, stdout, stderr io.Writer) error {
	entries := make([]testEntry, 0, len(args)*10)
	for _, dir := range args {
		entries = append(entries, scanDir(dir)...)
	}

	w := csv.NewWriter(stdout)
	if err := w.Write([]string{"file", "current_name", "pattern", "suggested_name"}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, e := range entries {
		if err := w.Write([]string{e.File, e.CurrentName, e.Pattern, e.SuggestedName}); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("flush csv: %w", err)
	}

	// Print summary to stderr.
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.Pattern]++
	}
	fmt.Fprintf(stderr, "\n=== Test Naming Audit Summary ===\n")
	fmt.Fprintf(stderr, "Total test functions: %d\n", len(entries))
	for _, p := range []string{Pattern3Part, Pattern2Part, PatternNoUnderscore, PatternTestCov, PatternOther, PatternSkip} {
		if c, ok := counts[p]; ok {
			fmt.Fprintf(stderr, "  %-16s %d\n", p+":", c)
		}
	}
	return nil
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
		if strings.HasSuffix(e.Name(), testFileSuffix) {
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

// runApply scans test files and renames functions to match suggested names.
// When dryRun is true it prints what would change without writing.
// runApply renames test functions across dirs and returns ok=false when any
// file or directory was skipped due to an error, so callers can exit non-zero.
func runApply(dirs []string, stdout, stderr io.Writer, dryRun bool) (ok bool) {
	totalRenames := 0
	totalFiles := 0
	ok = true
	for _, dir := range dirs {
		renames, files, dirOK := applyDir(dir, stdout, stderr, dryRun)
		totalRenames += renames
		totalFiles += files
		ok = ok && dirOK
	}
	mode := "applied"
	if dryRun {
		mode = "dry-run"
	}
	fmt.Fprintf(stderr, "\n=== Rename Summary (%s) ===\n", mode)
	fmt.Fprintf(stderr, "Files scanned: %d\n", totalFiles)
	fmt.Fprintf(stderr, "Renames: %d\n", totalRenames)
	return ok
}

// applyDir recursively walks a directory applying renames to test files. ok is
// false when readdir or any descended file/dir failed.
func applyDir(dir string, stdout, stderr io.Writer, dryRun bool) (renames, files int, ok bool) {
	cleanDir := filepath.Clean(dir)
	entries, err := os.ReadDir(cleanDir)
	if err != nil {
		fmt.Fprintf(stderr, "readdir %s: %v\n", cleanDir, err)
		return 0, 0, false
	}
	totalRenames := 0
	totalFiles := 0
	ok = true
	for _, e := range entries {
		path := filepath.Join(cleanDir, e.Name())
		if e.IsDir() {
			r, f, sub := applyDir(path, stdout, stderr, dryRun)
			totalRenames += r
			totalFiles += f
			ok = ok && sub
			continue
		}
		if strings.HasSuffix(e.Name(), testFileSuffix) {
			totalFiles++
			r, fileOK := applyFile(path, stdout, stderr, dryRun)
			totalRenames += r
			ok = ok && fileOK
		}
	}
	return totalRenames, totalFiles, ok
}

// collectRenames builds a map of old→new names for test functions that need
// renaming in the given file AST. Collision detection excludes targets that
// already exist as function names in the file.
func collectRenames(node *ast.File, cleanPath string, stderr io.Writer) map[string]string {
	existing := map[string]bool{}
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		existing[fn.Name.Name] = true
	}
	renames := map[string]string{}
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		name := fn.Name.Name
		runes := []rune(name)
		if !strings.HasPrefix(name, "Test") || (len(runes) > 4 && unicode.IsLower(runes[4])) || strings.HasPrefix(name, "TestMain") {
			continue
		}
		pattern, suggested := classify(name)
		if pattern == Pattern3Part || pattern == PatternSkip || pattern == Pattern2Part {
			continue
		}
		if suggested == "" || suggested == name || existing[suggested] {
			if suggested != "" && suggested != name && existing[suggested] {
				fmt.Fprintf(stderr, "  skip %s -> %s in %s: target name already exists\n", name, suggested, cleanPath)
			}
			continue
		}
		renames[name] = suggested
		// Reserve the target so a second legacy name cannot map to the same
		// suggestion and produce a duplicate (uncompilable) function name.
		existing[suggested] = true
	}
	return renames
}

// applyFile renames test functions in a single file. It returns the rename
// count and ok=false when the file was skipped due to a parse/read/write error
// or because the rename would produce invalid Go, so callers can surface the
// failure via a non-zero exit. A file with no renames is not a failure.
func applyFile(path string, stdout, stderr io.Writer, dryRun bool) (applied int, ok bool) {
	cleanPath := filepath.Clean(path)
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, cleanPath, nil, 0)
	if err != nil {
		fmt.Fprintf(stderr, "parse %s: %v\n", cleanPath, err)
		return 0, false
	}

	renames := collectRenames(node, cleanPath, stderr)
	if len(renames) == 0 {
		return 0, true
	}

	src, err := os.ReadFile(cleanPath)
	if err != nil {
		fmt.Fprintf(stderr, "read %s: %v\n", cleanPath, err)
		return 0, false
	}
	result := string(src)
	for old, newName := range renames {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(old) + `\b`)
		if re.MatchString(result) {
			result = re.ReplaceAllString(result, newName)
			applied++
			fmt.Fprintf(stdout, "%s: %s -> %s\n", filepath.ToSlash(cleanPath), old, newName)
		}
	}
	if applied == 0 {
		return 0, true
	}

	if _, parseErr := parser.ParseFile(token.NewFileSet(), cleanPath, []byte(result), 0); parseErr != nil {
		fmt.Fprintf(stderr, "  ABORT %s: rename would produce invalid Go: %v\n", cleanPath, parseErr)
		return 0, false
	}
	if dryRun {
		return applied, true
	}
	if writeErr := os.WriteFile(cleanPath, []byte(result), 0o600); writeErr != nil { //#nosec G306,G703 -- CLI tool, user provides paths intentionally
		fmt.Fprintf(stderr, "write %s: %v\n", cleanPath, writeErr)
		return 0, false
	}
	return applied, true
}
