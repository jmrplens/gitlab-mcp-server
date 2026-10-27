// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package structs

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// collectNestedInputs pairs every literal of a nested request struct in pkg
// with each tool struct its values are read from.
//
// A nested struct is one an options struct reaches through a field: a branch
// permission, an environment access level, a webhook's custom header. A
// handler builds it element by element out of a list its input carries, so
// the input it is held to is that list's element type
// (BranchPermissionInput), and the handler's own input is the wrong
// comparison. The struct read is the one a field selection in the literal
// names, or in an assignment to a field of the variable the literal is bound
// to: in
//
//	for _, p := range in {
//		opt := &gl.BranchPermissionOptions{UserID: p.UserID}
//		if p.AccessLevel != nil {
//			opt.AccessLevel = ...(*p.AccessLevel)
//		}
//	}
//
// it is p's type both times. A value that is a variable of the function is
// read through what the function assigns it: a url read into a local from
// input.Upload.URL, or from a deprecated flat input.UploadURL when Upload is
// nil, pairs the struct built from the local with both inputs, the flat one
// being an input that offers the nested fields under names of its own. A
// literal whose values read no field even so (one built from what a method
// of the input returns) is paired with the function's own input struct, as a
// handler's literal is. One built in a function with neither (out of a list
// of integers) pairs with nothing, and the rule that holds every nested
// struct to an input reports it.
func collectNestedInputs(pkg *packages.Package, out map[[2]string]structPair) {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			collectNestedIn(pkg, fn, out)
		}
	}
}

// collectNestedIn records the nested pairs of the literals in one function.
func collectNestedIn(pkg *packages.Package, fn *ast.FuncDecl, out map[[2]string]structPair) {
	info, body := pkg.TypesInfo, fn.Body
	inputNamed, inputStruct, hasInput := handlerInputStruct(pkg, fn)
	bound := literalBindings(info, body)
	values := assignedValues(info, body)
	ast.Inspect(body, func(node ast.Node) bool {
		lit, isLit := node.(*ast.CompositeLit)
		if !isLit {
			return true
		}
		named, st, isSDK := clientGoNamedStruct(info.TypeOf(lit))
		if !isSDK || !isNestedRequest(named) {
			return true
		}
		sources := map[*types.Named]*types.Struct{}
		for _, elt := range lit.Elts {
			if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
				elt = kv.Value
			}
			readStructs(info, elt, values, sources)
		}
		for variable, literal := range bound {
			if literal == lit {
				assignedStructs(info, body, variable, values, sources)
			}
		}
		if len(sources) == 0 && hasInput {
			sources[inputNamed] = inputStruct
		}
		for source, sourceStruct := range sources {
			recordInputPair(out, inputPair(source, sourceStruct, named, st, ViaNested))
		}
		return true
	})
}

// literalBindings maps every variable a function body binds a composite
// literal to, by := or =, a pointer to one included. A left-hand side that
// is no identifier (an element, a field) names no variable and binds
// nothing.
func literalBindings(info *types.Info, body *ast.BlockStmt) map[*types.Var]*ast.CompositeLit {
	bound := map[*types.Var]*ast.CompositeLit{}
	ast.Inspect(body, func(node ast.Node) bool {
		assign, isAssign := node.(*ast.AssignStmt)
		if !isAssign || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, rhs := range assign.Rhs {
			ident, _ := assign.Lhs[i].(*ast.Ident)
			lit, variable := compositeOf(rhs), identVar(info, ident)
			if lit != nil && variable != nil {
				bound[variable] = lit
			}
		}
		return true
	})
	return bound
}

// compositeOf returns the composite literal expr is, or takes the address of.
func compositeOf(expr ast.Expr) *ast.CompositeLit {
	expr = ast.Unparen(expr)
	if unary, isUnary := expr.(*ast.UnaryExpr); isUnary {
		expr = ast.Unparen(unary.X)
	}
	lit, _ := expr.(*ast.CompositeLit)
	return lit
}

