// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package structs

import (
	"go/ast"
	"go/types"
	"maps"
	"slices"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
)

// helperBuilds maps a helper, by [funcKey], to the request structs it builds:
// by a literal in its body, or by calling another helper with one of its own
// parameters.
type helperBuilds map[string][]*types.Named

// helperCache memoizes the helpers of a package: a package's helpers are read
// by every tool package importing it, and its body is walked once.
var helperCache sync.Map // *packages.Package -> helperBuilds

// collectCallerInputs pairs a request struct a helper builds with the input of
// every function of pkg that calls the helper with that input's fields.
//
// A helper is a function with no exported input struct of its own:
// search.searchOpts(page, perPage int, ref, searchType string),
// iterationdata.NewGroupListOptions, a method on a struct of captured
// arguments, or search.runScopedSearch, whose scopedSearchArgs is a struct of
// arguments every search handler fills for one shared body. A function that
// takes an exported input and references no service method is one too
// (projects.buildListOpts(input ListInput)): it builds options and sends
// nothing, so what it builds is its caller's request. The literal in a
// helper pairs with no input a model sends, so the options struct it builds
// was compared with no such input at all, and a field client-go added to it
// went unseen. Its callers are where the input is: a call that hands the
// helper a field of the caller's input, as an argument, inside a literal or
// as the receiver, pairs every struct the helper builds with that input,
// whichever package the helper is declared in.
//
// The input also travels inside an intermediate, a tool struct the caller
// fills from it (runners.runnerListRequest, issuestatistics.statisticsFilters,
// accesstokens.accessTokenCreateRequest) and hands on whole. A literal of an
// intermediate that reads the input, a variable of the caller's body assigned
// one, and the parameter of a closure handed on beside one carry the input:
// a helper handed one, or named as a value beside one with an intermediate
// among its parameters (build: groupIssueStatsOptions), builds for the
// caller. An intermediate the same body fills with constants alone carries
// nothing, whatever its type. A call of a function the package declares is
// read through its body: of several results assigned at once, only those
// the function hands back something of its arguments in carry the input,
// and a closure it is handed holds the input only in the parameters its
// calls of the closure hand something of it.
//
// Three conditions keep the pairing to the request the caller sends:
//
//   - the call hands the helper something of the input, so a lookup with
//     constants pairs nothing;
//   - when what it hands is read off the input directly, the caller itself
//     references a client-go service method that takes the struct, so a
//     helper that resolves an identifier by listing (a milestone looked up by
//     its iid) is not compared with an input it does not serve: the lookup's
//     request is the helper's own, never the caller's. A helper handed an
//     intermediate is handed the input whole under another type, which a
//     lookup never is, and what it builds is paired however it is sent (a
//     helper that builds and sends the options itself);
//   - the input is exported and named, which every input a model sends is:
//     an unexported struct of arguments is an intermediate, a helper in its
//     own right, and its callers carry the input.
func collectCallerInputs(pkg *packages.Package, out map[[2]string]structPair) {
	reachable := reachableHelpers(pkg)
	decls := funcDecls(pkg)
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			mcpNamed, mcpStruct, input, ok := handlerInputParam(pkg, fn)
			if !ok || input == nil || !mcpNamed.Obj().Exported() {
				continue
			}
			caller := callerInput{named: mcpNamed, st: mcpStruct, input: input, sent: sentStructs(pkg.TypesInfo, fn.Body)}
			caller.collect(pkg.TypesInfo, fn.Body, reachable, decls, out)
		}
	}
}

// funcDecls maps each function and method pkg declares with a body to its
// declaration.
func funcDecls(pkg *packages.Package) map[*types.Func]*ast.FuncDecl {
	decls := map[*types.Func]*ast.FuncDecl{}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			if obj, defined := pkg.TypesInfo.Defs[fn.Name].(*types.Func); defined {
				decls[obj] = fn
			}
		}
	}
	return decls
}

