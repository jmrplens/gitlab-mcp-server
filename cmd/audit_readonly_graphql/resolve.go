package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// maxResolveDepth bounds the interprocedural walk. The real chains are three
// or four calls deep (spec helper, NewReadActionSpec, NewActionSpec, the
// composite literal), so this is slack rather than a limit anyone reaches; it
// exists so a cyclic helper cannot hang the audit.
const maxResolveDepth = 32

// specTypeName and routeTypeName are the toolutil types the resolver follows.
const (
	specTypeName  = "ActionSpec"
	routeTypeName = "ActionRoute"
)

// handlerRef is one function an action routes to: either a declared function
// or a literal written at the route site.
type handlerRef struct {
	fn  *types.Func
	lit *ast.FuncLit
	pkg *packages.Package
}

// site is one resolved ActionSpec construction: the action name it declares
// and the handlers its route runs.
type site struct {
	pkgName  string
	name     string
	handlers []handlerRef
	pos      token.Pos
}

// frame is where an expression is being resolved: the package it was written
// in, the function body enclosing it (for local variables), and the parameter
// bindings that got us there.
type frame struct {
	pkg  *packages.Package
	decl *ast.FuncDecl
	env  map[*types.Var]binding
}

// binding is one parameter bound to the argument expression a caller passed,
// together with the frame that argument has to be resolved in.
type binding struct {
	expr  ast.Expr
	frame frame
}

// resolver resolves ActionSpec construction sites to (action name, handlers).
//
// It is a small interprocedural constant propagation rather than a pattern
// match on one spelling, because the specs are not written in one spelling:
// most domains call a package-local helper that adds shared options and
// forwards to toolutil, so the action name and the handler arrive at the
// toolutil constructor as parameters. Following the parameters is what makes
// the audit hold for a domain written in a shape nobody anticipated, and an
// action it cannot follow is reported rather than skipped.
type resolver struct {
	prog *program
}

// collectSites resolves every ActionSpec construction the loaded packages
// contain, keyed by the action name it declares.
func (r *resolver) collectSites() map[string][]site {
	sites := make(map[string][]site)
	for _, pkg := range r.prog.order {
		if pkg.TypesInfo == nil {
			continue
		}
		for _, file := range pkg.Syntax {
			r.collectFileSites(pkg, file, sites)
		}
	}
	return sites
}

// collectFileSites resolves the spec expressions in one file: the elements of
// every []ActionSpec literal and the elements appended to one.
func (r *resolver) collectFileSites(pkg *packages.Package, file *ast.File, sites map[string][]site) {
	var enclosing *ast.FuncDecl
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.FuncDecl:
			enclosing = typed
		case *ast.CompositeLit:
			if isSpecSliceType(pkg.TypesInfo.TypeOf(typed)) {
				r.resolveElements(pkg, enclosing, typed.Elts, sites)
			}
		case *ast.CallExpr:
			if isAppendCall(pkg, typed) {
				r.resolveElements(pkg, enclosing, typed.Args[1:], sites)
			}
		}
		return true
	})
}

// resolveElements resolves each expression that evaluates to one ActionSpec.
func (r *resolver) resolveElements(pkg *packages.Package, enclosing *ast.FuncDecl, elements []ast.Expr, sites map[string][]site) {
	for _, element := range elements {
		if !isSpecType(pkg.TypesInfo.TypeOf(element)) {
			continue
		}
		start := frame{pkg: pkg, decl: enclosing}
		name, handlers := r.resolveSpec(element, start, 0)
		if name == "" {
			continue
		}
		sites[name] = append(sites[name], site{
			pkgName:  pkg.Name,
			name:     name,
			handlers: handlers,
			pos:      element.Pos(),
		})
	}
}

