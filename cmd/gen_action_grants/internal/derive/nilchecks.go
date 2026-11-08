// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package derive

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
)

// This file is whether a client-go call can reach a route of the method that
// sits behind a nil check of one of its parameters. The label update and delete
// send /projects/:/labels/: when lid is not nil and /projects/:/labels when it
// is, and every handler here hands lid a string, so the second was derived as a
// way of running four actions that no call of them ever takes.

// neverSent reports whether a call hands an argument that cannot pass one of
// the checks a route sits behind. A method named without being called there has
// no arguments to read, and a check of a parameter the call leaves out of a
// variadic tail has none either, so neither decides anything.
func neverSent(checks []NilCheck, l *leaf, at *frame) bool {
	if l.call == nil {
		return false
	}
	for _, check := range checks {
		if check.Index >= len(l.call.Args) {
			continue
		}
		held := nilnessOf(l.call.Args[check.Index], at)
		if (check.Nil && held == neverNil) || (!check.Nil && held == alwaysNil) {
			return true
		}
	}
	return false
}

// nilness is what the reading can say of an argument at a nil check.
type nilness uint8

const (
	// eitherNil is an argument that may be nil or not.
	eitherNil nilness = iota
	// alwaysNil is the predeclared nil.
	alwaysNil
	// neverNil is a value of a type nil cannot be assigned to, which stays
	// non-nil when it is handed to a parameter of an interface type.
	neverNil
)

// nilnessOf reads an argument: the predeclared nil, a value of a type that
// cannot hold nil, and a parameter of a helper through what its caller handed
// it, as long as the helper leaves it as it was handed. Anything else that can
// hold nil, a pointer field among them, may be either: a nil pointer handed to
// an interface parameter is an interface that is not nil, which only says the
// reading stays on the side of deriving a way it cannot rule out. A value of a
// type parameter whose type set holds a type that cannot hold nil is not ruled
// out by that either, since the instantiation that reaches the call may be one
// that can.
func nilnessOf(expr ast.Expr, at *frame) nilness {
	expr = ast.Unparen(expr)
	value := at.pkg.TypesInfo.Types[expr]
	if value.IsNil() {
		return alwaysNil
	}
	_, isTypeParam := types.Unalias(value.Type).(*types.TypeParam)
	if !isTypeParam && !types.AssignableTo(types.Typ[types.UntypedNil], value.Type) {
		return neverNil
	}
	ident, isName := expr.(*ast.Ident)
	if !isName {
		return eitherNil
	}
	variable, _ := at.pkg.TypesInfo.Uses[ident].(*types.Var)
	if bound, isBound := at.env[variable]; isBound && !rewrites(at, variable) {
		return nilnessOf(bound.expr, bound.at)
	}
	return eitherNil
}

// rewrites reports whether a body can leave a variable holding something other
// than what it was handed: it assigns the variable, ranges into it, or takes
// its address.
func rewrites(at *frame, variable *types.Var) bool {
	if len(valuesOf(at, variable)) > 0 {
		return true
	}
	var written []ast.Expr
	ast.Inspect(at.body, func(n ast.Node) bool {
		switch typed := n.(type) {
		case *ast.RangeStmt:
			written = append(written, typed.Key, typed.Value)
		case *ast.UnaryExpr:
			if typed.Op == token.AND {
				written = append(written, typed.X)
			}
		}
		return true
	})
	return slices.ContainsFunc(written, func(expr ast.Expr) bool {
		ident, isName := expr.(*ast.Ident)
		return isName && at.pkg.TypesInfo.ObjectOf(ident) == variable
	})
}
