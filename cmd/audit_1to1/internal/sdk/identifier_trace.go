// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
)

// tracedFunc is one body the identifiers walk reads, with the type
// information of its package and the name the section prints it under.
type tracedFunc struct {
	funcBody
	info *types.Info
	// site is the function as the section prints it, its package first.
	site string
}

// tracedFuncs lists the bodies of every package of pkgs.
func tracedFuncs(pkgs []*packages.Package) []tracedFunc {
	var out []tracedFunc
	for _, pkg := range pkgs {
		name := shared.ShortPackage(pkg.PkgPath)
		for _, fn := range functionBodies(pkg) {
			out = append(out, tracedFunc{funcBody: fn, info: pkg.TypesInfo, site: name + "." + fn.name})
		}
	}
	return out
}

// origins is where a value comes from, as far as the trace follows it: the
// input fields it is read from, and the expressions the trace stopped at
// without knowing what they hold, each with the function it is in. A value
// read only from a client-go struct, a constant, a literal, a count or a
// position carries neither.
type origins struct {
	inputs  []string
	stopped []string
}

// add joins more to o.
func (o *origins) add(more origins) {
	o.inputs = append(o.inputs, more.inputs...)
	o.stopped = append(o.stopped, more.stopped...)
}

// stoppedAt is the origin of a value the trace cannot follow past what, in
// fn.
func stoppedAt(fn tracedFunc, what string) origins {
	return origins{stopped: []string{fn.site + ": " + what}}
}

// countingBuiltins are the built-in functions whose result is a count or a
// new value rather than one of their arguments.
var countingBuiltins = map[string]bool{"len": true, "cap": true, "make": true, "new": true, "copy": true}

// inputTrace follows an expression of a walked function back to the input
// fields it is read from, across every walked package.
type inputTrace struct {
	// bodies maps each walked function, by its object, to its body.
	bodies map[types.Object]tracedFunc
	// callers lists, for each walked function, the calls of it the walked
	// bodies make by name.
	callers map[types.Object][]callerSite
	// asValue holds the walked functions named other than by such a call
	// (handed on as a value, called as a method expression, or called
	// from a package-level initializer), whose parameters are handed what
	// the walk does not see.
	asValue map[types.Object]bool
}

// callerSite is one call of a walked function, with the function it is in.
type callerSite struct {
	fn   tracedFunc
	call *ast.CallExpr
}

// newInputTrace indexes the calls funcs make of one another, and the walked
// functions pkgs name anywhere else.
func newInputTrace(pkgs []*packages.Package, funcs []tracedFunc) *inputTrace {
	trace := &inputTrace{bodies: map[types.Object]tracedFunc{}, callers: map[types.Object][]callerSite{}, asValue: map[types.Object]bool{}}
	for _, fn := range funcs {
		trace.bodies[fn.object] = fn
	}
	called := map[*ast.Ident]bool{}
	for _, fn := range funcs {
		ast.Inspect(fn.body, func(node ast.Node) bool {
			if call, isCall := node.(*ast.CallExpr); isCall {
				if callee, walked := trace.walkedCallee(fn.info, call); walked {
					trace.callers[callee.object] = append(trace.callers[callee.object], callerSite{fn: fn, call: call})
					called[calledIdent(call)] = true
				}
			}
			return true
		})
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(node ast.Node) bool {
				if ident, isIdent := node.(*ast.Ident); isIdent && !called[ident] {
					if named, walked := trace.bodies[funcOrigin(pkg.TypesInfo.Uses[ident])]; walked {
						trace.asValue[named.object] = true
					}
				}
				return true
			})
		}
	}
	return trace
}

// walkedCallee returns the walked function a call calls by name, and false
// for a call of anything else and for a method expression, whose arguments
// start with the receiver.
func (t *inputTrace) walkedCallee(info *types.Info, call *ast.CallExpr) (tracedFunc, bool) {
	callee, walked := t.bodies[calledObject(info, call)]
	return callee, walked && !isMethodExpression(info, call.Fun)
}

// isMethodExpression reports whether fun names a method through its type
// (T.Method), not through a value.
func isMethodExpression(info *types.Info, fun ast.Expr) bool {
	sel, isSelector := ast.Unparen(fun).(*ast.SelectorExpr)
	return isSelector && info.Selections[sel] != nil && info.Selections[sel].Kind() == types.MethodExpr
}

// funcOrigin returns a function's generic origin, and any other object as it
// is.
func funcOrigin(obj types.Object) types.Object {
	if fn, isFunc := obj.(*types.Func); isFunc {
		return fn.Origin()
	}
	return obj
}