// resolveSpec resolves one ActionSpec expression to the action name it
// declares and the handlers its route runs.
func (r *resolver) resolveSpec(expr ast.Expr, at frame, depth int) (string, []handlerRef) {
	if depth > maxResolveDepth {
		return "", nil
	}
	switch typed := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return r.resolveSpecIdent(typed, at, depth)
	case *ast.CompositeLit:
		return r.resolveSpecLiteral(typed, at, depth)
	case *ast.CallExpr:
		return r.resolveSpecCall(typed, at, depth)
	}
	return "", nil
}

// resolveSpecIdent follows a spec held in a parameter or a local variable.
func (r *resolver) resolveSpecIdent(ident *ast.Ident, at frame, depth int) (string, []handlerRef) {
	var name string
	var handlers []handlerRef
	for _, next := range r.follow(ident, at) {
		nextName, nextHandlers := r.resolveSpec(next.expr, next.frame, depth+1)
		name, handlers = merge(name, handlers, nextName, nextHandlers)
	}
	return name, handlers
}

// merge combines two partial resolutions of the same spec. The first name wins
// and every handler is kept: a variable assigned in two branches routes to
// both, and dropping either would leave a mutation unclassified.
func merge(name string, handlers []handlerRef, nextName string, nextHandlers []handlerRef) (string, []handlerRef) {
	if name == "" {
		name = nextName
	}
	return name, append(handlers, nextHandlers...)
}

// resolveSpecLiteral reads the Name and Route fields of an ActionSpec literal.
func (r *resolver) resolveSpecLiteral(lit *ast.CompositeLit, at frame, depth int) (string, []handlerRef) {
	var name string
	var handlers []handlerRef
	for _, element := range lit.Elts {
		kv, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "Name":
			name = r.resolveString(kv.Value, at, depth+1)
		case "Route":
			handlers = r.resolveRoute(kv.Value, at, depth+1)
		}
	}
	return name, handlers
}

// resolveSpecCall resolves a call that produces an ActionSpec.
//
// A toolutil constructor taking (name string, route ActionRoute, ...) is the
// base case: its first two arguments are the answer. Anything else with a body
// is entered with its parameters bound to the arguments, and its return
// expressions resolved there.
func (r *resolver) resolveSpecCall(call *ast.CallExpr, at frame, depth int) (string, []handlerRef) {
	callee := calleeFunc(at.pkg, call)
	if callee == nil {
		return "", nil
	}
	if nameArg, routeArg, ok := specConstructorArgs(callee, call); ok {
		return r.resolveString(nameArg, at, depth+1), r.resolveRoute(routeArg, at, depth+1)
	}
	var name string
	var handlers []handlerRef
	for _, ret := range r.returnsOf(callee, call, at, specTypeName) {
		retName, retHandlers := r.resolveSpec(ret.expr, ret.frame, depth+1)
		name, handlers = merge(name, handlers, retName, retHandlers)
	}
	// A method on the spec, WithEmbeddedResource for one, returns the spec it
	// was called on with a field set, so the construction is its receiver:
	// helperSpec(...).WithEmbeddedResource(...) declares whatever helperSpec
	// declared.
	if receiver := methodReceiver(call); receiver != nil {
		recvName, recvHandlers := r.resolveSpec(receiver, at, depth+1)
		name, handlers = merge(name, handlers, recvName, recvHandlers)
	}
	return name, handlers
}

// resolveRoute resolves an ActionRoute expression to the handlers it runs.
func (r *resolver) resolveRoute(expr ast.Expr, at frame, depth int) []handlerRef {
	if depth > maxResolveDepth {
		return nil
	}
	switch typed := ast.Unparen(expr).(type) {
	case *ast.Ident:
		var found []handlerRef
		for _, next := range r.follow(typed, at) {
			found = append(found, r.resolveRoute(next.expr, next.frame, depth+1)...)
		}
		return found
	case *ast.CompositeLit:
		return r.resolveRouteLiteral(typed, at, depth)
	case *ast.CallExpr:
		return r.resolveRouteCall(typed, at, depth)
	}
	return nil
}

