// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// httptestPath is the package whose server this gate follows. The Close it
// looks for is matched on the method the type checker resolves, so a server
// held under any name, in a field, or embedded in a type of the test's own is
// the same server here.
const httptestPath = "net/http/httptest"

// testingPath is the package whose Run, Cleanup and Parallel decide when a
// registered or deferred close happens.
const testingPath = "testing"

// Finding is one httptest server closed while its address is still used
// later in the same function.
type Finding struct {
	Package string `json:"package"`
	File    string `json:"file"`
	// Line is where the server is closed.
	Line int `json:"line"`
	// Func is the declaration the close sits in, as the file spells it: the
	// bare name, `Type.Method` for a method, or the first variable of a
	// package-level declaration, which is where a function literal assigned
	// at package scope lives.
	Func string `json:"func"`
	// Server is the closed server as the source spells it.
	Server string `json:"server"`
	// Use is the name the address is used through after the close, and
	// UseLine is where: the server itself, or a variable that was given its
	// address.
	Use     string `json:"use"`
	UseLine int    `json:"use_line"`
}

// scanner accumulates what one load of the tree says about the httptest
// servers its functions close.
//
// A file is read once per variant of its package the load returns, which is
// twice for a package's own files when it has tests, so the closes and the
// findings are keyed by the position of the close and a file read twice is
// counted once.
type scanner struct {
	root string
	// overlay is the source a test supplies instead of the disk, consulted
	// when a file the load left out is read for its imports.
	overlay  map[string][]byte
	findings map[string]Finding
	// closes are the closes judged: every one but those that wait for the
	// top-level function's test to end, which nothing in the function can
	// follow. Those are a close registered on that test's Cleanup, or on the
	// Cleanup of a test a caller handed in, and one a parallel subtest of that
	// test defers or registers.
	closes map[string]struct{}
	// unjudged are the files a load left out that import net/http/httptest: a
	// build constraint this load did not satisfy hid every server in them,
	// and a gate that said nothing about that would be clean over source it
	// never read.
	unjudged map[string]struct{}
	// packages are the packages this run looked at, named the way the
	// repository names them. The declaration table is held against this set,
	// so a run over one package does not report every declaration elsewhere
	// as stale.
	packages map[string]struct{}
	// fields are the values given to struct fields anywhere in the load. They
	// are read before any function is judged, because a helper may build the
	// struct a test then takes both its server and its address from.
	fields *flows
}

// newScanner returns a scanner rooted at an absolute repository path.
func newScanner(root string, overlay map[string][]byte) *scanner {
	return &scanner{
		root:     root,
		overlay:  overlay,
		findings: map[string]Finding{},
		closes:   map[string]struct{}{},
		unjudged: map[string]struct{}{},
		packages: map[string]struct{}{},
		fields:   newFlows(),
	}
}

// observe judges every function of every package in one load, once the
// values given to fields across the load are known. The test main the go tool
// synthesizes for a package is passed over: its one generated file is
// nobody's source.
func (s *scanner) observe(loaded []*packages.Package) {
	underTest := goprogram.PackagesUnderTest(loaded)
	var judged []*packages.Package
	for _, pkg := range loaded {
		if goprogram.IsTestMain(pkg, underTest) {
			continue
		}
		judged = append(judged, pkg)
		for _, file := range pkg.Syntax {
			s.fields.readFields(pkg.TypesInfo, file)
		}
	}
	for _, pkg := range judged {
		name := goprogram.TrimModulePath(goprogram.VariantName(pkg.PkgPath))
		s.packages[name] = struct{}{}
		s.noteUnjudged(pkg)
		for _, file := range pkg.Syntax {
			s.observeFile(pkg, name, file)
		}
	}
}

// noteUnjudged records the files this load left out that import
// net/http/httptest, or that cannot be read: whether a hidden file could hold
// a server this gate would follow is all the question needs, and
// [goprogram.FilesImporting] answers it from the imports alone.
func (s *scanner) noteUnjudged(pkg *packages.Package) {
	for _, path := range goprogram.FilesImporting(goprogram.IgnoredGoFiles(pkg), httptestPath, s.overlay) {
		s.unjudged[goprogram.RelativePath(path, s.root)] = struct{}{}
	}
}

