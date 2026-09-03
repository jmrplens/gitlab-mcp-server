// Command audit_test_subtests finds case loops in test functions that assert
// without opening a subtest, and can rewrite the mechanical ones.
//
// The project's Go guidelines ask for table-driven tests whose cases run
// under t.Run: a failing case then names itself, a t.Fatal inside one case
// stops that case rather than the whole table, and `go test -run` can select
// a single case. The shape that violates the rule is a range over a table
// (a slice or map literal, inline or bound to a local variable) whose body
// calls t.Error, t.Errorf, t.Fatal, t.Fatalf, t.Fail or t.FailNow, or hands
// t to a helper, with no t.Run anywhere inside the loop body.
//
// A loop that walks a sequence of dependent steps rather than independent
// cases is not a table. Mark it with a comment on the line before the loop
// or at the end of its first line:
//
//	// sequential: each step depends on the state the previous one left
//	for _, step := range []step{...} {
//
// and the auditor records it as declared-sequential instead of reporting it.
// A loop inside a synctest.Test bubble is skipped too: the testing package
// panics on t.Run called inside a bubble, so the rule cannot apply there and
// the site is counted separately.
//
// -fix rewrites the sites whose subtest name is unambiguous: a []string
// table names each case after its element, a struct table after a string
// field called name, desc, description, label, title or id, and a
// map[string]... table after its key. The body is wrapped in
// t.Run(name, func(t *testing.T) { ... }) and a bare continue becomes
// return; bodies that break, goto, or use a table without such a name are
// left for a hand rewrite and stay in the report.
//
// Usage:
//
//	go run ./cmd/audit_test_subtests [-json out.json] [-check] [-fix] [dirs...]
//
// With no directories the module's test files under ./cmd, ./internal and
// ./test are scanned. -check exits non-zero when any site remains, so the
// tool gates CI.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Finding is one case loop that asserts without a subtest.
type Finding struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Test  string `json:"test"`
	Table string `json:"table"` // inline | named
	// Fix says how -fix would rewrite the site, or why it cannot:
	// element | field:<name> | key | needs-name | break | goto | blank-var.
	Fix string `json:"fix"`
}

// Report is the JSON work list.
type Report struct {
	Findings   []Finding `json:"findings"`
	Sequential []Finding `json:"sequential"`
	Synctest   []Finding `json:"synctest"`
	Summary    Summary   `json:"summary"`
}

// Summary aggregates what the sweep tracks.
type Summary struct {
	Sites      int `json:"sites"`
	Fixable    int `json:"fixable"`
	Sequential int `json:"sequential"`
	Synctest   int `json:"synctest"`
	Compliant  int `json:"compliant"`
	Files      int `json:"files"`
}

// sequentialMarker declares a loop of dependent steps rather than cases.
const sequentialMarker = "sequential:"

// The Fix vocabulary: the first three name a rewrite -fix can perform, the
// rest name why it cannot.
const (
	fixElement     = "element"    // a []string table: the element is the name
	fixKey         = "key"        // a map[string]... table: the key is the name
	fixFieldPrefix = "field:"     // a struct table: the named string field
	fixNeedsName   = "needs-name" // no name-like field to derive a name from
	fixBlankVar    = "blank-var"  // the loop variable is blank or absent
	fixBreak       = "break"      // a break aimed at the loop cannot cross a closure
	fixGoto        = "goto"       // a goto or a label cannot cross a closure
)

// nameFields are the struct fields, in order of preference, that name a case.
var nameFields = []string{"name", "desc", "description", "label", "title", "id"}

// assertMethods are the testing.TB methods that record a failure.
var assertMethods = map[string]bool{"Error": true, "Errorf": true, "Fatal": true, "Fatalf": true, "Fail": true, "FailNow": true}

