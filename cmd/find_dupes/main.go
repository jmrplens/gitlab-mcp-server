// Command find_dupes scans Go source files for duplicated string literals
// that appear three or more times and are not already declared as constants.
// It uses the go/ast parser to inspect string literals and filters out short
// strings (< 3 chars) and JSON field names.
//
// Usage:
//
//	go run ./cmd/find_dupes/ <dir|file>...
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// main finds duplicated string literals that should be extracted to constants.
func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/find_dupes/ <dir|file>...")
		os.Exit(1)
	}
	var files []string
	for _, arg := range args {
		info, err := os.Stat(arg) // #nosec G703 -- CLI tool: user provides paths intentionally
		if err != nil {
			fmt.Fprintf(os.Stderr, "stat error %s: %v\n", arg, err)
			continue
		}
		if !info.IsDir() {
			files = append(files, arg)
			continue
		}
		_ = filepath.Walk(arg, func(path string, fi os.FileInfo, err error) error { // #nosec G703 -- CLI tool: user provides paths intentionally
			if err != nil || fi.IsDir() {
				return err
			}
			if strings.HasSuffix(fi.Name(), ".go") && !strings.HasSuffix(fi.Name(), "_test.go") {
				files = append(files, path)
			}
			return nil
		})
	}
	for _, file := range files {
		findDupes(file)
	}
}

// entry pairs a string literal value with the number of times it appears.
type entry struct {
	val   string
	count int
}

// findDupes parses a single Go source file, counts string literal
// occurrences, and prints those that appear three or more times.
func findDupes(filename string) {
	fset := token.NewFileSet()
	src, err := os.ReadFile(filename) // #nosec G304,G703 -- CLI tool: user provides paths intentionally
	if err != nil {
		fmt.Fprintf(os.Stderr, "read error %s: %v\n", filename, err)
		return
	}
	node, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error %s: %v\n", filename, err)
		return
	}

	counts := countStringLiterals(node)
	constValues := collectConstValues(node)
	dupes := filterDuplicates(counts, constValues)

	sort.Slice(dupes, func(i, j int) bool {
		return dupes[i].count > dupes[j].count
	})

	if len(dupes) > 0 {
		printDuplicates(filename, dupes)
	}
}

// countStringLiterals walks the AST and counts occurrences of each
// string literal longer than 2 characters.
func countStringLiterals(node *ast.File) map[string]int {
	counts := map[string]int{}
	ast.Inspect(node, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		val, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		if len(val) < 3 {
			return true
		}
		counts[val]++
		return true
	})
	return counts
}

// collectConstValues returns a set of string values already assigned to
// const or var declarations, so they can be excluded from duplicate reports.
func collectConstValues(node *ast.File) map[string]bool {
	constValues := map[string]bool{}
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || (genDecl.Tok != token.CONST && genDecl.Tok != token.VAR) {
			continue
		}
		collectStringValues(genDecl, constValues)
	}
	return constValues
}

// collectStringValues extracts string literal values from a GenDecl
// (const or var block) and adds them to dest.
func collectStringValues(genDecl *ast.GenDecl, dest map[string]bool) {
	for _, spec := range genDecl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, v := range vs.Values {
			var lit *ast.BasicLit
			lit, ok = v.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			val, err := strconv.Unquote(lit.Value)
			if err == nil {
				dest[val] = true
			}
		}
	}
}

// filterDuplicates returns entries for string literals that appear at
// least 3 times and are not already defined as constants or JSON field names.
func filterDuplicates(counts map[string]int, constValues map[string]bool) []entry {
	var dupes []entry
	for val, count := range counts {
		if count < 3 || constValues[val] || isJSONFieldName(val) {
			continue
		}
		dupes = append(dupes, entry{val, count})
	}
	return dupes
}

// printDuplicates writes a formatted section for filename listing each
// duplicated string literal and its occurrence count.
func printDuplicates(filename string, dupes []entry) {
	short := filename
	if idx := strings.LastIndex(short, "/"); idx >= 0 {
		short = short[idx+1:]
	}
	if idx := strings.LastIndex(short, "\\"); idx >= 0 {
		short = short[idx+1:]
	}
	fmt.Printf("\n=== %s ===\n", short)
	for _, d := range dupes {
		fmt.Printf("  [%dx] %q\n", d.count, d.val)
	}
}

// isJSONFieldName reports whether s looks like a JSON field name
// (lowercase letters, digits, and underscores only, up to 25 characters).
func isJSONFieldName(s string) bool {
	if len(s) > 25 {
		return false
	}
	for _, c := range s {
		if c != '_' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
