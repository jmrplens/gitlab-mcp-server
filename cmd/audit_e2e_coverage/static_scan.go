package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// idSite is one constant ActionID in the source.
type idSite struct {
	// ID is the action named.
	ID string `json:"id"`
	// Pos is file:line, relative to the module root.
	Pos string `json:"pos"`
	// Placement is the runtime package: common, ce or ee.
	Placement string `json:"placement"`
	// Func is the function the site is in, empty at package level.
	Func string `json:"func,omitempty"`
}

// verbShape says where a harness verb takes its id and whether it hands a
// result back that a caller can throw away.
type verbShape struct {
	// idArg is the index of the ActionID parameter.
	idArg int
	// resultBearing is whether discarding the verb's answer is a finding:
	// Do and Eventually decode an answer the test is expected to read, and
	// Try hands both halves back for the test to judge.
	resultBearing bool
}

// harnessVerbs are the harness calls that name an action.
var harnessVerbs = map[string]verbShape{
	"Do":              {idArg: 1, resultBearing: true},
	"DoVoid":          {idArg: 1},
	"Try":             {idArg: 1, resultBearing: true},
	"Refused":         {idArg: 1},
	"ExpectToolError": {idArg: 1},
	"Eventually":      {idArg: 1, resultBearing: true},
}

// packageScanner reads one package at a time against the harness.
type packageScanner struct {
	harnessPath string
	// dir is the module root positions are made relative to.
	dir string
}

// newPackageScanner returns a scanner for the harness at harnessPath.
func newPackageScanner(harnessPath string, harness *packages.Package) *packageScanner {
	return &packageScanner{harnessPath: harnessPath, dir: moduleDir(harness)}
}

// moduleDir finds the module root from the harness package's own files:
// the harness sits at test/e2e/internal/harness under it.
func moduleDir(harness *packages.Package) string {
	if len(harness.GoFiles) == 0 {
		return ""
	}
	dir := filepath.Dir(harness.GoFiles[0])
	for range 4 {
		dir = filepath.Dir(dir)
	}
	return dir
}

// funcScan is what one function declaration holds.
type funcScan struct {
	// name is the function's key, as [declName] spells it.
	name string
	// isTest is whether the test binary runs it as a test.
	isTest bool
	// ids are the constant ids named inside it.
	ids map[string]bool
	// declaresUltimate is whether it passes Tier(edition.Ultimate) to Needs
	// itself.
	declaresUltimate bool
	// refs names the same-package functions and methods it references, keyed
	// as [declName] keys them.
	refs map[string]bool
}

// packageScan is what one package holds.
type packageScan struct {
	pkgPath   string
	placement string
	// sites lists every constant id site.
	sites []idSite
	// nonConstant lists the verb calls whose id is not a constant.
	nonConstant []staticNote
	// discarded lists the result-bearing calls whose answer is thrown away.
	discarded []staticNote
	// resultSites counts the result-bearing calls per constant id, and
	// discardedSites those among them that discard.
	resultSites    map[string]int
	discardedSites map[string]int
	// funcs holds every function declaration by name.
	funcs map[string]*funcScan
	// reach memoizes, per function, every function it reaches through its
	// references at any depth.
	reach map[string]map[string]bool
}

// scan reads one package.
func (s *packageScanner) scan(pkg *packages.Package, placement string) *packageScan {
	scan := &packageScan{
		pkgPath: pkg.PkgPath, placement: placement,
		resultSites: map[string]int{}, discardedSites: map[string]int{}, funcs: map[string]*funcScan{},
	}
	decls := s.declarations(pkg, scan)
	s.collectSites(pkg, scan, decls)
	for _, decl := range decls {
		s.walkFunction(pkg, scan, decl)
	}
	return scan
}

// declarations indexes every function declaration of the package.
func (s *packageScanner) declarations(pkg *packages.Package, scan *packageScan) []*ast.FuncDecl {
	var decls []*ast.FuncDecl
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			decls = append(decls, fn)
			name := declName(fn)
			scan.funcs[name] = &funcScan{
				name: name, isTest: fn.Recv == nil && isTestFunc(fn.Name.Name),
				ids: map[string]bool{}, refs: map[string]bool{},
			}
		}
	}
	return decls
}

