// Fix generates and inserts godoc-compliant comments (formerly add_docs).

package main

import (
	"bytes"
	"errors"
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

const helperReportsWhetherTemplate = "%s reports whether %s."

// insertion describes one doc-comment edit to splice into a source file.
//
// startLine and endLine are 1-based line numbers in the original file;
// inserting new comment text replaces this range (an empty range inserts
// the comment immediately before startLine). comment is the rendered
// doc-comment text without the leading "// " or indentation.
type insertion struct {
	startLine int
	endLine   int
	comment   string
}

var generatedDocMarkers = []string{
	"verifies the behavior of ",
	"verifies the expected behavior of ",
	"measures the performance of the ",
	"is an internal helper for the ",
	"holds data for ",
	" i ds",
	" open ai ",
	" git lab ",
	" m rs",
	" 2 fa",
	" using the GitLab API and returns ",
}

var generatedDocMarkerPairs = [][2]string{
	{" handles the ", " scenario correctly"},
	{"validates ", " across multiple scenarios using table-driven subtests"},
	{" performs the ", " operation"},
	{" handles ", " for the "},
	{" supports ", " tests for "},
	{" provides ", " test support for "},
	{" coordinates ", " logic for "},
	{" groups ", " fields used by "},
	{"describes ", " data used by the "},
	{" defines the ", " constant."},
	{" names the ", " value shared by this package."},
	{" stores the ", " value."},
	{" provides the ", " value shared by this package."},
}

// processPath processes a Go file or recursively processes a directory. It
// returns an error when a path could not be statted/read/parsed/written so the
// caller can exit non-zero instead of silently succeeding.
func processPath(path string) error {
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", cleanPath, err)
	}
	if info.IsDir() {
		return processDir(cleanPath)
	}
	if strings.HasSuffix(info.Name(), ".go") {
		return processFile(cleanPath)
	}
	return nil
}

// processDir recursively walks a directory and processes each .go file,
// joining any per-file errors so one failure does not hide the others.
func processDir(dir string) error {
	cleanDir := filepath.Clean(dir)
	entries, err := os.ReadDir(cleanDir)
	if err != nil {
		return fmt.Errorf("readdir %s: %w", cleanDir, err)
	}
	var errs []error
	for _, e := range entries {
		if e.IsDir() {
			errs = append(errs, processDir(filepath.Join(cleanDir, e.Name())))
			continue
		}
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		errs = append(errs, processFile(filepath.Join(cleanDir, e.Name())))
	}
	return errors.Join(errs...)
}

// processFile parses a single Go file and adds missing doc comments to
// undocumented functions, types, and methods.
func processFile(path string) error {
	cleanPath := filepath.Clean(path)
	src, err := os.ReadFile(cleanPath) //#nosec G304 -- paths come from CLI args, not user input
	if err != nil {
		return fmt.Errorf("read %s: %w", cleanPath, err)
	}
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, cleanPath, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", cleanPath, err)
	}
	pkgName := node.Name.Name
	isTest := strings.HasSuffix(cleanPath, "_test.go")

	insertions := collectDocInsertions(fset, node, pkgName, isTest)

	if len(insertions) == 0 {
		return nil
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

	if dryRun {
		fmt.Printf("// dry-run: would update %s (%d insertions)\n", cleanPath, len(insertions))
		return nil
	}

	if err = os.WriteFile(cleanPath, []byte(result), 0o600); err != nil { //#nosec G703 -- CLI tool, paths from args
		return fmt.Errorf("write %s: %w", cleanPath, err)
	}
	fmt.Printf("documented %s (%d symbols)\n", cleanPath, len(insertions))
	return nil
}

func collectDocInsertions(fset *token.FileSet, node *ast.File, pkgName string, isTest bool) []insertion {
	insertions := make([]insertion, 0)
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if ins, ok := funcDocInsertion(fset, d, pkgName, isTest); ok {
				insertions = append(insertions, ins)
			}
		case *ast.GenDecl:
			insertions = append(insertions, genDeclDocInsertions(fset, d, pkgName)...)
		}
	}
	return insertions
}

