package main

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// maxFoldDepth bounds how far a fold follows one helper into another. Three is
// past anything this tree writes and keeps a pair of helpers that call each
// other from running forever.
const maxFoldDepth = 3

// foldCall folds a call of a module-declared helper to the string it returns,
// by binding the helper's parameters to the constants the caller passed.
//
// It exists for one shape that is common and that no amount of constant
// folding by the type checker reaches on its own: a package writes its IDs as
// canonicalID(actionNameTokenGet), where canonicalID returns catalogDomain +
// "." + actionName. The type checker folds neither side, because the value
// depends on a parameter; a scan over literals sees a bare action name with no
// domain in front of it, which is how a regex over this package reported five
// phantoms that were never there.
//
// Only a helper whose body is one return of one expression is folded. A helper
// that branches returns different strings on different paths, and picking one
// of them would be a guess reported as a fact.
// argBound are the bindings in force where the call is written, which is not
// the same scope the helper's body is read in: one helper calling another
// passes its own parameter along, and folding that argument without the
// caller's bindings loses the value one step in.
//
// The bound is applied by [walker.foldExpr] and not here, although the depth
// is carried through both. Every way into this function has been through that
// guard already: a site enters at depth zero, and a nested call is reached
// only by folding an expression, which refuses past the bound before it looks
// at what the expression is. A second copy of the test here would be a branch
// no depth can take, and it read as the bound while the one that enforces it
// sat one function away.
func (w *walker) foldCall(call *ast.CallExpr, argBound map[*types.Var]string, depth int) (string, bool) {
	callee, ok := w.callee(call)
	if !ok {
		return "", false
	}
	declared, found := w.prog.decls[callee]
	if !found {
		return "", false
	}
	result, ok := singleReturnExpr(declared.decl)
	if !ok {
		return "", false
	}
	signature, ok := callee.Type().(*types.Signature)
	if !ok || signature.Params().Len() != len(call.Args) {
		return "", false
	}
	bound := map[*types.Var]string{}
	for index := range signature.Params().Len() {
		value, isConstant := w.foldExpr(w.pkg, call.Args[index], argBound, depth+1)
		if !isConstant {
			continue
		}
		bound[signature.Params().At(index)] = value
	}
	return w.foldExpr(declared.pkg, result, bound, depth+1)
}

// singleReturnExpr is the one expression a one-line helper returns, or nothing
// when the body is anything else.
func singleReturnExpr(decl *ast.FuncDecl) (ast.Expr, bool) {
	if decl.Body == nil || len(decl.Body.List) != 1 {
		return nil, false
	}
	ret, ok := decl.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil, false
	}
	return ret.Results[0], true
}

// foldExpr folds an expression to the string it denotes, with the caller's
// parameter bindings in scope.
//
// The type checker answers first and answers most of it: a literal, a
// constant, and a concatenation of constants all fold there. What is left is
// exactly what a parameter makes non-constant, so only the three shapes a
// concatenation can take are handled here: parentheses, a plus, and an
// identifier that names a bound parameter or another helper's call.
func (w *walker) foldExpr(pkg *packages.Package, expr ast.Expr, bound map[*types.Var]string, depth int) (string, bool) {
	if depth > maxFoldDepth {
		return "", false
	}
	if value, ok := constantStringIn(pkg, expr); ok {
		return value, true
	}
	switch typed := expr.(type) {
	case *ast.ParenExpr:
		return w.foldExpr(pkg, typed.X, bound, depth)
	case *ast.BinaryExpr:
		if typed.Op != token.ADD {
			return "", false
		}
		left, leftOK := w.foldExpr(pkg, typed.X, bound, depth)
		right, rightOK := w.foldExpr(pkg, typed.Y, bound, depth)
		if !leftOK || !rightOK {
			return "", false
		}
		return left + right, true
	case *ast.Ident:
		variable, isVar := pkg.TypesInfo.Uses[typed].(*types.Var)
		if !isVar {
			return "", false
		}
		value, isBound := bound[variable]
		return value, isBound
	case *ast.CallExpr:
		return w.inPackage(pkg).foldCall(typed, bound, depth)
	default:
		return "", false
	}
}

// constantStringIn folds an expression to the string constant it denotes in
// the package it was written in.
func constantStringIn(pkg *packages.Package, expr ast.Expr) (string, bool) {
	typed, ok := pkg.TypesInfo.Types[expr]
	if !ok || typed.Value == nil {
		return "", false
	}
	return stringValue(typed.Value)
}
