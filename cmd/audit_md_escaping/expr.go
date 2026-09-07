package main

import (
	"go/ast"
	"go/constant"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// maxDepth bounds the walk backwards from a value to where it came from. The
// real chains are three or four steps (a field, a helper's return, a local), so
// this is slack rather than a limit anyone reaches; it exists so a mutually
// recursive pair of helpers cannot hang the audit.
const maxDepth = 24

// classifyExpr answers whether the value expr produces can carry a character
// that changes the shape of the Markdown it lands in.
func (c *classifier) classifyExpr(pkg *packages.Package, expr ast.Expr, e *env, depth int) (outcome verdict, explanation string) {
	if depth > maxDepth {
		return unresolved, "the chain back to a source is deeper than the audit follows"
	}
	if expr == nil {
		return unresolved, "the value arrives through an assignment the audit does not follow"
	}
	expr = ast.Unparen(expr)
	tv, known := pkg.TypesInfo.Types[expr]
	if known && tv.Value != nil {
		return safe, "a compile-time constant"
	}
	if known && tv.Type != nil && !textual(tv.Type) {
		return safe, "a value of a type that renders as a number, a boolean or a timestamp"
	}
	switch typed := expr.(type) {
	case *ast.CallExpr:
		return c.classifyCall(pkg, typed, e, depth)
	case *ast.Ident:
		return c.classifyIdent(pkg, typed, e, depth)
	case *ast.BinaryExpr:
		return c.classifyBinary(pkg, typed, e, depth)
	case *ast.SelectorExpr:
		return c.classifySelector(pkg, typed, e, depth)
	case *ast.CompositeLit:
		return c.classifyComposite(pkg, typed, e, depth)
	case *ast.StarExpr:
		return c.classifyExpr(pkg, typed.X, e, depth+1)
	case *ast.TypeAssertExpr:
		return c.classifyExpr(pkg, typed.X, e, depth+1)
	case *ast.IndexExpr:
		// An element is judged by the collection it came from, which answers
		// for a lookup in a table of emoji the server wrote as readily as for
		// one in a slice filled from a GitLab response.
		return c.classifyExpr(pkg, typed.X, e, depth+1)
	case *ast.SliceExpr:
		return c.classifyExpr(pkg, typed.X, e, depth+1)
	default:
		return unresolved, "the value is written in a shape the audit does not follow"
	}
}

// classifyCall answers for a call: an escaper makes its result safe, a
// conversion passes the question to its operand, and a declared function is
// answered by what its returns are, with its parameters bound to what this call
// site passed.
func (c *classifier) classifyCall(pkg *packages.Package, call *ast.CallExpr, e *env, depth int) (outcome verdict, explanation string) {
	// A conversion is a call of a type, and takes exactly one operand, so the
	// question simply passes to it.
	if tv, ok := pkg.TypesInfo.Types[call.Fun]; ok && tv.IsType() {
		return c.classifyExpr(pkg, call.Args[0], e, depth+1)
	}
	callee := calleeOf(pkg, call)
	if callee == nil {
		return unresolved, "the call is of a function value rather than of a named function"
	}
	if isEscaper(callee) {
		return safe, "already through toolutil." + callee.Name()
	}
	if isSprintf(callee) {
		return c.classifySprintf(pkg, call, e, depth)
	}
	name := qualifiedName(callee)
	if externalSafe[name] {
		return safe, "a standard-library formatter of a non-textual value"
	}
	if carried, ok := passThroughArgs[name]; ok {
		return c.classifyPassThrough(pkg, call, carried, e, depth)
	}
	decl, ok := c.prog.decls[callee]
	if !ok {
		return unresolved, "the body of " + name + " is outside the audited packages"
	}
	return c.classifyReturns(decl, 0, bindParams(pkg, call, callee, e), depth)
}

// classifyPassThrough answers for a call whose result is made of the arguments
// it was given, by asking the same question of each of them.
func (c *classifier) classifyPassThrough(pkg *packages.Package, call *ast.CallExpr, carried []int, e *env, depth int) (outcome verdict, explanation string) {
	worst, reason := safe, "made only of values that are already safe"
	for _, index := range carried {
		if index >= len(call.Args) {
			return unresolved, "the call passes its arguments in a shape the audit does not follow"
		}
		got, why := c.classifyExpr(pkg, call.Args[index], e, depth+1)
		if got > worst {
			worst, reason = got, why
		}
	}
	return worst, reason
}

// bindParams binds a called function's parameters to the arguments this call
// passes. A variadic call and a call whose arguments do not line up with the
// signature (one spreading several results of another call, say) bind nothing,
// which leaves the callee's parameters to be resolved from every caller
// instead.
func bindParams(pkg *packages.Package, call *ast.CallExpr, callee *types.Func, caller *env) *env {
	sig := callee.Signature()
	if sig.Variadic() || sig.Params().Len() != len(call.Args) {
		return nil
	}
	binds := make(map[*types.Var]bound, len(call.Args))
	for i := range sig.Params().Len() {
		binds[sig.Params().At(i)] = bound{expr: call.Args[i], pkg: pkg, env: caller}
	}
	return &env{binds: binds}
}

// classifySprintf answers for a nested Sprintf by asking the same question of
// every value it interpolates. A Sprintf that composes a cell out of escaped
// halves is safe; one that composes it out of raw ones is not, and the reason
// names the raw half.
func (c *classifier) classifySprintf(pkg *packages.Package, call *ast.CallExpr, e *env, depth int) (outcome verdict, explanation string) {
	tv, ok := pkg.TypesInfo.Types[call.Args[0]]
	if !ok || tv.Value == nil {
		return unresolved, "the nested format string is not a constant"
	}
	args := call.Args[1:]
	worst, reason := safe, "every value the nested Sprintf interpolates is safe"
	for _, h := range parseVerbs(constantText(tv)) {
		if !stringishVerbs[h.verb] || h.arg >= len(args) {
			continue
		}
		got, why := c.classifyExpr(pkg, args[h.arg], e, depth+1)
		if got > worst {
			worst, reason = got, why
		}
	}
	return worst, reason
}

// classifyIdent answers for an identifier by following it to the values it
// holds: the argument its caller passed for a bound parameter, every argument
// any caller passes for an unbound one, and every assignment for a local.
func (c *classifier) classifyIdent(pkg *packages.Package, ident *ast.Ident, e *env, depth int) (outcome verdict, explanation string) {
	obj := pkg.TypesInfo.Uses[ident]
	if obj == nil {
		obj = pkg.TypesInfo.Defs[ident]
	}
	v, ok := obj.(*types.Var)
	if !ok {
		return unresolved, "the identifier names something other than a variable"
	}
	if b, bound := e.lookup(v); bound {
		c.bindHits++
		return c.classifyExpr(b.pkg, b.expr, b.env, depth+1)
	}
	if ref, isParam := c.params[v]; isParam {
		return c.classifyParam(ref, depth)
	}
	assigned, found := c.assigns[v]
	if !found || len(assigned) == 0 {
		return unresolved, "nothing the audit can see assigns " + ident.Name
	}
	// A variable built from itself is the common shape in these formatters:
	// a cell is escaped into a local and then wrapped in a link that
	// interpolates the local again. Meeting it a second time contributes
	// nothing, so the walk skips it and judges the rest of the assignment,
	// which is where the raw value would be.
	if c.inVar[v] {
		return safe, "a value already being resolved"
	}
	c.inVar[v] = true
	defer delete(c.inVar, v)

	worst, reason := safe, "every value assigned to "+ident.Name+" is safe"
	for _, rhs := range assigned {
		got, why := c.classifyExpr(c.packageOf(rhs, pkg), rhs, e, depth+1)
		if got > worst {
			worst, reason = got, why
		}
	}
	return worst, reason
}

// packageOf returns the package an indexed expression was written in, falling
// back to the package asking about it.
func (c *classifier) packageOf(expr ast.Expr, fallback *packages.Package) *packages.Package {
	if pkg, ok := c.pkgOf[expr]; ok {
		return pkg
	}
	return fallback
}

// classifyParam answers for a parameter no call site bound, by asking what
// every caller passes.
//
// This is the rule that keeps a shared row helper from being reported once per
// caller: a helper whose callers all pass escaped strings is safe, and a
// finding belongs at whichever call site passes a raw value.
func (c *classifier) classifyParam(ref paramRef, depth int) (outcome verdict, explanation string) {
	sites := c.prog.callers[ref.fn]
	if len(sites) == 0 {
		return unresolved, "nothing in the audited packages calls " + ref.fn.Name()
	}
	worst, reason := safe, "every caller of "+ref.fn.Name()+" passes a safe value"
	for _, site := range sites {
		if ref.index >= len(site.call.Args) {
			return unresolved, "a call of " + ref.fn.Name() + " passes its arguments in a shape the audit does not follow"
		}
		got, why := c.classifyExpr(site.pkg, site.call.Args[ref.index], nil, depth+1)
		if got > worst {
			worst, reason = got, "from a call site of "+ref.fn.Name()+": "+why
		}
	}
	return worst, reason
}

// classifyBinary answers for a concatenation by asking the same question of
// both halves.
func (c *classifier) classifyBinary(pkg *packages.Package, expr *ast.BinaryExpr, e *env, depth int) (outcome verdict, explanation string) {
	left, leftWhy := c.classifyExpr(pkg, expr.X, e, depth+1)
	right, rightWhy := c.classifyExpr(pkg, expr.Y, e, depth+1)
	if left >= right {
		return left, leftWhy
	}
	return right, rightWhy
}

// classifyComposite answers for a literal by taking the worst of the values it
// is built from. It is how a slice of cells spread into a row builder is
// judged, and how a struct built at a call site answers for its own fields.
func (c *classifier) classifyComposite(pkg *packages.Package, lit *ast.CompositeLit, e *env, depth int) (outcome verdict, explanation string) {
	worst, reason := safe, "every value in the literal is safe"
	for _, elt := range lit.Elts {
		value := elt
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			value = kv.Value
		}
		got, why := c.classifyExpr(pkg, value, e, depth+1)
		if got > worst {
			worst, reason = got, why
		}
	}
	return worst, reason
}

