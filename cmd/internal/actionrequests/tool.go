package actionrequests

import (
	"go/ast"
	"go/types"
)

// resolveSpecTool resolves the individual tool name an ActionSpec expression
// declares.
//
// It is the key a construction site is joined to its canonical ID on where the
// action name and the package do not decide: the project and group badge
// actions are declared by one package under one set of names and told apart
// only by the group each list is aggregated into, and the tool name each
// declares is unique to one of them. It follows the same shapes the action
// name is followed through, and then the options value into the
// IndividualTool field, through the helpers that build the options and the
// field assignments that amend them.
func (r *resolver) resolveSpecTool(expr ast.Expr, at frame, depth int) string {
	if tooDeep(depth) {
		return ""
	}
	switch typed := ast.Unparen(expr).(type) {
	case *ast.Ident:
		for _, next := range r.follow(typed, at) {
			if tool := r.resolveSpecTool(next.expr, next.frame, depth+1); tool != "" {
				return tool
			}
		}
	case *ast.CompositeLit:
		if value, ok := fieldValue(typed, individualToolField); ok {
			return r.resolveToolSpec(value, at, depth+1)
		}
	case *ast.CallExpr:
		return r.resolveSpecCallTool(typed, at, depth)
	}
	return ""
}

// resolveSpecCallTool resolves the tool name of a call that produces an
// ActionSpec: a toolutil constructor's options argument, the spec a helper
// returns, or the spec a decorating method was called on.
func (r *resolver) resolveSpecCallTool(call *ast.CallExpr, at frame, depth int) string {
	callee := calleeFunc(at.pkg, call)
	if callee == nil {
		return ""
	}
	if options, ok := specConstructorOptions(callee, call); ok {
		return r.resolveOptionsTool(options, at, depth+1)
	}
	for _, ret := range r.returnsOf(callee, call, at, specTypeName) {
		if tool := r.resolveSpecTool(ret.expr, ret.frame, depth+1); tool != "" {
			return tool
		}
	}
	if receiver := methodReceiver(call); receiver != nil {
		return r.resolveSpecTool(receiver, at, depth+1)
	}
	return ""
}

// resolveOptionsTool resolves the tool name an ActionSpecOptions expression
// carries.
//
// A variable is read twice over: an assignment to its IndividualTool field, or
// to that field's Name, written in the body that holds it, and then whatever
// it was initialized from. The field assignment is read first because it is
// the later write, which is what the value holds when it is handed on.
func (r *resolver) resolveOptionsTool(expr ast.Expr, at frame, depth int) string {
	if tooDeep(depth) {
		return ""
	}
	switch typed := ast.Unparen(expr).(type) {
	case *ast.Ident:
		if tool := r.assignedTool(typed, at, depth); tool != "" {
			return tool
		}
		for _, next := range r.follow(typed, at) {
			if tool := r.resolveOptionsTool(next.expr, next.frame, depth+1); tool != "" {
				return tool
			}
		}
	case *ast.CompositeLit:
		if value, ok := fieldValue(typed, individualToolField); ok {
			return r.resolveToolSpec(value, at, depth+1)
		}
	case *ast.CallExpr:
		return r.firstReturned(typed, at, depth, optionsTypeName, r.resolveOptionsTool)
	}
	return ""
}

// resolveToolSpec resolves the Name an IndividualToolSpec expression carries.
func (r *resolver) resolveToolSpec(expr ast.Expr, at frame, depth int) string {
	if tooDeep(depth) {
		return ""
	}
	switch typed := ast.Unparen(expr).(type) {
	case *ast.Ident:
		for _, next := range r.follow(typed, at) {
			if tool := r.resolveToolSpec(next.expr, next.frame, depth+1); tool != "" {
				return tool
			}
		}
	case *ast.CompositeLit:
		if value, ok := fieldValue(typed, nameField); ok {
			return r.resolveString(value, at, depth+1)
		}
	case *ast.CallExpr:
		return r.firstReturned(typed, at, depth, toolSpecTypeName, r.resolveToolSpec)
	}
	return ""
}

// firstReturned resolves the value of the wanted toolutil type a helper
// returns, with resolve, and gives the first that is not empty.
func (r *resolver) firstReturned(call *ast.CallExpr, at frame, depth int, typeName string,
	resolve func(ast.Expr, frame, int) string,
) string {
	callee := calleeFunc(at.pkg, call)
	if callee == nil {
		return ""
	}
	for _, ret := range r.returnsOf(callee, call, at, typeName) {
		if value := resolve(ret.expr, ret.frame, depth+1); value != "" {
			return value
		}
	}
	return ""
}

// assignedTool resolves the tool name written into an options variable by a
// field assignment in the body that holds it: `options.IndividualTool = ...`
// or `options.IndividualTool.Name = ...`. The last such assignment in the body
// is the one read, since it is the write the value carries when it is handed
// on.
func (r *resolver) assignedTool(ident *ast.Ident, at frame, depth int) string {
	variable, ok := at.pkg.TypesInfo.Uses[ident].(*types.Var)
	if !ok || at.decl == nil {
		return ""
	}
	var tool string
	ast.Inspect(at.decl.Body, func(node ast.Node) bool {
		assign, isAssign := node.(*ast.AssignStmt)
		if !isAssign || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, lhs := range assign.Lhs {
			if value := r.toolWrittenBy(lhs, assign.Rhs[i], variable, at, depth); value != "" {
				tool = value
			}
		}
		return true
	})
	return tool
}

// toolWrittenBy resolves the tool name one assignment writes into variable, or
// "" when the assignment writes something else.
func (r *resolver) toolWrittenBy(lhs, rhs ast.Expr, variable *types.Var, at frame, depth int) string {
	path, root := selectorPath(lhs)
	if root == nil || at.pkg.TypesInfo.Uses[root] != variable || len(path) == 0 || path[0] != individualToolField {
		return ""
	}
	if len(path) == 1 {
		return r.resolveToolSpec(rhs, at, depth+1)
	}
	if len(path) == 2 && path[1] == nameField {
		return r.resolveString(rhs, at, depth+1)
	}
	return ""
}

// selectorPath splits a field selection such as options.IndividualTool.Name
// into the identifier it starts from and the field names after it, or reports
// a nil root for an expression that is not a chain of selections on a name.
func selectorPath(expr ast.Expr) ([]string, *ast.Ident) {
	var path []string
	for {
		switch typed := ast.Unparen(expr).(type) {
		case *ast.SelectorExpr:
			path = append([]string{typed.Sel.Name}, path...)
			expr = typed.X
		case *ast.Ident:
			return path, typed
		default:
			return nil, nil
		}
	}
}

// fieldValue returns the value a keyed composite literal gives one field.
func fieldValue(lit *ast.CompositeLit, field string) (ast.Expr, bool) {
	for _, element := range lit.Elts {
		kv, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, isIdent := kv.Key.(*ast.Ident); isIdent && key.Name == field {
			return kv.Value, true
		}
	}
	return nil, false
}

// specConstructorOptions returns the options argument of a toolutil spec
// constructor, the third parameter of the (name, route, options) shape
// [specConstructorArgs] recognizes, when the constructor takes one.
func specConstructorOptions(callee *types.Func, call *ast.CallExpr) (ast.Expr, bool) {
	if _, _, ok := specConstructorArgs(callee, call); !ok {
		return nil, false
	}
	params := callee.Signature().Params()
	if params.Len() < 3 || len(call.Args) < 3 || !isToolutilType(params.At(2).Type(), optionsTypeName) {
		return nil, false
	}
	return call.Args[2], true
}
