// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"go/ast"
	"go/types"
	"slices"
)

// textPackages are the standard packages whose functions hand an address on
// when they are given one: trimming a scheme, joining a path, formatting a
// URL. A string built by any other call is taken to be what the call
// returned, an answer or a body, rather than the address it was handed.
var textPackages = map[string]bool{
	"fmt":           true,
	"net":           true,
	"net/url":       true,
	"path":          true,
	"path/filepath": true,
	"strings":       true,
}

// answersOfHTTP are the types of net/http that hold what an address answered
// rather than the address: reading one after the close dials nothing.
var answersOfHTTP = map[string]bool{
	"Header":   true,
	"Response": true,
}

// assignment is one value given to a variable or a field, which is how an
// address travels from the server to whatever uses it.
type assignment struct {
	// info resolves the names in value: the package the assignment is
	// written in, which for a field is not always the one being judged.
	info   *types.Info
	target types.Object
	value  ast.Expr
	// store marks a value written into a field of target rather than given
	// to target itself: target holds it from then on, but is not a copy of
	// it, so the server is never looked for through a store.
	store bool
}

// flows indexes assignments both ways: by what they give a value to, which is
// how the names a server was copied from are found, and by what their value
// names, which is how what was built from it is found.
type flows struct {
	to      map[types.Object][]assignment
	reading map[types.Object][]assignment
}

// newFlows returns an empty index.
func newFlows() *flows {
	return &flows{to: map[types.Object][]assignment{}, reading: map[types.Object][]assignment{}}
}

// add indexes one assignment.
func (f *flows) add(a assignment) {
	if !a.store {
		f.to[a.target] = append(f.to[a.target], a)
	}
	ast.Inspect(a.value, func(n ast.Node) bool {
		if ident, isIdent := n.(*ast.Ident); isIdent {
			object := a.info.Uses[ident]
			f.reading[object] = append(f.reading[object], a)
		}
		return true
	})
}

// readFields indexes every value one file gives a struct field: through an
// assignment to a selector, and through a composite literal, by key or by
// position.
func (f *flows) readFields(info *types.Info, file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for i, target := range n.Lhs {
				if field := fieldOf(info, target); field != nil {
					f.add(assignment{info: info, target: field, value: valueFor(n.Lhs, n.Rhs, i)})
				}
			}
		case *ast.CompositeLit:
			f.readLiteral(info, n)
		}
		return true
	})
}

// readLiteral indexes the fields a struct literal gives values to. A literal
// of any other type, a slice or a map, gives no field anything.
func (f *flows) readLiteral(info *types.Info, literal *ast.CompositeLit) {
	fields, isStruct := info.TypeOf(literal).Underlying().(*types.Struct)
	if !isStruct {
		return
	}
	for i, element := range literal.Elts {
		if keyed, isKeyed := element.(*ast.KeyValueExpr); isKeyed {
			f.add(assignment{info: info, target: info.Uses[nameOf(keyed.Key)], value: keyed.Value})
			continue
		}
		f.add(assignment{info: info, target: fields.Field(i), value: element})
	}
}

// firstUseAfter returns the first use, once the close has happened, of the
// closed server or a name its address was given to.
//
// Names that could only hold the address are followed from the server
// through every assignment, to a fixed point, before any use is looked for:
// a URL saved before the close, a client built on it, a string made from it.
// After an assignment gives one of them a new value, it is no longer followed
// from the end of that assignment to the end of the block or case it is in:
// what its right-hand side reads, such as the old value a trim is made from,
// is still a use, what is built from the name afterwards on that branch is
// built from its new value, and past the branch the old value may still be
// there.
//
// The uses are looked for in three places, in this order: the identifiers
// after the close; those a parallel subtest reads once it has waited for the
// body that started it to return, when the close happens before then; and,
// for a deferred close, what its function returns after the defer, which the
// caller holds once the close has run.
func (fn *function) firstUseAfter(site closeSite, fields *flows) (use, bool) {
	end := len(fn.idents)
	if site.until > 0 {
		end = site.until
	}
	past := fn.pastRebinds(site.after, end)
	tracked := fn.derive(site.object, fields, past)
	windows := []span{{from: site.after, to: end}}
	for i, subtest := range fn.parallels {
		if waitsFor(subtest, i >= site.parallelsBefore, site) {
			windows = append(windows, subtest.waits)
		}
	}
	for _, window := range windows {
		for i := window.from; i < window.to; i++ {
			if ident := fn.idents[i]; fn.isUse(ident, tracked, past) {
				return use{name: ident.Name, pos: ident.Pos()}, true
			}
		}
	}
	return fn.escapeOf(site, tracked, past)
}

// isUse reports whether an identifier reads a tracked name, other than to
// close the server, to give the name a new value, or while an assignment
// has given it one.
func (fn *function) isUse(ident *ast.Ident, tracked map[types.Object]bool, past map[*ast.Ident]bool) bool {
	if !tracked[fn.info.Uses[ident]] || fn.receivers[ident] || past[ident] {
		return false
	}
	_, rebinds := fn.rebinds[ident]
	return !rebinds
}

