// Command add_docs adds godoc-compliant doc comments to Go source and test
// files that are missing documentation. It uses go/ast to parse files,
// identify undocumented symbols (functions, types, methods), and inserts
// context-aware doc comments based on naming conventions.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// main walks the specified directory and adds godoc comments to undocumented symbols.
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/add_docs/ <dir>...")
		os.Exit(1)
	}
	for _, dir := range os.Args[1:] {
		processDir(dir)
	}
}

// processDir recursively walks a directory and processes each .go file.
func processDir(dir string) {
	cleanDir := filepath.Clean(dir)
	entries, err := os.ReadDir(cleanDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "readdir %s: %v\n", cleanDir, err)
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			processDir(filepath.Join(cleanDir, e.Name()))
			continue
		}
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		processFile(filepath.Join(cleanDir, e.Name()))
	}
}

// processFile parses a single Go file and adds missing doc comments to
// undocumented functions, types, and methods.
func processFile(path string) {
	cleanPath := filepath.Clean(path)
	src, err := os.ReadFile(cleanPath) //#nosec G304 -- paths come from CLI args, not user input
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", cleanPath, err)
		return
	}
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, cleanPath, src, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", cleanPath, err)
		return
	}
	pkgName := node.Name.Name
	isTest := strings.HasSuffix(cleanPath, "_test.go")

	type insertion struct {
		line    int
		comment string
	}
	var insertions []insertion

	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Doc != nil && len(d.Doc.List) > 0 {
				continue
			}
			if d.Name.Name == "init" {
				continue
			}
			line := fset.Position(d.Pos()).Line
			comment := generateFuncDoc(d, pkgName, isTest)
			if comment != "" {
				insertions = append(insertions, insertion{line: line, comment: comment})
			}
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if ts.Doc != nil && len(ts.Doc.List) > 0 {
					continue
				}
				if d.Doc != nil && len(d.Doc.List) > 0 {
					continue
				}
				line := fset.Position(d.Pos()).Line
				comment := generateTypeDoc(ts, pkgName)
				if comment != "" {
					insertions = append(insertions, insertion{line: line, comment: comment})
				}
			}
		}
	}

	if len(insertions) == 0 {
		return
	}

	lines := splitLines(src)
	for _, ins := range slices.Backward(insertions) {
		idx := ins.line - 1
		if idx < 0 || idx >= len(lines) {
			continue
		}
		indent := getIndent(lines[idx])
		commentLines := formatComment(ins.comment, indent)
		newLines := make([]string, 0, len(lines)+len(commentLines))
		newLines = append(newLines, lines[:idx]...)
		newLines = append(newLines, commentLines...)
		newLines = append(newLines, lines[idx:]...)
		lines = newLines
	}

	result := strings.Join(lines, "\n")
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	err = os.WriteFile(cleanPath, []byte(result), 0o600) //#nosec G703 -- CLI tool, paths from args
	if err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", cleanPath, err)
		return
	}
	fmt.Printf("documented %s (%d symbols)\n", cleanPath, len(insertions))
}

// splitLines splits a string into individual lines.
func splitLines(src []byte) []string {
	s := strings.TrimRight(string(src), "\n")
	return strings.Split(s, "\n")
}

// getIndent returns the leading whitespace of the given line.
func getIndent(line string) string {
	for i, c := range line {
		if c != '\t' && c != ' ' {
			return line[:i]
		}
	}
	return ""
}

// formatComment wraps a doc string as a Go line comment with proper indentation.
func formatComment(text, indent string) []string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	for _, l := range lines {
		result = append(result, indent+"// "+l)
	}
	return result
}

// generateFuncDoc generates a doc comment for an unexported function based
// on its name, parameters, and return types.
func generateFuncDoc(d *ast.FuncDecl, pkgName string, isTest bool) string {
	name := d.Name.Name
	if isTest && strings.HasPrefix(name, "Test") {
		return generateTestDoc(d, pkgName)
	}
	if isTest && strings.HasPrefix(name, "Benchmark") {
		return fmt.Sprintf("%s measures the performance of the %s operation.", name, camelToWords(strings.TrimPrefix(name, "Benchmark")))
	}
	if isTest && strings.HasPrefix(name, "Fuzz") {
		return fmt.Sprintf("%s tests that %s handles arbitrary inputs without panicking.", name, camelToWords(strings.TrimPrefix(name, "Fuzz")))
	}
	if isTest && strings.HasPrefix(name, "Example") {
		return fmt.Sprintf("%s demonstrates usage of %s.", name, camelToWords(strings.TrimPrefix(name, "Example")))
	}
	if d.Recv != nil {
		return generateMethodDoc(d)
	}
	if !d.Name.IsExported() {
		return generateHandlerDoc(d, pkgName)
	}
	return generateExportedFuncDoc(d, pkgName)
}

