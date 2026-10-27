// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"cmp"
	"go/ast"
	"go/types"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
)

// boundMethod is a client-go service method, named as the section names it
// ("Users.GetUserStatus"), with its signature.
type boundMethod struct {
	key       string
	signature *types.Signature
}

// heldMethod is a client-go service method a variable of func type holds.
// via is the call that handed it over when the variable is the parameter of
// the function the method value was passed to, so the arguments a call
// through it hands on are read from that call alone.
type heldMethod struct {
	method boundMethod
	via    *ast.CallExpr
}

// unfollowedValue is a client-go service method with a parameter of type
// any, taken as a value somewhere the walk does not follow it to the calls
// made through it.
type unfollowedValue struct {
	// Site is the function the value is in, Method the client-go method
	// and Value the expression handed on.
	Site   string `json:"site"`
	Method string `json:"method"`
	Value  string `json:"value"`
}

// methodValues is where the client-go service methods the walked bodies
// take as values go.
type methodValues struct {
	// held maps each variable a method value reaches (a parameter, a local
	// or a struct field, by its origin) to the methods it holds.
	held map[types.Object][]heldMethod
	// judged maps each call of the result of a call (pick(service)(id)) to
	// the methods that result is.
	judged map[*ast.CallExpr][]heldMethod
	// unfollowed is every method value the walk does not follow, sorted.
	unfollowed []unfollowedValue
}

// flowEdge is one hop of a value of func type from one variable to another:
// via is the call when the hop hands it to a parameter.
type flowEdge struct {
	to  types.Object
	via *ast.CallExpr
}

// valueEscape is a variable of func type handed somewhere the walk does not
// follow.
type valueEscape struct {
	from        types.Object
	site, value string
}

// returnedMethod is a client-go service method a function returns as its
// one result, with the function and the expression it is written in.
type returnedMethod struct {
	method      boundMethod
	site, value string
}

// funcEntity is a function whose returns the walk reads: a walked function
// by its object, or a function literal.
type funcEntity struct {
	object types.Object
	lit    *ast.FuncLit
}

// placedExpr is an expression of a walked body with the nearest of its
// ancestors that is not a parenthesis.
type placedExpr struct {
	fn     tracedFunc
	expr   ast.Expr
	parent ast.Node
}

// methodFlow gathers, over every walked body, where the client-go service
// methods taken as values go.
type methodFlow struct {
	trace *inputTrace
	held  map[types.Object][]heldMethod
	// returning maps each variable holding a function that returns methods
	// to those methods, and returns each such function to its own.
	returning map[types.Object][]returnedMethod
	returns   map[funcEntity][]returnedMethod
	edges     map[types.Object][]flowEdge
	escapes   []valueEscape
	// calls places every call, and lits every function literal, of the
	// walked bodies.
	calls      map[*ast.CallExpr]placedExpr
	lits       []placedExpr
	judged     map[*ast.CallExpr][]heldMethod
	unfollowed []unfollowedValue
}

// followMethodValues follows every client-go service method with a
// parameter of type any that funcs take as a value, instead of calling it
// where they name it, to the variables it reaches: the parameter of a walked
// function it is passed to, a field of a struct literal, a local, or a field
// it is assigned to, and on from each of those the same way. A method value
// a function returns as its one result goes where each call of the function
// goes: a function literal's by where the literal is handed, a walked
// function's by its callers, and the call of that result is judged with it.
// Anything else (a function the walk does not read, a variadic parameter, a
// collection, a method expression, one of several results, the result of a
// function named other than by a call or called by no walked body) is
// unfollowed.
func followMethodValues(funcs []tracedFunc, trace *inputTrace) methodValues {
	flow := &methodFlow{
		trace: trace, held: map[types.Object][]heldMethod{}, returning: map[types.Object][]returnedMethod{},
		returns: map[funcEntity][]returnedMethod{}, edges: map[types.Object][]flowEdge{},
		calls: map[*ast.CallExpr]placedExpr{}, judged: map[*ast.CallExpr][]heldMethod{}, unfollowed: make([]unfollowedValue, 0),
	}
	for _, fn := range funcs {
		flow.collect(fn)
	}
	flow.resolve()
	slices.SortFunc(flow.unfollowed, func(a, b unfollowedValue) int {
		return cmp.Or(strings.Compare(a.Site, b.Site), strings.Compare(a.Method, b.Method), strings.Compare(a.Value, b.Value))
	})
	return methodValues{held: flow.held, judged: flow.judged, unfollowed: slices.Compact(flow.unfollowed)}
}