// observeFile judges every top-level function of one file, and every function
// literal a package-level variable holds. Any other declaration is walked for
// the variables it declares and not below them, since a value is where a
// function literal at package scope lives.
func (s *scanner) observeFile(pkg *packages.Package, pkgName string, file *ast.File) {
	for _, decl := range file.Decls {
		if fn, isFunc := decl.(*ast.FuncDecl); isFunc {
			s.observeFunc(pkg, pkgName, fn, sourcegate.FuncDeclName(fn))
			continue
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			value, isValue := n.(*ast.ValueSpec)
			if isValue {
				s.observeFunc(pkg, pkgName, value, value.Names[0].Name)
			}
			return !isValue
		})
	}
}

// observeFunc judges every close below one top-level node, the closures in it
// included, against the uses that follow it in the same node.
func (s *scanner) observeFunc(pkg *packages.Package, pkgName string, node ast.Node, enclosing string) {
	fn := readFunction(pkg.TypesInfo, node)
	for _, site := range fn.closes {
		at := pkg.Fset.Position(site.call.Pos())
		key := fmt.Sprintf("%s:%d:%d", at.Filename, at.Line, at.Column)
		s.closes[key] = struct{}{}
		use, found := fn.firstUseAfter(site, s.fields)
		if !found {
			continue
		}
		s.findings[key] = Finding{
			Package: pkgName,
			File:    goprogram.RelativePath(at.Filename, s.root),
			Line:    at.Line,
			Func:    enclosing,
			Server:  types.ExprString(site.server),
			Use:     use.name,
			UseLine: pkg.Fset.Position(use.pos).Line,
		}
	}
}

// closeSite is one Close of an httptest server that something in the
// function holding it can still follow.
type closeSite struct {
	// call is the Close call, a call of a variable holding the Close, or the
	// Cleanup call a Close is handed to as a method value.
	call *ast.CallExpr
	// server is the closed expression, and object what it names: the
	// variable, or the field when it is a selector.
	server ast.Expr
	object types.Object
	// enclosing are the nodes around the close, outermost first, ending
	// with the call itself.
	enclosing []ast.Node
	// anchor is the node the close has happened by once it is left, as
	// [closesBy] and [testEnd] name it. It is always one of enclosing.
	anchor ast.Node
	// deferredIn is the function whose return runs a deferred close: the
	// innermost function literal around the defer, or the top-level
	// function. It is nil for a close that is not deferred.
	deferredIn ast.Node
	// escapes are the return statements of deferredIn written after the
	// defer that can run it: what they hand out is handed out as the close
	// runs.
	escapes []*ast.ReturnStmt
	// parallelsBefore is how many parallel subtests the node had started
	// where the close is written; the ones after it are written after it.
	parallelsBefore int
	// after is the index, into the function's identifiers in source order,
	// of the first one read once the close has happened: right after the
	// call, or, for one deferred or registered inside a function literal,
	// after that literal, and after the last identifier for one the
	// top-level function defers.
	after int
	// bound is the block, or case, around the close whose last statement
	// ends the function, as [endingBlock] finds it, and until the index of
	// the first identifier after it, which the close cannot reach. They are
	// nil and 0 when every identifier after the close can follow it.
	bound ast.Node
	until int
}

// use is where the address of a closed server is used after the close: the
// name it is used through, and where.
type use struct {
	name string
	pos  token.Pos
}

// span is a run of the function's identifiers, from the index of the first to
// the index of the first after it.
type span struct{ from, to int }

// parallelSubtest is a function literal handed to Run that calls Parallel on
// its own test. What it runs from that call on waits until the body of the
// function that started it has returned, the closes that body defers
// included, and runs before that test's Cleanup.
type parallelSubtest struct {
	literal *ast.FuncLit
	// starter is the function whose body ran the Run call: the innermost
	// function literal around it, or the top-level node.
	starter ast.Node
	// enclosing are the nodes around the call of Parallel, outermost first.
	enclosing []ast.Node
	// waits are the identifiers the literal reads from its call of Parallel
	// to its end.
	waits span
}