// testNameRe splits test names that follow the TestSubject_Scenario convention.
var testNameRe = regexp.MustCompile(`^Test([A-Z]\w+?)_(\w+)$`)

// testSimpleRe matches test names that only identify the subject under test.
var testSimpleRe = regexp.MustCompile(`^Test([A-Z]\w+)$`)

// generateTestDoc generates a doc comment for a Test function based on its
// name and the inferred scenario.
func generateTestDoc(d *ast.FuncDecl, pkgName string) string {
	name := d.Name.Name
	isTableDriven := false
	if d.Body != nil {
		ast.Inspect(d.Body, func(n ast.Node) bool {
			if cl, ok := n.(*ast.CompositeLit); ok {
				var at *ast.ArrayType
				if at, ok = cl.Type.(*ast.ArrayType); ok {
					if _, ok = at.Elt.(*ast.StructType); ok {
						isTableDriven = true
						return false
					}
				}
			}
			return true
		})
	}
	if m := testNameRe.FindStringSubmatch(name); m != nil {
		funcPart := m[1]
		scenario := camelToWords(m[2])
		if isTableDriven {
			return fmt.Sprintf("%s validates %s across multiple scenarios using table-driven subtests\nfor the %s case.", name, camelToWords(funcPart), scenario)
		}
		return fmt.Sprintf("%s verifies that %s handles the %s scenario correctly.", name, funcPart, scenario)
	}
	if m := testSimpleRe.FindStringSubmatch(name); m != nil {
		funcPart := m[1]
		if isTableDriven {
			return fmt.Sprintf("%s validates %s across multiple scenarios using table-driven subtests.", name, camelToWords(funcPart))
		}
		return fmt.Sprintf("%s verifies the behavior of %s.", name, camelToWords(funcPart))
	}
	return fmt.Sprintf("%s verifies the expected behavior of %s.", name, pkgName)
}

// generateMethodDoc generates a doc comment for a method based on its
// receiver type and name.
func generateMethodDoc(d *ast.FuncDecl) string {
	name := d.Name.Name
	recvType := ""
	if d.Recv != nil && len(d.Recv.List) > 0 {
		recvType = exprToString(d.Recv.List[0].Type)
	}
	if d.Type.Results != nil && len(d.Type.Results.List) == 1 {
		if ident, ok := d.Type.Results.List[0].Type.(*ast.Ident); ok && ident.Name == "bool" {
			return fmt.Sprintf("%s reports whether the %s satisfies the %s condition.", name, recvType, camelToWords(name))
		}
	}
	return fmt.Sprintf("%s performs the %s operation on %s.", name, camelToWords(name), recvType)
}

// generateHandlerDoc generates a doc comment for an MCP tool handler
// function based on its name and input type.
func generateHandlerDoc(d *ast.FuncDecl, pkgName string) string {
	name := d.Name.Name
	if d.Type.Results != nil && len(d.Type.Results.List) == 2 {
		returnType := exprToString(d.Type.Results.List[0].Type)
		action := inferAction(name)
		return fmt.Sprintf("%s %s using the GitLab API and returns [%s].", name, action, returnType)
	}
	if strings.Contains(name, "ToOutput") || strings.HasPrefix(name, "to") {
		return fmt.Sprintf("%s converts the GitLab API response to the tool output format.", name)
	}
	if strings.HasPrefix(name, "format") || strings.HasPrefix(name, "Format") {
		return fmt.Sprintf("%s renders the result as a formatted string.", name)
	}
	if strings.HasPrefix(name, "build") || strings.HasPrefix(name, "Build") {
		return fmt.Sprintf("%s constructs the request parameters from the input.", name)
	}
	return fmt.Sprintf("%s is an internal helper for the %s package.", name, pkgName)
}