// callerInput is a function that takes an input a model sends: the input's
// struct, the variable it is bound to and the client-go structs the
// function's body references a service method taking.
type callerInput struct {
	named *types.Named
	st    *types.Struct
	input *types.Var
	sent  map[*types.TypeName]bool
}

// collect records what every helper body hands the input to builds: one
// handed something read off the input when the body sends the struct, and
// one handed an intermediate filled from the input, or named as a value
// beside one, whatever it sends.
func (c callerInput) collect(info *types.Info, body *ast.BlockStmt, reachable helperBuilds, decls map[*types.Func]*ast.FuncDecl, out map[[2]string]structPair) {
	direct := map[*types.Var]bool{c.input: true}
	held := holdIntermediates(info, body, c.input, intermediateTypes(info, body, c.input), decls)
	readsInput := func(arg ast.Expr) bool { return reads(info, arg, direct) }
	ast.Inspect(body, func(node ast.Node) bool {
		if call, isCall := node.(*ast.CallExpr); isCall {
			c.record(helperCall(info, call, reachable, readsInput), false, out)
			c.record(helperCall(info, call, reachable, held.carries), true, out)
		}
		return true
	})
	for _, ident := range held.besideValues {
		c.record(helperValue(info, ident, reachable, held.carried), true, out)
	}
}

// record pairs the input with each struct built, one read off the input
// directly only when the body sends it.
func (c callerInput) record(built []*types.Named, viaIntermediate bool, out map[[2]string]structPair) {
	for _, named := range built {
		if !viaIntermediate && !c.sent[named.Obj()] {
			continue
		}
		_, st, _ := derefNamedStruct(named)
		recordInputPair(out, inputPair(c.named, c.st, named, st, ViaCaller))
	}
}

// intermediateTypes returns the tool structs body fills from input: the type
// of every composite literal in body that reads input. A client-go struct
// filled from the input is the request itself, never a carrier of it.
func intermediateTypes(info *types.Info, body *ast.BlockStmt, input *types.Var) map[*types.TypeName]bool {
	from := map[*types.Var]bool{input: true}
	carried := map[*types.TypeName]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		lit, isLit := node.(*ast.CompositeLit)
		if !isLit {
			return true
		}
		named, _, isStruct := derefNamedStruct(info.TypeOf(lit))
		if isStruct && isToolStruct(named) && reads(info, lit, from) {
			carried[named.Obj()] = true
		}
		return true
	})
	return carried
}

// intermediates is what a caller's body holds of its input under the types
// it fills from it: the variables holding something of the input, and the
// helpers named as values beside an intermediate filled from it.
type intermediates struct {
	info    *types.Info
	input   *types.Var
	carried map[*types.TypeName]bool
	// from is the input and every variable of the body that holds something
	// of it: a local assigned a value read off it or off another such
	// variable, the key or value a range over one binds, and a closure's
	// parameter of an intermediate type when the closure is handed on beside
	// an intermediate filled from the input, which is what whoever calls the
	// closure hands it unless the function it is handed to is read and
	// calls it with something else.
	from map[*types.Var]bool
	// besideValues are the identifiers, once each in source order, that
	// name a function as a value beside an intermediate filled from the
	// input, among a call's arguments or a literal's elements.
	besideValues []*ast.Ident
	beside       map[*ast.Ident]bool
	// decls are the functions of the package with a body. A call of one is
	// read through its body: which of its results hold what it is handed,
	// and what it hands a closure it is handed.
	decls map[*types.Func]*ast.FuncDecl
	// reading are the functions whose bodies are being read, so a function
	// reached again through its own calls is read as one the package does
	// not declare.
	reading map[*types.Func]bool
}

// holdIntermediates reads body for what it holds of input under the types of
// carried, a call of one of decls read through its body.
func holdIntermediates(info *types.Info, body *ast.BlockStmt, input *types.Var, carried map[*types.TypeName]bool, decls map[*types.Func]*ast.FuncDecl) *intermediates {
	held := &intermediates{
		info: info, input: input, carried: carried, from: map[*types.Var]bool{input: true},
		beside: map[*ast.Ident]bool{}, decls: decls, reading: map[*types.Func]bool{},
	}
	held.hold(body)
	return held
}

