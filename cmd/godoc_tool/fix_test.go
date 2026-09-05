// main_test.go covers the add_docs command's documentation generation
// heuristics.
//
// Tests verify processFile preserves manually authored docs, regenerates
// stale generated docs, and produces the expected phrasing for tests,
// benchmarks, fuzz, examples, methods, and common helper name patterns.
package main

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
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

// captureFixStdout captures everything the fixer prints to os.Stdout while
// action runs.
func captureFixStdout(t *testing.T, action func()) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = original })

	action()

	os.Stdout = original
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatalf("writer.Close() error = %v", closeErr)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	if closeErr := reader.Close(); closeErr != nil {
		t.Fatalf("reader.Close() error = %v", closeErr)
	}
	return string(output)
}

// writeFixFile writes content to name inside dir and returns the full path.
func writeFixFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", name, err)
	}
	return path
}

// readFixFile returns the current content of path.
func readFixFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //#nosec G304 -- test fixture path from t.TempDir.
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}

// firstTypeSpec returns the first type declaration in source.
func firstTypeSpec(t *testing.T, source string) *ast.TypeSpec {
	t.Helper()
	node, err := parser.ParseFile(token.NewFileSet(), "sample.go", source, parser.ParseComments)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range genDecl.Specs {
			if typeSpec, isType := spec.(*ast.TypeSpec); isType {
				return typeSpec
			}
		}
	}
	t.Fatal("no type declaration found")
	return nil
}

// firstValueSpec returns the first const or var declaration in source along
// with the token that introduced it.
func firstValueSpec(t *testing.T, source string) (*ast.ValueSpec, token.Token) {
	t.Helper()
	node, err := parser.ParseFile(token.NewFileSet(), "sample.go", source, parser.ParseComments)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || (genDecl.Tok != token.CONST && genDecl.Tok != token.VAR) {
			continue
		}
		for _, spec := range genDecl.Specs {
			if valueSpec, isValue := spec.(*ast.ValueSpec); isValue {
				return valueSpec, genDecl.Tok
			}
		}
	}
	t.Fatal("no const or var declaration found")
	return nil, token.ILLEGAL
}

// TestProcessPath_FilesDirectoriesAndErrors verifies the path walker: a Go
// file is documented, a directory is walked recursively, a non-Go file is
// ignored, and a path that cannot be statted is an error.
func TestProcessPath_FilesDirectoriesAndErrors(t *testing.T) {
	t.Run("documents a single Go file", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFixFile(t, dir, "sample.go", "package sample\n\nfunc ListProjects() {}\n")

		out := captureFixStdout(t, func() {
			if err := processPath(path); err != nil {
				t.Errorf("processPath() error = %v", err)
			}
		})
		if want := "documented " + path + " (1 symbols)\n"; out != want {
			t.Errorf("stdout = %q, want %q", out, want)
		}
		assertContains(t, readFixFile(t, path), "// ListProjects lists projects for the sample package.")
	})

	t.Run("walks a directory recursively", func(t *testing.T) {
		dir := t.TempDir()
		nested := filepath.Join(dir, "nested")
		if err := os.MkdirAll(nested, 0o750); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		top := writeFixFile(t, dir, "top.go", "package sample\n\nfunc GetThing() {}\n")
		inner := writeFixFile(t, nested, "inner.go", "package nested\n\nfunc DeleteThing() {}\n")
		writeFixFile(t, dir, "notes.txt", "func Ignored() {}\n")

		captureFixStdout(t, func() {
			if err := processPath(dir); err != nil {
				t.Errorf("processPath(dir) error = %v", err)
			}
		})
		assertContains(t, readFixFile(t, top), "// GetThing retrieves thing for the sample package.")
		assertContains(t, readFixFile(t, inner), "// DeleteThing deletes thing for the nested package.")
		if got := readFixFile(t, filepath.Join(dir, "notes.txt")); got != "func Ignored() {}\n" {
			t.Errorf("non-Go file was rewritten: %q", got)
		}
	})

	t.Run("ignores a non-Go file", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFixFile(t, dir, "readme.md", "# not Go\n")
		if err := processPath(path); err != nil {
			t.Fatalf("processPath() error = %v", err)
		}
		if got := readFixFile(t, path); got != "# not Go\n" {
			t.Fatalf("file changed: %q", got)
		}
	})

	t.Run("missing path is an error", func(t *testing.T) {
		err := processPath(filepath.Join(t.TempDir(), "missing.go"))
		if err == nil || !strings.HasPrefix(err.Error(), "stat ") {
			t.Fatalf("processPath(missing) error = %v, want stat error", err)
		}
	})
}