// function is what one top-level node says about its servers: where they are
// closed, what is assigned from what, and every identifier in source order.
type function struct {
	info   *types.Info
	closes []closeSite
	// assigned are the values this node gives its variables. What it gives a
	// field is a fact of the whole load, read into [scanner.fields].
	assigned *flows
	idents   []*ast.Ident
	// receivers are the identifiers that name the server in a Close of it,
	// or a variable holding that Close where it is called or registered,
	// which do not use its address.
	receivers map[*ast.Ident]bool
	// rebinds are the identifiers an assignment gives a new value to, each
	// with the span of identifiers over which the name holds that value:
	// from the first after the assignment to the end of the block or case
	// the assignment is in, or of the range statement that gives it. Within
	// the span the name no longer holds what the closed server gave it; the
	// right-hand side is read before, so a new value built from the old one
	// is a use of the old one, and past the end of a branch the old value
	// may still be there.
	rebinds map[*ast.Ident]span
	// closers are the variables a Close of an httptest server was given to
	// as a method value, `stop := srv.Close`, each with that Close, for as
	// long as no later assignment gives the variable something else.
	closers map[types.Object]*ast.SelectorExpr
	// parallels are the parallel subtests the node starts, in source order.
	parallels []parallelSubtest
}

// readFunction walks one top-level node once and keeps what the judgement of
// its closes needs.
func readFunction(info *types.Info, node ast.Node) *function {
	w := &walk{
		fn: &function{
			info:      info,
			assigned:  newFlows(),
			receivers: map[*ast.Ident]bool{},
			rebinds:   map[*ast.Ident]span{},
			closers:   map[types.Object]*ast.SelectorExpr{},
		},
		waiting:   map[ast.Node][]int{},
		bounding:  map[ast.Node][]int{},
		rebinding: map[ast.Node][]*ast.Ident{},
		unbinding: map[ast.Node][]*ast.Ident{},
		waits:     map[ast.Node]int{},
	}
	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			w.leave()
		} else {
			w.enter(n)
		}
		return true
	})
	return w.fn
}

// walk is one pass of [readFunction] over a top-level node.
//
// ast.Inspect announces a node's end with a nil call and names no node, so
// the path of open nodes is kept here: a close is judged by what encloses it,
// and it has happened once the node it waits for is left, which is where the
// identifiers read after it begin. A rebind takes effect the same way, once
// its assignment is left, or, for the names a range statement gives values
// to, once the expression it ranges over is, and stops holding once the
// block or case around the assignment, or the range statement, is left.
type walk struct {
	fn   *function
	open []ast.Node
	// waiting holds the closes that have happened once a node is left,
	// bounding the closes whose uses end there, rebinding the names whose
	// new value is theirs from there on, and unbinding the names whose new
	// value may not be theirs past it.
	waiting   map[ast.Node][]int
	bounding  map[ast.Node][]int
	rebinding map[ast.Node][]*ast.Ident
	unbinding map[ast.Node][]*ast.Ident
	// waits maps each parallel subtest's literal to its index in
	// [function.parallels], so its end is recorded once it is left.
	waits map[ast.Node]int
}

// enter records what one node says before the nodes below it are visited,
// and opens it.
func (w *walk) enter(n ast.Node) {
	fn := w.fn
	switch n := n.(type) {
	case *ast.Ident:
		fn.idents = append(fn.idents, n)
	case *ast.SelectorExpr:
		if isHTTPTestClose(fn.info, n) {
			if name := nameOf(n.X); name != nil {
				fn.receivers[name] = true
			}
		}
	case *ast.CallExpr:
		w.noteParallel(n)
		if anchor := fn.noteClose(n, w.open); anchor != nil {
			w.waiting[anchor] = append(w.waiting[anchor], len(fn.closes)-1)
			if bound := endingBlock(fn.info, w.open); bound != nil {
				fn.closes[len(fn.closes)-1].bound = bound
				w.bounding[bound] = append(w.bounding[bound], len(fn.closes)-1)
			}
		}
	case *ast.AssignStmt:
		rebound := fn.noteAssignments(n.Lhs, n.Rhs)
		if replaces(n.Tok) {
			w.rebinding[n] = rebound
			branch := innermostBranch(w.open)
			w.unbinding[branch] = append(w.unbinding[branch], rebound...)
		}
	case *ast.ValueSpec:
		fn.noteAssignments(sourcegate.IdentsAsExprs(n.Names), n.Values)
	case *ast.RangeStmt:
		rebound := fn.noteAssignments([]ast.Expr{n.Key, n.Value}, []ast.Expr{n.X})
		w.rebinding[n.X] = rebound
		w.unbinding[n] = rebound
	case *ast.ReturnStmt:
		fn.noteReturn(n, w.open)
	}
	w.open = append(w.open, n)
}