// collect reads one body: its calls and function literals, its method
// values, and every hop of a variable of func type.
func (f *methodFlow) collect(fn tracedFunc) {
	inspectBody(fn.body, func(node, parent ast.Node, lit *ast.FuncLit) {
		expr, isExpr := node.(ast.Expr)
		if !isExpr {
			return
		}
		at := placedExpr{fn: fn, expr: expr, parent: parent}
		switch typed := expr.(type) {
		case *ast.CallExpr:
			f.calls[typed] = at
		case *ast.FuncLit:
			f.lits = append(f.lits, at)
		}
		if method, expression, isMethod := methodValueOf(fn.info, expr); isMethod {
			in := funcEntity{lit: lit}
			if lit == nil {
				in = funcEntity{object: fn.object}
			}
			f.methodValue(at, method, expression, in)
			return
		}
		if from := funcVariable(fn.info, expr); from != nil && flowsOn(parent, expr) {
			if to, via := f.trace.destination(fn.info, parent, expr); to != nil {
				f.edges[from] = append(f.edges[from], flowEdge{to: to, via: via})
			} else {
				f.escapes = append(f.escapes, valueEscape{from: from, site: fn.site, value: types.ExprString(expr)})
			}
		}
	})
}

// methodValue places a method value written in the function in: one it
// returns as its one result is that function's, a method expression is
// unfollowed, and any other is handed where it is written. A method with no
// parameter of type any, and one called where it is named, need nothing.
func (f *methodFlow) methodValue(at placedExpr, method boundMethod, expression bool, in funcEntity) {
	if !takesAny(method.signature) || (!expression && isCallee(at.parent, at.expr)) {
		return
	}
	written := returnedMethod{method: method, site: at.fn.site, value: types.ExprString(at.expr)}
	if expression {
		f.unfollowed = append(f.unfollowed, unfollowedValue{Site: written.site, Method: method.key, Value: written.value})
		return
	}
	if ret, isReturn := at.parent.(*ast.ReturnStmt); isReturn && len(ret.Results) == 1 {
		f.returns[in] = append(f.returns[in], written)
		return
	}
	f.hand(at, written)
}

// hand places a method an expression is: the call that calls it, the
// variable it is handed to, or unfollowed.
func (f *methodFlow) hand(at placedExpr, method returnedMethod) {
	if isCallee(at.parent, at.expr) {
		call, _ := at.parent.(*ast.CallExpr)
		f.judged[call] = append(f.judged[call], heldMethod{method: method.method})
		return
	}
	if to, via := f.trace.destination(at.fn.info, at.parent, at.expr); to != nil {
		f.held[to] = append(f.held[to], heldMethod{method: method.method, via: via})
		return
	}
	f.unfollowed = append(f.unfollowed, unfollowedValue{Site: at.fn.site, Method: method.method.key, Value: types.ExprString(at.expr)})
}

// unfollow lists methods returned by a function whose calls the walk does
// not see, where each is written.
func (f *methodFlow) unfollow(methods []returnedMethod) {
	for _, method := range methods {
		f.unfollowed = append(f.unfollowed, unfollowedValue{Site: method.site, Method: method.method.key, Value: method.value})
	}
}

// resolve follows the functions that return methods to the calls of their
// results, carries what each variable holds along the hops out of it, and
// lists what the variables handed where the walk does not follow hold.
func (f *methodFlow) resolve() {
	for _, at := range f.lits {
		f.placeLiteral(at)
	}
	for entity, methods := range f.returns {
		f.placeReturned(entity, methods)
	}
	propagate(f.returning, f.edges, func(method returnedMethod, _ *ast.CallExpr) returnedMethod { return method })
	for call, at := range f.calls {
		f.handEach(at, f.returning[funcVariable(at.fn.info, ast.Unparen(call.Fun))])
	}
	propagate(f.held, f.edges, func(method heldMethod, via *ast.CallExpr) heldMethod {
		return heldMethod{method: method.method, via: via}
	})
	for _, escape := range f.escapes {
		for _, method := range f.held[escape.from] {
			f.unfollowed = append(f.unfollowed, unfollowedValue{Site: escape.site, Method: method.method.key, Value: escape.value})
		}
		for _, method := range f.returning[escape.from] {
			f.unfollowed = append(f.unfollowed, unfollowedValue{Site: escape.site, Method: method.method.key, Value: escape.value})
		}
	}
}