func main() {
	jsonPath := flag.String("json", "", "write the JSON work list to this path")
	check := flag.Bool("check", false, "exit non-zero when any case loop still asserts without a subtest")
	fix := flag.Bool("fix", false, "rewrite the sites whose subtest name is unambiguous, then report what remains")
	flag.Parse()

	dirs := flag.Args()
	if len(dirs) == 0 {
		dirs = []string{"./cmd", "./internal", "./test"}
	}

	if *fix {
		rewritten, err := fixAll(dirs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "audit_test_subtests: fix: %v\n", err)
			os.Exit(2)
		}
		fmt.Printf("fix: rewrote %d site(s)\n", rewritten)
	}

	report, err := scan(dirs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audit_test_subtests: %v\n", err)
		os.Exit(2)
	}

	printHuman(report)

	if *jsonPath != "" {
		data, marshalErr := json.MarshalIndent(report, "", "  ")
		if marshalErr != nil {
			fmt.Fprintf(os.Stderr, "audit_test_subtests: marshal: %v\n", marshalErr)
			os.Exit(2)
		}
		if writeErr := os.WriteFile(*jsonPath, append(data, '\n'), 0o600); writeErr != nil {
			fmt.Fprintf(os.Stderr, "audit_test_subtests: write %s: %v\n", *jsonPath, writeErr)
			os.Exit(2)
		}
		fmt.Printf("work list written to %s\n", *jsonPath)
	}

	if *check && len(report.Findings) > 0 {
		fmt.Printf("check: FAIL. %d case loop(s) assert without a subtest (%d declared sequential)\n",
			len(report.Findings), len(report.Sequential))
		os.Exit(1)
	}
	if *check {
		fmt.Printf("check: PASS. Every case loop runs its cases under t.Run (%d declared sequential)\n",
			len(report.Sequential))
	}
}

// scan walks every _test.go file under dirs and collects findings.
func scan(dirs []string) (*Report, error) {
	report := &Report{}
	files := map[string]bool{}
	for _, dir := range dirs {
		if err := filepath.WalkDir(dir, visitTestFiles(report, files)); err != nil {
			return nil, err
		}
	}
	sortFindings(report.Findings)
	sortFindings(report.Sequential)
	sortFindings(report.Synctest)
	report.Summary.Sites = len(report.Findings)
	report.Summary.Sequential = len(report.Sequential)
	report.Summary.Synctest = len(report.Synctest)
	report.Summary.Files = len(files)
	return report, nil
}

// visitTestFiles is the WalkDir callback that parses each test file and
// records its sites in the report.
func visitTestFiles(report *Report, files map[string]bool) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		for _, s := range scanFile(fset, path, file) {
			files[path] = true
			recordSite(report, s)
		}
		return nil
	}
}

// skipDir reports directories the walk never enters.
func skipDir(name string) bool {
	return name == "node_modules" || name == "dist" || (strings.HasPrefix(name, ".") && name != ".")
}

// recordSite files one site under the report list its classification names.
func recordSite(report *Report, s site) {
	switch {
	case s.sequential:
		report.Sequential = append(report.Sequential, s.finding)
	case s.synctest:
		report.Synctest = append(report.Synctest, s.finding)
	case s.compliant:
		report.Summary.Compliant++
	default:
		report.Findings = append(report.Findings, s.finding)
		if fixable(s.finding.Fix) {
			report.Summary.Fixable++
		}
	}
}

// site is one case loop as found in a file, with the AST handles -fix needs.
type site struct {
	finding    Finding
	sequential bool
	synctest   bool
	compliant  bool
	loop       *ast.RangeStmt
	continues  []*ast.BranchStmt // bare continue statements targeting this loop
	nameExpr   string            // subtest name expression when fixable
}

// testScan is what classifying a loop needs to know about the test function
// it sits in: where the file is, and which literals are synctest bubbles.
type testScan struct {
	fset    *token.FileSet
	file    *ast.File
	path    string
	test    string
	bubbles []*ast.FuncLit
}

// scanFile returns every case loop in the file's Test functions.
func scanFile(fset *token.FileSet, path string, file *ast.File) []site {
	var sites []site
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}
		tables := localTables(fn.Body)
		scan := &testScan{fset: fset, file: file, path: path, test: fn.Name.Name, bubbles: synctestBubbles(fn.Body)}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			loop, isLoop := n.(*ast.RangeStmt)
			if !isLoop {
				return true
			}
			lit, kind := tableOf(loop.X, tables)
			if lit == nil {
				return true
			}
			s, keep := scan.classify(loop, lit, kind)
			if keep {
				sites = append(sites, s)
			}
			return false
		})
	}
	return sites
}