func funcDocInsertion(fset *token.FileSet, decl *ast.FuncDecl, pkgName string, isTest bool) (insertion, bool) {
	if reusableDoc(decl.Doc) || decl.Name.Name == "init" {
		return insertion{}, false
	}
	comment := generateFuncDoc(decl, pkgName, isTest)
	if comment == "" {
		return insertion{}, false
	}
	startLine, endLine := editRangeForDoc(fset, decl.Doc, decl.Pos())
	return insertion{startLine: startLine, endLine: endLine, comment: comment}, true
}

func genDeclDocInsertions(fset *token.FileSet, decl *ast.GenDecl, pkgName string) []insertion {
	insertions := make([]insertion, 0)
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if ins, ok := typeSpecDocInsertion(fset, decl, s, pkgName); ok {
				insertions = append(insertions, ins)
			}
		case *ast.ValueSpec:
			if ins, ok := valueSpecDocInsertion(fset, decl, s); ok {
				insertions = append(insertions, ins)
			}
		}
	}
	return insertions
}

func typeSpecDocInsertion(fset *token.FileSet, decl *ast.GenDecl, spec *ast.TypeSpec, pkgName string) (insertion, bool) {
	if decl.Tok != token.TYPE || reusableSpecDoc(spec.Doc, decl.Doc) {
		return insertion{}, false
	}
	comment := generateTypeDoc(spec, pkgName)
	if comment == "" {
		return insertion{}, false
	}
	startLine, endLine := editRangeForDoc(fset, firstDoc(spec.Doc, decl.Doc), spec.Pos())
	return insertion{startLine: startLine, endLine: endLine, comment: comment}, true
}

func valueSpecDocInsertion(fset *token.FileSet, decl *ast.GenDecl, spec *ast.ValueSpec) (insertion, bool) {
	if decl.Tok != token.CONST && decl.Tok != token.VAR || reusableSpecDoc(spec.Doc, decl.Doc) {
		return insertion{}, false
	}
	comment := generateValueDoc(spec, decl.Tok)
	if comment == "" {
		return insertion{}, false
	}
	startLine, endLine := editRangeForDoc(fset, firstDoc(spec.Doc, decl.Doc), spec.Pos())
	return insertion{startLine: startLine, endLine: endLine, comment: comment}, true
}

func reusableSpecDoc(primary, fallback *ast.CommentGroup) bool {
	return reusableDoc(primary) || primary == nil && reusableDoc(fallback)
}