// resolveRouteLiteral reads the Handler field of an ActionRoute literal.
func (r *resolver) resolveRouteLiteral(lit *ast.CompositeLit, at frame, depth int) []handlerRef {
	var found []handlerRef
	for _, element := range lit.Elts {
		kv, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, isIdent := kv.Key.(*ast.Ident); isIdent && key.Name == "Handler" {
			found = append(found, r.resolveHandler(kv.Value, at, depth+1)...)
		}
	}
	return found
}

// resolveRouteCall resolves a call that produces an ActionRoute.
//
// A toolutil route constructor is the base case: its function-typed arguments
// are the handlers. Anything else with a body is entered the same way a spec
// constructor is, and a decorating method (route.WithTags(...)) resolves to its
// receiver.
func (r *resolver) resolveRouteCall(call *ast.CallExpr, at frame, depth int) []handlerRef {
	callee := calleeFunc(at.pkg, call)
	if callee == nil {
		return nil
	}
	if isToolutilFunc(callee) {
		var found []handlerRef
		for _, arg := range call.Args {
			found = append(found, r.resolveHandler(arg, at, depth+1)...)
		}
		if len(found) > 0 {
			return found
		}
	}
	var found []handlerRef
	for _, ret := range r.returnsOf(callee, call, at, routeTypeName) {
		found = append(found, r.resolveRoute(ret.expr, ret.frame, depth+1)...)
	}
	if len(found) > 0 {
		return found
	}
	if receiver := methodReceiver(call); receiver != nil {
		return r.resolveRoute(receiver, at, depth+1)
	}
	return nil
}

// resolveHandler resolves one argument to the function it names, if it names
// one. Arguments that are not functions (the GitLab client, an options value)
// resolve to nothing.
func (r *resolver) resolveHandler(expr ast.Expr, at frame, depth int) []handlerRef {
	if depth > maxResolveDepth {
		return nil
	}
	unwrapped := ast.Unparen(expr)
	switch typed := unwrapped.(type) {
	case *ast.FuncLit:
		return []handlerRef{{lit: typed, pkg: at.pkg}}
	case *ast.IndexExpr:
		return r.resolveHandler(typed.X, at, depth+1)
	case *ast.IndexListExpr:
		return r.resolveHandler(typed.X, at, depth+1)
	}
	if _, isSignature := at.pkg.TypesInfo.TypeOf(unwrapped).Underlying().(*types.Signature); !isSignature {
		return nil
	}
	switch typed := unwrapped.(type) {
	case *ast.Ident:
		if fn, ok := at.pkg.TypesInfo.Uses[typed].(*types.Func); ok {
			return []handlerRef{{fn: fn, pkg: at.pkg}}
		}
		var found []handlerRef
		for _, next := range r.follow(typed, at) {
			found = append(found, r.resolveHandler(next.expr, next.frame, depth+1)...)
		}
		return found
	case *ast.SelectorExpr:
		if fn, ok := at.pkg.TypesInfo.Uses[typed.Sel].(*types.Func); ok {
			return []handlerRef{{fn: fn, pkg: at.pkg}}
		}
	}
	return nil
}

// resolveString resolves an expression to the constant string it evaluates to.
//
// Constant folding answers most of it: literals, named constants, and
// concatenations all carry a value in the type information. Parameters do not,
// so they are followed to the argument the caller passed, and the one
// pass-through the constructors apply (strings.TrimSpace) is followed through.
func (r *resolver) resolveString(expr ast.Expr, at frame, depth int) string {
	if depth > maxResolveDepth {
		return ""
	}
	unwrapped := ast.Unparen(expr)
	if tv, ok := at.pkg.TypesInfo.Types[unwrapped]; ok && tv.Value != nil {
		if value, isString := constantString(tv.Value); isString {
			return value
		}
	}
	switch typed := unwrapped.(type) {
	case *ast.Ident:
		for _, next := range r.follow(typed, at) {
			if value := r.resolveString(next.expr, next.frame, depth+1); value != "" {
				return value
			}
		}
	case *ast.CallExpr:
		callee := calleeFunc(at.pkg, typed)
		if callee == nil {
			return ""
		}
		if isStringPassThrough(callee) && len(typed.Args) == 1 {
			return strings.TrimSpace(r.resolveString(typed.Args[0], at, depth+1))
		}
		for _, ret := range r.returnsOf(callee, typed, at, "") {
			if value := r.resolveString(ret.expr, ret.frame, depth+1); value != "" {
				return value
			}
		}
	}
	return ""
}

