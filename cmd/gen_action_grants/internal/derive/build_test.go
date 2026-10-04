package derive

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
)

// TestBuilderDecl_ADeclarationThatIsNotGeneral_ReadsNothing verifies a
// declaration statement holding anything but a var, const or type
// declaration reads as sending nothing rather than failing the walk; the type
// checker admits no other inside a body, so the case is built by hand.
func TestBuilderDecl_ADeclarationThatIsNotGeneral_ReadsNothing(t *testing.T) {
	b := &builder{}
	if got := b.decl(&ast.DeclStmt{Decl: &ast.BadDecl{}}); !got.isEmpty() {
		t.Errorf("decl(BadDecl) = %+v, want an empty node", got)
	}
}

// TestBuilderRaw_ACallShortOfTheVerbOrPath_KeepsWhatItHas verifies a request
// constructor call carrying fewer arguments than its verb and path, which the
// type checker refuses in real source, leaves the missing ones unset rather
// than reading past the end.
func TestBuilderRaw_ACallShortOfTheVerbOrPath_KeepsWhatItHas(t *testing.T) {
	b := &builder{}
	constructor := types.NewFunc(token.NoPos, nil, "NewRequest", types.NewSignatureType(nil, nil, nil, nil, nil, false))
	verb := ast.NewIdent("verb")
	cases := []struct {
		name     string
		args     []ast.Expr
		wantVerb ast.Expr
	}{
		{name: "no arguments"},
		{name: "verb only", args: []ast.Expr{verb}, wantVerb: verb},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := b.raw(constructor, &ast.CallExpr{Args: testCase.args}, 0).leaf
			if got.verb != testCase.wantVerb || got.path != nil || got.toURL {
				t.Errorf("raw() = verb %v, path %v, toURL %t; want verb %v and no path", got.verb, got.path, got.toURL, testCase.wantVerb)
			}
		})
	}
}