// classifySelector answers for a field access.
//
// A string field is where the audit stops looking and starts reporting, unless
// the struct holding it was built where the audit can see it. Every struct
// these formatters render is filled from a GitLab response, so a string field
// on one holds whatever the person who opened the issue, pushed the branch or
// named the label typed. A struct built at the call site is different: an
// options struct carrying a title the server itself wrote answers for that
// title, which is why the literal is looked for first.
func (c *classifier) classifySelector(pkg *packages.Package, sel *ast.SelectorExpr, e *env, depth int) (outcome verdict, explanation string) {
	obj := pkg.TypesInfo.Uses[sel.Sel]
	if v, ok := obj.(*types.Var); ok && packageLevel(v) {
		return c.classifyIdent(pkg, sel.Sel, e, depth+1)
	}
	if _, ok := obj.(*types.Func); ok {
		return unresolved, "the value is a method value the audit does not follow"
	}
	base := c.resolveBase(pkg, sel.X, e, depth)
	if lit, ok := ast.Unparen(base.expr).(*ast.CompositeLit); ok {
		return c.classifyField(base.pkg, lit, base.env, sel.Sel.Name, depth)
	}
	if ref, isParam := c.paramOf(base); isParam {
		return c.classifyParamField(ref, sel.Sel.Name, depth)
	}
	return unescaped, "a field of a value filled from a GitLab response"
}