// classify decides what one table loop is: declared sequential, inside a
// synctest bubble, already compliant, a non-asserting setup loop (dropped),
// or a finding with the rewrite it qualifies for.
func (sc *testScan) classify(loop *ast.RangeStmt, lit *ast.CompositeLit, kind string) (s site, keep bool) {
	fset, file := sc.fset, sc.file
	s = site{loop: loop, finding: Finding{File: sc.path, Line: fset.Position(loop.Pos()).Line, Test: sc.test, Table: kind}}
	switch {
	case declaredSequential(fset, file, loop):
		s.sequential = true
	case insideAny(loop, sc.bubbles):
		s.synctest = true
	case hasRun(loop.Body):
		s.compliant = true
	case !asserts(loop.Body):
		return s, false
	default:
		s.nameExpr, s.finding.Fix = subtestName(loop, lit, file)
		if fixable(s.finding.Fix) {
			if reason, continues := bodyControlFlow(loop.Body); reason != "" {
				s.finding.Fix = reason
			} else {
				s.continues = continues
			}
		}
	}
	return s, true
}

// localTables maps identifiers bound to a table literal inside the function.
func localTables(body *ast.BlockStmt) map[string]*ast.CompositeLit {
	tables := map[string]*ast.CompositeLit{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range assign.Rhs {
			if lit := tableLiteral(rhs); lit != nil && i < len(assign.Lhs) {
				if id, isIdent := assign.Lhs[i].(*ast.Ident); isIdent {
					tables[id.Name] = lit
				}
			}
		}
		return true
	})
	return tables
}

// tableOf resolves the ranged expression to a table literal, inline or named.
func tableOf(x ast.Expr, tables map[string]*ast.CompositeLit) (lit *ast.CompositeLit, kind string) {
	if inline := tableLiteral(x); inline != nil {
		return inline, "inline"
	}
	if id, ok := x.(*ast.Ident); ok {
		if named := tables[id.Name]; named != nil {
			return named, "named"
		}
	}
	return nil, ""
}

// tableLiteral reports a slice, array or map literal holding at least two
// elements: one element is a single case, not a table.
func tableLiteral(e ast.Expr) *ast.CompositeLit {
	lit, ok := e.(*ast.CompositeLit)
	if !ok || len(lit.Elts) < 2 {
		return nil
	}
	switch lit.Type.(type) {
	case *ast.ArrayType, *ast.MapType:
		return lit
	}
	return nil
}

// declaredSequential reports the escape-hatch comment on the loop line or the
// line before it.
func declaredSequential(fset *token.FileSet, file *ast.File, loop *ast.RangeStmt) bool {
	line := fset.Position(loop.Pos()).Line
	for _, group := range file.Comments {
		for _, c := range group.List {
			at := fset.Position(c.Pos()).Line
			if (at == line || at == line-1) && strings.Contains(c.Text, sequentialMarker) {
				return true
			}
		}
	}
	return false
}

// synctestBubbles returns the function literals handed to synctest.Test or
// synctest.Run: everything inside runs in a bubble where t.Run panics.
func synctestBubbles(body *ast.BlockStmt) []*ast.FuncLit {
	var bubbles []*ast.FuncLit
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel || (sel.Sel.Name != "Test" && sel.Sel.Name != "Run") {
			return true
		}
		if pkg, isIdent := sel.X.(*ast.Ident); !isIdent || pkg.Name != "synctest" {
			return true
		}
		for _, arg := range call.Args {
			if lit, isLit := arg.(*ast.FuncLit); isLit {
				bubbles = append(bubbles, lit)
			}
		}
		return true
	})
	return bubbles
}

// insideAny reports whether the loop sits inside one of the literals.
func insideAny(loop *ast.RangeStmt, lits []*ast.FuncLit) bool {
	for _, lit := range lits {
		if loop.Pos() >= lit.Pos() && loop.End() <= lit.End() {
			return true
		}
	}
	return false
}

// hasRun reports a t.Run (any receiver's Run) call inside the body.
func hasRun(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, isSel := call.Fun.(*ast.SelectorExpr); isSel && sel.Sel.Name == "Run" {
				found = true
			}
		}
		return !found
	})
	return found
}