// declName names a declaration uniquely within its package: a method by its
// receiver type and name, so it never collides with a function of the same
// name. A pointer receiver and a value receiver on one type spell the same
// key, since a reference resolves to the method and not to how it was
// declared, and [methodKey] must produce the same string from the type
// checker's side.
func declName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return receiverTypeName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

// receiverTypeName reads the type name out of a receiver expression, through
// the pointer and the type parameters a receiver may carry.
func receiverTypeName(expr ast.Expr) string {
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.Ident:
			return e.Name
		default:
			return types.ExprString(expr)
		}
	}
}

// methodKey spells a function object the way [declName] spells its
// declaration: Type.Method for a method, the bare name otherwise.
func methodKey(fn *types.Func) string {
	if recv := fn.Signature().Recv(); recv != nil {
		if named := receiverNamed(recv.Type()); named != nil {
			return named.Obj().Name() + "." + fn.Name()
		}
	}
	return fn.Name()
}

// collectSites reads every constant of the ActionID type off the type
// checker's record, which is what sees a literal passed to a helper's
// ActionID parameter as the constant it was converted to.
func (s *packageScanner) collectSites(pkg *packages.Package, scan *packageScan, decls []*ast.FuncDecl) {
	seen := map[idSite]bool{}
	for expr, tv := range pkg.TypesInfo.Types {
		if tv.Value == nil || tv.Value.Kind() != constant.String || !isActionIDType(tv.Type, s.harnessPath) {
			continue
		}
		fn := enclosing(decls, expr.Pos())
		site := idSite{ID: constant.StringVal(tv.Value), Pos: s.position(pkg, expr.Pos()), Placement: scan.placement}
		if fn != nil {
			site.Func = declName(fn)
			scan.funcs[site.Func].ids[site.ID] = true
		}
		if !seen[site] {
			seen[site] = true
			scan.sites = append(scan.sites, site)
		}
	}
	sort.Slice(scan.sites, func(i, j int) bool {
		if scan.sites[i].Pos != scan.sites[j].Pos {
			return scan.sites[i].Pos < scan.sites[j].Pos
		}
		return scan.sites[i].ID < scan.sites[j].ID
	})
}

// enclosing finds the function declaration a position sits in.
func enclosing(decls []*ast.FuncDecl, pos token.Pos) *ast.FuncDecl {
	for _, decl := range decls {
		if decl.Pos() <= pos && pos < decl.End() {
			return decl
		}
	}
	return nil
}

// position spells a position as file:line relative to the module root.
func (s *packageScanner) position(pkg *packages.Package, pos token.Pos) string {
	p := pkg.Fset.Position(pos)
	name := p.Filename
	if rel, err := filepath.Rel(s.dir, name); err == nil && !strings.HasPrefix(rel, "..") {
		name = filepath.ToSlash(rel)
	}
	return fmt.Sprintf("%s:%d", name, p.Line)
}

// walkFunction reads the verb calls, the tier declaration, the discards and
// the same-package references out of one function body.
func (s *packageScanner) walkFunction(pkg *packages.Package, scan *packageScan, decl *ast.FuncDecl) {
	fn := scan.funcs[declName(decl)]
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			s.noteCall(pkg, scan, fn, n)
		case *ast.Ident:
			s.noteReference(pkg, fn, n)
		case *ast.AssignStmt:
			s.noteAssignment(pkg, scan, n)
		case *ast.ExprStmt:
			if call, isCall := n.X.(*ast.CallExpr); isCall {
				s.noteDiscard(pkg, scan, call, true)
			}
		}
		return true
	})
}