// resolveBase follows the value a field is read from back to where it was
// built, through the parameter bindings and the single-assignment locals the
// audit records. A variable with more than one assignment answers for none of
// them, because a field read from it could have come from either.
func (c *classifier) resolveBase(pkg *packages.Package, expr ast.Expr, e *env, depth int) bound {
	here := bound{expr: expr, pkg: pkg, env: e}
	if depth > maxDepth || expr == nil {
		return here
	}
	switch typed := ast.Unparen(expr).(type) {
	case *ast.UnaryExpr:
		return c.resolveBase(pkg, typed.X, e, depth+1)
	case *ast.StarExpr:
		return c.resolveBase(pkg, typed.X, e, depth+1)
	case *ast.Ident:
		v, ok := pkg.TypesInfo.Uses[typed].(*types.Var)
		if !ok {
			return here
		}
		if b, isBound := e.lookup(v); isBound {
			c.bindHits++
			return c.resolveBase(b.pkg, b.expr, b.env, depth+1)
		}
		if assigned := c.assigns[v]; len(assigned) == 1 && assigned[0] != nil && !c.fieldWritten[v] {
			return c.resolveBase(c.packageOf(assigned[0], pkg), assigned[0], e, depth+1)
		}
		return bound{expr: typed, pkg: pkg, env: e}
	default:
		return here
	}
}