// noteReturn hands a return statement to every close deferred earlier in the
// function it returns from that the return can run: one with no block around
// it that ends the function, or one whose block holds the return. A return
// inside a function literal returns from the literal, not from the function
// around it.
func (fn *function) noteReturn(ret *ast.ReturnStmt, open []ast.Node) {
	from := enclosingFunction(open)
	for i := range fn.closes {
		site := &fn.closes[i]
		if site.deferredIn == from && (site.bound == nil || slices.Contains(open, site.bound)) {
			site.escapes = append(site.escapes, ret)
		}
	}
}

// leave closes the innermost open node and settles what waited for its end
// at the index of the first identifier after it.
func (w *walk) leave() {
	left := w.open[len(w.open)-1]
	w.open = w.open[:len(w.open)-1]
	at := len(w.fn.idents)
	for _, i := range w.waiting[left] {
		w.fn.closes[i].after = at
	}
	for _, i := range w.bounding[left] {
		w.fn.closes[i].until = at
	}
	for _, ident := range w.rebinding[left] {
		w.fn.rebinds[ident] = span{from: at}
	}
	for _, ident := range w.unbinding[left] {
		held := w.fn.rebinds[ident]
		held.to = at
		w.fn.rebinds[ident] = held
	}
	if i, waited := w.waits[left]; waited {
		w.fn.parallels[i].waits.to = at
	}
}

// innermostBranch returns the innermost block, or case of a switch or a
// select, around an assignment. Every assignment is a statement of one, so
// there always is one.
func innermostBranch(open []ast.Node) ast.Node {
	i := len(open) - 1
	for statementsOf(open[i]) == nil {
		i--
	}
	return open[i]
}

// enclosingFunction returns the function a node sits in: the innermost
// function literal around it, or the top-level node.
func enclosingFunction(open []ast.Node) ast.Node {
	for _, node := range slices.Backward(open) {
		if literal, isLiteral := node.(*ast.FuncLit); isLiteral {
			return literal
		}
	}
	return open[0]
}

// noteParallel records the parallel subtest a call of Parallel makes of the
// function literal, handed to Run, that declares the test it is called on.
// What the literal runs before the call runs at once, inside Run, so the
// identifiers it waits with begin at the call.
func (w *walk) noteParallel(call *ast.CallExpr) {
	fn := w.fn
	parallel := testingCall(fn.info, call, "Parallel")
	if parallel == nil {
		return
	}
	test := fn.info.Uses[nameOf(parallel.X)]
	for i, node := range slices.Backward(w.open) {
		literal, isLiteral := node.(*ast.FuncLit)
		if !isLiteral || !defines(fn.info, literal, test) {
			continue
		}
		if testingCallOf(fn.info, w.open, i, "Run") == nil {
			return
		}
		w.waits[literal] = len(fn.parallels)
		fn.parallels = append(fn.parallels, parallelSubtest{
			literal:   literal,
			starter:   enclosingFunction(w.open[:i]),
			enclosing: slices.Clone(w.open),
			waits:     span{from: len(fn.idents)},
		})
		return
	}
}

// noteClose records a call that closes an httptest server something names,
// or registers its Close on a test, when something in the top-level function
// can still follow the close, and returns the node the close has happened by
// once it is left. It returns nil for any call it does not record.
func (fn *function) noteClose(call *ast.CallExpr, open []ast.Node) ast.Node {
	closed, when := fn.closeIn(open, call)
	if closed == nil {
		return nil
	}
	name := nameOf(closed.X)
	if name == nil || when.anchor == nil {
		return nil
	}
	fn.closes = append(fn.closes, closeSite{
		call:            call,
		server:          closed.X,
		object:          fn.info.Uses[name],
		enclosing:       append(slices.Clone(open), call),
		anchor:          when.anchor,
		deferredIn:      when.deferredIn,
		parallelsBefore: len(fn.parallels),
	})
	return when.anchor
}

// closing says when a close happens: the node it has happened by once that
// node is left, and, for a deferred close, the function whose return runs
// it.
type closing struct {
	anchor     ast.Node
	deferredIn ast.Node
}

