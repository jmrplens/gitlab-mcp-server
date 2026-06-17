// main_test.go covers the add_docs command's documentation generation
// heuristics.
//
// Tests verify processFile preserves manually authored docs, regenerates
// stale generated docs, and produces the expected phrasing for tests,
// benchmarks, fuzz, examples, methods, and common helper name patterns.
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProcessFile_DocumentsMissingSymbols verifies processFile inserts docs for functions, types, and values.
func TestProcessFile_DocumentsMissingSymbols(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.go")
	source := `package sample

func ListProjects() {}

type ProjectInput struct{}

const defaultLimit = 20
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	processFile(path)

	updatedBytes, err := os.ReadFile(path) //#nosec G304 -- test fixture path from t.TempDir.
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	updated := string(updatedBytes)
	assertContains(t, updated, "// ListProjects lists projects for the sample package.")
	assertContains(t, updated, "// ProjectInput defines parameters for the project operation.")
	assertContains(t, updated, "// defaultLimit identifies the default limit constant used by this package.")
}

// TestProcessFile_PreservesManualDocsAndSkipsInit verifies processFile avoids overwriting useful existing docs.
func TestProcessFile_PreservesManualDocsAndSkipsInit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.go")
	source := `package sample

// Config keeps runtime settings.
type Config struct{}

func init() {}
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	processFile(path)

	updatedBytes, err := os.ReadFile(path) //#nosec G304 -- test fixture path from t.TempDir.
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if updated := string(updatedBytes); updated != source {
		t.Fatalf("processFile() changed documented file:\n%s", updated)
	}
}

// TestProcessFile_ReplacesGeneratedDocs verifies processFile regenerates stale helper comments from earlier tool versions.
func TestProcessFile_ReplacesGeneratedDocs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.go")
	source := `package sample

// helper verifies the behavior of helper.
func helper() {}
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	processFile(path)

	updatedBytes, err := os.ReadFile(path) //#nosec G304 -- test fixture path from t.TempDir.
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	updated := string(updatedBytes)
	if strings.Contains(updated, "verifies the behavior of helper") {
		t.Fatalf("processFile() kept generated doc:\n%s", updated)
	}
	assertContains(t, updated, "// helper implements the helper helper used by sample.")
}

// TestGenerateFuncDoc_TestFunctionVariants verifies generateFuncDoc handles test, benchmark, fuzz, and example naming patterns.
func TestGenerateFuncDoc_TestFunctionVariants(t *testing.T) {
	testFunction := parseFuncDecl(t, `package sample
import "testing"
func TestCreateIssue_ValidInput_ReturnsIssue(t *testing.T) { cases := []struct{name string}{{"ok"}}; _ = cases }
`, "TestCreateIssue_ValidInput_ReturnsIssue")
	if got := generateFuncDoc(testFunction, "sample", true); got != "TestCreateIssue_ValidInput_ReturnsIssue covers CreateIssue with table-driven subtests for valid input returns issue." {
		t.Fatalf("generateFuncDoc(test) = %q", got)
	}

	benchmark := parseFuncDecl(t, `package sample
import "testing"
func BenchmarkDynamicSearch(b *testing.B) {}
`, "BenchmarkDynamicSearch")
	if got := generateFuncDoc(benchmark, "sample", true); got != "BenchmarkDynamicSearch measures dynamic search search and dispatch overhead." {
		t.Fatalf("generateFuncDoc(benchmark) = %q", got)
	}

	fuzz := parseFuncDecl(t, `package sample
import "testing"
func FuzzActionID(f *testing.F) {}
`, "FuzzActionID")
	if got := generateFuncDoc(fuzz, "sample", true); got != "FuzzActionID tests that action ID handles arbitrary inputs without panicking." {
		t.Fatalf("generateFuncDoc(fuzz) = %q", got)
	}

	example := parseFuncDecl(t, `package sample
func ExampleCatalog() {}
`, "ExampleCatalog")
	if got := generateFuncDoc(example, "sample", true); got != "ExampleCatalog demonstrates usage of catalog." {
		t.Fatalf("generateFuncDoc(example) = %q", got)
	}
}

// TestGenerateMethodDoc_CommonMethods verifies generateMethodDoc describes common method conventions.
func TestGenerateMethodDoc_CommonMethods(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{name: "string", src: `package sample
type client struct{}
func (client) String() string { return "" }
`, want: "String returns the display label for client."},
		{name: "getter", src: `package sample
type config struct{}
func (config) GetToken() string { return "" }
`, want: "GetToken returns the token value from config."},
		{name: "boolean", src: `package sample
