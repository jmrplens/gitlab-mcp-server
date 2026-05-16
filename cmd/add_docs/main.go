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

// main walks the specified paths and adds godoc comments to undocumented symbols.
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/add_docs/ <path>...")
		os.Exit(1)
	}
	for _, path := range os.Args[1:] {
		processPath(path)
	}
}

// processPath processes a Go file or recursively processes a directory.
func processPath(path string) {
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stat %s: %v\n", cleanPath, err)
		return
	}
	if info.IsDir() {
		processDir(cleanPath)
		return
	}
	if strings.HasSuffix(info.Name(), ".go") {
		processFile(cleanPath)
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
		startLine int
		endLine   int
		comment   string
	}
	var insertions []insertion

	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Doc != nil && len(d.Doc.List) > 0 && !isGeneratedDoc(d.Doc.Text()) {
				continue
			}
			if d.Name.Name == "init" {
				continue
			}
			startLine, endLine := editRangeForDoc(fset, d.Doc, d.Pos())
			comment := generateFuncDoc(d, pkgName, isTest)
			if comment != "" {
				insertions = append(insertions, insertion{startLine: startLine, endLine: endLine, comment: comment})
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if d.Tok != token.TYPE {
						continue
					}
					if s.Doc != nil && len(s.Doc.List) > 0 && !isGeneratedDoc(s.Doc.Text()) {
						continue
					}
					if s.Doc == nil && d.Doc != nil && len(d.Doc.List) > 0 && !isGeneratedDoc(d.Doc.Text()) {
						continue
					}
					startLine, endLine := editRangeForDoc(fset, firstDoc(s.Doc, d.Doc), s.Pos())
					comment := generateTypeDoc(s, pkgName)
					if comment != "" {
						insertions = append(insertions, insertion{startLine: startLine, endLine: endLine, comment: comment})
					}
				case *ast.ValueSpec:
					if d.Tok != token.CONST && d.Tok != token.VAR {
						continue
					}
					if s.Doc != nil && len(s.Doc.List) > 0 && !isGeneratedDoc(s.Doc.Text()) {
						continue
					}
					if s.Doc == nil && d.Doc != nil && len(d.Doc.List) > 0 && !isGeneratedDoc(d.Doc.Text()) {
						continue
					}
					startLine, endLine := editRangeForDoc(fset, firstDoc(s.Doc, d.Doc), s.Pos())
					comment := generateValueDoc(s, d.Tok)
					if comment != "" {
						insertions = append(insertions, insertion{startLine: startLine, endLine: endLine, comment: comment})
					}
				}
			}
		}
	}

	if len(insertions) == 0 {
		return
	}

	lines := splitLines(src)
	for _, ins := range slices.Backward(insertions) {
		startIdx := ins.startLine - 1
		endIdx := ins.endLine
		if startIdx < 0 || startIdx > len(lines) || endIdx < startIdx || endIdx > len(lines) {
			continue
		}
		indentIdx := startIdx
		indent := getIndent(lines[indentIdx])
		commentLines := formatComment(ins.comment, indent)
		newLines := make([]string, 0, len(lines)+len(commentLines))
		newLines = append(newLines, lines[:startIdx]...)
		newLines = append(newLines, commentLines...)
		newLines = append(newLines, lines[endIdx:]...)
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

// editRangeForDoc returns the line range to replace for an existing doc comment,
// or an empty insertion range immediately before pos when no doc exists.
func editRangeForDoc(fset *token.FileSet, doc *ast.CommentGroup, pos token.Pos) (startLine, endLine int) {
	if doc == nil {
		line := fset.Position(pos).Line
		return line, line - 1
	}
	return fset.Position(doc.Pos()).Line, fset.Position(doc.End()).Line
}

// firstDoc returns the symbol-specific doc when present, otherwise the enclosing
// declaration doc used by grouped const, var, or type declarations.
func firstDoc(primary, fallback *ast.CommentGroup) *ast.CommentGroup {
	if primary != nil {
		return primary
	}
	return fallback
}

// isGeneratedDoc reports whether a comment matches the generic phrases produced
// by earlier versions of this helper and can be safely regenerated.
func isGeneratedDoc(text string) bool {
	text = strings.TrimSpace(text)
	return strings.Contains(text, " handles the ") && strings.Contains(text, " scenario correctly") ||
		strings.Contains(text, "validates ") && strings.Contains(text, " across multiple scenarios using table-driven subtests") ||
		strings.Contains(text, "verifies the behavior of ") ||
		strings.Contains(text, "verifies the expected behavior of ") ||
		strings.Contains(text, " performs the ") && strings.Contains(text, " operation") ||
		strings.Contains(text, " handles ") && strings.Contains(text, " for the ") ||
		strings.Contains(text, " supports ") && strings.Contains(text, " tests for ") ||
		strings.Contains(text, " provides ") && strings.Contains(text, " test support for ") ||
		strings.Contains(text, " coordinates ") && strings.Contains(text, " logic for ") ||
		strings.Contains(text, "measures the performance of the ") ||
		strings.Contains(text, "is an internal helper for the ") ||
		strings.Contains(text, "holds data for ") ||
		strings.Contains(text, " groups ") && strings.Contains(text, " fields used by ") ||
		strings.Contains(text, "describes ") && strings.Contains(text, " data used by the ") ||
		strings.Contains(text, " i ds") ||
		strings.Contains(text, " open ai ") ||
		strings.Contains(text, " git lab ") ||
		strings.Contains(text, " m rs") ||
		strings.Contains(text, " 2 fa") ||
		strings.Contains(text, " using the GitLab API and returns ") ||
		strings.Contains(text, " defines the ") && strings.Contains(text, " constant.") ||
		strings.Contains(text, " names the ") && strings.Contains(text, " value shared by this package.") ||
		strings.Contains(text, " stores the ") && strings.Contains(text, " value.") ||
		strings.Contains(text, " provides the ") && strings.Contains(text, " value shared by this package.")
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
		return fmt.Sprintf("%s measures %s search and dispatch overhead.", name, camelToWords(strings.TrimPrefix(name, "Benchmark")))
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
	if isTest && !d.Name.IsExported() {
		return generateTestHelperDoc(d, pkgName)
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
		scenario := m[2]
		if isTableDriven {
			return fmt.Sprintf("%s covers %s with table-driven subtests for %s.", name, docIdentifier(funcPart), scenarioPhrase(scenario))
		}
		return fmt.Sprintf("%s verifies %s.", name, subjectScenarioPhrase(funcPart, scenario))
	}
	if m := testSimpleRe.FindStringSubmatch(name); m != nil {
		funcPart := m[1]
		if isTableDriven {
			return fmt.Sprintf("%s covers %s with table-driven subtests.", name, docIdentifier(funcPart))
		}
		return fmt.Sprintf("%s verifies %s.", name, docIdentifier(funcPart))
	}
	return fmt.Sprintf("%s verifies the expected behavior of %s.", name, pkgName)
}

// subjectScenarioPhrase combines the subject and scenario portions of a test name
// into a readable sentence fragment.
func subjectScenarioPhrase(subject, scenario string) string {
	parts := strings.Split(scenario, "_")
	if len(parts) > 1 {
		context := scenarioPhrase(parts[0])
		behavior := scenarioPhrase(strings.Join(parts[1:], "_"))
		if startsWithPredicate(behavior) {
			return fmt.Sprintf("%s %s with %s", docIdentifier(subject), behavior, context)
		}
		return fmt.Sprintf("%s when %s %s", docIdentifier(subject), context, behavior)
	}
	phrase := scenarioPhrase(scenario)
	if startsWithPredicate(phrase) {
		return fmt.Sprintf("%s %s", docIdentifier(subject), phrase)
	}
	if before, after, ok := splitAtPredicate(phrase); ok {
		return fmt.Sprintf("%s %s for %s", docIdentifier(subject), after, before)
	}
	return fmt.Sprintf("%s when %s", docIdentifier(subject), phrase)
}

// splitAtPredicate divides a phrase around the first predicate that can move
// before the scenario context in generated test comments.
func splitAtPredicate(phrase string) (before, after string, ok bool) {
	words := strings.Fields(phrase)
	for i := 1; i < len(words); i++ {
		if !isReorderablePredicate(words[i]) {
			continue
		}
		return strings.Join(words[:i], " "), strings.Join(words[i:], " "), true
	}
	return "", "", false
}

// startsWithPredicate reports whether phrase begins with a predicate-like word
// that reads naturally after an identifier in a test comment.
func startsWithPredicate(phrase string) bool {
	first, _, _ := strings.Cut(strings.TrimSpace(phrase), " ")
	return isReorderablePredicate(first) || first == "is"
}

// isReorderablePredicate reports whether a generated scenario verb can move before its context.
func isReorderablePredicate(word string) bool {
	switch word {
	case "accepts", "allows", "applies", "avoids", "binds", "blocks", "captures", "catches", "checks", "classifies", "clamps", "computes", "converts", "creates", "deduplicates", "derives", "detects", "does", "excludes", "falls", "flags", "flows", "handles", "ignores", "includes", "isolates", "leaves", "lists", "matches", "omits", "parses", "passes", "prefers", "preserves", "projects", "records", "rejects", "repairs", "reports", "requires", "respects", "retries", "returns", "scales", "selects", "sorts", "strips", "sums", "suppresses", "syncs", "uses", "validates", "writes":
		return true
	default:
		return false
	}
}

// generateTestHelperDoc describes common test helper roles without hiding them
// behind a generic helper comment.
func generateTestHelperDoc(d *ast.FuncDecl, pkgName string) string {
	name := d.Name.Name
	phrase := camelToWords(name)
	switch {
	case strings.HasPrefix(name, "assert"):
		return fmt.Sprintf("%s checks %s invariants for tests.", name, camelToWords(strings.TrimPrefix(name, "assert")))
	case strings.HasPrefix(name, "require"):
		return fmt.Sprintf("%s returns %s test data or fails the test.", name, camelToWords(strings.TrimPrefix(name, "require")))
	case strings.HasPrefix(name, "mustBuild"):
		return fmt.Sprintf("%s builds %s test fixtures and fails the test on error.", name, camelToWords(strings.TrimPrefix(name, "mustBuild")))
	case strings.HasPrefix(name, "must"):
		return fmt.Sprintf("%s prepares %s test fixtures and fails the test on error.", name, camelToWords(strings.TrimPrefix(name, "must")))
	case strings.HasPrefix(name, "write"):
		return fmt.Sprintf("%s writes %s fixture data for tests.", name, camelToWords(strings.TrimPrefix(name, "write")))
	case strings.HasPrefix(name, "find"):
		return fmt.Sprintf("%s locates %s fixture data for assertions.", name, camelToWords(strings.TrimPrefix(name, "find")))
	case strings.HasPrefix(name, "has") || strings.HasPrefix(name, "contains") || strings.HasPrefix(name, "is"):
		return fmt.Sprintf("%s reports whether %s.", name, phrase)
	case strings.HasPrefix(name, "load"):
		return fmt.Sprintf("%s loads %s fixture data for tests.", name, camelToWords(strings.TrimPrefix(name, "load")))
	case strings.HasPrefix(name, "new"):
		return fmt.Sprintf("%s constructs %s test fixtures.", name, camelToWords(strings.TrimPrefix(name, "new")))
	case strings.HasPrefix(name, "seed"):
		return fmt.Sprintf("%s seeds %s test fixtures.", name, camelToWords(strings.TrimPrefix(name, "seed")))
	case strings.HasPrefix(name, "schema"):
		return fmt.Sprintf("%s extracts %s details for schema assertions.", name, phrase)
	case strings.HasPrefix(name, "normalize") || strings.HasPrefix(name, "normalized"):
		return fmt.Sprintf("%s normalizes %s for stable test assertions.", name, camelToWords(strings.TrimPrefix(strings.TrimPrefix(name, "normalized"), "normalize")))
	case strings.HasPrefix(name, "compare"):
		return fmt.Sprintf("%s compares %s snapshots and reports drift.", name, camelToWords(strings.TrimPrefix(name, "compare")))
	case strings.HasPrefix(name, "append"):
		return fmt.Sprintf("%s appends %s diagnostics to the test diff.", name, camelToWords(strings.TrimPrefix(name, "append")))
	case strings.HasPrefix(name, "sort"):
		return fmt.Sprintf("%s sorts %s fixtures into deterministic order.", name, camelToWords(strings.TrimPrefix(name, "sort")))
	case strings.HasPrefix(name, "missing"):
		return fmt.Sprintf("%s returns missing %s values for assertion messages.", name, camelToWords(strings.TrimPrefix(name, "missing")))
	case strings.HasPrefix(name, "text"):
		return fmt.Sprintf("%s extracts %s from MCP result content for assertions.", name, phrase)
	default:
		return fmt.Sprintf("%s supports %s assertions in %s tests.", name, phrase, pkgName)
	}
}

// generateMethodDoc generates a doc comment for a method based on its
// receiver type and name.
func generateMethodDoc(d *ast.FuncDecl) string {
	name := d.Name.Name
	recvType := ""
	if d.Recv != nil && len(d.Recv.List) > 0 {
		recvType = exprToString(d.Recv.List[0].Type)
	}
	subject := strings.TrimPrefix(strings.TrimPrefix(recvType, "*"), "[]")
	if subject == "" {
		subject = "receiver"
	}
	switch name {
	case "String":
		return fmt.Sprintf("String returns the display label for %s.", subject)
	case "Error":
		return fmt.Sprintf("Error returns the error message for %s.", subject)
	case "Read":
		return fmt.Sprintf("Read streams data from %s into p.", subject)
	case "RoundTrip":
		return fmt.Sprintf("RoundTrip executes an HTTP request through %s.", subject)
	case "MarshalJSON":
		return fmt.Sprintf("MarshalJSON encodes %s into the JSON shape expected by the provider.", subject)
	case "UnmarshalJSON":
		return fmt.Sprintf("UnmarshalJSON decodes %s from the provider JSON shape.", subject)
	case "callOnce":
		return fmt.Sprintf("callOnce sends one model request through %s and reports whether failures are retryable.", subject)
	case "prepare":
		return fmt.Sprintf("prepare creates or updates the live fixture resources tracked by %s.", subject)
	case "bestEffort":
		return fmt.Sprintf("bestEffort runs cleanup work for %s without aborting fixture preparation.", subject)
	case "notef":
		return fmt.Sprintf("notef records a fixture preparation note for %s.", subject)
	}
	if d.Type.Results != nil && len(d.Type.Results.List) == 1 {
		if ident, ok := d.Type.Results.List[0].Type.(*ast.Ident); ok && ident.Name == "bool" {
			return fmt.Sprintf("%s reports whether the %s satisfies the %s condition.", name, recvType, camelToWords(name))
		}
	}
	if suffix, ok := strings.CutPrefix(name, "Get"); ok {
		return fmt.Sprintf("%s returns the %s value from %s.", name, camelToWords(suffix), subject)
	}
	if suffix, ok := strings.CutPrefix(name, "Set"); ok {
		return fmt.Sprintf("%s updates the %s value on %s.", name, camelToWords(suffix), subject)
	}
	if suffix, ok := strings.CutPrefix(name, "ensure"); ok {
		return fmt.Sprintf("%s ensures %s exists for %s.", name, camelToWords(suffix), subject)
	}
	if strings.HasPrefix(name, "cleanup") || strings.HasPrefix(name, "delete") {
		return fmt.Sprintf("%s removes %s fixture resources for %s when present.", name, camelToWords(name), subject)
	}
	return fmt.Sprintf("%s handles %s for %s.", name, camelToWords(name), subject)
}

// generateHandlerDoc generates a doc comment for an MCP tool handler
// function based on its name and input type.
func generateHandlerDoc(d *ast.FuncDecl, pkgName string) string {
	name := d.Name.Name
	if d.Type.Results != nil && len(d.Type.Results.List) == 2 {
		returnType := exprToString(d.Type.Results.List[0].Type)
		action := inferAction(name)
		return fmt.Sprintf("%s %s and returns [%s].", name, action, returnType)
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
	return helperIntentDoc(name, pkgName)
}

// helperIntentDoc describes package-private helpers using naming conventions
// that are more useful than generic "internal helper" comments.
func helperIntentDoc(name, pkgName string) string {
	words := camelToWords(name)
	switch {
	case name == "main":
		return "main starts the command-line workflow."
	case name == "run":
		return "run executes the command workflow after arguments are parsed."
	case strings.HasPrefix(name, "is") || strings.HasPrefix(name, "has") || strings.HasPrefix(name, "should") || strings.HasPrefix(name, "valid") || strings.HasPrefix(name, "routeLooks") || strings.HasPrefix(name, "routeUnavailable") || strings.HasPrefix(name, "taskHas") || strings.HasPrefix(name, "taskUses") || strings.HasPrefix(name, "taskMatches") || strings.HasPrefix(name, "taskNeeds") || strings.HasPrefix(name, "taskArchives") || strings.HasPrefix(name, "taskUnavailable") || strings.HasPrefix(name, "catalogHas") || strings.HasPrefix(name, "reportMentions") || strings.Contains(name, "Available") || strings.Contains(name, "Unavailable"):
		return fmt.Sprintf("%s reports whether %s.", name, words)
	case strings.HasPrefix(name, "normalize") || strings.HasPrefix(name, "normalized"):
		return fmt.Sprintf("%s normalizes %s for stable comparisons.", name, camelToWords(strings.TrimPrefix(strings.TrimPrefix(name, "normalized"), "normalize")))
	case strings.HasPrefix(name, "filter"):
		return fmt.Sprintf("%s filters %s using evaluator options.", name, camelToWords(strings.TrimPrefix(name, "filter")))
	case strings.HasPrefix(name, "order"):
		return fmt.Sprintf("%s orders %s deterministically.", name, camelToWords(strings.TrimPrefix(name, "order")))
	case strings.HasPrefix(name, "split"):
		return fmt.Sprintf("%s splits %s into parsed fields.", name, camelToWords(strings.TrimPrefix(name, "split")))
	case strings.HasPrefix(name, "sort") || strings.HasPrefix(name, "sorted"):
		return fmt.Sprintf("%s sorts %s deterministically.", name, camelToWords(strings.TrimPrefix(strings.TrimPrefix(name, "sorted"), "sort")))
	case strings.HasPrefix(name, "default"):
		return fmt.Sprintf("%s returns the default %s.", name, camelToWords(strings.TrimPrefix(name, "default")))
	case strings.HasPrefix(name, "parse"):
		return fmt.Sprintf("%s parses %s from evaluator input.", name, camelToWords(strings.TrimPrefix(name, "parse")))
	case strings.HasPrefix(name, "load"):
		return fmt.Sprintf("%s loads %s from evaluator inputs.", name, camelToWords(strings.TrimPrefix(name, "load")))
	case strings.HasPrefix(name, "publish"):
		return fmt.Sprintf("%s publishes %s into managed documentation.", name, camelToWords(strings.TrimPrefix(name, "publish")))
	case strings.HasPrefix(name, "report"):
		return fmt.Sprintf("%s extracts %s from generated reports.", name, camelToWords(strings.TrimPrefix(name, "report")))
	case strings.HasPrefix(name, "section"):
		return fmt.Sprintf("%s extracts %s from a managed Markdown section.", name, camelToWords(strings.TrimPrefix(name, "section")))
	case strings.HasPrefix(name, "replace"):
		return fmt.Sprintf("%s replaces %s placeholders in evaluation prompts.", name, camelToWords(strings.TrimPrefix(name, "replace")))
	case strings.HasPrefix(name, "fixture"):
		return fmt.Sprintf("%s returns %s fixture content.", name, camelToWords(strings.TrimPrefix(name, "fixture")))
	case strings.HasPrefix(name, "live"):
		return fmt.Sprintf("%s returns %s for live evaluation runs.", name, camelToWords(strings.TrimPrefix(name, "live")))
	case strings.HasPrefix(name, "suffix"):
		return fmt.Sprintf("%s appends %s to isolate live evaluation resources.", name, camelToWords(strings.TrimPrefix(name, "suffix")))
	case strings.Contains(name, "Path") || strings.Contains(name, "URL") || strings.Contains(name, "Endpoint"):
		return fmt.Sprintf("%s returns the %s used by evaluator requests.", name, words)
	case strings.Contains(name, "Schema") || strings.Contains(name, "Enum"):
		return fmt.Sprintf("%s derives %s from tool schema metadata.", name, words)
	case strings.Contains(name, "Prompt") || strings.Contains(name, "Preamble") || strings.Contains(name, "Guidance"):
		return fmt.Sprintf("%s builds %s for evaluator prompts.", name, words)
	case strings.Contains(name, "Message") || strings.Contains(name, "Payload") || strings.Contains(name, "Envelope") || strings.Contains(name, "Hint"):
		return fmt.Sprintf("%s builds %s for retry and repair feedback.", name, words)
	case strings.Contains(name, "Param") || strings.Contains(name, "Params") || strings.Contains(name, "Role") || strings.Contains(name, "Provenance"):
		return fmt.Sprintf("%s derives %s from task and schema inputs.", name, words)
	case strings.Contains(name, "Route") || strings.Contains(name, "Routes") || strings.Contains(name, "Catalog"):
		return fmt.Sprintf("%s derives %s from catalog metadata.", name, words)
	case strings.Contains(name, "Tool") || strings.Contains(name, "Tools") || strings.Contains(name, "Action"):
		return fmt.Sprintf("%s resolves %s for evaluator execution.", name, words)
	case strings.Contains(name, "Result") || strings.Contains(name, "Results") || strings.Contains(name, "Content") || strings.Contains(name, "Response"):
		return fmt.Sprintf("%s formats %s for evaluator output.", name, words)
	case strings.Contains(name, "Metric") || strings.Contains(name, "Metrics") || strings.Contains(name, "Cost") || strings.Contains(name, "Percent"):
		return fmt.Sprintf("%s calculates %s for evaluation summaries.", name, words)
	case strings.Contains(name, "Failure") || strings.Contains(name, "Diagnostic") || strings.Contains(name, "Miss"):
		return fmt.Sprintf("%s classifies %s for evaluation diagnostics.", name, words)
	case strings.Contains(name, "Pricing"):
		return fmt.Sprintf("%s reports whether model pricing data is configured.", name)
	case strings.Contains(name, "Set") || strings.HasPrefix(name, "unique") || strings.HasPrefix(name, "missing") || strings.HasPrefix(name, "covered") || strings.HasPrefix(name, "uncovered") || strings.HasPrefix(name, "count"):
		return fmt.Sprintf("%s derives %s from evaluator collections.", name, words)
	case strings.Contains(name, "From") || strings.Contains(name, "To"):
		return fmt.Sprintf("%s maps %s between API and evaluator models.", name, words)
	case strings.HasPrefix(name, "sanitize"):
		return fmt.Sprintf("%s sanitizes %s for provider compatibility.", name, camelToWords(strings.TrimPrefix(name, "sanitize")))
	case strings.Contains(name, "Column") || strings.Contains(name, "Label") || strings.Contains(name, "Status") || strings.Contains(name, "Date") || strings.Contains(name, "Rank"):
		return fmt.Sprintf("%s formats %s for report output.", name, words)
	case strings.HasPrefix(name, "clone") || strings.HasPrefix(name, "deepClone"):
		return fmt.Sprintf("%s clones %s without sharing mutable maps.", name, camelToWords(strings.TrimPrefix(strings.TrimPrefix(name, "deepClone"), "clone")))
	case strings.HasPrefix(name, "required"):
		return fmt.Sprintf("%s returns required %s names for provider schemas.", name, camelToWords(strings.TrimPrefix(name, "required")))
	case strings.HasPrefix(name, "new"):
		return fmt.Sprintf("%s constructs %s.", name, camelToWords(strings.TrimPrefix(name, "new")))
	case strings.HasPrefix(name, "current"):
		return fmt.Sprintf("%s collects current %s metadata.", name, camelToWords(strings.TrimPrefix(name, "current")))
	case strings.HasPrefix(name, "first"):
		return fmt.Sprintf("%s returns the first %s value that is set.", name, camelToWords(strings.TrimPrefix(name, "first")))
	case strings.HasPrefix(name, "metrics"):
		return fmt.Sprintf("%s computes %s from comparison data.", name, camelToWords(strings.TrimPrefix(name, "metrics")))
	case strings.HasPrefix(name, "aggregate"):
		return fmt.Sprintf("%s aggregates %s across reports.", name, camelToWords(strings.TrimPrefix(name, "aggregate")))
	case strings.HasPrefix(name, "append"):
		return fmt.Sprintf("%s appends %s to the output builder.", name, camelToWords(strings.TrimPrefix(name, "append")))
	case strings.HasPrefix(name, "apply"):
		return fmt.Sprintf("%s applies %s transformations.", name, camelToWords(strings.TrimPrefix(name, "apply")))
	case strings.HasPrefix(name, "read"):
		return fmt.Sprintf("%s reads %s from disk.", name, camelToWords(strings.TrimPrefix(name, "read")))
	case strings.HasPrefix(name, "write"):
		return fmt.Sprintf("%s writes %s to disk.", name, camelToWords(strings.TrimPrefix(name, "write")))
	case strings.HasPrefix(name, "wait"):
		return fmt.Sprintf("%s waits for %s to become available.", name, camelToWords(strings.TrimPrefix(name, "wait")))
	case strings.HasPrefix(name, "ensure"):
		return fmt.Sprintf("%s ensures %s exists for live evaluation.", name, camelToWords(strings.TrimPrefix(name, "ensure")))
	case strings.HasPrefix(name, "taskSteps"):
		return fmt.Sprintf("%s returns expected tool steps for an evaluation task.", name)
	case strings.HasPrefix(name, "promptNames"):
		return fmt.Sprintf("%s reports whether a prompt names the target entity.", name)
	case strings.HasPrefix(name, "standaloneDynamicActionCandidates"):
		return fmt.Sprintf("%s returns dynamic fallback action candidates for standalone tools.", name)
	case strings.HasPrefix(name, "superDispatcherAction"):
		return fmt.Sprintf("%s returns the meta-tool dispatcher action for a task step.", name)
	case strings.HasPrefix(name, "close") || strings.HasPrefix(name, "cleanup") || strings.HasPrefix(name, "delete"):
		return fmt.Sprintf("%s removes %s resources when present.", name, camelToWords(name))
	case strings.HasPrefix(name, "openAI") || strings.HasPrefix(name, "google") || strings.HasPrefix(name, "qwen") || strings.HasPrefix(name, "model") || strings.HasPrefix(name, "provider") || strings.HasPrefix(name, "doModel"):
		return fmt.Sprintf("%s prepares %s for model-provider evaluation.", name, words)
	default:
		return fmt.Sprintf("%s implements the %s helper used by %s.", name, words, pkgName)
	}
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
	if prefix, ok := strings.CutSuffix(name, "Case"); ok {
		return fmt.Sprintf("%s describes one %s table-driven test case.", name, camelToWords(prefix))
	}
	if prefix, ok := strings.CutSuffix(name, "Alias"); ok {
		return fmt.Sprintf("%s describes one %s alias mapping used by tests.", name, camelToWords(prefix))
	}
	words := camelToWords(name)
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "openai"):
		return fmt.Sprintf("%s models the OpenAI-compatible %s payload.", name, words)
	case strings.Contains(lower, "google"):
		return fmt.Sprintf("%s models the Google Gemini %s payload.", name, words)
	case strings.Contains(lower, "anthropic"):
		return fmt.Sprintf("%s models the Anthropic %s payload.", name, words)
	case strings.Contains(lower, "provider"):
		return fmt.Sprintf("%s captures model-provider %s data.", name, words)
	case strings.Contains(lower, "publish"):
		return fmt.Sprintf("%s captures %s data for published evaluation reports.", name, words)
	case strings.Contains(lower, "fixture"):
		return fmt.Sprintf("%s captures %s data for live evaluation fixtures.", name, words)
	case strings.Contains(lower, "trace"):
		return fmt.Sprintf("%s records %s data in evaluation traces.", name, words)
	case strings.Contains(lower, "task"):
		return fmt.Sprintf("%s captures %s data for one evaluation task.", name, words)
	case strings.Contains(lower, "metric") || strings.Contains(lower, "summary") || strings.Contains(lower, "pricing"):
		return fmt.Sprintf("%s captures %s data for evaluation summaries.", name, words)
	default:
		return fmt.Sprintf("%s holds %s data for the %s package.", name, words, pkgName)
	}
}