// closeIn returns the Close selector a call makes or registers, with when the
// close happens: once the call itself is over for a close made in place, and
// what [closesBy] says for a deferred one. The call makes the close when what
// it calls is the Close or a variable holding it. A close handed to a test's
// Cleanup, as the method value `t.Cleanup(srv.Close)` or a variable holding
// it, happens when that test ends. The selector is nil for a call that closes
// no httptest server.
func (fn *function) closeIn(open []ast.Node, call *ast.CallExpr) (*ast.SelectorExpr, closing) {
	if closed := fn.closeThrough(call.Fun); closed != nil {
		return closed, closesBy(fn.info, open, call)
	}
	cleanup := testingCall(fn.info, call, "Cleanup")
	if cleanup == nil {
		return nil, closing{}
	}
	registered := fn.closeThrough(call.Args[0])
	if registered == nil {
		return nil, closing{}
	}
	return registered, closing{anchor: testEnd(fn.info, open, cleanup.X)}
}

// closeThrough returns the Close of an httptest server an expression names:
// the method itself, `srv.Close`, or a variable it was given to as a method
// value, which is then marked as naming the server rather than using its
// address. It returns nil for anything else.
func (fn *function) closeThrough(expr ast.Expr) *ast.SelectorExpr {
	switch e := ast.Unparen(expr).(type) {
	case *ast.SelectorExpr:
		if isHTTPTestClose(fn.info, e) {
			return e
		}
	case *ast.Ident:
		closed := fn.closers[fn.info.Uses[e]]
		if closed != nil {
			fn.receivers[e] = true
		}
		return closed
	}
	return nil
}

// closesBy says when a close happens, given the nodes that enclose it,
// innermost last.
//
// A deferred close waits for the function that deferred it: the innermost
// function literal around the defer, by the reading of [literalEnd], or the
// top-level function when there is none, whose end is the end of the walk. A
// close in a function literal handed to a test's Cleanup waits for that test
// to end, by the reading of [testEnd]. Any other close has happened once its
// own call is over.
func closesBy(info *types.Info, open []ast.Node, call *ast.CallExpr) closing {
	deferred := false
	for i, node := range slices.Backward(open) {
		switch node := node.(type) {
		case *ast.DeferStmt:
			deferred = true
		case *ast.FuncLit:
			if cleanup := testingCallOf(info, open, i, "Cleanup"); cleanup != nil {
				return closing{anchor: testEnd(info, open[:i], cleanup.X)}
			}
			if deferred {
				return closing{anchor: literalEnd(info, open, i, node), deferredIn: node}
			}
		}
	}
	if deferred {
		return closing{anchor: open[0], deferredIn: open[0]}
	}
	return closing{anchor: call}
}

// testEnd names the node a test has ended by once it is left, given the
// expression its *testing.T or testing.TB is named by and the nodes around
// it, innermost last. A subtest, a function literal handed to Run, ends when
// it returns, as [literalEnd] reads it. The test of the top-level function,
// and a test a function literal was handed by whoever called it, end outside
// this function, and the answer is nil.
func testEnd(info *types.Info, open []ast.Node, test ast.Expr) ast.Node {
	object := info.Uses[nameOf(test)]
	for i, node := range slices.Backward(open) {
		literal, isLiteral := node.(*ast.FuncLit)
		if !isLiteral || !defines(info, literal, object) {
			continue
		}
		if testingCallOf(info, open, i, "Run") == nil {
			return nil
		}
		return literalEnd(info, open, i, literal)
	}
	return nil
}

// literalEnd names the node a function literal, open[i], has returned by once
// it is left: the literal itself, unless it is a subtest that calls Parallel
// on its own test. Such a subtest goes on only once the body of the test that
// started it has returned, so it ends with that test.
func literalEnd(info *types.Info, open []ast.Node, i int, literal *ast.FuncLit) ast.Node {
	run := testingCallOf(info, open, i, "Run")
	if run == nil || !callsParallel(info, literal) {
		return literal
	}
	return testEnd(info, open[:i], run.X)
}

// callsParallel reports whether a function literal calls Parallel on a test
// it takes as a parameter.
func callsParallel(info *types.Info, literal *ast.FuncLit) bool {
	found := false
	ast.Inspect(literal.Body, func(n ast.Node) bool {
		if call, isCall := n.(*ast.CallExpr); isCall {
			parallel := testingCall(info, call, "Parallel")
			found = parallel != nil && defines(info, literal, info.Uses[nameOf(parallel.X)])
		}
		return !found
	})
	return found
}