// asserts reports whether the body records a failure on t directly or hands t
// to a helper that may.
func asserts(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !found
		}
		if sel, isSel := call.Fun.(*ast.SelectorExpr); isSel {
			if id, isIdent := sel.X.(*ast.Ident); isIdent && (id.Name == "t" || id.Name == "tb") && assertMethods[sel.Sel.Name] {
				found = true
			}
		}
		for _, arg := range call.Args {
			if id, isIdent := arg.(*ast.Ident); isIdent && id.Name == "t" {
				found = true
			}
		}
		return !found
	})
	return found
}

// subtestName derives the subtest name expression for the loop, or the reason
// none is unambiguous.
func subtestName(loop *ast.RangeStmt, lit *ast.CompositeLit, file *ast.File) (expr, fix string) {
	switch typ := lit.Type.(type) {
	case *ast.MapType:
		return mapCaseName(loop, typ)
	case *ast.ArrayType:
		return arrayCaseName(loop, typ, file)
	}
	return "", fixNeedsName
}

// mapCaseName names a case after the key of a map[string]... table.
func mapCaseName(loop *ast.RangeStmt, typ *ast.MapType) (expr, fix string) {
	if !isString(typ.Key) {
		return "", fixNeedsName
	}
	key, _ := loop.Key.(*ast.Ident)
	if key == nil || key.Name == "_" {
		return "", fixBlankVar
	}
	return key.Name, fixKey
}

// arrayCaseName names a case after the element of a []string table or after
// a name-like string field of a struct table.
func arrayCaseName(loop *ast.RangeStmt, typ *ast.ArrayType, file *ast.File) (expr, fix string) {
	value, _ := loop.Value.(*ast.Ident)
	blank := value == nil || value.Name == "_"
	if isString(typ.Elt) {
		if blank {
			return "", fixBlankVar
		}
		return value.Name, fixElement
	}
	field := nameField(structFields(typ.Elt, file))
	if field == "" {
		return "", fixNeedsName
	}
	if blank {
		return "", fixBlankVar
	}
	return value.Name + "." + field, fixFieldPrefix + field
}

// nameField picks the preferred name-like string field, or "" when none.
func nameField(fields []field) string {
	for _, want := range nameFields {
		for _, f := range fields {
			if f.name == want && f.isString {
				return f.name
			}
		}
	}
	return ""
}

// field is a struct field the name search looks at.
type field struct {
	name     string
	isString bool
}

// structFields lists the fields of an inline struct type, or of a struct type
// declared in the same file under the element's type name.
func structFields(elt ast.Expr, file *ast.File) []field {
	var st *ast.StructType
	switch t := elt.(type) {
	case *ast.StructType:
		st = t
	case *ast.Ident:
		st = declaredStruct(file, t.Name)
	}
	if st == nil {
		return nil
	}
	var fields []field
	for _, f := range st.Fields.List {
		for _, name := range f.Names {
			fields = append(fields, field{name: name.Name, isString: isString(f.Type)})
		}
	}
	return fields
}

// declaredStruct finds a struct type declared in the file under name.
func declaredStruct(file *ast.File, name string) *ast.StructType {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			if ts, isType := spec.(*ast.TypeSpec); isType && ts.Name.Name == name {
				st, _ := ts.Type.(*ast.StructType)
				return st
			}
		}
	}
	return nil
}

// isString reports the string type.
func isString(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "string"
}

// bodyControlFlow returns the reason a body cannot be wrapped in a closure
// (break or goto aimed at the loop, or labels), and otherwise the bare
// continue statements that must become returns.
func bodyControlFlow(body *ast.BlockStmt) (reason string, continues []*ast.BranchStmt) {
	c := &controlFlow{}
	c.walk(body, false, false)
	return c.reason, c.continues
}

// controlFlow accumulates the verdict of one loop body.
type controlFlow struct {
	reason    string
	continues []*ast.BranchStmt
}