func reusableDoc(doc *ast.CommentGroup) bool {
	return doc != nil && len(doc.List) > 0 && !isGeneratedDoc(doc.Text())
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
	for _, marker := range generatedDocMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	for _, pair := range generatedDocMarkerPairs {
		if strings.Contains(text, pair[0]) && strings.Contains(text, pair[1]) {
			return true
		}
	}
	return false
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
	isTableDriven := testHasTableDrivenCases(d)
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

func testHasTableDrivenCases(d *ast.FuncDecl) bool {
	if d.Body == nil {
		return false
	}
	found := false
	ast.Inspect(d.Body, func(n ast.Node) bool {
		if isTableDrivenCompositeLit(n) {
			found = true
			return false
		}
		return true
	})
	return found
}

func isTableDrivenCompositeLit(n ast.Node) bool {
	cl, ok := n.(*ast.CompositeLit)
	if !ok {
		return false
	}
	at, ok := cl.Type.(*ast.ArrayType)
	if !ok {
		return false
	}
	_, ok = at.Elt.(*ast.StructType)
	return ok
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
	if doc := testHelperPrefixDoc(name, phrase); doc != "" {
		return doc
	}
	return fmt.Sprintf("%s supports %s assertions in %s tests.", name, phrase, pkgName)
}

// prefixDocRule defines a doc-comment template applied to helpers whose
// name starts with one of prefixes.
//
// template is a fmt-style format string with two %s verbs (name, subject)
// or one %s (name) for nameOnly rules. subject is invoked to derive the
// subject text from the helper name and the matched prefix.
type prefixDocRule struct {
	prefixes []string
	template string
	subject  func(name, prefix, phrase string) string
}

var testHelperPrefixRules = []prefixDocRule{
	{[]string{"assert"}, "%s checks %s invariants for tests.", trimmedPrefixSubject},
	{[]string{"require"}, "%s returns %s test data or fails the test.", trimmedPrefixSubject},
	{[]string{"mustBuild"}, "%s builds %s test fixtures and fails the test on error.", trimmedPrefixSubject},
	{[]string{"must"}, "%s prepares %s test fixtures and fails the test on error.", trimmedPrefixSubject},
	{[]string{"write"}, "%s writes %s fixture data for tests.", trimmedPrefixSubject},
	{[]string{"find"}, "%s locates %s fixture data for assertions.", trimmedPrefixSubject},
	{[]string{"has", "contains", "is"}, helperReportsWhetherTemplate, phraseSubject},
	{[]string{"load"}, "%s loads %s fixture data for tests.", trimmedPrefixSubject},
	{[]string{"new"}, "%s constructs %s test fixtures.", trimmedPrefixSubject},
	{[]string{"seed"}, "%s seeds %s test fixtures.", trimmedPrefixSubject},
	{[]string{"schema"}, "%s extracts %s details for schema assertions.", phraseSubject},
	{[]string{"normalize", "normalized"}, "%s normalizes %s for stable test assertions.", normalizedSubject},
	{[]string{"compare"}, "%s compares %s snapshots and reports drift.", trimmedPrefixSubject},
	{[]string{"append"}, "%s appends %s diagnostics to the test diff.", trimmedPrefixSubject},
	{[]string{"sort"}, "%s sorts %s fixtures into deterministic order.", trimmedPrefixSubject},
	{[]string{"missing"}, "%s returns missing %s values for assertion messages.", trimmedPrefixSubject},
	{[]string{"text"}, "%s extracts %s from MCP result content for assertions.", phraseSubject},
}

func testHelperPrefixDoc(name, phrase string) string {
	for _, rule := range testHelperPrefixRules {
		for _, prefix := range rule.prefixes {
			if strings.HasPrefix(name, prefix) {
				return fmt.Sprintf(rule.template, name, rule.subject(name, prefix, phrase))
			}
		}
	}
	return ""
}

func trimmedPrefixSubject(name, prefix, _ string) string {
	return camelToWords(strings.TrimPrefix(name, prefix))
}

func phraseSubject(_, _, phrase string) string {
	return phrase
}

func normalizedSubject(name, _, _ string) string {
	return camelToWords(strings.TrimPrefix(strings.TrimPrefix(name, "normalized"), "normalize"))
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
	if doc, ok := exactMethodDoc(name, subject); ok {
		return doc
	}
	if doc, ok := prefixMethodDoc(name, subject); ok {
		return doc
	}
	if d.Type.Results != nil && len(d.Type.Results.List) == 1 {
		if ident, ok := d.Type.Results.List[0].Type.(*ast.Ident); ok && ident.Name == "bool" {
			return fmt.Sprintf("%s reports whether the %s satisfies the %s condition.", name, recvType, camelToWords(name))
		}
	}
	return fmt.Sprintf("%s handles %s for %s.", name, camelToWords(name), subject)
}

func exactMethodDoc(name, subject string) (string, bool) {
	if template, ok := exactMethodTemplates[name]; ok {
		return fmt.Sprintf(template, subject), true
	}
	return "", false
}

var exactMethodTemplates = map[string]string{
	"String":        "String returns the display label for %s.",
	"Error":         "Error returns the error message for %s.",
	"Read":          "Read streams data from %s into p.",
	"RoundTrip":     "RoundTrip executes an HTTP request through %s.",
	"MarshalJSON":   "MarshalJSON encodes %s into the JSON shape expected by the provider.",
	"UnmarshalJSON": "UnmarshalJSON decodes %s from the provider JSON shape.",
	"callOnce":      "callOnce sends one model request through %s and reports whether failures are retryable.",
	"prepare":       "prepare creates or updates the live fixture resources tracked by %s.",
	"bestEffort":    "bestEffort runs cleanup work for %s without aborting fixture preparation.",
	"notef":         "notef records a fixture preparation note for %s.",
}

func prefixMethodDoc(name, subject string) (string, bool) {
	if suffix, ok := strings.CutPrefix(name, "Get"); ok {
		return fmt.Sprintf("%s returns the %s value from %s.", name, camelToWords(suffix), subject), true
	}
	if suffix, ok := strings.CutPrefix(name, "Set"); ok {
		return fmt.Sprintf("%s updates the %s value on %s.", name, camelToWords(suffix), subject), true
	}
	if suffix, ok := strings.CutPrefix(name, "ensure"); ok {
		return fmt.Sprintf("%s ensures %s exists for %s.", name, camelToWords(suffix), subject), true
	}
	if strings.HasPrefix(name, "cleanup") || strings.HasPrefix(name, "delete") {
		return fmt.Sprintf("%s removes %s fixture resources for %s when present.", name, camelToWords(name), subject), true
	}
	return "", false
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
		return name + " converts the GitLab API response to the tool output format."
	}
	if strings.HasPrefix(name, "format") || strings.HasPrefix(name, "Format") {
		return name + " renders the result as a formatted string."
	}
	if strings.HasPrefix(name, "build") || strings.HasPrefix(name, "Build") {
		return name + " constructs the request parameters from the input."
	}
	return helperIntentDoc(name, pkgName)
}

