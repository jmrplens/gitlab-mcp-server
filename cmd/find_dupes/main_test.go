package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// TestRun_NoArgsReturnsUsage verifies the CLI entry point reports usage without
// exiting the test process when no scan targets are provided.
func TestRun_NoArgsReturnsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := run(nil, &stdout, &stderr); code != 1 {
		t.Fatalf("run() code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "usage: go run ./cmd/find_dupes/") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

// TestRun_ScansFilesAndDirectories verifies CLI orchestration accepts explicit
// files and directories while reporting stat errors for missing inputs.
func TestRun_ScansFilesAndDirectories(t *testing.T) {
	dir := t.TempDir()
	writeDuplicateSource(t, filepath.Join(dir, "source.go"), "directory duplicate", 3)
	writeDuplicateSource(t, filepath.Join(dir, "ignored_test.go"), "ignored duplicate", 3)
	file := filepath.Join(t.TempDir(), "single.go")
	writeDuplicateSource(t, file, "file duplicate", 4)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{dir, file, filepath.Join(dir, "missing.go")}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, want 0", code)
	}
	output := stdout.String()
	if !strings.Contains(output, "directory duplicate") {
		t.Fatalf("stdout = %q, want directory duplicate", output)
	}
	if !strings.Contains(output, "file duplicate") {
		t.Fatalf("stdout = %q, want file duplicate", output)
	}
	if strings.Contains(output, "ignored duplicate") {
		t.Fatalf("stdout = %q, want _test.go files skipped", output)
	}
	if !strings.Contains(stderr.String(), "stat error") {
		t.Fatalf("stderr = %q, want stat error", stderr.String())
	}
}

// TestCountStringLiterals_CollectsEligibleValues verifies AST scanning counts
// only valid string literals whose values are long enough to be meaningful.
func TestCountStringLiterals_CollectsEligibleValues(t *testing.T) {
	node := parseSource(t, `package sample

func f() {
	_ = "dupe"
	_ = "dupe"
	_ = "ok"
	_ = 42
}
`)

	got := countStringLiterals(node)
	want := map[string]int{"dupe": 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("countStringLiterals() = %v, want %v", got, want)
	}
}

// TestCollectConstValues_CollectsStringConstantsAndVariables verifies duplicate
// reporting ignores values that already have a named constant or variable.
func TestCollectConstValues_CollectsStringConstantsAndVariables(t *testing.T) {
	node := parseSource(t, `package sample

const namedConst = "already_const"
const numberConst = 1
var namedVar = "already_var"
var computed = strings.TrimSpace("not_collected")
`)

	got := collectConstValues(node)
	want := map[string]bool{
		"already_const": true,
		"already_var":   true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectConstValues() = %v, want %v", got, want)
	}
}

// TestFilterDuplicates_ExcludesConstantsAndJSONFields verifies only actionable
// duplicated literals are reported to avoid noisy audit output.
func TestFilterDuplicates_ExcludesConstantsAndJSONFields(t *testing.T) {
	counts := map[string]int{
		"actionable duplicate": 3,
		"too few":              2,
		"already named":        5,
		"json_field_name":      4,
	}
	constValues := map[string]bool{"already named": true}

	got := filterDuplicates(counts, constValues)
	want := []entry{{val: "actionable duplicate", count: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterDuplicates() = %v, want %v", got, want)
	}
}

// TestFindDupes_PrintsSortedDuplicateSummary verifies file-level scanning sorts
// duplicates by count and prints only the source file base name.
func TestFindDupes_PrintsSortedDuplicateSummary(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "sample.go")
	source := `package sample

func f() {
	_ = "highest duplicate"
	_ = "highest duplicate"
	_ = "highest duplicate"
	_ = "highest duplicate"
	_ = "lower duplicate"
	_ = "lower duplicate"
	_ = "lower duplicate"
	_ = "json_field"
	_ = "json_field"
	_ = "json_field"
}
`
	if err := os.WriteFile(filename, []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	findDupes(filename, &stdout, &stderr)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "=== sample.go ===") {
		t.Fatalf("output = %q, want file header", output)
	}
	if strings.Contains(output, "json_field") {
		t.Fatalf("output = %q, want JSON-like fields excluded", output)
	}
	highestIndex := strings.Index(output, `[4x] "highest duplicate"`)
	lowerIndex := strings.Index(output, `[3x] "lower duplicate"`)
	if highestIndex < 0 || lowerIndex < 0 {
		t.Fatalf("output = %q, want both duplicate summaries", output)
	}
	if highestIndex > lowerIndex {
		t.Fatalf("output = %q, want highest count before lower count", output)
	}
}

// TestIsJSONFieldName_Scenarios verifies the heuristic used to suppress common
// JSON key literals accepts compact snake_case names and rejects larger values.
func TestIsJSONFieldName_Scenarios(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "lowercase", in: "project_id", want: true},
		{name: "digits", in: "sha256", want: true},
		{name: "uppercase", in: "ProjectID", want: false},
		{name: "hyphen", in: "project-id", want: false},
		{name: "too long", in: "this_json_field_name_is_too_long", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isJSONFieldName(tt.in); got != tt.want {
				t.Fatalf("isJSONFieldName(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestCountStringLiterals_IgnoresUnquotableLiterals verifies that
// countStringLiterals silently skips BasicLit values that fail
// strconv.Unquote. This branch is defensive dead code in practice because
// parser.ParseFile only produces quotable STRING tokens; we exercise the
// branch by parsing a valid file, then swapping the literal value to an
// unterminated raw string and walking it through countStringLiterals.
func TestCountStringLiterals_IgnoresUnquotableLiterals(t *testing.T) {
	node := parseSource(t, "package sample\nconst bad = \"valid_initial_value\"\n")
	// Locate the basic literal in the parsed source and replace its value
	// with a token that strconv.Unquote rejects.
	for _, decl := range node.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			vs, isValueSpec := spec.(*ast.ValueSpec)
			if !isValueSpec {
				continue
			}
			for _, v := range vs.Values {
				if lit, isLit := v.(*ast.BasicLit); isLit && lit.Kind == token.STRING {
					lit.Value = "`unterminated"
				}
			}
		}
	}
	got := countStringLiterals(node)
	// After replacement, the only string in the AST has an unquotable
	// value, so countStringLiterals must return an empty map.
	if len(got) != 0 {
		t.Errorf("countStringLiterals() = %v, want empty (unquotable literal should be skipped)", got)
	}
}

// TestCollectStringValues_IgnoresUnquotableLiterals verifies that
// collectStringValues silently skips literal values that strconv.Unquote
// rejects, leaving dest unchanged.
func TestCollectStringValues_IgnoresUnquotableLiterals(t *testing.T) {
	node := parseSource(t, "package sample\nvar bad = \"valid_initial_value\"\n")
	for _, decl := range node.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			vs, isValueSpec := spec.(*ast.ValueSpec)
			if !isValueSpec {
				continue
			}
			for _, v := range vs.Values {
				if lit, isLit := v.(*ast.BasicLit); isLit && lit.Kind == token.STRING {
					lit.Value = "`unterminated"
				}
			}
		}
	}
	dest := map[string]bool{}
	collectStringValues(node.Decls[0].(*ast.GenDecl), dest)
	if len(dest) != 0 {
		t.Errorf("dest = %v, want empty (unquotable literal should be skipped)", dest)
	}
}