// defines reports whether a function literal declares an object as one of its
// parameters.
func defines(info *types.Info, literal *ast.FuncLit, object types.Object) bool {
	for _, field := range literal.Type.Params.List {
		for _, name := range field.Names {
			if info.Defs[name] == object {
				return true
			}
		}
	}
	return false
}

// testingCallOf returns the selector of the testing method the function
// literal at open[i] is an argument of, when that method is the one named,
// and nil otherwise. A function literal is never the top-level node itself,
// so open[i-1] exists.
func testingCallOf(info *types.Info, open []ast.Node, i int, method string) *ast.SelectorExpr {
	call, isCall := open[i-1].(*ast.CallExpr)
	if !isCall {
		return nil
	}
	return testingCall(info, call, method)
}

// testingCall returns the selector of a call of the named method of a test,
// such as t.Run or tb.Cleanup, and nil for any other call.
func testingCall(info *types.Info, call *ast.CallExpr, method string) *ast.SelectorExpr {
	sel, isSel := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !isSel || sel.Sel.Name != method {
		return nil
	}
	callee, isFunc := info.Uses[sel.Sel].(*types.Func)
	if !isFunc || packagePath(callee) != testingPath {
		return nil
	}
	return sel
}

// endingBlock returns the innermost block around a close, or case of a switch
// or a select, whose last statement ends the top-level function: a return, a
// panic, or a test's Fatal, FailNow or Skip. Nothing after such a block can
// follow the close, so the uses looked for end with it. It returns nil when
// no block around the close ends the function, and for a close inside any
// function literal, whose return leaves the function around it running.
//
// The error branch of a helper that closes the server it could not use and
// then panics is the shape this answers: the success path after the branch
// never meets the close. Every block on the path holds the close, so none is
// empty: a case holds it in its body, since a Close gives no value a case
// expression could test.
func endingBlock(info *types.Info, open []ast.Node) ast.Node {
	var bound ast.Node
	for _, node := range slices.Backward(open) {
		if _, isLiteral := node.(*ast.FuncLit); isLiteral {
			return nil
		}
		body := statementsOf(node)
		if bound == nil && body != nil && endsTheFunction(info, body[len(body)-1]) {
			bound = node
		}
	}
	return bound
}

// statementsOf returns the statements a block, or a case of a switch or a
// select, runs in order, and nil for any other node.
func statementsOf(node ast.Node) []ast.Stmt {
	switch node := node.(type) {
	case *ast.BlockStmt:
		return node.List
	case *ast.CaseClause:
		return node.Body
	case *ast.CommClause:
		return node.Body
	}
	return nil
}

// abortMethods are the methods of a test that end its function where they
// are called.
var abortMethods = map[string]bool{
	"FailNow": true,
	"Fatal":   true,
	"Fatalf":  true,
	"Skip":    true,
	"SkipNow": true,
	"Skipf":   true,
}

// endsTheFunction reports whether a statement leaves the function it is in:
// a return, a call of the builtin panic, or of a test's method that stops it.
func endsTheFunction(info *types.Info, stmt ast.Stmt) bool {
	if _, isReturn := stmt.(*ast.ReturnStmt); isReturn {
		return true
	}
	expr, isExpr := stmt.(*ast.ExprStmt)
	if !isExpr {
		return false
	}
	call, isCall := expr.X.(*ast.CallExpr)
	if !isCall {
		return false
	}
	switch callee := info.Uses[nameOf(call.Fun)].(type) {
	case *types.Builtin:
		return callee.Name() == "panic"
	case *types.Func:
		return packagePath(callee) == testingPath && abortMethods[callee.Name()]
	}
	return false
}

// isHTTPTestClose reports whether a selector is the Close method of an
// httptest server, promoted through an embedding or not.
func isHTTPTestClose(info *types.Info, sel *ast.SelectorExpr) bool {
	if sel.Sel.Name != "Close" {
		return false
	}
	method, isFunc := info.Uses[sel.Sel].(*types.Func)
	return isFunc && packagePath(method) == httptestPath
}

