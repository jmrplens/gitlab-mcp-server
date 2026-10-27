// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sourcegate

import "go/ast"

// FuncDeclName spells a function the way its file does: the bare name, or
// `Type.Method` with the receiver's type stripped of its pointer and of its
// type parameters. It is the function half of a declaration key, so a finding
// and the entry excusing it name the place the same way.
//
// The receiver is read through its parentheses, as the type checker reads it:
// `(w (walker))` and `(w *(walker))` declare methods of walker as much as
// `(w walker)` does, and a key that dropped the receiver there would excuse
// nothing a reader wrote for it. A receiver that is not a type name once those
// are gone, or a receiver list with nothing in it, does not type-check, and is
// spelled by the method's name alone rather than guessed at.
func FuncDeclName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	receiver := ast.Unparen(fn.Recv.List[0].Type)
	if star, isPointer := receiver.(*ast.StarExpr); isPointer {
		receiver = ast.Unparen(star.X)
	}
	switch generic := receiver.(type) {
	case *ast.IndexExpr:
		receiver = generic.X
	case *ast.IndexListExpr:
		receiver = generic.X
	}
	if ident, isIdent := receiver.(*ast.Ident); isIdent {
		return ident.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// IdentsAsExprs widens a name list so a declaration and an assignment are read
// by one function: the names a `var` or `const` spec declares become the
// left-hand side an assignment would have.
func IdentsAsExprs(names []*ast.Ident) []ast.Expr {
	exprs := make([]ast.Expr, len(names))
	for i, name := range names {
		exprs[i] = name
	}
	return exprs
}