// noteCall records a verb call's id, and a Tier declaration.
func (s *packageScanner) noteCall(pkg *packages.Package, scan *packageScan, fn *funcScan, call *ast.CallExpr) {
	callee := s.harnessCallee(pkg, call)
	if callee == "" {
		return
	}
	if callee == needsFuncName {
		if s.needsUltimate(pkg, call) {
			fn.declaresUltimate = true
		}
		return
	}
	verb, isVerb := harnessVerbs[callee]
	if !isVerb || len(call.Args) <= verb.idArg {
		return
	}
	arg := call.Args[verb.idArg]
	tv := pkg.TypesInfo.Types[arg]
	if tv.Value == nil {
		scan.nonConstant = append(scan.nonConstant, staticNote{
			Pos: s.position(pkg, arg.Pos()), Text: callee + " is called with the non-constant id " + types.ExprString(arg),
		})
		return
	}
	if verb.resultBearing && tv.Value.Kind() == constant.String {
		scan.resultSites[constant.StringVal(tv.Value)]++
	}
}

// needsUltimate reports whether a Needs call takes Tier(edition.Ultimate)
// among its arguments.
//
// The declaration is the argument and not the Tier call on its own: a Need
// only means something once Needs hands it to New, so a Tier(edition.Ultimate)
// assigned to a blank or passed anywhere else declares nothing, and reading
// it as a declaration would let a test name an Ultimate action while
// requiring nothing of the runtime.
func (s *packageScanner) needsUltimate(pkg *packages.Package, needs *ast.CallExpr) bool {
	for _, arg := range needs.Args {
		tier, isCall := arg.(*ast.CallExpr)
		if !isCall || s.harnessCallee(pkg, tier) != tierFuncName {
			continue
		}
		if len(tier.Args) == 1 && isUltimate(pkg.TypesInfo.Types[tier.Args[0]]) {
			return true
		}
	}
	return false
}

// isUltimate reports whether a constant expression is edition.Ultimate.
//
// The comparison is on the value rather than on the package, so a fake
// edition package in a test fixture with the same constants is read the same
// way; the value is the ordering the whole tier model rests on.
func isUltimate(tv types.TypeAndValue) bool {
	if tv.Value == nil || tv.Value.Kind() != constant.Int {
		return false
	}
	want := constant.MakeInt64(int64(edition.Ultimate))
	return constant.Compare(tv.Value, token.EQL, want)
}

// harnessCallee names the harness function a call invokes, and returns the
// empty string for any other call.
func (s *packageScanner) harnessCallee(pkg *packages.Package, call *ast.CallExpr) string {
	var ident *ast.Ident
	switch f := unwrapCallee(call.Fun).(type) {
	case *ast.Ident:
		ident = f
	case *ast.SelectorExpr:
		ident = f.Sel
	default:
		return ""
	}
	obj, isFunc := pkg.TypesInfo.Uses[ident].(*types.Func)
	if !isFunc || obj.Pkg() == nil || obj.Pkg().Path() != s.harnessPath || obj.Signature().Recv() != nil {
		return ""
	}
	return obj.Name()
}

// unwrapCallee strips the parentheses and the type arguments off a callee,
// so Do[Output](...) and (harness.Do)(...) name the same function as
// Do(...).
func unwrapCallee(fun ast.Expr) ast.Expr {
	for {
		switch f := fun.(type) {
		case *ast.ParenExpr:
			fun = f.X
		case *ast.IndexExpr:
			fun = f.X
		case *ast.IndexListExpr:
			fun = f.X
		default:
			return fun
		}
	}
}

// noteReference records a use of a same-package function or method, which is
// how a helper's ids and declarations are attributed to the tests that call
// it. The reference is keyed as the declaration is, so a method reached
// through a value or a pointer finds the declaration [declName] indexed.
func (s *packageScanner) noteReference(pkg *packages.Package, fn *funcScan, ident *ast.Ident) {
	obj, isFunc := pkg.TypesInfo.Uses[ident].(*types.Func)
	if !isFunc || obj.Pkg() != pkg.Types {
		return
	}
	if key := methodKey(obj); key != fn.name {
		fn.refs[key] = true
	}
}