// helperIntentDoc describes package-private helpers using naming conventions
// that are more useful than generic "internal helper" comments.
func helperIntentDoc(name, pkgName string) string {
	words := camelToWords(name)
	if doc, ok := helperIntentDocByPrefix(name, words); ok {
		return doc
	}
	if doc, ok := helperIntentDocByContent(name, words); ok {
		return doc
	}
	if doc, ok := helperIntentDocByEvaluatorPrefix(name, words); ok {
		return doc
	}
	return fmt.Sprintf("%s implements the %s helper used by %s.", name, words, pkgName)
}

// helperDocRule defines a helper-doc template applied when both the
// prefix and contains constraints match the helper name.
//
// prefixes is the list of name prefixes to match (all-or-nothing; empty
// means any name). contains is the list of substring markers to require.
// trimPrefixes are stripped from the helper name before producing the
// subject. template is the format string. useWords controls whether the
// already-split word form is used as the subject; nameOnly emits only
// the helper name as the sole %s verb.
type helperDocRule struct {
	prefixes     []string
	contains     []string
	trimPrefixes []string
	template     string
	useWords     bool
	nameOnly     bool
}

var helperExactDocs = map[string]string{
	"main": "main starts the command-line workflow.",
	"run":  "run executes the command workflow after arguments are parsed.",
}