// hold runs the passes over body. They repeat until one adds nothing, which
// they reach because what is held only grows and body's variables are
// finite.
func (h *intermediates) hold(body *ast.BlockStmt) {
	for grew := true; grew; {
		grew = false
		ast.Inspect(body, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.AssignStmt:
				grew = h.assign(typed.Lhs, typed.Rhs) || grew
			case *ast.ValueSpec:
				names := make([]ast.Expr, len(typed.Names))
				for i, name := range typed.Names {
					names[i] = name
				}
				grew = h.assign(names, typed.Values) || grew
			case *ast.RangeStmt:
				grew = h.assign([]ast.Expr{typed.Key, typed.Value}, []ast.Expr{typed.X}) || grew
			case *ast.CallExpr:
				grew = h.handOn(typed, typed.Args) || grew
			case *ast.CompositeLit:
				grew = h.handOn(nil, elementValues(typed)) || grew
			}
			return true
		})
	}
}

// assign marks the variable under each of lhs (the one a field is written
// into included) as holding the input when the value assigned to it reads
// the input or a variable holding it. A single value assigned to several
// marks each of them (a range's key and value), but the results of a call
// of a function the package declares, each of which marks only the target
// in the place of a result the function hands something of what the call
// hands it.
func (h *intermediates) assign(lhs, rhs []ast.Expr) bool {
	grew := false
	var results []bool
	for i, target := range lhs {
		var value ast.Expr
		switch {
		case len(rhs) == 1:
			value = rhs[0]
		case i < len(rhs):
			value = rhs[i]
		default:
			continue
		}
		variable := rootVar(h.info, target)
		if variable == nil || variable == h.input || h.from[variable] || !reads(h.info, value, h.from) {
			continue
		}
		if len(lhs) > 1 && len(rhs) == 1 && results == nil {
			results = h.resultsHolding(value, len(lhs))
		}
		if i < len(results) && !results[i] {
			continue
		}
		h.from[variable] = true
		grew = true
	}
	return grew
}

// resultsHolding says, for value a call of a function the package declares
// with n results, which of them hold something of what the call hands it,
// and returns nil for anything else (a range, a type assertion, a call of a
// function of another package), whose every target holds what it reads.
func (h *intermediates) resultsHolding(value ast.Expr, n int) []bool {
	call, isCall := ast.Unparen(value).(*ast.CallExpr)
	if !isCall {
		return nil
	}
	inner, decl := h.enter(call)
	if inner == nil || decl.Type.Results.NumFields() != n {
		return nil
	}
	results := make([]bool, n)
	named := fieldVars(h.info, decl.Type.Results)
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			inner.returned(typed, named, results)
		}
		return true
	})
	return results
}

// returned marks in results each result one return statement hands back
// something h holds in: one of its values, every one of a call whose
// results it hands back whole unless that call is itself read through its
// body, and a named result a bare return hands back.
func (h *intermediates) returned(ret *ast.ReturnStmt, named []*types.Var, results []bool) {
	switch len(ret.Results) {
	case len(results):
		for i, value := range ret.Results {
			results[i] = results[i] || reads(h.info, value, h.from)
		}
	case 1:
		if !reads(h.info, ret.Results[0], h.from) {
			return
		}
		nested := h.resultsHolding(ret.Results[0], len(results))
		for i := range results {
			results[i] = results[i] || nested == nil || nested[i]
		}
	case 0:
		for i, variable := range named {
			results[i] = results[i] || h.from[variable]
		}
	}
}