type route struct{}
func (route) Available() bool { return true }
`, want: "Available reports whether the route satisfies the available condition."},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			functionDecl := firstFuncDecl(t, testCase.src)
			if got := generateMethodDoc(functionDecl); got != testCase.want {
				t.Fatalf("generateMethodDoc() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestHelperTextGeneration_CoversIdentifiersAndInitialisms verifies naming helpers preserve project terminology.
func TestHelperTextGeneration_CoversIdentifiersAndInitialisms(t *testing.T) {
	if got := camelToWords("GitLabAPIURL2FA"); got != "GitLab API URL 2FA" {
		t.Fatalf("camelToWords() = %q, want GitLab API URL 2FA", got)
	}
	if got := subjectScenarioPhrase("BuildCatalog", "InvalidInput_ReturnsError"); got != "BuildCatalog returns error with invalid input" {
		t.Fatalf("subjectScenarioPhrase() = %q", got)
	}
	if before, after, ok := splitAtPredicate("invalid input returns error"); !ok || before != "invalid input" || after != "returns error" {
		t.Fatalf("splitAtPredicate() = %q, %q, %t", before, after, ok)
	}
	if got := inferAction("UploadAvatar"); got != "uploads avatar" {
		t.Fatalf("inferAction() = %q, want uploads avatar", got)
	}
	if got := formatComment("Line one\nLine two", "\t"); len(got) != 2 || got[0] != "\t// Line one" || got[1] != "\t// Line two" {
		t.Fatalf("formatComment() = %#v", got)
	}
}

// TestHelperDocRuleMatches_CombinesMixedConstraints verifies mixed helper doc rules require every configured constraint.
func TestHelperDocRuleMatches_CombinesMixedConstraints(t *testing.T) {
	mixedRule := helperDocRule{prefixes: []string{"is"}, contains: []string{"Available"}}
	if !mixedRule.matches("isProjectAvailable") {
		t.Fatal("mixed rule did not match name with required prefix and marker")
	}
	if mixedRule.matches("isProject") {
		t.Fatal("mixed rule matched name missing required marker")
	}

	if !(helperDocRule{prefixes: []string{"is"}}).matches("isProject") {
		t.Fatal("prefix-only rule did not match by prefix")
	}
	if !(helperDocRule{contains: []string{"Available"}}).matches("projectAvailable") {
		t.Fatal("contains-only rule did not match by marker")
	}
}

// TestExprToString_FormatsCommonExpressionShapes verifies exprToString renders common AST type forms.
func TestExprToString_FormatsCommonExpressionShapes(t *testing.T) {
	expressions := []struct {
		name string
		expr ast.Expr
		want string
	}{
		{name: "ident", expr: ast.NewIdent("string"), want: "string"},
		{name: "star", expr: &ast.StarExpr{X: ast.NewIdent("Client")}, want: "*Client"},
		{name: "selector", expr: &ast.SelectorExpr{X: ast.NewIdent("time"), Sel: ast.NewIdent("Duration")}, want: "time.Duration"},
		{name: "array", expr: &ast.ArrayType{Elt: ast.NewIdent("string")}, want: "[]string"},
		{name: "map", expr: &ast.MapType{Key: ast.NewIdent("string"), Value: ast.NewIdent("any")}, want: "map[string]any"},
		{name: "unknown", expr: &ast.ChanType{Value: ast.NewIdent("string")}, want: "any"},
	}

	for _, expression := range expressions {
		t.Run(expression.name, func(t *testing.T) {
			if got := exprToString(expression.expr); got != expression.want {
				t.Fatalf("exprToString() = %q, want %q", got, expression.want)
			}
		})
	}
}

func parseFuncDecl(t *testing.T, source, name string) *ast.FuncDecl {
	t.Helper()
	node, err := parser.ParseFile(token.NewFileSet(), "sample.go", source, 0)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	for _, decl := range node.Decls {
		functionDecl, ok := decl.(*ast.FuncDecl)
		if ok && functionDecl.Name.Name == name {
			return functionDecl
		}
	}
	t.Fatalf("function %q not found", name)
	return nil
}

func firstFuncDecl(t *testing.T, source string) *ast.FuncDecl {
	t.Helper()
	node, err := parser.ParseFile(token.NewFileSet(), "sample.go", source, 0)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	for _, decl := range node.Decls {
		functionDecl, ok := decl.(*ast.FuncDecl)
		if ok {
			return functionDecl
		}
	}
	t.Fatal("function declaration not found")
	return nil
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("text missing %q:\n%s", want, text)
	}
}