var helperPrefixDocRules = []helperDocRule{
	{prefixes: []string{"is", "has", "should", "valid", "routeLooks", "routeUnavailable", "taskHas", "taskUses", "taskMatches", "taskNeeds", "taskArchives", "taskUnavailable", "catalogHas", "reportMentions"}, template: helperReportsWhetherTemplate, useWords: true},
	{contains: []string{"Available", "Unavailable"}, template: helperReportsWhetherTemplate, useWords: true},
	{prefixes: []string{"normalize", "normalized"}, trimPrefixes: []string{"normalized", "normalize"}, template: "%s normalizes %s for stable comparisons."},
	{prefixes: []string{"filter"}, trimPrefixes: []string{"filter"}, template: "%s filters %s using evaluator options."},
	{prefixes: []string{"order"}, trimPrefixes: []string{"order"}, template: "%s orders %s deterministically."},
	{prefixes: []string{"split"}, trimPrefixes: []string{"split"}, template: "%s splits %s into parsed fields."},
	{prefixes: []string{"sort", "sorted"}, trimPrefixes: []string{"sorted", "sort"}, template: "%s sorts %s deterministically."},
	{prefixes: []string{"default"}, trimPrefixes: []string{"default"}, template: "%s returns the default %s."},
	{prefixes: []string{"parse"}, trimPrefixes: []string{"parse"}, template: "%s parses %s from evaluator input."},
	{prefixes: []string{"load"}, trimPrefixes: []string{"load"}, template: "%s loads %s from evaluator inputs."},
	{prefixes: []string{"publish"}, trimPrefixes: []string{"publish"}, template: "%s publishes %s into managed documentation."},
	{prefixes: []string{"report"}, trimPrefixes: []string{"report"}, template: "%s extracts %s from generated reports."},
	{prefixes: []string{"section"}, trimPrefixes: []string{"section"}, template: "%s extracts %s from a managed Markdown section."},
	{prefixes: []string{"replace"}, trimPrefixes: []string{"replace"}, template: "%s replaces %s placeholders in evaluation prompts."},
	{prefixes: []string{"fixture"}, trimPrefixes: []string{"fixture"}, template: "%s returns %s fixture content."},
	{prefixes: []string{"live"}, trimPrefixes: []string{"live"}, template: "%s returns %s for live evaluation runs."},
	{prefixes: []string{"suffix"}, trimPrefixes: []string{"suffix"}, template: "%s appends %s to isolate live evaluation resources."},
}

var helperContentDocRules = []helperDocRule{
	{contains: []string{"Path", "URL", "Endpoint"}, template: "%s returns the %s used by evaluator requests.", useWords: true},
	{contains: []string{"Schema", "Enum"}, template: "%s derives %s from tool schema metadata.", useWords: true},
	{contains: []string{"Prompt", "Preamble", "Guidance"}, template: "%s builds %s for evaluator prompts.", useWords: true},
	{contains: []string{"Message", "Payload", "Envelope", "Hint"}, template: "%s builds %s for retry and repair feedback.", useWords: true},
	{contains: []string{"Param", "Params", "Role", "Provenance"}, template: "%s derives %s from task and schema inputs.", useWords: true},
	{contains: []string{"Route", "Routes", "Catalog"}, template: "%s derives %s from catalog metadata.", useWords: true},
	{contains: []string{"Tool", "Tools", "Action"}, template: "%s resolves %s for evaluator execution.", useWords: true},
	{contains: []string{"Result", "Results", "Content", "Response"}, template: "%s formats %s for evaluator output.", useWords: true},
	{contains: []string{"Metric", "Metrics", "Cost", "Percent"}, template: "%s calculates %s for evaluation summaries.", useWords: true},
	{contains: []string{"Failure", "Diagnostic", "Miss"}, template: "%s classifies %s for evaluation diagnostics.", useWords: true},
	{contains: []string{"Pricing"}, template: "%s reports whether model pricing data is configured.", nameOnly: true},
	{prefixes: []string{"unique", "missing", "covered", "uncovered", "count"}, template: "%s derives %s from evaluator collections.", useWords: true},
	{contains: []string{"Set"}, template: "%s derives %s from evaluator collections.", useWords: true},
	{contains: []string{"From", "To"}, template: "%s maps %s between API and evaluator models.", useWords: true},
	{contains: []string{"Column", "Label", "Status", "Date", "Rank"}, template: "%s formats %s for report output.", useWords: true},
}