// paramOf reports whether a resolved base is a parameter no call site bound,
// which is what lets a field of an options struct be answered by the literals
// callers build rather than assumed to be GitLab-authored.
func (c *classifier) paramOf(base bound) (paramRef, bool) {
	ident, ok := ast.Unparen(base.expr).(*ast.Ident)
	if !ok {
		return paramRef{}, false
	}
	v, ok := base.pkg.TypesInfo.Uses[ident].(*types.Var)
	if !ok {
		return paramRef{}, false
	}
	ref, isParam := c.params[v]
	return ref, isParam
}

// classifyParamField answers for a field of a struct a caller passed, by
// asking what every caller put in that field.
//
// The shared formatters in toolutil take their titles and column names this
// way, and the callers write them as literals, so without this rule a title
// the server itself wrote would be reported as GitLab-authored text.
func (c *classifier) classifyParamField(ref paramRef, field string, depth int) (outcome verdict, explanation string) {
	sites := c.prog.callers[ref.fn]
	if len(sites) == 0 {
		// A Markdown formatter is registered by value rather than called, so
		// the audit sees no call site for the one function whose parameter is
		// always the output struct filled from a GitLab response. Reading
		// that as a value the audit cannot follow would excuse the whole
		// registered surface, which is most of it.
		return unescaped, "a field of the value handed to " + ref.fn.Name() + ", which nothing in the audited packages calls"
	}
	worst, reason := safe, "every caller of "+ref.fn.Name()+" passes a safe "+field
	for _, site := range sites {
		if ref.index >= len(site.call.Args) {
			return unresolved, "a call of " + ref.fn.Name() + " passes its arguments in a shape the audit does not follow"
		}
		base := c.resolveBase(site.pkg, site.call.Args[ref.index], nil, depth+1)
		lit, ok := ast.Unparen(base.expr).(*ast.CompositeLit)
		if !ok {
			return unescaped, "a field of a value a caller of " + ref.fn.Name() + " filled from a GitLab response"
		}
		got, why := c.classifyField(base.pkg, lit, base.env, field, depth+1)
		if got > worst {
			worst, reason = got, "from a call site of "+ref.fn.Name()+": "+why
		}
	}
	return worst, reason
}

// classifyField answers for one field of a literal the audit resolved.
//
// A keyed literal that names no such field left it at its zero value, which is
// the empty string and carries nothing. A literal written positionally is not
// followed: matching an element to a field would need the struct's declared
// order, and no formatter in this repository builds its options that way.
func (c *classifier) classifyField(pkg *packages.Package, lit *ast.CompositeLit, e *env, field string, depth int) (outcome verdict, explanation string) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return unresolved, "the struct is built positionally, so the audit cannot tell which element is " + field
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != field {
			continue
		}
		return c.classifyExpr(pkg, kv.Value, e, depth+1)
	}
	return safe, "a field the literal that built the struct leaves at its zero value"
}

// classifyReturns answers for a declared function by taking the worst of its
// return expressions at the given result index, judged with its parameters
// bound to what the call site passed.
func (c *classifier) classifyReturns(decl *funcDecl, index int, e *env, depth int) (outcome verdict, explanation string) {
	key := resultKey{fn: decl.obj, index: index}
	if cached, ok := c.returnMemo[key]; ok {
		return cached, "what " + decl.obj.Name() + " returns"
	}
	if c.inProgress[key] {
		return safe, "a recursive call already being resolved"
	}
	c.inProgress[key] = true
	defer delete(c.inProgress, key)

	before := c.bindHits
	worst, reason := safe, "everything "+decl.obj.Name()+" returns is safe"
	found := false
	ast.Inspect(decl.decl.Body, func(node ast.Node) bool {
		ret, ok := node.(*ast.ReturnStmt)
		if !ok || index >= len(ret.Results) {
			return true
		}
		found = true
		got, why := c.classifyExpr(decl.pkg, ret.Results[index], e, depth+1)
		if got > worst {
			worst, reason = got, why
		}
		return true
	})
	if !found {
		return unresolved, decl.obj.Name() + " returns through a named result the audit does not follow"
	}
	// A verdict reached through a caller's argument holds for that call site
	// and not for the function, so only a verdict the body settled on its own
	// is worth remembering.
	if c.bindHits == before {
		c.returnMemo[key] = worst
	}
	return worst, reason
}