// walk visits a body. nested is true inside an inner loop, whose break and
// continue are its own; inSwitch is true inside a switch or select, where a
// bare break targets that statement and only continue reaches the loop.
func (c *controlFlow) walk(n ast.Node, nested, inSwitch bool) {
	ast.Inspect(n, func(m ast.Node) bool {
		if m == n {
			return true
		}
		if c.reason != "" {
			return false
		}
		switch v := m.(type) {
		case *ast.FuncLit:
			return false // its own control flow
		case *ast.ForStmt, *ast.RangeStmt:
			c.walk(v, true, false)
			return false
		case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			c.walk(v, nested, true)
			return false
		case *ast.LabeledStmt:
			c.reason = fixGoto
			return false
		case *ast.BranchStmt:
			c.branch(v, nested, inSwitch)
		}
		return true
	})
}

// branch classifies one branch statement.
func (c *controlFlow) branch(b *ast.BranchStmt, nested, inSwitch bool) {
	switch {
	case b.Tok == token.GOTO || b.Label != nil:
		c.reason = fixGoto
	case b.Tok == token.BREAK && !nested && !inSwitch:
		c.reason = fixBreak
	case b.Tok == token.CONTINUE && !nested:
		c.continues = append(c.continues, b)
	}
}

// fixable reports whether a Fix value names a rewrite rather than a blocker.
func fixable(fix string) bool {
	return fix == fixElement || fix == fixKey || strings.HasPrefix(fix, fixFieldPrefix)
}

// fixAll rewrites every fixable site under dirs and returns how many.
func fixAll(dirs []string) (int, error) {
	total := 0
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, "_test.go") {
				return nil
			}
			n, fixErr := fixFile(path)
			total += n
			return fixErr
		})
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// fixFile rewrites the fixable sites of one file in place.
func fixFile(path string) (int, error) {
	src, err := os.ReadFile(path) //#nosec G304,G703 -- a test file found by walking the scanned directories, never a user-supplied path
	if err != nil {
		return 0, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	var edits []edit
	count := 0
	for _, s := range scanFile(fset, path, file) {
		if s.sequential || s.synctest || s.compliant || !fixable(s.finding.Fix) {
			continue
		}
		count++
		body := s.loop.Body
		edits = append(edits,
			edit{at: fset.Position(body.Lbrace).Offset + 1, insert: "\nt.Run(" + s.nameExpr + ", func(t *testing.T) {"},
			edit{at: fset.Position(body.Rbrace).Offset, insert: "})\n"},
		)
		for _, c := range s.continues {
			edits = append(edits, edit{at: fset.Position(c.Pos()).Offset, cut: len("continue"), insert: "return"})
		}
	}
	if count == 0 {
		return 0, nil
	}
	formatted, err := format.Source(apply(src, edits))
	if err != nil {
		return 0, fmt.Errorf("%s: rewritten source does not parse: %w", path, err)
	}
	if writeErr := os.WriteFile(path, formatted, 0o600); writeErr != nil { //#nosec G306,G703 -- rewriting the test file the walk found, never a user-supplied path
		return 0, writeErr
	}
	return count, nil
}

// edit is one byte-offset splice: cut bytes at `at`, then insert text there.
type edit struct {
	at     int
	cut    int
	insert string
}

// apply performs the edits from the end of the file backwards so earlier
// offsets stay valid.
func apply(src []byte, edits []edit) []byte {
	sort.Slice(edits, func(i, j int) bool { return edits[i].at > edits[j].at })
	out := append([]byte(nil), src...)
	for _, e := range edits {
		out = append(out[:e.at], append([]byte(e.insert), out[e.at+e.cut:]...)...)
	}
	return out
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
}

// printHuman writes the per-file tallies and the summary.
func printHuman(report *Report) {
	perFile := map[string][2]int{}
	for _, f := range report.Findings {
		c := perFile[f.File]
		c[0]++
		if fixable(f.Fix) {
			c[1]++
		}
		perFile[f.File] = c
	}
	files := make([]string, 0, len(perFile))
	for f := range perFile {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		c := perFile[f]
		fmt.Printf("%-72s sites=%-3d fixable=%d\n", f, c[0], c[1])
	}
	fmt.Printf("\nsummary: %d case loop(s) assert without a subtest (%d fixable by -fix), %d declared sequential, %d inside synctest bubbles, %d compliant, across %d file(s)\n",
		report.Summary.Sites, report.Summary.Fixable, report.Summary.Sequential, report.Summary.Synctest, report.Summary.Compliant, report.Summary.Files)
}