// placeLiteral places the methods a function literal returns as its one
// result: where the call that calls the literal goes, on the variable the
// literal is handed to, or unfollowed.
func (f *methodFlow) placeLiteral(at placedExpr) {
	lit, _ := at.expr.(*ast.FuncLit)
	methods := f.returns[funcEntity{lit: lit}]
	if len(methods) == 0 {
		return
	}
	if isCallee(at.parent, at.expr) {
		call, _ := at.parent.(*ast.CallExpr)
		f.handEach(f.calls[call], methods)
		return
	}
	if to, _ := f.trace.destination(at.fn.info, at.parent, at.expr); to != nil {
		f.returning[to] = append(f.returning[to], methods...)
		return
	}
	f.unfollow(methods)
}

// placeReturned places the methods a walked function returns as its one
// result where each call of it goes, or unfollows them when the walk does
// not see every call. A function literal's are placed by placeLiteral.
func (f *methodFlow) placeReturned(entity funcEntity, methods []returnedMethod) {
	if entity.object == nil {
		return
	}
	if f.trace.unseenCallers(f.trace.bodies[entity.object]) {
		f.unfollow(methods)
		return
	}
	for _, caller := range f.trace.callers[entity.object] {
		f.handEach(f.calls[caller.call], methods)
	}
}

// handEach hands every method a call's result is to where the call goes.
func (f *methodFlow) handEach(at placedExpr, methods []returnedMethod) {
	for _, method := range methods {
		f.hand(at, method)
	}
}

// propagate carries what each variable holds along every edge out of it,
// through carry, until nothing changes.
func propagate[T comparable](held map[types.Object][]T, edges map[types.Object][]flowEdge, carry func(T, *ast.CallExpr) T) {
	var work []types.Object
	for from := range held {
		work = append(work, from)
	}
	for len(work) > 0 {
		from := work[len(work)-1]
		work = work[:len(work)-1]
		for _, edge := range edges[from] {
			for _, value := range held[from] {
				next := carry(value, edge.via)
				if !slices.Contains(held[edge.to], next) {
					held[edge.to] = append(held[edge.to], next)
					work = append(work, edge.to)
				}
			}
		}
	}
}

// heldBy lists the methods a call reaches: the client-go service method it
// calls by name, the methods the result it calls is, or every method the
// variable of func type it calls holds. A method expression is unfollowed
// instead.
func (v methodValues) heldBy(info *types.Info, call *ast.CallExpr) []heldMethod {
	fun := ast.Unparen(call.Fun)
	if method, expression, isMethod := methodValueOf(info, fun); isMethod {
		if expression {
			return nil
		}
		return []heldMethod{{method: method}}
	}
	if judged, isJudged := v.judged[call]; isJudged {
		return judged
	}
	return v.held[funcVariable(info, fun)]
}

// methodValueOf returns the client-go service method a selector names, and
// whether it names it through the interface type, as a method expression,
// rather than through a value. An interface has no fields, so a selector on
// one names a method.
func methodValueOf(info *types.Info, expr ast.Expr) (method boundMethod, expression, ok bool) {
	sel, isSelector := expr.(*ast.SelectorExpr)
	if !isSelector {
		return boundMethod{}, false, false
	}
	named, isService := shared.ClientGoServiceInterface(info.TypeOf(sel.X))
	if !isService {
		return boundMethod{}, false, false
	}
	signature, _ := info.TypeOf(sel).(*types.Signature)
	return boundMethod{key: shared.ServiceName(named) + "." + sel.Sel.Name, signature: signature}, info.Selections[sel].Kind() == types.MethodExpr, true
}

// takesAny reports whether a signature has a parameter of type any.
func takesAny(signature *types.Signature) bool {
	for param := range signature.Params().Variables() {
		if isEmptyInterface(param.Type()) {
			return true
		}
	}
	return false
}

// funcVariable returns the variable of func type an identifier or a field
// selector names, by its origin, and nil for anything else.
func funcVariable(info *types.Info, expr ast.Expr) types.Object {
	var variable *types.Var
	switch typed := expr.(type) {
	case *ast.Ident:
		variable, _ = info.Uses[typed].(*types.Var)
	case *ast.SelectorExpr:
		if selection := info.Selections[typed]; selection != nil && selection.Kind() == types.FieldVal {
			variable, _ = selection.Obj().(*types.Var)
		}
	}
	if variable == nil {
		return nil
	}
	if _, isFunc := variable.Type().Underlying().(*types.Signature); !isFunc {
		return nil
	}
	return variable.Origin()
}