var helperEvaluatorPrefixDocRules = []helperDocRule{
	{prefixes: []string{"sanitize"}, trimPrefixes: []string{"sanitize"}, template: "%s sanitizes %s for provider compatibility."},
	{prefixes: []string{"clone", "deepClone"}, trimPrefixes: []string{"deepClone", "clone"}, template: "%s clones %s without sharing mutable maps."},
	{prefixes: []string{"required"}, trimPrefixes: []string{"required"}, template: "%s returns required %s names for provider schemas."},
	{prefixes: []string{"new"}, trimPrefixes: []string{"new"}, template: "%s constructs %s."},
	{prefixes: []string{"current"}, trimPrefixes: []string{"current"}, template: "%s collects current %s metadata."},
	{prefixes: []string{"first"}, trimPrefixes: []string{"first"}, template: "%s returns the first %s value that is set."},
	{prefixes: []string{"metrics"}, trimPrefixes: []string{"metrics"}, template: "%s computes %s from comparison data."},
	{prefixes: []string{"aggregate"}, trimPrefixes: []string{"aggregate"}, template: "%s aggregates %s across reports."},
	{prefixes: []string{"append"}, trimPrefixes: []string{"append"}, template: "%s appends %s to the output builder."},
	{prefixes: []string{"apply"}, trimPrefixes: []string{"apply"}, template: "%s applies %s transformations."},
	{prefixes: []string{"read"}, trimPrefixes: []string{"read"}, template: "%s reads %s from disk."},
	{prefixes: []string{"write"}, trimPrefixes: []string{"write"}, template: "%s writes %s to disk."},
	{prefixes: []string{"wait"}, trimPrefixes: []string{"wait"}, template: "%s waits for %s to become available."},
	{prefixes: []string{"ensure"}, trimPrefixes: []string{"ensure"}, template: "%s ensures %s exists for live evaluation."},
	{prefixes: []string{"taskSteps"}, template: "%s returns expected tool steps for an evaluation task.", nameOnly: true},
	{prefixes: []string{"promptNames"}, template: "%s reports whether a prompt names the target entity.", nameOnly: true},
	{prefixes: []string{"standaloneDynamicActionCandidates"}, template: "%s returns dynamic fallback action candidates for standalone tools.", nameOnly: true},
	{prefixes: []string{"superDispatcherAction"}, template: "%s returns the meta-tool dispatcher action for a task step.", nameOnly: true},
	{prefixes: []string{"close", "cleanup", "delete"}, template: "%s removes %s resources when present."},
	{prefixes: []string{"openAI", "google", "qwen", "model", "provider", "doModel"}, template: "%s prepares %s for model-provider evaluation.", useWords: true},
}

func helperIntentDocByPrefix(name, words string) (string, bool) {
	if doc, ok := helperExactDocs[name]; ok {
		return doc, true
	}
	return helperIntentDocFromRules(name, words, helperPrefixDocRules)
}

func helperIntentDocByContent(name, words string) (string, bool) {
	return helperIntentDocFromRules(name, words, helperContentDocRules)
}

func helperIntentDocByEvaluatorPrefix(name, words string) (string, bool) {
	return helperIntentDocFromRules(name, words, helperEvaluatorPrefixDocRules)
}

func helperIntentDocFromRules(name, words string, rules []helperDocRule) (string, bool) {
	for _, rule := range rules {
		if rule.matches(name) {
			return rule.format(name, words), true
		}
	}
	return "", false
}

func (rule helperDocRule) matches(name string) bool {
	prefixMatch := len(rule.prefixes) == 0
	for _, prefix := range rule.prefixes {
		if strings.HasPrefix(name, prefix) {
			prefixMatch = true
			break
		}
	}
	containsMatch := len(rule.contains) == 0
	for _, marker := range rule.contains {
		if strings.Contains(name, marker) {
			containsMatch = true
			break
		}
	}
	return prefixMatch && containsMatch
}

func (rule helperDocRule) format(name, words string) string {
	if rule.nameOnly {
		return fmt.Sprintf(rule.template, name)
	}
	subject := words
	if !rule.useWords {
		subject = name
		for _, prefix := range rule.trimPrefixes {
			subject = strings.TrimPrefix(subject, prefix)
		}
		subject = camelToWords(subject)
	}
	return fmt.Sprintf(rule.template, name, subject)
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
	for _, rule := range typeKeywordDocRules {
		for _, keyword := range rule.keywords {
			if strings.Contains(lower, keyword) {
				return fmt.Sprintf(rule.template, name, words)
			}
		}
	}
	return fmt.Sprintf("%s holds %s data for the %s package.", name, words, pkgName)
}