// packagePath is the import path of the package an object belongs to, and
// empty for one of the universe, such as the predeclared error's Error.
func packagePath(obj types.Object) string {
	if obj.Pkg() == nil {
		return ""
	}
	return obj.Pkg().Path()
}

// replaces reports whether an assignment's operator gives its names values
// that replace the ones they held: `=`, and `:=` for a name it redeclares. An
// operator such as `+=` reads the value it changes, so the name goes on
// holding what it held.
func replaces(tok token.Token) bool {
	return tok == token.ASSIGN || tok == token.DEFINE
}

// noteAssignments records the values one assignment, declaration or range
// statement gives, and returns the names it gives new values to that already
// held one. A single value on the right of several names is a call returning
// them all, or what a range statement ranges over, and each name is taken to
// come from it.
//
// A value stored into a field of a variable, `cfg.base = srv.URL`, is also
// recorded as held by the variable, so the struct used whole afterwards
// carries what was stored in it. The field itself is a fact of the whole
// load, read by [flows.readFields], and is not recorded here.
func (fn *function) noteAssignments(lhs, rhs []ast.Expr) []*ast.Ident {
	var rebound []*ast.Ident
	for i, target := range lhs {
		name := nameOf(target)
		if name == nil {
			continue
		}
		if fn.info.Uses[name] != nil {
			rebound = append(rebound, name)
		}
		value := valueFor(lhs, rhs, i)
		if value == nil {
			continue
		}
		if root := storeRoot(fn.info, target); root != nil {
			fn.assigned.add(assignment{info: fn.info, target: root, value: value, store: true})
			continue
		}
		object := fn.info.Defs[name]
		if object == nil {
			object = fn.info.Uses[name]
		}
		if object == nil || isField(object) {
			continue
		}
		fn.noteCloser(object, value)
		fn.assigned.add(assignment{info: fn.info, target: object, value: value})
	}
	return rebound
}

// noteCloser keeps a variable given the Close of an httptest server as a
// method value, `stop := srv.Close`, as a way to close that server, and
// forgets one given anything else, since a call of it then closes nothing.
func (fn *function) noteCloser(object types.Object, value ast.Expr) {
	if closed, isSel := ast.Unparen(value).(*ast.SelectorExpr); isSel && isHTTPTestClose(fn.info, closed) {
		fn.closers[object] = closed
		return
	}
	delete(fn.closers, object)
}

// valueFor returns the value the i-th of the names on the left is given: its
// own expression when there is one per name, the single expression on the
// right when there is one for all of them, and nil for a declaration that
// gives none.
func valueFor(lhs, rhs []ast.Expr, i int) ast.Expr {
	if len(rhs) == len(lhs) {
		return rhs[i]
	}
	if len(rhs) == 1 {
		return rhs[0]
	}
	return nil
}

// isField reports whether an object is a struct field.
func isField(object types.Object) bool {
	v, isVar := object.(*types.Var)
	return isVar && v.IsField()
}

// fieldOf returns the field a selector on the left of an assignment writes,
// and nil for anything else, such as a variable of another package.
func fieldOf(info *types.Info, target ast.Expr) types.Object {
	sel, isSel := ast.Unparen(target).(*ast.SelectorExpr)
	if !isSel {
		return nil
	}
	selection, isField := info.Selections[sel]
	if !isField {
		return nil
	}
	return selection.Obj()
}

// storeRoot returns the variable a field store writes into: the name at the
// root of a chain of field selections, cfg in `cfg.base = v` and in
// `cfg.inner.base = v`. It returns nil for anything else: a name, a variable
// of another package, and a store through an index, a call or a dereference,
// which this gate does not follow.
func storeRoot(info *types.Info, target ast.Expr) types.Object {
	sel, isSel := ast.Unparen(target).(*ast.SelectorExpr)
	if !isSel {
		return nil
	}
	if _, isField := info.Selections[sel]; !isField {
		return nil
	}
	held := ast.Unparen(sel.X)
	if root, isName := held.(*ast.Ident); isName {
		return info.Uses[root]
	}
	return storeRoot(info, held)
}

// nameOf is the identifier an expression names its object through: itself,
// or the selected name of a selector, which is the field or the qualified
// identifier. Anything else names nothing this gate can follow.
func nameOf(expr ast.Expr) *ast.Ident {
	switch e := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return e
	case *ast.SelectorExpr:
		return e.Sel
	}
	return nil
}