// returnsOf enters a callee's body with its parameters bound to the caller's
// arguments and yields every returned expression of the wanted type. An empty
// typeName wants the single result of a one-result function, which is how the
// string pass-throughs are followed.
func (r *resolver) returnsOf(callee *types.Func, call *ast.CallExpr, at frame, typeName string) []binding {
	fn, ok := r.prog.funcs[callee]
	if !ok || fn.decl.Body == nil {
		return nil
	}
	inner := frame{pkg: fn.pkg, decl: fn.decl, env: bindParams(callee, call, at)}
	index, ok := resultIndex(callee, typeName)
	if !ok {
		return nil
	}
	var found []binding
	ast.Inspect(fn.decl.Body, func(node ast.Node) bool {
		if lit, isLit := node.(*ast.FuncLit); isLit {
			_ = lit
			return false
		}
		ret, isReturn := node.(*ast.ReturnStmt)
		if !isReturn || index >= len(ret.Results) {
			return true
		}
		found = append(found, binding{expr: ret.Results[index], frame: inner})
		return true
	})
	return found
}

// follow resolves an identifier to the expressions it can hold: the argument
// bound to it if it is a parameter, or the right-hand sides assigned to it if
// it is a local variable.
func (r *resolver) follow(ident *ast.Ident, at frame) []binding {
	variable, ok := at.pkg.TypesInfo.Uses[ident].(*types.Var)
	if !ok {
		return nil
	}
	if bound, isBound := at.env[variable]; isBound {
		return []binding{bound}
	}
	if at.decl == nil {
		return nil
	}
	var found []binding
	for _, assigned := range assignmentsTo(at.pkg, at.decl, variable) {
		found = append(found, binding{expr: assigned, frame: at})
	}
	return found
}

// assignmentsTo returns every right-hand side assigned to a local variable in
// one function body.
func assignmentsTo(pkg *packages.Package, decl *ast.FuncDecl, variable *types.Var) []ast.Expr {
	var found []ast.Expr
	record := func(names, values []ast.Expr) {
		if len(names) != len(values) {
			return
		}
		for i, name := range names {
			ident, ok := name.(*ast.Ident)
			if !ok {
				continue
			}
			if pkg.TypesInfo.Defs[ident] == variable || pkg.TypesInfo.Uses[ident] == variable {
				found = append(found, values[i])
			}
		}
	}
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.AssignStmt:
			record(typed.Lhs, typed.Rhs)
		case *ast.ValueSpec:
			names := make([]ast.Expr, 0, len(typed.Names))
			for _, name := range typed.Names {
				names = append(names, name)
			}
			record(names, typed.Values)
		}
		return true
	})
	return found
}

// bindParams binds a callee's parameters to the caller's argument expressions.
// A variadic parameter is left unbound: no spec or route travels through one.
func bindParams(callee *types.Func, call *ast.CallExpr, at frame) map[*types.Var]binding {
	sig, ok := callee.Type().(*types.Signature)
	if !ok {
		return nil
	}
	params := sig.Params()
	env := make(map[*types.Var]binding, params.Len())
	for i := 0; i < params.Len() && i < len(call.Args); i++ {
		if sig.Variadic() && i == params.Len()-1 {
			break
		}
		env[params.At(i)] = binding{expr: call.Args[i], frame: at}
	}
	return env
}