// waitsFor reports whether a parallel subtest goes on only once a close has
// happened, given whether the subtest is written after the close.
//
// The close is not one the subtest makes, and it happens while the body that
// started the subtest is still running: deferred by that body, or made in it,
// in place or in what it runs before it returns, a subtest and what that
// subtest defers or registers included, which is a close whose anchor that
// body holds. A close registered on the Cleanup of the body's own test, or of
// an outer one, happens after the subtest. A subtest written after a block
// that holds the close and ends the function is never started once that
// close has happened.
func waitsFor(subtest parallelSubtest, writtenAfter bool, site closeSite) bool {
	if slices.Contains(site.enclosing, ast.Node(subtest.literal)) {
		return false
	}
	if writtenAfter && site.bound != nil && !slices.Contains(subtest.enclosing, site.bound) {
		return false
	}
	if site.deferredIn == subtest.starter {
		return true
	}
	anchored := slices.Index(site.enclosing, site.anchor)
	return slices.Contains(site.enclosing[:anchored], subtest.starter)
}

// escapeOf returns, for a deferred close, the first use of the address in
// what its function returns after the defer: the values of a return
// statement, or the named results a bare one hands out. The caller holds them
// once the close has run, so a helper that defers the close of the server
// whose address it returns hands out an address nothing answers on.
func (fn *function) escapeOf(site closeSite, tracked map[types.Object]bool, past map[*ast.Ident]bool) (use, bool) {
	for _, ret := range site.escapes {
		results := resultsOf(fn.info, site.deferredIn)
		if escaped, found := fn.returned(ret, results, tracked, past); found {
			return escaped, true
		}
	}
	return use{}, false
}

// returned returns the first use of the address in what one return statement
// hands out: a value that can carry it, read as [carrier] reads one of its
// type, or a named result a bare return hands out that was given it.
func (fn *function) returned(ret *ast.ReturnStmt, results *types.Tuple, tracked map[types.Object]bool, past map[*ast.Ident]bool) (use, bool) {
	if len(ret.Results) == 0 {
		for result := range results.Variables() {
			if tracked[result] {
				return use{name: result.Name(), pos: ret.Pos()}, true
			}
		}
		return use{}, false
	}
	for _, expr := range ret.Results {
		for _, kind := range valueTypes(fn.info.TypeOf(expr)) {
			if !canCarry(kind) {
				continue
			}
			if ident := carrier(fn.info, expr, tracked, past, isText(kind)); ident != nil {
				return use{name: ident.Name, pos: ident.Pos()}, true
			}
		}
	}
	return use{}, false
}

// resultsOf returns the results of a function a deferred close waits for: a
// function literal, whose type is recorded for the literal, or the top-level
// declaration, whose type is recorded for its name. A defer is only ever
// written inside one of them.
func resultsOf(info *types.Info, function ast.Node) *types.Tuple {
	var typed ast.Expr
	if literal, isLiteral := function.(*ast.FuncLit); isLiteral {
		typed = literal
	}
	if decl, isDecl := function.(*ast.FuncDecl); isDecl {
		typed = decl.Name
	}
	signature, _ := info.TypeOf(typed).(*types.Signature)
	return signature.Results()
}

// valueTypes returns the types of the values an expression gives: each
// result of a call that returns several, or the expression's own type.
func valueTypes(t types.Type) []types.Type {
	tuple, isTuple := t.(*types.Tuple)
	if !isTuple {
		return []types.Type{t}
	}
	var kinds []types.Type
	for value := range tuple.Variables() {
		kinds = append(kinds, value.Type())
	}
	return kinds
}

// pastRebinds returns the identifiers, among those from after to end, that
// read a name while an assignment there has given it a new value: from the
// end of the assignment to the end of the branch it is on. A rebind on an
// outer branch goes on holding past the end of an inner one.
func (fn *function) pastRebinds(after, end int) map[*ast.Ident]bool {
	type rebind struct {
		object types.Object
		until  int
	}
	starts := map[int][]rebind{}
	holds := map[types.Object]int{}
	past := map[*ast.Ident]bool{}
	for i := after; i < end; i++ {
		for _, r := range starts[i] {
			holds[r.object] = max(holds[r.object], r.until)
		}
		ident := fn.idents[i]
		object := fn.info.Uses[ident]
		if i < holds[object] {
			past[ident] = true
		}
		if held, rebinds := fn.rebinds[ident]; rebinds {
			starts[held.from] = append(starts[held.from], rebind{object: object, until: held.to})
		}
	}
	return past
}