// enter reads the body of the function call calls, when the package
// declares it and it is not being read already: its parameters, and its
// receiver, hold what h holds when the call hands them something of it, and
// the passes run over its body as over the caller's. It returns nil for any
// other call.
func (h *intermediates) enter(call *ast.CallExpr) (*intermediates, *ast.FuncDecl) {
	fn, receiver := calledFunc(h.info, call)
	if fn == nil || h.reading[fn.Origin()] {
		return nil, nil
	}
	decl := h.decls[fn.Origin()]
	if decl == nil {
		return nil, nil
	}
	inner := &intermediates{
		info: h.info, carried: h.carried, from: map[*types.Var]bool{},
		beside: map[*ast.Ident]bool{}, decls: h.decls, reading: maps.Clone(h.reading),
	}
	inner.reading[fn.Origin()] = true
	if receivers := fieldVars(h.info, decl.Recv); len(receivers) == 1 && receivers[0] != nil && receiver != nil && reads(h.info, receiver, h.from) {
		inner.from[receivers[0]] = true
	}
	params := fieldVars(h.info, decl.Type.Params)
	for i, arg := range call.Args {
		param := paramAt(params, i)
		if param != nil && reads(h.info, arg, h.from) {
			inner.from[param] = true
		}
	}
	inner.hold(decl.Body)
	return inner, decl
}

// handOn reads one call's arguments or one literal's elements: when one of
// them carries an intermediate filled from the input, each closure among
// the others holds it in its parameters of an intermediate type, and each
// function named among them as a value is named beside it. A closure handed
// to a function the package declares that calls it holds it only in the
// parameters those calls hand something of it.
func (h *intermediates) handOn(call *ast.CallExpr, elements []ast.Expr) bool {
	if !slices.ContainsFunc(elements, h.carries) {
		return false
	}
	grew := false
	for position, element := range elements {
		switch typed := ast.Unparen(element).(type) {
		case *ast.FuncLit:
			handed := h.closureArgs(call, position)
			for i, param := range fieldVars(h.info, typed.Type.Params) {
				if param == nil || h.from[param] || !h.isCarried(param.Type()) || !handedAt(handed, i) {
					continue
				}
				h.from[param] = true
				grew = true
			}
		case *ast.Ident:
			h.nameBeside(typed)
		case *ast.SelectorExpr:
			h.nameBeside(typed.Sel)
		}
	}
	return grew
}

// closureArgs says, for a closure handed as the argument at position of
// call, which of its parameters the function call calls hands something of
// what call hands it, in the calls it makes of the parameter the closure is
// bound to, none when that parameter has no name. It returns nil when that
// is out of sight: a literal's element, a function the package does not
// declare, and one that does anything with the parameter but call it,
// handing it on to whoever calls it in turn.
func (h *intermediates) closureArgs(call *ast.CallExpr, position int) []bool {
	if call == nil {
		return nil
	}
	inner, decl := h.enter(call)
	if inner == nil {
		return nil
	}
	handed := []bool{}
	param := paramAt(fieldVars(h.info, decl.Type.Params), position)
	if param == nil {
		// A parameter with no name is never called.
		return handed
	}
	called := map[*ast.Ident]bool{}
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		invoke, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		fun, isIdent := ast.Unparen(invoke.Fun).(*ast.Ident)
		if !isIdent || h.info.Uses[fun] != param {
			return true
		}
		called[fun] = true
		for i, arg := range invoke.Args {
			for len(handed) <= i {
				handed = append(handed, false)
			}
			handed[i] = handed[i] || reads(h.info, arg, inner.from)
		}
		return true
	})
	handedOn := false
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		ident, isIdent := node.(*ast.Ident)
		handedOn = handedOn || (isIdent && h.info.Uses[ident] == param && !called[ident])
		return !handedOn
	})
	if handedOn {
		return nil
	}
	return handed
}

// handedAt reports whether the closure parameter at i is handed something
// of the input: every one is when handed is nil, which says the calls of
// the closure are out of sight.
func handedAt(handed []bool, i int) bool {
	if handed == nil {
		return true
	}
	return i < len(handed) && handed[i]
}