// resultIndex finds which result of a signature carries the wanted toolutil
// type. An empty name wants a single result of any type.
func resultIndex(callee *types.Func, typeName string) (int, bool) {
	sig, ok := callee.Type().(*types.Signature)
	if !ok {
		return 0, false
	}
	results := sig.Results()
	if typeName == "" {
		if results.Len() != 1 {
			return 0, false
		}
		return 0, true
	}
	for i := range results.Len() {
		if isToolutilType(results.At(i).Type(), typeName) {
			return i, true
		}
	}
	return 0, false
}

// specConstructorArgs recognizes the toolutil constructor shape
// (name string, route ActionRoute, ...) ActionSpec and returns the two arguments
// that answer the question. Matching the shape rather than a list of names
// means a constructor added later is covered without editing this audit.
func specConstructorArgs(callee *types.Func, call *ast.CallExpr) (nameArg, routeArg ast.Expr, ok bool) {
	if !isToolutilFunc(callee) {
		return nil, nil, false
	}
	sig, isSignature := callee.Type().(*types.Signature)
	if !isSignature || sig.Params().Len() < 2 || len(call.Args) < 2 {
		return nil, nil, false
	}
	if sig.Params().At(0).Type() != types.Typ[types.String] {
		return nil, nil, false
	}
	if !isToolutilType(sig.Params().At(1).Type(), routeTypeName) {
		return nil, nil, false
	}
	if _, found := resultIndex(callee, specTypeName); !found {
		return nil, nil, false
	}
	return call.Args[0], call.Args[1], true
}

// isStringPassThrough reports whether a call returns its string argument for
// the purposes of an action name. TrimSpace is the only one the constructors
// apply, and the resolver applies it too, because the catalog holds the
// trimmed name.
func isStringPassThrough(callee *types.Func) bool {
	return callee.Pkg() != nil && callee.Pkg().Path() == "strings" && callee.Name() == "TrimSpace"
}

// calleeFunc resolves the function a call expression invokes.
func calleeFunc(pkg *packages.Package, call *ast.CallExpr) *types.Func {
	expr := ast.Unparen(call.Fun)
	for {
		switch typed := expr.(type) {
		case *ast.IndexExpr:
			expr = ast.Unparen(typed.X)
			continue
		case *ast.IndexListExpr:
			expr = ast.Unparen(typed.X)
			continue
		case *ast.Ident:
			fn, _ := pkg.TypesInfo.Uses[typed].(*types.Func)
			return fn
		case *ast.SelectorExpr:
			fn, _ := pkg.TypesInfo.Uses[typed.Sel].(*types.Func)
			return fn
		}
		return nil
	}
}

// methodReceiver returns the receiver expression of a method call, or nil.
func methodReceiver(call *ast.CallExpr) ast.Expr {
	selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	return selector.X
}

// isAppendCall reports whether a call is append(slice, elem...) with no spread.
func isAppendCall(pkg *packages.Package, call *ast.CallExpr) bool {
	ident, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if !ok || ident.Name != "append" || call.Ellipsis != token.NoPos || len(call.Args) < 2 {
		return false
	}
	builtin, ok := pkg.TypesInfo.Uses[ident].(*types.Builtin)
	return ok && builtin.Name() == "append"
}

// isToolutilFunc reports whether a function is declared in toolutil.
func isToolutilFunc(callee *types.Func) bool {
	return callee.Pkg() != nil && callee.Pkg().Path() == toolutilPath
}

// isToolutilType reports whether a type is the named toolutil type.
func isToolutilType(typ types.Type, name string) bool {
	named, ok := typ.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == name && obj.Pkg() != nil && obj.Pkg().Path() == toolutilPath
}

// isSpecType reports whether a type is toolutil.ActionSpec.
func isSpecType(typ types.Type) bool {
	return typ != nil && isToolutilType(typ, specTypeName)
}

// isSpecSliceType reports whether a type is []toolutil.ActionSpec.
func isSpecSliceType(typ types.Type) bool {
	slice, ok := typ.(*types.Slice)
	return ok && isSpecType(slice.Elem())
}