// identVar returns the variable an identifier declares or uses.
func identVar(info *types.Info, ident *ast.Ident) *types.Var {
	if declared, isVar := info.Defs[ident].(*types.Var); isVar {
		return declared
	}
	used, _ := info.Uses[ident].(*types.Var)
	return used
}

// assignedValues maps every variable body assigns, by := or =, to the values
// it assigns it, in source order. A left-hand side that is no identifier
// assigns no variable.
func assignedValues(info *types.Info, body *ast.BlockStmt) map[*types.Var][]ast.Expr {
	values := map[*types.Var][]ast.Expr{}
	ast.Inspect(body, func(node ast.Node) bool {
		assign, isAssign := node.(*ast.AssignStmt)
		if !isAssign || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, lhs := range assign.Lhs {
			ident, _ := lhs.(*ast.Ident)
			if variable := identVar(info, ident); variable != nil {
				values[variable] = append(values[variable], assign.Rhs[i])
			}
		}
		return true
	})
	return values
}

// assignedStructs adds to sources the tool structs read on the right of every
// assignment, in body, to a field of variable.
func assignedStructs(info *types.Info, body *ast.BlockStmt, variable *types.Var, values map[*types.Var][]ast.Expr, sources map[*types.Named]*types.Struct) {
	ast.Inspect(body, func(node ast.Node) bool {
		assign, isAssign := node.(*ast.AssignStmt)
		if !isAssign || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, lhs := range assign.Lhs {
			if rootVar(info, lhs) == variable {
				if _, isSelector := ast.Unparen(lhs).(*ast.SelectorExpr); isSelector {
					readStructs(info, assign.Rhs[i], values, sources)
				}
			}
		}
		return true
	})
}

// unaliasPointer unwraps an alias, and an alias a pointer points to, so a
// value whose type is spelled through one (type DeployAccessLevelInput =
// protectedenvdata.DeployAccessLevelInput) is read as the struct it names.
func unaliasPointer(t types.Type) types.Type {
	t = types.Unalias(t)
	if ptr, isPtr := t.(*types.Pointer); isPtr {
		return types.NewPointer(types.Unalias(ptr.Elem()))
	}
	return t
}

// rootVar returns the variable at the root of a selector chain (opt in
// opt.AccessLevel), or nil.
func rootVar(info *types.Info, expr ast.Expr) *types.Var {
	for {
		switch e := ast.Unparen(expr).(type) {
		case *ast.SelectorExpr:
			expr = e.X
		case *ast.StarExpr:
			expr = e.X
		case *ast.Ident:
			return identVar(info, e)
		default:
			return nil
		}
	}
}

// readStructs adds to sources every tool struct a field selection in expr
// reads a field of: of a chain (in.Stages.Issues), the struct the last field
// is read from, which is the one the value belongs to. A variable expr reads
// is read through every value values says the function assigns it, each
// variable once however the assignments refer to one another. A nested
// composite literal of a client-go struct is a value of its own and is not
// entered.
func readStructs(info *types.Info, expr ast.Expr, values map[*types.Var][]ast.Expr, sources map[*types.Named]*types.Struct) {
	followed := map[*types.Var]bool{}
	var read func(ast.Node) bool
	read = func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.CompositeLit:
			_, _, isSDK := clientGoNamedStruct(info.TypeOf(typed))
			return !isSDK
		case *ast.Ident:
			variable, _ := info.Uses[typed].(*types.Var)
			if !followed[variable] {
				followed[variable] = true
				for _, value := range values[variable] {
					ast.Inspect(value, read)
				}
			}
		case *ast.SelectorExpr:
			selection, isSelection := info.Selections[typed]
			if !isSelection || selection.Kind() != types.FieldVal {
				return true
			}
			if named, st, isStruct := derefNamedStruct(unaliasPointer(info.TypeOf(typed.X))); isStruct && isToolStruct(named) {
				sources[named] = st
				return false
			}
		}
		return true
	}
	ast.Inspect(expr, read)
}