// unseenCallers reports whether a walked function is handed arguments the
// walk does not see: it is named other than by a call, or called by no
// walked body.
func (t *inputTrace) unseenCallers(fn tracedFunc) bool {
	return t.asValue[fn.object] || len(t.callers[fn.object]) == 0
}

// origins follows expr, an expression of fn, back to where its value comes
// from. via, when set, is the one call of fn whose arguments the parameters
// and the receiver of fn are read from.
func (t *inputTrace) origins(fn tracedFunc, expr ast.Expr, via *ast.CallExpr) origins {
	return t.follow(fn, expr, via, map[ast.Expr]bool{})
}

// follow is origins with the expressions already followed, which keep a
// function that calls itself from being read again.
func (t *inputTrace) follow(fn tracedFunc, expr ast.Expr, via *ast.CallExpr, seen map[ast.Expr]bool) origins {
	expr = ast.Unparen(expr)
	if seen[expr] {
		return origins{}
	}
	seen[expr] = true
	if value := fn.info.Types[expr]; value.Value != nil || value.IsType() {
		return origins{}
	}
	switch typed := expr.(type) {
	case *ast.CallExpr:
		return t.called(fn, typed, 0, via, seen)
	case *ast.SelectorExpr:
		return t.selected(fn, typed, via, seen)
	case *ast.Ident:
		return t.identified(fn, typed, via, seen)
	case *ast.StarExpr:
		return t.follow(fn, typed.X, via, seen)
	case *ast.IndexExpr:
		return t.follow(fn, typed.X, via, seen)
	case *ast.SliceExpr:
		return t.follow(fn, typed.X, via, seen)
	case *ast.UnaryExpr:
		return t.follow(fn, typed.X, via, seen)
	case *ast.TypeAssertExpr:
		return t.follow(fn, typed.X, via, seen)
	case *ast.BinaryExpr:
		found := t.follow(fn, typed.X, via, seen)
		found.add(t.follow(fn, typed.Y, via, seen))
		return found
	case *ast.CompositeLit:
		var found origins
		for _, elt := range typed.Elts {
			if pair, isPair := elt.(*ast.KeyValueExpr); isPair {
				elt = pair.Value
			}
			found.add(t.follow(fn, elt, via, seen))
		}
		return found
	}
	return stoppedAt(fn, types.ExprString(expr))
}

// results follows the result at index of expr, a call or the comma-ok form
// of an index, an assertion or a receive.
func (t *inputTrace) results(fn tracedFunc, expr ast.Expr, index int, via *ast.CallExpr, seen map[ast.Expr]bool) origins {
	if call, isCall := ast.Unparen(expr).(*ast.CallExpr); isCall {
		return t.called(fn, call, index, via, seen)
	}
	return t.follow(fn, expr, via, seen)
}

// called follows the result at index of a call: the converted value of a
// conversion, what a walked function returns there when called from this
// call, nothing for a built-in that counts or allocates, and the receiver
// and the arguments of any other function or method, whose result is taken
// to come from them. A call of a function value, a function literal
// included, is not followed.
func (t *inputTrace) called(fn tracedFunc, call *ast.CallExpr, index int, via *ast.CallExpr, seen map[ast.Expr]bool) origins {
	if fn.info.Types[call.Fun].IsType() {
		return t.follow(fn, call.Args[0], via, seen)
	}
	if callee, walked := t.walkedCallee(fn.info, call); walked {
		return t.returned(callee, index, call, seen)
	}
	switch callee := calledObject(fn.info, call).(type) {
	case nil, *types.Var:
		return stoppedAt(fn, types.ExprString(call))
	case *types.Builtin:
		if countingBuiltins[callee.Name()] {
			return origins{}
		}
	}
	var found origins
	if sel, isSelector := ast.Unparen(call.Fun).(*ast.SelectorExpr); isSelector && fn.info.Selections[sel] != nil {
		found.add(t.follow(fn, sel.X, via, seen))
	}
	for _, arg := range call.Args {
		found.add(t.follow(fn, arg, via, seen))
	}
	return found
}

// returned follows the result at index of every return statement of fn,
// whose parameters are read from via; the returns of a function literal in
// it are its own. A bare return hands back named results, which are not
// followed.
func (t *inputTrace) returned(fn tracedFunc, index int, via *ast.CallExpr, seen map[ast.Expr]bool) origins {
	var found origins
	ast.Inspect(fn.body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			found.add(t.returnedBy(fn, typed, index, via, seen))
		}
		return true
	})
	return found
}