// fieldVars returns the variable each entry of a parameter, result or
// receiver list is bound to, in order, nil for one with no name or an
// underscore; nil for no list.
func fieldVars(info *types.Info, list *ast.FieldList) []*types.Var {
	if list == nil {
		return nil
	}
	var out []*types.Var
	for _, field := range list.List {
		if len(field.Names) == 0 {
			out = append(out, nil)
			continue
		}
		for _, name := range field.Names {
			bound, _ := info.Defs[name].(*types.Var)
			out = append(out, bound)
		}
	}
	return out
}

// paramAt returns the parameter an argument at position is bound to: the
// last one for each argument past it, which a variadic function takes. A
// type-checked call hands an argument only to a function with parameters,
// so params is never empty here.
func paramAt(params []*types.Var, position int) *types.Var {
	return params[min(position, len(params)-1)]
}

// nameBeside records ident as a function named as a value beside an
// intermediate, when it names a function.
func (h *intermediates) nameBeside(ident *ast.Ident) {
	if _, isFunc := h.info.Uses[ident].(*types.Func); isFunc && !h.beside[ident] {
		h.beside[ident] = true
		h.besideValues = append(h.besideValues, ident)
	}
}

// carries reports whether expr hands on an intermediate filled from the
// input: a literal of one of carried that reads the input or a variable
// holding it, a variable of one of those types that holds it (a local, a
// closure's parameter), or a field of one of those types read off such a
// variable. The input itself is never one, even when the body fills a copy
// of its type, so a call handed the input is held to the conditions of a
// direct read; nor is an intermediate the body fills with constants alone,
// nor a closure, whose own calls are read where they are made.
func (h *intermediates) carries(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CompositeLit:
			found = found || (h.isCarried(h.info.TypeOf(typed)) && reads(h.info, typed, h.from))
		case *ast.SelectorExpr:
			_, isField := h.info.Uses[typed.Sel].(*types.Var)
			found = found || (isField && h.isCarried(h.info.TypeOf(typed)) && reads(h.info, typed.X, h.from))
		case *ast.Ident:
			variable, isVar := h.info.Uses[typed].(*types.Var)
			found = found || (isVar && variable != h.input && h.from[variable] && h.isCarried(variable.Type()))
		}
		return !found
	})
	return found
}

// isCarried reports whether t is, or points to, one of carried.
func (h *intermediates) isCarried(t types.Type) bool {
	named, _, isStruct := derefNamedStruct(t)
	return isStruct && h.carried[named.Obj()]
}

// elementValues returns the values a composite literal lists, the value of
// each keyed element.
func elementValues(lit *ast.CompositeLit) []ast.Expr {
	values := make([]ast.Expr, len(lit.Elts))
	for i, elt := range lit.Elts {
		if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
			elt = kv.Value
		}
		values[i] = elt
	}
	return values
}

// helperValue returns what the helper ident names builds, when the helper
// takes one of carried among its parameters: a helper handed on as a value
// beside an intermediate, which whoever calls it hands the intermediate.
func helperValue(info *types.Info, ident *ast.Ident, helpers helperBuilds, carried map[*types.TypeName]bool) []*types.Named {
	fn, isFunc := info.Uses[ident].(*types.Func)
	if !isFunc {
		return nil
	}
	built, isHelper := helpers[funcKey(fn)]
	if !isHelper {
		return nil
	}
	for param := range fn.Signature().Params().Variables() {
		if named, _, isStruct := derefNamedStruct(param.Type()); isStruct && carried[named.Obj()] {
			return built
		}
	}
	return nil
}

// sentStructs returns the client-go structs the service methods body
// references take, called in place or handed on as a method value.
func sentStructs(info *types.Info, body *ast.BlockStmt) map[*types.TypeName]bool {
	sent := map[*types.TypeName]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		sel, isSelector := node.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		if _, isService := shared.ClientGoServiceInterface(info.TypeOf(sel.X)); !isService {
			return true
		}
		if method, isFunc := info.Uses[sel.Sel].(*types.Func); isFunc {
			for _, received := range ReceivedStructs(method.Signature()) {
				sent[received.Obj()] = true
			}
		}
		return true
	})
	return sent
}