// TestPrintDuplicates_WindowsPathSeparator verifies printDuplicates strips
// the Windows path separator as well as the Unix one when shortening
// filenames.
func TestPrintDuplicates_WindowsPathSeparator(t *testing.T) {
	var stdout bytes.Buffer
	printDuplicates(&stdout, `dir\subdir\file.go`, []entry{
		{val: "duplicated literal", count: 4},
	})
	output := stdout.String()
	if !strings.Contains(output, "=== file.go ===") {
		t.Errorf("output = %q, want shortened basename after Windows path separator", output)
	}
}

// TestFindDupes_ReadError writes a path that does not exist and verifies the
// function returns cleanly with a stderr message rather than panicking.
func TestFindDupes_ReadError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	findDupes(filepath.Join(t.TempDir(), "does_not_exist.go"), &stdout, &stderr)
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty on read error", stdout.String())
	}
	if !strings.Contains(stderr.String(), "read error") {
		t.Errorf("stderr = %q, want 'read error' prefix", stderr.String())
	}
}

// TestFindDupes_ParseError writes a malformed Go file and verifies the
// function returns cleanly with a stderr message rather than panicking.
func TestFindDupes_ParseError(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "broken.go")
	if err := os.WriteFile(filename, []byte("this is not go code @@@"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	findDupes(filename, &stdout, &stderr)
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty on parse error", stdout.String())
	}
	if !strings.Contains(stderr.String(), "parse error") {
		t.Errorf("stderr = %q, want 'parse error' prefix", stderr.String())
	}
}

// TestFindDupes_NoDuplicates writes a Go file with no duplicate string
// literals long enough to be reported, and verifies that the function
// produces no output.
func TestFindDupes_NoDuplicates(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "unique.go")
	source := `package unique

func f() {
	_ = "abc"
	_ = "def"
	_ = "ghi"
}
`
	if err := os.WriteFile(filename, []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	findDupes(filename, &stdout, &stderr)
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty when no duplicates", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty when no duplicates", stderr.String())
	}
}

func parseSource(t *testing.T, source string) *ast.File {
	t.Helper()
	node, err := parser.ParseFile(token.NewFileSet(), "sample.go", source, 0)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	return node
}

func writeDuplicateSource(t *testing.T, filename, value string, count int) {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("package sample\n\nfunc f() {\n")
	for range count {
		buf.WriteString("\t_ = ")
		buf.WriteString(strconv.Quote(value))
		buf.WriteString("\n")
	}
	buf.WriteString("}\n")
	if err := os.WriteFile(filename, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