// isCallee reports whether child is what parent, a call, calls.
func isCallee(parent ast.Node, child ast.Expr) bool {
	call, isCall := parent.(*ast.CallExpr)
	return isCall && ast.Unparen(call.Fun) == child
}

// flowsOn reports whether an expression of func type is handed on where it
// is written: not called, compared, selected from, named as a key or
// assigned to.
func flowsOn(parent ast.Node, child ast.Expr) bool {
	switch typed := parent.(type) {
	case *ast.CallExpr:
		return ast.Unparen(typed.Fun) != child
	case *ast.BinaryExpr, *ast.SelectorExpr:
		return false
	case *ast.KeyValueExpr:
		return typed.Key != child
	case *ast.AssignStmt:
		return !slices.ContainsFunc(typed.Lhs, func(lhs ast.Expr) bool { return ast.Unparen(lhs) == child })
	}
	return true
}

// destination returns the variable a value of func type written as child
// in parent is handed to, by its origin, and the call when it is a
// parameter: the parameter of a walked function it is an argument of (a
// variadic one excluded, whose values are a slice), the field of a struct
// literal it fills, or the variable or field an assignment or a declaration
// writes it to. Nil for anywhere else.
func (t *inputTrace) destination(info *types.Info, parent ast.Node, child ast.Expr) (types.Object, *ast.CallExpr) {
	isChild := func(expr ast.Expr) bool { return ast.Unparen(expr) == child }
	switch typed := parent.(type) {
	case *ast.CallExpr:
		callee, walked := t.walkedCallee(info, typed)
		if !walked {
			return nil, nil
		}
		params := callee.signature.Params()
		for i, arg := range typed.Args {
			if isChild(arg) && (!callee.signature.Variadic() || i < params.Len()-1) {
				return params.At(i), typed
			}
		}
	case *ast.KeyValueExpr:
		key, _ := typed.Key.(*ast.Ident)
		if field, isVariable := info.Uses[key].(*types.Var); isVariable && field.IsField() {
			return field.Origin(), nil
		}
	case *ast.CompositeLit:
		if fields, isStruct := info.TypeOf(typed).Underlying().(*types.Struct); isStruct {
			return fields.Field(slices.IndexFunc(typed.Elts, isChild)).Origin(), nil
		}
	case *ast.AssignStmt:
		return assigned(info, typed.Lhs[slices.IndexFunc(typed.Rhs, isChild)]), nil
	case *ast.ValueSpec:
		return info.Defs[typed.Names[slices.IndexFunc(typed.Values, isChild)]], nil
	}
	return nil, nil
}

// assigned returns the variable an assignment writes to, by its origin: a
// local or a parameter it names, or a field it selects. Nil for anything
// else: an element, a pointer's target, the blank identifier, and a
// package's variable, which another package can call through by its
// qualified name, a call the walk does not judge.
func assigned(info *types.Info, target ast.Expr) types.Object {
	switch typed := ast.Unparen(target).(type) {
	case *ast.Ident:
		variable := info.Defs[typed]
		if variable == nil {
			variable = info.Uses[typed]
		}
		if variable == nil || variable.Parent() == variable.Pkg().Scope() {
			return nil
		}
		return variable
	case *ast.SelectorExpr:
		if selection := info.Selections[typed]; selection != nil {
			field, _ := selection.Obj().(*types.Var)
			return field.Origin()
		}
	}
	return nil
}

// bodyFrame is what the children of a node entered get: the parent they
// see, past a parenthesis, and the function literal they are in.
type bodyFrame struct {
	parent ast.Node
	lit    *ast.FuncLit
}

// inspectBody calls visit with every node below root, the nearest of its
// ancestors that is not a parenthesis (nil for root itself), and the
// innermost function literal it is in (nil in root's own function).
func inspectBody(root ast.Node, visit func(node, parent ast.Node, lit *ast.FuncLit)) {
	var frames []bodyFrame
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			frames = frames[:len(frames)-1]
			return true
		}
		var here bodyFrame
		if len(frames) > 0 {
			here = frames[len(frames)-1]
		}
		visit(node, here.parent, here.lit)
		next := bodyFrame{parent: node, lit: here.lit}
		switch typed := node.(type) {
		case *ast.ParenExpr:
			next.parent = here.parent
		case *ast.FuncLit:
			next.lit = typed
		}
		frames = append(frames, next)
		return true
	})
}