// referencesService reports whether body names a method of a client-go
// service, called in place or handed on as a value.
func referencesService(info *types.Info, body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if sel, isSelector := node.(*ast.SelectorExpr); isSelector {
			_, isService := shared.ClientGoServiceInterface(info.TypeOf(sel.X))
			found = found || isService
		}
		return !found
	})
	return found
}

// reachableHelpers is every helper a function of pkg can call: its own and
// those of the tool packages it imports.
func reachableHelpers(pkg *packages.Package) helperBuilds {
	reachable := maps.Clone(helpersOf(pkg))
	for _, imported := range importedToolPackages(pkg) {
		maps.Copy(reachable, helpersOf(imported))
	}
	return reachable
}

// importedToolPackages returns the tool packages pkg imports that were loaded
// with their syntax, which is every one of them in a run over the tree.
func importedToolPackages(pkg *packages.Package) []*packages.Package {
	var out []*packages.Package
	for path, imported := range pkg.Imports {
		if !strings.Contains(path, shared.ToolsPkgInfix) || imported.TypesInfo == nil || len(imported.Syntax) == 0 {
			continue
		}
		out = append(out, imported)
	}
	return out
}

// helpersOf returns, memoized, the helpers pkg declares.
func helpersOf(pkg *packages.Package) helperBuilds {
	if cached, ok := helperCache.Load(pkg); ok {
		found, _ := cached.(helperBuilds)
		return found
	}
	found := collectHelpers(pkg)
	helperCache.Store(pkg, found)
	return found
}

// helperDecl is one function without an input struct of its own: its
// declaration and the variables its parameters and receiver are bound to.
type helperDecl struct {
	key    string
	fn     *ast.FuncDecl
	params map[*types.Var]bool
}

// collectHelpers finds pkg's helpers and what each builds, a helper that
// hands one of its parameters to another helper building what that one
// builds, however deep the chain.
func collectHelpers(pkg *packages.Package) helperBuilds {
	decls, builds := declaredHelpers(pkg)
	imported := helperBuilds{}
	for _, dep := range importedToolPackages(pkg) {
		maps.Copy(imported, helpersOf(dep))
	}
	closeHelpers(pkg.TypesInfo, decls, builds, imported)
	return builds
}

// declaredHelpers lists pkg's helpers, each with the request structs its own
// literals build. A function that takes an exported input is a handler when
// it references a service method, since it sends its own request, and a
// helper when it references none, since it only builds one for its caller.
func declaredHelpers(pkg *packages.Package) ([]helperDecl, helperBuilds) {
	var decls []helperDecl
	builds := helperBuilds{}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			if input, _, hasInput := handlerInputStruct(pkg, fn); hasInput && input.Obj().Exported() && referencesService(pkg.TypesInfo, fn.Body) {
				continue
			}
			obj, isFunc := pkg.TypesInfo.Defs[fn.Name].(*types.Func)
			if !isFunc {
				continue
			}
			helper := helperDecl{key: funcKey(obj), fn: fn, params: boundParams(pkg.TypesInfo, fn)}
			decls = append(decls, helper)
			builds[helper.key] = requestLiterals(pkg.TypesInfo, fn.Body)
		}
	}
	return decls, builds
}

// closeHelpers adds to builds what each helper builds by handing one of its
// parameters to another helper, of its package or of imported. The passes
// repeat until one adds nothing, which they reach because what a helper
// builds only grows and the structs it can build are finite.
func closeHelpers(info *types.Info, decls []helperDecl, builds, imported helperBuilds) {
	for grew := true; grew; {
		grew = false
		reachable := maps.Clone(imported)
		maps.Copy(reachable, builds)
		for _, helper := range decls {
			ast.Inspect(helper.fn.Body, func(node ast.Node) bool {
				call, isCall := node.(*ast.CallExpr)
				if !isCall {
					return true
				}
				readsParam := func(arg ast.Expr) bool { return reads(info, arg, helper.params) }
				for _, built := range helperCall(info, call, reachable, readsParam) {
					before := len(builds[helper.key])
					builds[helper.key] = appendNamed(builds[helper.key], built)
					grew = grew || len(builds[helper.key]) > before
				}
				return true
			})
		}
	}
}