// derive returns the closed server's object with every other name for the
// same server, and every object an assignment gave something carrying its
// address to.
//
// The other names are found backwards from the one the server is closed
// through: what it was copied from, `other := srv`, and what a field it is
// closed through was given, `holder{server: srv}`, to a fixed point. Only
// then is anything followed forwards, from all of them, so two servers
// stored in one field are not taken for one another merely because both
// were stored there.
func (fn *function) derive(server types.Object, fields *flows, past map[*ast.Ident]bool) map[types.Object]bool {
	tracked := map[types.Object]bool{}
	var pending []types.Object
	add := func(object types.Object) {
		if !tracked[object] {
			tracked[object] = true
			pending = append(pending, object)
		}
	}
	add(server)
	var names []types.Object
	for len(pending) > 0 {
		object := pending[0]
		pending = pending[1:]
		names = append(names, object)
		for _, a := range slices.Concat(fn.assigned.to[object], fields.to[object]) {
			if source := copiedFrom(a); source != nil {
				add(source)
			}
		}
	}
	pending = names
	for len(pending) > 0 {
		object := pending[0]
		pending = pending[1:]
		for _, a := range slices.Concat(fn.assigned.reading[object], fields.reading[object]) {
			if canCarry(a.target.Type()) && carries(a.info, a.value, tracked, past, isText(a.target.Type())) {
				add(a.target)
			}
		}
	}
	return tracked
}

// copiedFrom returns the name an assignment copies its target's value from,
// when the value is a name of the same type, and nil otherwise.
func copiedFrom(a assignment) types.Object {
	name := nameOf(a.value)
	if name == nil {
		return nil
	}
	source := a.info.Uses[name]
	if !types.Identical(source.Type(), a.target.Type()) {
		return nil
	}
	return source
}

// carries reports whether an expression names something tracked, other than
// through a name read while an assignment has given it a new value, as
// [carrier] reads it.
func carries(info *types.Info, expr ast.Expr, tracked map[types.Object]bool, past map[*ast.Ident]bool, text bool) bool {
	return carrier(info, expr, tracked, past, text) != nil
}

// carrier returns the first identifier through which an expression names
// something tracked, other than through a name read while an assignment has
// given it a new value, and nil when there is none.
//
// For text the expression is read as an address is built: through
// selectors, operators, conversions and the calls of [textPackages], and not
// into the arguments of any other call, whose result is taken to be what the
// call returned rather than what it was handed. Anything else, a client, a
// function or a struct, is taken to keep whatever it was built from.
func carrier(info *types.Info, expr ast.Expr, tracked map[types.Object]bool, past map[*ast.Ident]bool, text bool) *ast.Ident {
	var found *ast.Ident
	ast.Inspect(expr, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		switch n := n.(type) {
		case *ast.Ident:
			if tracked[info.Uses[n]] && !past[n] {
				found = n
			}
		case *ast.CallExpr:
			if text && !passesTextOn(info, n) {
				found = carrier(info, n.Fun, tracked, past, text)
				return false
			}
		}
		return true
	})
	return found
}

// passesTextOn reports whether a call hands on the text it is given: a
// conversion, or a function of [textPackages].
func passesTextOn(info *types.Info, call *ast.CallExpr) bool {
	if info.Types[call.Fun].IsType() {
		return true
	}
	callee, isFunc := info.Uses[nameOf(call.Fun)].(*types.Func)
	return isFunc && textPackages[packagePath(callee)]
}

// errorType is the predeclared error interface.
var errorType = types.Universe.Lookup("error").Type()

// canCarry reports whether a value of a type can hold an address: anything
// but an error, a boolean, a number, or an answer, which can say what an
// address answered and cannot be dialed.
func canCarry(t types.Type) bool {
	if types.Identical(t, errorType) || isAnswer(t) {
		return false
	}
	basic, isBasic := t.Underlying().(*types.Basic)
	return !isBasic || isText(basic)
}

// isText reports whether a type is text: a string, or a slice of bytes, named
// or not.
func isText(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Info()&types.IsString != 0
	case *types.Slice:
		return types.Identical(u.Elem(), types.Typ[types.Byte])
	}
	return false
}

// isAnswer reports whether a type holds what an address answered: a response
// or a header of net/http, a stream of io, a pointer to one, or a struct that
// embeds one, as a client's own response type does.
func isAnswer(t types.Type) bool {
	t = deref(t)
	if isAnswerType(t) {
		return true
	}
	fields, isStruct := t.Underlying().(*types.Struct)
	if !isStruct {
		return false
	}
	for field := range fields.Fields() {
		if field.Embedded() && isAnswerType(deref(field.Type())) {
			return true
		}
	}
	return false
}

// isAnswerType reports whether a type is itself one of the answers
// [isAnswer] names.
func isAnswerType(t types.Type) bool {
	named, isNamed := types.Unalias(t).(*types.Named)
	if !isNamed {
		return false
	}
	switch packagePath(named.Obj()) {
	case "io":
		return true
	case "net/http":
		return answersOfHTTP[named.Obj().Name()]
	}
	return false
}

// deref is the type a pointer points to, and any other type as it is.
func deref(t types.Type) types.Type {
	if pointer, isPointer := types.Unalias(t).(*types.Pointer); isPointer {
		return pointer.Elem()
	}
	return t
}