// generateValueDoc generates a doc comment for a package-level const or var.
func generateValueDoc(vs *ast.ValueSpec, tok token.Token) string {
	names := make([]string, 0, len(vs.Names))
	for _, name := range vs.Names {
		if name.Name != "_" {
			names = append(names, name.Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	name := names[0]
	if tok == token.CONST {
		return fmt.Sprintf("%s identifies the %s constant used by this package.", name, camelToWords(name))
	}
	return fmt.Sprintf("%s stores the package-level %s state.", name, camelToWords(name))
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
	return "coordinates " + camelToWords(name)
}

// docIdentifier returns the identifier as prose while preserving Go-style
// initialisms for comments that mention a symbol by name.
func docIdentifier(s string) string {
	if s == "" {
		return "the subject under test"
	}
	return s
}

// scenarioPhrase converts a scenario suffix from a test name to lowercase prose.
func scenarioPhrase(s string) string {
	return camelToWords(s)
}

// camelToWords splits a Go identifier into lowercase words while preserving
// common initialisms such as API, JSON, MCP, and URL.
func camelToWords(s string) string {
	if s == "" {
		return "resources"
	}
	s = strings.ReplaceAll(s, "_", " ")
	var buf bytes.Buffer
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && shouldSplitIdentifier(runes, i) {
			buf.WriteByte(' ')
		}
		buf.WriteRune(r)
	}
	words := strings.Fields(buf.String())
	for i, word := range words {
		upper := strings.ToUpper(word)
		if replacement, ok := commonInitialisms[upper]; ok {
			words[i] = replacement
			continue
		}
		words[i] = strings.ToLower(word)
	}
	result := strings.Join(words, " ")
	result = strings.NewReplacer(
		"i ds", "IDs",
		"git lab", "GitLab",
		"open ai", "OpenAI",
		"open AI", "OpenAI",
		"m rs", "MRs",
		"qwen", "Qwen",
		"google", "Google",
		"2 fa", "2FA",
		"ci cd", "CI/CD",
	).Replace(result)
	if result == "" {
		return "resources"
	}
	return result
}

// commonInitialisms maps uppercase identifier tokens to their preferred spelling
// in generated prose.
var commonInitialisms = map[string]string{
	"AI": "AI", "API": "API", "ASCII": "ASCII", "AST": "AST", "CE": "CE", "CI": "CI", "CICD": "CI/CD", "CLI": "CLI", "CPU": "CPU", "CRUD": "CRUD", "CSS": "CSS", "CSV": "CSV", "DORA": "DORA", "EE": "EE", "E2E": "E2E", "EOF": "EOF", "HTML": "HTML", "HTTP": "HTTP", "HTTPS": "HTTPS", "ID": "ID", "IDS": "IDs", "IID": "IID", "JSON": "JSON", "JWT": "JWT", "LDAP": "LDAP", "LFS": "LFS", "LRU": "LRU", "MCP": "MCP", "MR": "MR", "OAUTH": "OAuth", "PAT": "PAT", "REST": "REST", "SAML": "SAML", "SHA": "SHA", "SSH": "SSH", "TLS": "TLS", "TTL": "TTL", "UI": "UI", "URL": "URL", "UUID": "UUID", "XML": "XML", "YAML": "YAML",
}

// shouldSplitIdentifier reports whether a word boundary belongs before
// runes[index].
func shouldSplitIdentifier(runes []rune, index int) bool {
	current := runes[index]
	previous := runes[index-1]
	if current == ' ' || previous == ' ' {
		return false
	}
	if isUpper(current) && isLower(previous) {
		return true
	}
	if isUpper(current) && isUpper(previous) && index+1 < len(runes) && isLower(runes[index+1]) {
		return true
	}
	if isDigit(current) && !isDigit(previous) {
		return true
	}
	if !isDigit(current) && isDigit(previous) {
		return true
	}
	return false
}

// isUpper reports whether r is an ASCII uppercase letter.
func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }

// isLower reports whether r is an ASCII lowercase letter.
func isLower(r rune) bool { return r >= 'a' && r <= 'z' }

// isDigit reports whether r is an ASCII decimal digit.
func isDigit(r rune) bool { return r >= '0' && r <= '9' }

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