// requestLiterals lists, once each in source order, the request structs a
// composite literal in body builds.
func requestLiterals(info *types.Info, body *ast.BlockStmt) []*types.Named {
	var out []*types.Named
	ast.Inspect(body, func(node ast.Node) bool {
		lit, isLit := node.(*ast.CompositeLit)
		if !isLit {
			return true
		}
		for _, named := range builtRequests(info.TypeOf(lit)) {
			out = appendNamed(out, named)
		}
		return true
	})
	return out
}

// appendNamed appends named to list unless it is already there.
func appendNamed(list []*types.Named, named *types.Named) []*types.Named {
	for _, existing := range list {
		if existing.Obj() == named.Obj() {
			return list
		}
	}
	return append(list, named)
}

// boundParams returns the variables a function's receiver and parameters are
// bound to, the unnamed ones aside.
func boundParams(info *types.Info, fn *ast.FuncDecl) map[*types.Var]bool {
	out := map[*types.Var]bool{}
	for _, list := range []*ast.FieldList{fn.Recv, fn.Type.Params} {
		if list == nil {
			continue
		}
		for _, field := range list.List {
			for _, name := range field.Names {
				if bound, isVar := info.Defs[name].(*types.Var); isVar {
					out[bound] = true
				}
			}
		}
	}
	return out
}

// helperCall returns what the helper call calls builds, when one of its
// arguments, or its receiver, is one handed says the caller hands on.
func helperCall(info *types.Info, call *ast.CallExpr, helpers helperBuilds, handed func(ast.Expr) bool) []*types.Named {
	key, receiver := callee(info, call)
	built, isHelper := helpers[key]
	if !isHelper {
		return nil
	}
	for _, arg := range append([]ast.Expr{receiver}, call.Args...) {
		if arg != nil && handed(arg) {
			return built
		}
	}
	return nil
}

// callee returns the [funcKey] of the function a call calls statically, and
// the receiver expression of a method call; an empty key for anything else (a
// function value, a field of function type, a method expression, a
// conversion, a builtin).
func callee(info *types.Info, call *ast.CallExpr) (string, ast.Expr) {
	fn, receiver := calledFunc(info, call)
	if fn == nil {
		return "", nil
	}
	return funcKey(fn), receiver
}

// calledFunc returns the function a call calls statically, and the receiver
// expression of a method call; nil for anything else (a function value, a
// field of function type, a method expression, a conversion, a builtin).
func calledFunc(info *types.Info, call *ast.CallExpr) (*types.Func, ast.Expr) {
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		if fn, isFunc := info.Uses[fun].(*types.Func); isFunc {
			return fn, nil
		}
	case *ast.SelectorExpr:
		if selection, isSelection := info.Selections[fun]; isSelection {
			if fn, isFunc := selection.Obj().(*types.Func); isFunc && selection.Kind() == types.MethodVal {
				return fn, fun.X
			}
			return nil, nil
		}
		if fn, isFunc := info.Uses[fun.Sel].(*types.Func); isFunc {
			return fn, nil
		}
	}
	return nil, nil
}

// funcKey names a function the same way from its declaration and from a call
// to it in another package: the generic origin's full name, receiver
// included.
func funcKey(fn *types.Func) string { return fn.Origin().FullName() }

// reads reports whether expr mentions one of the variables in from. An
// identifier that names no variable reads as nil, which from never holds.
func reads(info *types.Info, expr ast.Expr, from map[*types.Var]bool) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if ident, isIdent := node.(*ast.Ident); isIdent {
			used, _ := info.Uses[ident].(*types.Var)
			found = found || from[used]
		}
		return !found
	})
	return found
}