// generateExportedFuncDoc generates a doc comment for an exported function
// based on its name, parameters, and return types.
func generateExportedFuncDoc(d *ast.FuncDecl, pkgName string) string {
	name := d.Name.Name
	if name == "RegisterTools" {
		return fmt.Sprintf("RegisterTools registers all %s-related MCP tools on the given server.", pkgName)
	}
	if name == "RegisterMeta" {
		return fmt.Sprintf("RegisterMeta registers the %s domain meta-tool on the given server.", pkgName)
	}
	if strings.HasPrefix(name, "FormatMarkdown") {
		return fmt.Sprintf("%s renders the %s result as a Markdown-formatted MCP response.", name, pkgName)
	}
	action := inferAction(name)
	return fmt.Sprintf("%s %s for the %s package.", name, action, pkgName)
}

// generateTypeDoc generates a doc comment for a type declaration based on
// its name and kind (struct, interface, etc.).
func generateTypeDoc(ts *ast.TypeSpec, pkgName string) string {
	name := ts.Name.Name
	if action, ok := strings.CutSuffix(name, "Input"); ok {
		if action == "" {
			return fmt.Sprintf("%s defines parameters for the %s tool.", name, pkgName)
		}
		return fmt.Sprintf("%s defines parameters for the %s operation.", name, camelToWords(action))
	}
	if action, ok := strings.CutSuffix(name, "Output"); ok {
		if action == "" {
			return fmt.Sprintf("%s represents the response from a %s operation.", name, pkgName)
		}
		return fmt.Sprintf("%s represents the response from the %s operation.", name, camelToWords(action))
	}
	if _, ok := ts.Type.(*ast.InterfaceType); ok {
		return fmt.Sprintf("%s defines the contract for %s operations.", name, camelToWords(name))
	}
	return fmt.Sprintf("%s holds data for %s operations.", name, pkgName)
}

// inferAction infers the CRUD action from a function name by matching
// common prefixes like create, get, list, update, and delete.
func inferAction(name string) string {
	lower := strings.ToLower(name)
	actions := []struct{ prefix, verb string }{
		{"list", "lists"}, {"get", "retrieves"}, {"create", "creates"},
		{"update", "updates"}, {"delete", "deletes"}, {"set", "configures"},
		{"protect", "protects"}, {"unprotect", "removes protection from"},
		{"merge", "merges"}, {"approve", "approves"}, {"search", "searches for"},
		{"publish", "publishes"}, {"download", "downloads"}, {"upload", "uploads"},
		{"close", "closes"}, {"reopen", "reopens"}, {"rebase", "rebases"},
		{"cancel", "cancels"}, {"retry", "retries"}, {"lint", "validates"},
		{"add", "adds"}, {"remove", "removes"}, {"edit", "edits"},
		{"run", "runs"}, {"lock", "locks"}, {"unlock", "unlocks"},
		{"resolve", "resolves"}, {"unresolve", "unresolves"},
		{"restore", "restores"}, {"play", "triggers"}, {"erase", "erases"},
		{"trace", "retrieves the trace of"}, {"subscribe", "subscribes to"},
		{"unsubscribe", "unsubscribes from"}, {"transfer", "transfers"},
		{"fork", "forks"}, {"archive", "archives"}, {"unarchive", "unarchives"},
		{"star", "stars"}, {"unstar", "unstars"}, {"share", "shares"},
		{"unshare", "unshares"}, {"promote", "promotes"}, {"request", "requests"},
		{"accept", "accepts"}, {"reject", "rejects"}, {"revoke", "revokes"},
		{"rotate", "rotates"}, {"trigger", "triggers"}, {"check", "checks"},
		{"mark", "marks"}, {"browse", "browses"}, {"compare", "compares"},
		{"render", "renders"}, {"validate", "validates"},
	}
	for _, a := range actions {
		if strings.HasPrefix(lower, a.prefix) {
			rest := camelToWords(name[len(a.prefix):])
			if rest == "" || rest == "resources" {
				return a.verb + " resources"
			}
			return a.verb + " " + rest
		}
	}
	return "performs the " + camelToWords(name) + " operation"
}

// camelToWords splits a camelCase identifier into lowercase words.
func camelToWords(s string) string {
	if s == "" {
		return "resources"
	}
	var buf bytes.Buffer
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			buf.WriteByte(' ')
		}
		buf.WriteRune(r)
	}
	result := strings.ToLower(strings.TrimSpace(buf.String()))
	if result == "" {
		return "resources"
	}
	return result
}

// exprToString converts an AST expression node to its source string
// representation.
func exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + exprToString(e.X)
	case *ast.SelectorExpr:
		return exprToString(e.X) + "." + e.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprToString(e.Elt)
	case *ast.MapType:
		return "map[" + exprToString(e.Key) + "]" + exprToString(e.Value)
	default:
		return "any"
	}
}
