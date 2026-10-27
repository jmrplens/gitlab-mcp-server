// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sourcegate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"
)

// TestFuncDeclName_SpellsTheReceiverAsTheFileDoes covers every receiver form
// a declaration key has to name: none, a value, a pointer, a parenthesized one
// with and without a pointer, a generic type with one parameter and with two,
// and a receiver that is not a type name, which keeps the method's name alone.
func TestFuncDeclName_SpellsTheReceiverAsTheFileDoes(t *testing.T) {
	const source = `package p

func Plain() {}
func (T) Value() {}
func (t *T) Pointer() {}
func ((T)) Parenthesized() {}
func (w *(T)) PointerToParenthesized() {}
func (g G[A]) OneParameter() {}
func (g *G[A, B]) TwoParameters() {}
func (x pkg.T) Qualified() {}
`
	file, err := parser.ParseFile(token.NewFileSet(), "p.go", source, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var got []string
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc {
			t.Fatalf("declaration %T is not a function", decl)
		}
		got = append(got, FuncDeclName(fn))
	}
	want := []string{
		"Plain", "T.Value", "T.Pointer", "T.Parenthesized", "T.PointerToParenthesized",
		"G.OneParameter", "G.TwoParameters", "Qualified",
	}
	if !slices.Equal(got, want) {
		t.Errorf("names = %v, want %v", got, want)
	}
}

// TestFuncDeclName_AnEmptyReceiverList_KeepsTheMethodName verifies a receiver
// list with nothing in it, which no load that type-checked hands over, is
// spelled by the method's name rather than read past its end.
func TestFuncDeclName_AnEmptyReceiverList_KeepsTheMethodName(t *testing.T) {
	fn := &ast.FuncDecl{Name: ast.NewIdent("Method"), Recv: &ast.FieldList{}}
	if got := FuncDeclName(fn); got != "Method" {
		t.Errorf("FuncDeclName = %q, want Method", got)
	}
}

// TestIdentsAsExprs_KeepsEveryNameInOrder verifies the widened list holds the
// very identifiers it was given, in their order, and that no names give an
// empty list.
func TestIdentsAsExprs_KeepsEveryNameInOrder(t *testing.T) {
	names := []*ast.Ident{ast.NewIdent("a"), ast.NewIdent("b")}
	exprs := IdentsAsExprs(names)
	if len(exprs) != len(names) {
		t.Fatalf("IdentsAsExprs gave %d expressions, want %d", len(exprs), len(names))
	}
	for i, expr := range exprs {
		if expr != names[i] {
			t.Errorf("expression %d = %v, want the identifier %v itself", i, expr, names[i])
		}
	}
	if empty := IdentsAsExprs(nil); len(empty) != 0 {
		t.Errorf("IdentsAsExprs(nil) = %v, want none", empty)
	}
}