// returnedBy follows the result at index of one return statement of fn: a
// bare return stops the trace, a return of one call that hands back all of
// fn's results is that call's result at index, and any other the value in
// its place.
func (t *inputTrace) returnedBy(fn tracedFunc, ret *ast.ReturnStmt, index int, via *ast.CallExpr, seen map[ast.Expr]bool) origins {
	if len(ret.Results) == 0 {
		return stoppedAt(fn, "return")
	}
	if len(ret.Results) < fn.signature.Results().Len() {
		return t.results(fn, ret.Results[0], index, via, seen)
	}
	return t.follow(fn, ret.Results[index], via, seen)
}

// selected follows a selector: a field of a client-go struct is what an
// answer or an option carries, a field of a struct of the repository's own
// is an input field, and a field of an unnamed struct is read from what the
// struct is built from. A qualified variable, a function and a method are
// not followed.
func (t *inputTrace) selected(fn tracedFunc, sel *ast.SelectorExpr, via *ast.CallExpr, seen map[ast.Expr]bool) origins {
	selection := fn.info.Selections[sel]
	if selection == nil || selection.Kind() != types.FieldVal {
		return stoppedAt(fn, types.ExprString(sel))
	}
	recv := selection.Recv()
	if pointer, isPointer := recv.(*types.Pointer); isPointer {
		recv = pointer.Elem()
	}
	named, isNamed := types.Unalias(recv).(*types.Named)
	if !isNamed {
		return t.follow(fn, sel.X, via, seen)
	}
	if shared.IsClientGoObject(named.Obj()) {
		return origins{}
	}
	return origins{inputs: []string{shared.ShortPackage(named.Obj().Pkg().Path()) + "." + named.Obj().Name() + "." + selection.Obj().Name()}}
}

// identified follows a variable of fn: a parameter to what the calls of fn
// hand it, the receiver to what they call fn on, and a local to everything
// assigned to it. A package-level variable, and a function named as a value,
// are not followed.
func (t *inputTrace) identified(fn tracedFunc, ident *ast.Ident, via *ast.CallExpr, seen map[ast.Expr]bool) origins {
	variable, isVariable := fn.info.Uses[ident].(*types.Var)
	if !isVariable || variable.Parent() == variable.Pkg().Scope() {
		return stoppedAt(fn, ident.Name)
	}
	if index := paramIndex(fn.info, fn.signature, ident, false); index >= 0 {
		return t.parameter(fn, index, via, seen)
	}
	if fn.signature.Recv() == variable {
		return t.receiver(fn, via, seen)
	}
	return t.local(fn, variable, via, seen)
}

// parameter follows the parameter at index of fn to what each call of it
// hands it, or the one call via names; a function the walk does not see
// every call of is handed what it cannot follow too.
func (t *inputTrace) parameter(fn tracedFunc, index int, via *ast.CallExpr, seen map[ast.Expr]bool) origins {
	params := fn.signature.Params()
	var found origins
	if via == nil && t.unseenCallers(fn) {
		found.add(stoppedAt(fn, params.At(index).Name()))
	}
	for _, caller := range t.callers[fn.object] {
		if via != nil && caller.call != via {
			continue
		}
		args := caller.call.Args
		if len(args) == 1 {
			// A call that hands over the results of another call whole.
			if _, isTuple := caller.fn.info.TypeOf(args[0]).(*types.Tuple); isTuple {
				found.add(t.results(caller.fn, args[0], index, nil, seen))
				continue
			}
		}
		end := index + 1
		if fn.signature.Variadic() && index == params.Len()-1 {
			end = len(args)
		}
		for _, arg := range args[index:end] {
			found.add(t.follow(caller.fn, arg, nil, seen))
		}
	}
	return found
}

// receiver follows the receiver of fn, a method, to what each call of it,
// or the one call via names, calls it on.
func (t *inputTrace) receiver(fn tracedFunc, via *ast.CallExpr, seen map[ast.Expr]bool) origins {
	var found origins
	if via == nil && t.unseenCallers(fn) {
		found.add(stoppedAt(fn, fn.signature.Recv().Name()))
	}
	for _, caller := range t.callers[fn.object] {
		if via != nil && caller.call != via {
			continue
		}
		// A method is called by name only through a selector on a value.
		sel, _ := ast.Unparen(caller.call.Fun).(*ast.SelectorExpr)
		found.add(t.follow(caller.fn, sel.X, nil, seen))
	}
	return found
}