// TestProcessDir_ErrorsAreJoined verifies an unreadable directory fails, and
// that a parse failure in one file does not stop the sibling from being
// documented.
func TestProcessDir_ErrorsAreJoined(t *testing.T) {
	t.Run("unreadable directory", func(t *testing.T) {
		err := processDir(filepath.Join(t.TempDir(), "missing"))
		if err == nil || !strings.HasPrefix(err.Error(), "readdir ") {
			t.Fatalf("processDir(missing) error = %v, want readdir error", err)
		}
	})

	t.Run("a broken file does not stop its sibling", func(t *testing.T) {
		dir := t.TempDir()
		writeFixFile(t, dir, "broken.go", "package sample\n\nfunc (\n")
		good := writeFixFile(t, dir, "good.go", "package sample\n\nfunc CreateThing() {}\n")

		var err error
		captureFixStdout(t, func() { err = processDir(dir) })
		if err == nil || !strings.Contains(err.Error(), "parse ") {
			t.Fatalf("processDir() error = %v, want a parse error", err)
		}
		assertContains(t, readFixFile(t, good), "// CreateThing creates thing for the sample package.")
	})
}

// TestProcessFile_DryRunReportsWithoutWriting verifies --dry-run announces
// the file and its insertion count while leaving the source untouched.
func TestProcessFile_DryRunReportsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	source := "package sample\n\nfunc ListProjects() {}\n\ntype ProjectInput struct{}\n"
	path := writeFixFile(t, dir, "sample.go", source)

	dryRun = true
	t.Cleanup(func() { dryRun = false })

	out := captureFixStdout(t, func() {
		if err := processFile(path); err != nil {
			t.Errorf("processFile() error = %v", err)
		}
	})
	if want := "// dry-run: would update " + path + " (2 insertions)\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if got := readFixFile(t, path); got != source {
		t.Errorf("dry-run rewrote the file:\n%s", got)
	}
}

// TestProcessFile_ReadAndParseErrors verifies a missing file and an
// unparseable one are reported rather than silently skipped.
func TestProcessFile_ReadAndParseErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		err := processFile(filepath.Join(t.TempDir(), "missing.go"))
		if err == nil || !strings.HasPrefix(err.Error(), "read ") {
			t.Fatalf("processFile(missing) error = %v, want read error", err)
		}
	})

	t.Run("unparseable file", func(t *testing.T) {
		path := writeFixFile(t, t.TempDir(), "broken.go", "package sample\n\nfunc (\n")
		err := processFile(path)
		if err == nil || !strings.HasPrefix(err.Error(), "parse ") {
			t.Fatalf("processFile(broken) error = %v, want parse error", err)
		}
	})
}

// TestProcessFile_FullyDocumentedFileIsUntouched verifies a file whose
// symbols all carry hand-written comments is left byte-for-byte alone and
// prints nothing.
func TestProcessFile_FullyDocumentedFileIsUntouched(t *testing.T) {
	source := `package sample

// Widget is a documented type.
type Widget struct{}

// Build returns a widget.
func Build() Widget { return Widget{} }
`
	path := writeFixFile(t, t.TempDir(), "sample.go", source)

	out := captureFixStdout(t, func() {
		if err := processFile(path); err != nil {
			t.Errorf("processFile() error = %v", err)
		}
	})
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}
	if got := readFixFile(t, path); got != source {
		t.Errorf("documented file was rewritten:\n%s", got)
	}
}

// TestProcessFile_TestFileGetsTestAndHelperDocs verifies the fixer's
// rewrite of a test file down to the exact bytes: the test function gets a
// subject-and-scenario sentence, the helper gets its prefix-rule sentence,
// the import declaration is left alone, and indentation is preserved.
func TestProcessFile_TestFileGetsTestAndHelperDocs(t *testing.T) {
	source := `package sample

import "testing"

func TestWidget_Renders(t *testing.T) {}

func assertBody(t *testing.T) {}

type widgetCase struct{ name string }
`
	want := `package sample

import "testing"

// TestWidget_Renders verifies Widget when renders.
func TestWidget_Renders(t *testing.T) {}

// assertBody checks body invariants for tests.
func assertBody(t *testing.T) {}

// widgetCase describes one widget table-driven test case.
type widgetCase struct{ name string }
`
	path := writeFixFile(t, t.TempDir(), "sample_test.go", source)

	captureFixStdout(t, func() {
		if err := processFile(path); err != nil {
			t.Errorf("processFile() error = %v", err)
		}
	})
	if got := readFixFile(t, path); got != want {
		t.Fatalf("rewritten file =\n%s\nwant\n%s", got, want)
	}
}