// packageLevel reports whether v is declared at a package's top level, which
// is what tells a qualified variable of another package apart from a field of
// a struct: a package scope is the one whose parent is the universe.
func packageLevel(v *types.Var) bool {
	scope := v.Parent()
	return scope != nil && scope.Parent() == types.Universe
}

// isEscaper reports whether callee is one of the toolutil helpers that makes a
// value safe to interpolate.
func isEscaper(callee *types.Func) bool {
	if callee.Pkg() == nil || callee.Pkg().Path() != toolutilPath {
		return false
	}
	return escapers[callee.Name()]
}

// isSprintf reports whether callee is fmt.Sprintf.
func isSprintf(callee *types.Func) bool {
	return callee.Pkg() != nil && callee.Pkg().Path() == "fmt" && callee.Name() == "Sprintf"
}

// qualifiedName renders a function as package.Name, with the receiver type
// included for a method so a whitelist entry cannot match the wrong one.
func qualifiedName(callee *types.Func) string {
	pkgName := ""
	if callee.Pkg() != nil {
		pkgName = callee.Pkg().Name()
	}
	recv := callee.Signature().Recv()
	if recv == nil {
		return pkgName + "." + callee.Name()
	}
	recvName := recv.Type().String()
	if idx := strings.LastIndex(recvName, "."); idx >= 0 {
		recvName = recvName[idx+1:]
	}
	return pkgName + "." + strings.TrimPrefix(recvName, "*") + "." + callee.Name()
}

// textual reports whether a value of type t can render as text this server did
// not write.
//
// A number, a boolean and a timestamp cannot: whatever GitLab put in them, the
// rendering is digits, "true" or a formatted instant. Everything else is
// treated as able to carry a string, an interface included, since its dynamic
// type is not knowable here.
func textual(t types.Type) bool {
	return textualDepth(t, 0, map[types.Type]bool{})
}

// textualDepth is textual with a guard against a type that contains itself.
//
// The alias is unwrapped first, and that is load-bearing: since Go 1.23 the
// type checker reports `any` as an alias rather than as the interface it
// names, so a field declared `any` would fall past every case below and be
// read as a value that cannot carry text.
func textualDepth(t types.Type, depth int, seen map[types.Type]bool) bool {
	if t == nil || depth > 8 || seen[t] {
		return false
	}
	seen[t] = true
	t = types.Unalias(t)
	if named, ok := t.(*types.Named); ok {
		if rendersAsInstant(named) {
			return false
		}
		return textualDepth(named.Underlying(), depth+1, seen)
	}
	switch typed := t.(type) {
	case *types.Basic:
		return typed.Info()&types.IsString != 0
	case *types.Interface:
		return true
	case *types.Pointer:
		return textualDepth(typed.Elem(), depth+1, seen)
	case *types.Slice:
		return textualDepth(typed.Elem(), depth+1, seen)
	case *types.Array:
		return textualDepth(typed.Elem(), depth+1, seen)
	case *types.Map:
		return textualDepth(typed.Key(), depth+1, seen) || textualDepth(typed.Elem(), depth+1, seen)
	case *types.Struct:
		for field := range typed.Fields() {
			if textualDepth(field.Type(), depth+1, seen) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// rendersAsInstant names the standard-library types whose own String method
// renders them as a timestamp or a duration, so their fields never reach the
// page even under %v.
func rendersAsInstant(named *types.Named) bool {
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != "time" {
		return false
	}
	switch obj.Name() {
	case "Time", "Duration", "Month", "Weekday":
		return true
	default:
		return false
	}
}

// constantText unwraps a constant string value.
func constantText(tv types.TypeAndValue) string {
	if tv.Value == nil || tv.Value.Kind() != constant.String {
		return ""
	}
	return constant.StringVal(tv.Value)
}
