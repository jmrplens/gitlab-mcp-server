package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestEnclosing_TheDeclarationIsHalfOpen holds enclosing to the span a
// function declaration covers, from its func keyword up to and not including
// the position after its closing brace, with no GitLab and no module: a site
// is attributed to the function it sits in, and a position on either side of
// a declaration to none.
//
// The scanner never asks about a position on either edge, since a site is an
// expression inside a body, so the edges are asked here directly: a
// declaration's own func keyword is inside it, and the first position after
// it belongs to whatever follows.
func TestEnclosing_TheDeclarationIsHalfOpen(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sites.go", "package sites\n\nfunc first() {}\n\nvar between = 1\n\nfunc second() {}\n", 0)
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	var decls []*ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			decls = append(decls, fn)
		}
	}
	first, second := decls[0], decls[1]
	tests := []struct {
		name string
		pos  token.Pos
		want *ast.FuncDecl
	}{
		{name: "the func keyword of the first", pos: first.Pos(), want: first},
		{name: "the closing brace of the first", pos: first.End() - 1, want: first},
		{name: "just past the first", pos: first.End(), want: nil},
		{name: "before the first", pos: first.Pos() - 1, want: nil},
		{name: "the func keyword of the second", pos: second.Pos(), want: second},
		{name: "just past the second", pos: second.End(), want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := enclosing(decls, tt.pos); got != tt.want {
				t.Errorf("enclosing(%s) = %v, want %v", fset.Position(tt.pos), declNameOrNone(got), declNameOrNone(tt.want))
			}
		})
	}
}

// declNameOrNone names a declaration for a failure message, and says none
// when there is none.
func declNameOrNone(decl *ast.FuncDecl) string {
	if decl == nil {
		return "none"
	}
	return decl.Name.Name
}