// TestProcessFile_IndentedDeclarationKeepsItsIndentation verifies a comment
// inserted before an indented declaration is indented to match, so the
// rewritten file still compiles and stays gofmt-clean.
func TestProcessFile_IndentedDeclarationKeepsItsIndentation(t *testing.T) {
	source := "package sample\n\nvar _ = 1\n\ntype (\n\tprojectInput struct{}\n)\n"
	want := "package sample\n\nvar _ = 1\n\ntype (\n\t// projectInput defines parameters for the project operation.\n\tprojectInput struct{}\n)\n"
	path := writeFixFile(t, t.TempDir(), "sample.go", source)

	captureFixStdout(t, func() {
		if err := processFile(path); err != nil {
			t.Errorf("processFile() error = %v", err)
		}
	})
	if got := readFixFile(t, path); got != want {
		t.Fatalf("rewritten file = %q, want %q", got, want)
	}
}

// TestGenerateTypeDoc_TypeShapes verifies the type-comment generator across
// every shape it recognizes: the Input/Output suffixes with and without a
// prefix, interfaces, the test-fixture Case and Alias suffixes, the
// keyword-driven templates, and the generic fallback.
func TestGenerateTypeDoc_TypeShapes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		decl string
		want string
	}{
		{name: "input with a prefix", decl: "type ProjectInput struct{}", want: "ProjectInput defines parameters for the project operation."},
		{name: "bare input", decl: "type Input struct{}", want: "Input defines parameters for the sample tool."},
		{name: "output with a prefix", decl: "type ProjectOutput struct{}", want: "ProjectOutput represents the response from the project operation."},
		{name: "bare output", decl: "type Output struct{}", want: "Output represents the response from a sample operation."},
		{name: "interface", decl: "type Reader interface{}", want: "Reader defines the contract for reader operations."},
		{name: "test case struct", decl: "type widgetCase struct{}", want: "widgetCase describes one widget table-driven test case."},
		{name: "alias struct", decl: "type widgetAlias struct{}", want: "widgetAlias describes one widget alias mapping used by tests."},
		{name: "google keyword", decl: "type googlePayload struct{}", want: "googlePayload models the Google Gemini Google payload payload."},
		{name: "anthropic keyword", decl: "type anthropicMessage struct{}", want: "anthropicMessage models the Anthropic anthropic message payload."},
		{name: "provider keyword", decl: "type providerConfig struct{}", want: "providerConfig captures model-provider provider config data."},
		{name: "publish keyword", decl: "type publishTarget struct{}", want: "publishTarget captures publish target data for published evaluation reports."},
		{name: "fixture keyword", decl: "type fixtureSet struct{}", want: "fixtureSet captures fixture set data for live evaluation fixtures."},
		{name: "trace keyword", decl: "type traceEntry struct{}", want: "traceEntry records trace entry data in evaluation traces."},
		{name: "task keyword", decl: "type taskState struct{}", want: "taskState captures task state data for one evaluation task."},
		{name: "summary keyword", decl: "type summaryRow struct{}", want: "summaryRow captures summary row data for evaluation summaries."},
		{name: "fallback", decl: "type widget struct{}", want: "widget holds widget data for the sample package."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spec := firstTypeSpec(t, "package sample\n\n"+tc.decl+"\n")
			if got := generateTypeDoc(spec, "sample"); got != tc.want {
				t.Errorf("generateTypeDoc() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestGenerateHandlerDoc_HelperShapes verifies the unexported-function
// generator: a two-result handler names its output type, the conversion,
// formatting and building prefixes get their fixed sentences, and anything
// else falls through to the helper-intent rules.
func TestGenerateHandlerDoc_HelperShapes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		decl string
		want string
	}{
		{
			name: "two results name the output type",
			decl: "func listThings(ctx context.Context) (*Output, error) { return nil, nil }",
			want: "listThings lists things and returns [*Output].",
		},
		{
			name: "conversion helper",
			decl: "func toOutput() {}",
			want: "toOutput converts the GitLab API response to the tool output format.",
		},
		{
			name: "formatter",
			decl: "func formatRow() {}",
			want: "formatRow renders the result as a formatted string.",
		},
		{
			name: "builder",
			decl: "func buildParams() {}",
			want: "buildParams constructs the request parameters from the input.",
		},
		{
			name: "falls through to the helper rules",
			decl: "func widgetize() {}",
			want: "widgetize implements the widgetize helper used by sample.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			decl := firstFuncDecl(t, "package sample\n\n"+tc.decl+"\n")
			if got := generateHandlerDoc(decl, "sample"); got != tc.want {
				t.Errorf("generateHandlerDoc() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestHelperIntentDoc_RuleFamilies verifies each family of helper-doc rules
// in the order they are consulted: the exact names, the prefix rules, the
// content-marker rules, the evaluator prefixes, and the fallback.
func TestHelperIntentDoc_RuleFamilies(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want string
	}{
		{name: "main", want: "main starts the command-line workflow."},
		{name: "run", want: "run executes the command workflow after arguments are parsed."},
		{name: "isReady", want: "isReady reports whether is ready."},
		{name: "routeAvailable", want: "routeAvailable reports whether route available."},
		{name: "normalizeName", want: "normalizeName normalizes name for stable comparisons."},
		{name: "filterTasks", want: "filterTasks filters tasks using evaluator options."},
		{name: "orderRows", want: "orderRows orders rows deterministically."},
		{name: "splitLine", want: "splitLine splits line into parsed fields."},
		{name: "defaultTimeout", want: "defaultTimeout returns the default timeout."},
		{name: "parseFlags", want: "parseFlags parses flags from evaluator input."},
		{name: "publishDocs", want: "publishDocs publishes docs into managed documentation."},
		{name: "sectionBody", want: "sectionBody extracts body from a managed Markdown section."},
		{name: "requestPath", want: "requestPath returns the request path used by evaluator requests."},
		{name: "modelPricing", want: "modelPricing reports whether model pricing data is configured."},
		{name: "sanitizeInput", want: "sanitizeInput sanitizes input for provider compatibility."},
		{name: "taskSteps", want: "taskSteps returns expected tool steps for an evaluation task."},
		{name: "widgetize", want: "widgetize implements the widgetize helper used by sample."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := helperIntentDoc(tc.name, "sample"); got != tc.want {
				t.Errorf("helperIntentDoc(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestHelperDocRuleFormat_SubjectSelection verifies how a rule builds its
// subject: nameOnly emits the helper name alone, useWords passes the
// already-split words through, and otherwise the configured prefixes are
// trimmed before the name is split.
func TestHelperDocRuleFormat_SubjectSelection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		rule helperDocRule
		want string
	}{
		{
			name: "nameOnly",
			rule: helperDocRule{template: "%s is fixed.", nameOnly: true},
			want: "normalizeProjectName is fixed.",
		},
		{
			name: "useWords",
			rule: helperDocRule{template: "%s handles %s.", useWords: true},
			want: "normalizeProjectName handles normalize project name.",
		},
		{
			name: "trimmed prefixes",
			rule: helperDocRule{template: "%s handles %s.", trimPrefixes: []string{"normalize"}},
			want: "normalizeProjectName handles project name.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			const name = "normalizeProjectName"
			if got := tc.rule.format(name, camelToWords(name)); got != tc.want {
				t.Errorf("format() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestGenerateTestHelperDoc_PrefixRules verifies every prefix rule that
// describes an unexported helper in a test file, including the two rules
// whose subject is the whole phrase rather than the trimmed name, and the
// fallback for a helper matching no prefix.
func TestGenerateTestHelperDoc_PrefixRules(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want string
	}{
		{name: "assertBody", want: "assertBody checks body invariants for tests."},
		{name: "requireProject", want: "requireProject returns project test data or fails the test."},
		{name: "mustBuildClient", want: "mustBuildClient builds client test fixtures and fails the test on error."},
		{name: "mustCreate", want: "mustCreate prepares create test fixtures and fails the test on error."},
		{name: "writeFixture", want: "writeFixture writes fixture fixture data for tests."},
		{name: "findIssue", want: "findIssue locates issue fixture data for assertions."},
		{name: "hasLabel", want: "hasLabel reports whether has label."},
		{name: "loadGolden", want: "loadGolden loads golden fixture data for tests."},
		{name: "newServer", want: "newServer constructs server test fixtures."},
		{name: "seedIssues", want: "seedIssues seeds issues test fixtures."},
		{name: "schemaFor", want: "schemaFor extracts schema for details for schema assertions."},
		{name: "normalizedJSON", want: "normalizedJSON normalizes JSON for stable test assertions."},
		{name: "compareTrees", want: "compareTrees compares trees snapshots and reports drift."},
		{name: "appendDiff", want: "appendDiff appends diff diagnostics to the test diff."},
		{name: "sortRows", want: "sortRows sorts rows fixtures into deterministic order."},
		{name: "missingKeys", want: "missingKeys returns missing keys values for assertion messages."},
		{name: "textOf", want: "textOf extracts text of from MCP result content for assertions."},
		{name: "helperThing", want: "helperThing supports helper thing assertions in sample tests."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			decl := firstFuncDecl(t, "package sample\n\nfunc "+tc.name+"() {}\n")
			if got := generateFuncDoc(decl, "sample", true); got != tc.want {
				t.Errorf("generateFuncDoc(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestGenerateMethodDoc_TemplatesAndPrefixes verifies the method generator:
// the exact-name templates, the Get/Set/ensure/cleanup prefixes, the
// bool-result sentence, and the fallbacks for an unrecognized name and a
// missing receiver.
func TestGenerateMethodDoc_TemplatesAndPrefixes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		decl string
		want string
	}{
		{name: "error", decl: "func (client) Error() string { return \"\" }", want: "Error returns the error message for client."},
		{name: "read", decl: "func (client) Read(p []byte) (int, error) { return 0, nil }", want: "Read streams data from client into p."},
		{name: "round trip", decl: "func (client) RoundTrip(r *http.Request) (*http.Response, error) { return nil, nil }", want: "RoundTrip executes an HTTP request through client."},
		{name: "marshal", decl: "func (client) MarshalJSON() ([]byte, error) { return nil, nil }", want: "MarshalJSON encodes client into the JSON shape expected by the provider."},
		{name: "unmarshal", decl: "func (client) UnmarshalJSON(b []byte) error { return nil }", want: "UnmarshalJSON decodes client from the provider JSON shape."},
		{name: "pointer receiver is unwrapped", decl: "func (*client) String() string { return \"\" }", want: "String returns the display label for client."},
		{name: "setter", decl: "func (config) SetToken(v string) {}", want: "SetToken updates the token value on config."},
		{name: "ensure", decl: "func (fixture) ensureProject() {}", want: "ensureProject ensures project exists for fixture."},
		{name: "cleanup", decl: "func (fixture) cleanupProject() {}", want: "cleanupProject removes cleanup project fixture resources for fixture when present."},
		{name: "delete", decl: "func (fixture) deleteProject() {}", want: "deleteProject removes delete project fixture resources for fixture when present."},
		{name: "unrecognized name", decl: "func (widget) Render() string { return \"\" }", want: "Render handles render for widget."},
		{name: "no receiver falls back", decl: "func doThing() {}", want: "doThing handles do thing for receiver."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			decl := firstFuncDecl(t, "package sample\n\n"+tc.decl+"\n")
			if got := generateMethodDoc(decl); got != tc.want {
				t.Errorf("generateMethodDoc() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestGenerateExportedFuncDoc_SpecialNames verifies the registration and
// Markdown-formatter names get their fixed sentences and any other exported
// function is described by its inferred action.
func TestGenerateExportedFuncDoc_SpecialNames(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want string
	}{
		{name: "RegisterTools", want: "RegisterTools registers all sample-related MCP tools on the given server."},
		{name: "RegisterMeta", want: "RegisterMeta registers the sample domain meta-tool on the given server."},
		{name: "FormatMarkdownList", want: "FormatMarkdownList renders the sample result as a Markdown-formatted MCP response."},
		{name: "ListProjects", want: "ListProjects lists projects for the sample package."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			decl := firstFuncDecl(t, "package sample\n\nfunc "+tc.name+"() {}\n")
			if got := generateFuncDoc(decl, "sample", false); got != tc.want {
				t.Errorf("generateFuncDoc(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestGenerateValueDoc_ConstVarAndBlankNames verifies constants and
// variables get their respective sentences and that a declaration naming
// only the blank identifier is skipped.
func TestGenerateValueDoc_ConstVarAndBlankNames(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		decl string
		want string
	}{
		{name: "const", decl: "const defaultLimit = 20", want: "defaultLimit identifies the default limit constant used by this package."},
		{name: "var", decl: "var cache = 1", want: "cache stores the package-level cache state."},
		{name: "blank only", decl: "var _ = 1", want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spec, tok := firstValueSpec(t, "package sample\n\n"+tc.decl+"\n")
			if got := generateValueDoc(spec, tok); got != tc.want {
				t.Errorf("generateValueDoc() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestGenerateTestDoc_NameShapes verifies the test-comment generator for the
// subject-and-scenario form, the subject-only form, each with and without a
// case table, and the fallback for a name matching neither pattern.
func TestGenerateTestDoc_NameShapes(t *testing.T) {
	t.Parallel()

	const table = "cases := []struct{ name string }{{\"ok\"}}; _ = cases"
	testCases := []struct {
		name string
		decl string
		want string
	}{
		{
			name: "subject and scenario without a table",
			decl: "func TestWidget_Renders(t *testing.T) {}",
			want: "TestWidget_Renders verifies Widget when renders.",
		},
		{
			name: "subject and scenario with a table",
			decl: "func TestWidget_Renders(t *testing.T) { " + table + " }",
			want: "TestWidget_Renders covers Widget with table-driven subtests for renders.",
		},
		{
			name: "subject only without a table",
			decl: "func TestWidget(t *testing.T) {}",
			want: "TestWidget verifies Widget.",
		},
		{
			name: "subject only with a table",
			decl: "func TestWidget(t *testing.T) { " + table + " }",
			want: "TestWidget covers Widget with table-driven subtests.",
		},
		{
			name: "name matching neither pattern",
			decl: "func Test(t *testing.T) {}",
			want: "Test verifies the expected behavior of sample.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			decl := firstFuncDecl(t, "package sample\n\nimport \"testing\"\n\n"+tc.decl+"\n")
			if got := generateFuncDoc(decl, "sample", true); got != tc.want {
				t.Errorf("generateFuncDoc() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSubjectScenarioPhrase_ClauseOrdering verifies how a scenario is turned
// into prose: a predicate moves ahead of its context, a non-predicate
// behavior reads as a "when" clause, and a single-segment scenario is split
// at its predicate when it has one.
func TestSubjectScenarioPhrase_ClauseOrdering(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		scenario string
		want     string
	}{
		{name: "predicate behavior moves first", scenario: "Invalid_ReturnsError", want: "Build returns error with invalid"},
		{name: "non-predicate behavior reads as when", scenario: "Empty_State", want: "Build when empty state"},
		{name: "phrase starting with a predicate", scenario: "ReturnsError", want: "Build returns error"},
		{name: "predicate inside the phrase", scenario: "InvalidInputReturnsError", want: "Build returns error for invalid input"},
		{name: "no predicate at all", scenario: "Empty", want: "Build when empty"},
		{name: "is reads as a predicate", scenario: "IsEmpty", want: "Build is empty"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := subjectScenarioPhrase("Build", tc.scenario); got != tc.want {
				t.Errorf("subjectScenarioPhrase(Build, %q) = %q, want %q", tc.scenario, got, tc.want)
			}
		})
	}
}

// TestIsGeneratedDoc_MarkersAndPairs verifies a comment is treated as
// regenerable when it carries a single marker phrase or both halves of a
// marker pair, and is preserved when it carries only one half.
func TestIsGeneratedDoc_MarkersAndPairs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		text string
		want bool
	}{
		{name: "single marker", text: "helper verifies the behavior of helper.", want: true},
		{name: "both halves of a pair", text: "helper handles the error scenario correctly.", want: true},
		{name: "only the first half", text: "helper handles the error path.", want: false},
		{name: "only the second half", text: "helper covers the scenario correctly.", want: false},
		{name: "hand-written comment", text: "helper resolves the project from its remote URL.", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isGeneratedDoc(tc.text); got != tc.want {
				t.Errorf("isGeneratedDoc(%q) = %t, want %t", tc.text, got, tc.want)
			}
		})
	}
}

// TestInferAction_PrefixesAndFallback verifies the CRUD verb inference,
// including the empty remainder that reads as "resources" and the fallback
// for a name with no known prefix.
func TestInferAction_PrefixesAndFallback(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		in   string
		want string
	}{
		{name: "list with a remainder", in: "ListProjects", want: "lists projects"},
		{name: "list with no remainder", in: "List", want: "lists resources"},
		{name: "unprotect", in: "UnprotectBranch", want: "removes protection from branch"},
		{name: "trace", in: "TraceJob", want: "retrieves the trace of job"},
		{name: "unsubscribe", in: "UnsubscribeIssue", want: "unsubscribes from issue"},
		{name: "no known prefix", in: "Widgetize", want: "coordinates widgetize"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := inferAction(tc.in); got != tc.want {
				t.Errorf("inferAction(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestCamelToWords_SplitsAndInitialisms verifies identifier-to-prose
// conversion: the empty name, digit boundaries, underscores, preserved
// initialisms and the multi-word replacements.
func TestCamelToWords_SplitsAndInitialisms(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "resources"},
		{name: "underscore only collapses to nothing", in: "_", want: "resources"},
		{name: "underscores", in: "project_id", want: "project ID"},
		{name: "digit boundary", in: "v2Client", want: "v 2 client"},
		{name: "initialism", in: "JSONPayload", want: "JSON payload"},
		{name: "multi-word replacement", in: "GitLabCICD", want: "GitLab CI/CD"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := camelToWords(tc.in); got != tc.want {
				t.Errorf("camelToWords(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestDocIdentifier_EmptySubject verifies an empty subject is named rather
// than left as a blank in the generated sentence.
func TestDocIdentifier_EmptySubject(t *testing.T) {
	t.Parallel()

	if got := docIdentifier(""); got != "the subject under test" {
		t.Errorf("docIdentifier(\"\") = %q, want the subject under test", got)
	}
	if got := docIdentifier("Widget"); got != "Widget" {
		t.Errorf("docIdentifier(Widget) = %q, want Widget", got)
	}
}

// TestGetIndent_LeadingWhitespace verifies the indentation helper returns
// the leading tabs or spaces, and the empty string for a line that has no
// content to indent.
func TestGetIndent_LeadingWhitespace(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		line string
		want string
	}{
		{name: "tab", line: "\tfunc x() {}", want: "\t"},
		{name: "spaces", line: "    func x() {}", want: "    "},
		{name: "none", line: "func x() {}", want: ""},
		{name: "whitespace only", line: "   ", want: ""},
		{name: "empty", line: "", want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := getIndent(tc.line); got != tc.want {
				t.Errorf("getIndent(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

// TestFirstDoc_PrefersTheSymbolComment verifies the symbol's own doc wins
// over the enclosing declaration's, and the declaration's is used when the
// symbol has none.
func TestFirstDoc_PrefersTheSymbolComment(t *testing.T) {
	t.Parallel()

	primary := &ast.CommentGroup{List: []*ast.Comment{{Text: "// primary"}}}
	fallback := &ast.CommentGroup{List: []*ast.Comment{{Text: "// fallback"}}}
	if got := firstDoc(primary, fallback); got != primary {
		t.Errorf("firstDoc(primary, fallback) = %v, want the primary comment", got)
	}
	if got := firstDoc(nil, fallback); got != fallback {
		t.Errorf("firstDoc(nil, fallback) = %v, want the fallback comment", got)
	}
}

// TestDocInsertions_SkipTokensTheyDoNotOwn verifies the type and value
// insertion helpers ignore a declaration introduced by another token, which
// is what keeps import blocks out of the rewrite.
func TestDocInsertions_SkipTokensTheyDoNotOwn(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	t.Run("value spec under an import declaration", func(t *testing.T) {
		t.Parallel()
		decl := &ast.GenDecl{Tok: token.IMPORT}
		spec := &ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent("x")}}
		if _, ok := valueSpecDocInsertion(fset, decl, spec); ok {
			t.Error("valueSpecDocInsertion() = true for an import declaration, want false")
		}
	})
	t.Run("type spec under a const declaration", func(t *testing.T) {
		t.Parallel()
		decl := &ast.GenDecl{Tok: token.CONST}
		spec := &ast.TypeSpec{Name: ast.NewIdent("Widget")}
		if _, ok := typeSpecDocInsertion(fset, decl, spec, "sample"); ok {
			t.Error("typeSpecDocInsertion() = true for a const declaration, want false")
		}
	})
}

// TestGenerateFuncDoc_MethodDecl_UsesMethodDoc verifies generateFuncDoc routes
// a declaration with a receiver to the method generator rather than the
// function generators, which the isTest/exported branches above it never
// exercise for a method.
func TestGenerateFuncDoc_MethodDecl_UsesMethodDoc(t *testing.T) {
	t.Parallel()

	method := firstFuncDecl(t, "package sample\ntype widget struct{}\nfunc (widget) Run() bool { return true }\n")
	got := generateFuncDoc(method, "sample", false)
	if want := "Run reports whether the widget satisfies the run condition."; got != want {
		t.Fatalf("generateFuncDoc(method) = %q, want %q", got, want)
	}
}

// TestTestHasTableDrivenCases_BodylessDeclReturnsFalse verifies a declaration
// with no body (a forward declaration for an assembly implementation) is
// reported as not table-driven instead of panicking on a nil body.
func TestTestHasTableDrivenCases_BodylessDeclReturnsFalse(t *testing.T) {
	t.Parallel()

	decl := firstFuncDecl(t, "package sample\nfunc TestWidget(t *testing.T)\n")
	if testHasTableDrivenCases(decl) {
		t.Error("testHasTableDrivenCases(bodyless) = true, want false")
	}
}

// TestIsTableDrivenCompositeLit_Shapes verifies the composite-literal
// classifier: only an array of structs is a table, while a struct literal
// (non-array type) and an array of a non-struct element are not.
func TestIsTableDrivenCompositeLit_Shapes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		expr string
		want bool
	}{
		{name: "array of structs is table-driven", expr: "[]struct{ name string }{}", want: true},
		{name: "struct literal is not an array", expr: "point{}", want: false},
		{name: "array of non-structs is not table-driven", expr: "[]int{}", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			node, err := parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatalf("ParseExpr(%q) error = %v", tc.expr, err)
			}
			if got := isTableDrivenCompositeLit(node); got != tc.want {
				t.Errorf("isTableDrivenCompositeLit(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

// TestProcessFile_OutOfRangeInsertionIsSkipped verifies the bounds guard in the
// insertion loop drops an insertion whose line range falls outside the file,
// leaving the source unchanged. A real parse never yields such a range, so it
// drives the collectInsertions seam.
func TestProcessFile_OutOfRangeInsertionIsSkipped(t *testing.T) {
	original := collectInsertions
	collectInsertions = func(*token.FileSet, *ast.File, string, bool) []insertion {
		return []insertion{{startLine: 9999, endLine: 9998, comment: "ignored"}}
	}
	t.Cleanup(func() { collectInsertions = original })

	source := "package sample\n\nfunc ListProjects() {}\n"
	path := writeFixFile(t, t.TempDir(), "sample.go", source)

	captureFixStdout(t, func() {
		if err := processFile(path); err != nil {
			t.Errorf("processFile() error = %v", err)
		}
	})
	if got := readFixFile(t, path); got != source {
		t.Fatalf("processFile() rewrote the file for an out-of-range insertion:\n%s", got)
	}
}

// TestProcessFile_WriteErrorIsReported verifies processFile surfaces a write
// failure. A filesystem the test owns, running as root, never refuses the
// write, so it drives the writeSource seam.
func TestProcessFile_WriteErrorIsReported(t *testing.T) {
	original := writeSource
	writeSource = func(string, []byte) error { return errors.New("disk full") }
	t.Cleanup(func() { writeSource = original })

	path := writeFixFile(t, t.TempDir(), "sample.go", "package sample\n\nfunc ListProjects() {}\n")
	if err := processFile(path); err == nil || !strings.HasPrefix(err.Error(), "write ") {
		t.Fatalf("processFile() error = %v, want write error", err)
	}
}

// TestFuncDocInsertion_EmptyCommentIsSkipped verifies funcDocInsertion reports
// no insertion when the generator yields an empty comment. Every real
// declaration produces a non-empty comment, so it drives the genFuncDoc seam.
func TestFuncDocInsertion_EmptyCommentIsSkipped(t *testing.T) {
	original := genFuncDoc
	genFuncDoc = func(*ast.FuncDecl, string, bool) string { return "" }
	t.Cleanup(func() { genFuncDoc = original })

	decl := firstFuncDecl(t, "package sample\nfunc Widget() {}\n")
	if _, ok := funcDocInsertion(token.NewFileSet(), decl, "sample", false); ok {
		t.Error("funcDocInsertion() = true for an empty comment, want false")
	}
}

// TestTypeSpecDocInsertion_EmptyCommentIsSkipped verifies typeSpecDocInsertion
// reports no insertion when the generator yields an empty comment. Every real
// type produces a non-empty comment, so it drives the genTypeDoc seam.
func TestTypeSpecDocInsertion_EmptyCommentIsSkipped(t *testing.T) {
	original := genTypeDoc
	genTypeDoc = func(*ast.TypeSpec, string) string { return "" }
	t.Cleanup(func() { genTypeDoc = original })

	decl := &ast.GenDecl{Tok: token.TYPE}
	spec := &ast.TypeSpec{Name: ast.NewIdent("Widget")}
	if _, ok := typeSpecDocInsertion(token.NewFileSet(), decl, spec, "sample"); ok {
		t.Error("typeSpecDocInsertion() = true for an empty comment, want false")
	}
}