// noteAssignment records a result-bearing call whose every value is assigned
// to the blank identifier.
func (s *packageScanner) noteAssignment(pkg *packages.Package, scan *packageScan, assign *ast.AssignStmt) {
	if len(assign.Rhs) != 1 {
		return
	}
	call, isCall := assign.Rhs[0].(*ast.CallExpr)
	if !isCall {
		return
	}
	allBlank := true
	for _, lhs := range assign.Lhs {
		ident, isIdent := lhs.(*ast.Ident)
		if !isIdent || ident.Name != "_" {
			allBlank = false
		}
	}
	s.noteDiscard(pkg, scan, call, allBlank)
}

// noteDiscard records a discard of a result-bearing verb's answer.
func (s *packageScanner) noteDiscard(pkg *packages.Package, scan *packageScan, call *ast.CallExpr, discarded bool) {
	if !discarded {
		return
	}
	callee := s.harnessCallee(pkg, call)
	verb, isVerb := harnessVerbs[callee]
	if !isVerb || !verb.resultBearing || len(call.Args) <= verb.idArg {
		return
	}
	id := "a non-constant id"
	if tv := pkg.TypesInfo.Types[call.Args[verb.idArg]]; tv.Value != nil && tv.Value.Kind() == constant.String {
		id = constant.StringVal(tv.Value)
		scan.discardedSites[id]++
	}
	scan.discarded = append(scan.discarded, staticNote{
		Pos:  s.position(pkg, call.Pos()),
		Text: fmt.Sprintf("the result of %s(%s) is discarded: read it, or call DoVoid", callee, id),
	})
}

// reachable names every same-package function a function reaches through
// its references, at any depth, each visited once so a cycle ends the walk
// rather than the program. The function itself is in the set only when a
// cycle leads back to it.
//
// The walk is transitive because a helper chain is the ordinary shape of a
// test package: a test calls the helper that opens its session, which calls
// the helper that declares the tier, and an id or a declaration two calls
// away is exactly as much the test's as one call away. A one-level walk
// answered "no test reaches this" for the second helper, which read as a
// clean gate and was a silence.
func (p *packageScan) reachable(name string) map[string]bool {
	if seen, done := p.reach[name]; done {
		return seen
	}
	seen := map[string]bool{}
	stack := []string{name}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		fn, known := p.funcs[current]
		if !known {
			continue
		}
		for ref := range fn.refs {
			if !seen[ref] {
				seen[ref] = true
				stack = append(stack, ref)
			}
		}
	}
	if p.reach == nil {
		p.reach = map[string]map[string]bool{}
	}
	p.reach[name] = seen
	return seen
}

// testsReaching names the Test functions a site is reached from: the test
// it is in, or every test that reaches the helper it is in, at any depth.
func (p *packageScan) testsReaching(site idSite) []string {
	fn, known := p.funcs[site.Func]
	if !known {
		return nil
	}
	if fn.isTest {
		return []string{fn.name}
	}
	var tests []string
	for name, other := range p.funcs {
		if other.isTest && p.reachable(name)[fn.name] {
			tests = append(tests, name)
		}
	}
	sort.Strings(tests)
	return tests
}

// declaresUltimate reports whether a test declares the Ultimate tier, itself
// or through any helper it reaches.
func (p *packageScan) declaresUltimate(test string) bool {
	fn, known := p.funcs[test]
	if !known {
		return false
	}
	if fn.declaresUltimate {
		return true
	}
	for helper := range p.reachable(test) {
		if other, exists := p.funcs[helper]; exists && other.declaresUltimate {
			return true
		}
	}
	return false
}

// testIDs maps each Test function to the ids it names, itself or through
// any helper it reaches.
func (p *packageScan) testIDs() map[string]map[string]bool {
	byTest := map[string]map[string]bool{}
	for name, fn := range p.funcs {
		if !fn.isTest {
			continue
		}
		ids := map[string]bool{}
		for id := range fn.ids {
			ids[id] = true
		}
		for helper := range p.reachable(name) {
			other, exists := p.funcs[helper]
			if !exists {
				continue
			}
			for id := range other.ids {
				ids[id] = true
			}
		}
		byTest[name] = ids
	}
	return byTest
}
