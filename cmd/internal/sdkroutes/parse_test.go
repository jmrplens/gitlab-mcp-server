package sdkroutes

import (
	"go/ast"
	"go/token"
	"reflect"
	"testing"
)

// TestStringLiteral_ShapesThatAreNoString verifies that only a well-formed
// string literal unquotes: a number, a name, and a literal whose quoting is
// broken, which the parser never produces and a hand-built tree can.
func TestStringLiteral_ShapesThatAreNoString(t *testing.T) {
	cases := map[string]ast.Expr{
		"number":       &ast.BasicLit{Kind: token.INT, Value: "42"},
		"name":         ast.NewIdent("routeProjects"),
		"broken quote": &ast.BasicLit{Kind: token.STRING, Value: `"open`},
	}
	for name, expr := range cases {
		t.Run(name, func(t *testing.T) {
			if value, ok := stringLiteral(expr); ok {
				t.Errorf("stringLiteral(%s) = %q, true; want false", name, value)
			}
		})
	}
	if value, ok := stringLiteral(&ast.BasicLit{Kind: token.STRING, Value: `"projects"`}); !ok || value != "projects" {
		t.Errorf("stringLiteral(\"projects\") = %q, %v; want projects, true", value, ok)
	}
}

// TestParseDir_RecordsTheDeclarationsTheBodiesAreReadAgainst verifies what the
// declaration pass keeps: route templates and string constants by name, a
// variable's initializer and not a constant's, and the fields of a struct but
// not of an alias.
func TestParseDir_RecordsTheDeclarationsTheBodiesAreReadAgainst(t *testing.T) {
	parsed := parseDir(fixtureDir)
	if got := parsed.templates["routeSearch"]; got != "search?scope=%s" {
		t.Errorf("templates[routeSearch] = %q, want the template route() registered", got)
	}
	for _, name := range []string{"notARoute", "tooManyArgs", "notALiteral", "pickedFromAList", "secondOfTwo"} {
		t.Run(name, func(t *testing.T) {
			if template, ok := parsed.templates[name]; ok {
				t.Errorf("templates[%s] = %q, want none", name, template)
			}
		})
	}
	if got := parsed.consts["ProjectResource"]; got != "projects" {
		t.Errorf("consts[ProjectResource] = %q, want projects", got)
	}
	if _, ok := parsed.consts["answer"]; ok {
		t.Error("consts holds answer, a constant that is no string")
	}
	if _, ok := parsed.vars["workItemTemplate"]; !ok {
		t.Error("vars has no workItemTemplate, whose initializer a document is written in")
	}
	for _, name := range []string{"listAchievementsQuery", "declaredWithoutValue"} {
		t.Run(name, func(t *testing.T) {
			if _, ok := parsed.vars[name]; ok {
				t.Errorf("vars holds %s, which has no initializer to follow", name)
			}
		})
	}
	if want := map[string]string{"client": "Client", "timeStats": "timeStatsService", "other": "Helper"}; !reflect.DeepEqual(parsed.fields["IssuesService"], want) {
		t.Errorf("fields[IssuesService] = %v, want %v", parsed.fields["IssuesService"], want)
	}
	if _, ok := parsed.fields["Count"]; ok {
		t.Error("fields holds Count, an alias with no fields")
	}
}

// TestParamNames_UnnamedParameterKeepsItsPlace verifies that a parameter with
// no name is still counted, so the arguments after it bind to the right
// names.
func TestParamNames_UnnamedParameterKeepsItsPlace(t *testing.T) {
	blank := parseDir(fixtureDir).funcs["blank"]
	if want := []string{unnamedParam, unnamedParam}; !reflect.DeepEqual(blank.params, want) {
		t.Errorf("params = %v, want %v", blank.params, want)
	}
}

// TestElementName_SpellingsOfAType verifies the type name read through every
// wrapper a signature writes.
func TestElementName_SpellingsOfAType(t *testing.T) {
	cases := []struct {
		name string
		expr ast.Expr
		want string
		many bool
	}{
		{name: "pointer", expr: &ast.StarExpr{X: ast.NewIdent("Issue")}, want: "Issue"},
		{name: "slice", expr: &ast.ArrayType{Elt: &ast.StarExpr{X: ast.NewIdent("Issue")}}, want: "Issue", many: true},
		{name: "instantiated", expr: &ast.IndexExpr{X: ast.NewIdent("Generic"), Index: ast.NewIdent("int")}, want: "Generic"},
		{name: "two type arguments", expr: &ast.IndexListExpr{X: ast.NewIdent("Pair")}, want: "Pair"},
		{name: "qualified", expr: &ast.SelectorExpr{X: ast.NewIdent("bytes"), Sel: ast.NewIdent("Buffer")}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, many := elementName(tc.expr)
			if got != tc.want || many != tc.many {
				t.Errorf("elementName() = %q, %v; want %q, %v", got, many, tc.want, tc.many)
			}
		})
	}
}