// local follows a local variable of fn to everything assigned to it: each
// assignment and declaration, the results of a call it is declared from,
// and the collection a range over it reads. A key ranging over a slice, an
// array or a string is a position, and a declaration without a value the
// zero value; a local whose address is taken can be written through it, and
// one nothing in fn assigns (a parameter of a function literal) is not
// followed.
func (t *inputTrace) local(fn tracedFunc, variable *types.Var, via *ast.CallExpr, seen map[ast.Expr]bool) origins {
	var found origins
	defined := false
	ast.Inspect(fn.body, func(node ast.Node) bool {
		more, defines := t.definition(fn, node, variable, via, seen)
		found.add(more)
		if defines {
			defined = true
		}
		return true
	})
	if !defined {
		found.add(stoppedAt(fn, variable.Name()))
	}
	return found
}

// definition follows one node of fn as a write of variable, and reports
// whether it defines it: an assignment, a declaration or a range that
// writes it, and the address taken, which defines nothing the trace sees.
func (t *inputTrace) definition(fn tracedFunc, node ast.Node, variable *types.Var, via *ast.CallExpr, seen map[ast.Expr]bool) (origins, bool) {
	switch typed := node.(type) {
	case *ast.AssignStmt:
		return t.assignment(fn, typed, variable, via, seen)
	case *ast.ValueSpec:
		return t.declaration(fn, typed, variable, via, seen)
	case *ast.RangeStmt:
		return t.ranged(fn, typed, variable, via, seen)
	case *ast.UnaryExpr:
		if operand, _ := typed.X.(*ast.Ident); typed.Op == token.AND && fn.info.Uses[operand] == variable {
			return stoppedAt(fn, "&"+operand.Name), false
		}
	}
	return origins{}, false
}

// assignment follows what an assignment writes to variable: the value in
// its place, or the result in its place of the one call on the right.
func (t *inputTrace) assignment(fn tracedFunc, assign *ast.AssignStmt, variable *types.Var, via *ast.CallExpr, seen map[ast.Expr]bool) (found origins, defines bool) {
	for i, lhs := range assign.Lhs {
		if target, _ := lhs.(*ast.Ident); !declares(fn.info, target, variable) {
			continue
		}
		defines = true
		if len(assign.Lhs) == len(assign.Rhs) {
			found.add(t.follow(fn, assign.Rhs[i], via, seen))
		} else {
			found.add(t.results(fn, assign.Rhs[0], i, via, seen))
		}
	}
	return found, defines
}

// declaration follows what a declaration gives variable: its value, the
// result in its place of the one call it is declared from, or the zero
// value.
func (t *inputTrace) declaration(fn tracedFunc, spec *ast.ValueSpec, variable *types.Var, via *ast.CallExpr, seen map[ast.Expr]bool) (found origins, defines bool) {
	for i, name := range spec.Names {
		if !declares(fn.info, name, variable) {
			continue
		}
		defines = true
		switch len(spec.Values) {
		case 0:
			// The zero value.
		case len(spec.Names):
			found.add(t.follow(fn, spec.Values[i], via, seen))
		default:
			found.add(t.results(fn, spec.Values[0], i, via, seen))
		}
	}
	return found, defines
}

// ranged follows what a range gives variable as its key or its value: the
// collection it reads, a key that is a position aside.
func (t *inputTrace) ranged(fn tracedFunc, loop *ast.RangeStmt, variable *types.Var, via *ast.CallExpr, seen map[ast.Expr]bool) (found origins, defines bool) {
	key, _ := loop.Key.(*ast.Ident)
	value, _ := loop.Value.(*ast.Ident)
	if declares(fn.info, key, variable) {
		defines = true
		if !keyIsPosition(fn.info.TypeOf(loop.X)) {
			found.add(t.follow(fn, loop.X, via, seen))
		}
	}
	if declares(fn.info, value, variable) {
		defines = true
		found.add(t.follow(fn, loop.X, via, seen))
	}
	return found, defines
}

// declares reports whether an assignment's or a declaration's target is
// variable; a nil target is none.
func declares(info *types.Info, target *ast.Ident, variable *types.Var) bool {
	if defined, isDefined := info.Defs[target]; isDefined {
		return defined == variable
	}
	return info.Uses[target] == variable
}

// keyIsPosition reports whether the key of a range over a value of type t
// is a position in it: over a slice, an array, a pointer to one or a
// string, not over a map, a channel, an integer or a function.
func keyIsPosition(t types.Type) bool {
	switch typed := t.Underlying().(type) {
	case *types.Slice, *types.Array, *types.Pointer:
		return true
	case *types.Basic:
		return typed.Info()&types.IsString != 0
	}
	return false
}