// keywordDocRule defines a type-doc template applied when the type name
// contains any of keywords (case-insensitive).
//
// template is a fmt-style format string with two %s verbs (name, words).
type keywordDocRule struct {
	keywords []string
	template string
}

var typeKeywordDocRules = []keywordDocRule{
	{[]string{"openai"}, "%s models the OpenAI-compatible %s payload."},
	{[]string{"google"}, "%s models the Google Gemini %s payload."},
	{[]string{"anthropic"}, "%s models the Anthropic %s payload."},
	{[]string{"provider"}, "%s captures model-provider %s data."},
	{[]string{"publish"}, "%s captures %s data for published evaluation reports."},
	{[]string{"fixture"}, "%s captures %s data for live evaluation fixtures."},
	{[]string{"trace"}, "%s records %s data in evaluation traces."},
	{[]string{"task"}, "%s captures %s data for one evaluation task."},
	{[]string{"metric", "summary", "pricing"}, "%s captures %s data for evaluation summaries."},
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
		{"list", "lists"},
		{"get", "retrieves"},
		{"create", "creates"},
		{"update", "updates"},
		{"delete", "deletes"},
		{"set", "configures"},
		{"protect", "protects"},
		{"unprotect", "removes protection from"},
		{"merge", "merges"},
		{"approve", "approves"},
		{"search", "searches for"},
		{"publish", "publishes"},
		{"download", "downloads"},
		{"upload", "uploads"},
		{"close", "closes"},
		{"reopen", "reopens"},
		{"rebase", "rebases"},
		{"cancel", "cancels"},
		{"retry", "retries"},
		{"lint", "validates"},
		{"add", "adds"},
		{"remove", "removes"},
		{"edit", "edits"},
		{"run", "runs"},
		{"lock", "locks"},
		{"unlock", "unlocks"},
		{"resolve", "resolves"},
		{"unresolve", "unresolves"},
		{"restore", "restores"},
		{"play", "triggers"},
		{"erase", "erases"},
		{"trace", "retrieves the trace of"},
		{"subscribe", "subscribes to"},
		{"unsubscribe", "unsubscribes from"},
		{"transfer", "transfers"},
		{"fork", "forks"},
		{"archive", "archives"},
		{"unarchive", "unarchives"},
		{"star", "stars"},
		{"unstar", "unstars"},
		{"share", "shares"},
		{"unshare", "unshares"},
		{"promote", "promotes"},
		{"request", "requests"},
		{"accept", "accepts"},
		{"reject", "rejects"},
		{"revoke", "revokes"},
		{"rotate", "rotates"},
		{"trigger", "triggers"},
		{"check", "checks"},
		{"mark", "marks"},
		{"browse", "browses"},
		{"compare", "compares"},
		{"render", "renders"},
		{"validate", "validates"},
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
	"AI": "AI", "API": "API", "APIURL": "API URL", "ASCII": "ASCII", "AST": "AST", "CE": "CE", "CI": "CI", "CICD": "CI/CD", "CLI": "CLI", "CPU": "CPU", "CRUD": "CRUD", "CSS": "CSS", "CSV": "CSV", "DORA": "DORA", "EE": "EE", "E2E": "E2E", "EOF": "EOF", "HTML": "HTML", "HTTP": "HTTP", "HTTPS": "HTTPS", "ID": "ID", "IDS": "IDs", "IID": "IID", "JSON": "JSON", "JWT": "JWT", "LDAP": "LDAP", "LFS": "LFS", "LRU": "LRU", "MCP": "MCP", "MR": "MR", "OAUTH": "OAuth", "PAT": "PAT", "REST": "REST", "SAML": "SAML", "SHA": "SHA", "SSH": "SSH", "TLS": "TLS", "TTL": "TTL", "UI": "UI", "URL": "URL", "UUID": "UUID", "XML": "XML", "YAML": "YAML",
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
