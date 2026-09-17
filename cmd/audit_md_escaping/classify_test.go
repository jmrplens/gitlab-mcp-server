package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// fixtureClassifier builds the classifier over the shared fixture.
func fixtureClassifier(t *testing.T) *classifier {
	t.Helper()
	return newClassifier(loadFixture(t, caseFixture))
}

// assignedNames renders what the classifier recorded for the fixture's
// variables: the variable's name against the expressions it holds, with an
// assignment the audit cannot follow written as a dash.
func assignedNames(t *testing.T, c *classifier) map[string][]string {
	t.Helper()
	recorded := map[string][]string{}
	for v, exprs := range c.assigns {
		if !strings.Contains(v.Pkg().Path(), fixtureDir) {
			continue
		}
		for _, expr := range exprs {
			if expr == nil {
				recorded[v.Name()] = append(recorded[v.Name()], "-")
				continue
			}
			recorded[v.Name()] = append(recorded[v.Name()], types.ExprString(expr))
		}
	}
	return recorded
}

// TestNewClassifier_Fixture_RecordsWhereAValueCanComeFrom checks each way the
// classifier learns what a variable holds, since a shape it does not index is
// a value it cannot follow.
func TestNewClassifier_Fixture_RecordsWhereAValueCanComeFrom(t *testing.T) {
	recorded := assignedNames(t, fixtureClassifier(t))

	cases := []struct {
		name     string
		variable string
		want     []string
	}{
		// go/types abbreviates a composite literal, which is what a finding
		// prints too: the value is named by the variable holding it.
		{name: "a short variable declaration", variable: "cells", want: []string{"[]string{…}"}},
		{name: "an escaped local reassigned from itself", variable: "name", want: []string{
			"toolutil.EscapeMdTableCell(item.Title)", "fmt.Sprintf(\"[%s](%s)\", name, item.URL)",
		}},
		{name: "a declaration with no value", variable: "mutableTitle", want: []string{"-"}},
		{name: "the value half of a comma-ok lookup", variable: "icon", want: []string{"icons[item.Title]"}},
		{name: "a result the audit cannot follow", variable: "value", want: []string{"-"}},
		{name: "a package-level table", variable: "icons", want: []string{"map[string]string{…}"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := recorded[tc.variable]
			if len(got) != len(tc.want) {
				t.Fatalf("%s holds %v, want %v", tc.variable, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("%s holds %q at %d, want %q", tc.variable, got[i], i, tc.want[i])
				}
			}
		})
	}
}

// TestNewClassifier_Fixture_RecordsFieldWrites checks the guard that keeps a
// struct filled after it was built from answering through the empty literal
// that built it.
func TestNewClassifier_Fixture_RecordsFieldWrites(t *testing.T) {
	c := fixtureClassifier(t)

	var found bool
	for v := range c.fieldWritten {
		if v.Name() == "pair" {
			found = true
		}
	}
	if !found {
		t.Error("no field write was recorded for the struct the fixture fills after building it")
	}
}

// TestNewClassifier_Fixture_RecordsParameters checks that a parameter is
// indexed against its position, which is what lets it be resolved to the
// arguments callers pass.
func TestNewClassifier_Fixture_RecordsParameters(t *testing.T) {
	c := fixtureClassifier(t)

	positions := map[string]int{}
	for v, ref := range c.params {
		if ref.fn.Name() == "FormatVariadic" {
			positions[v.Name()] = ref.index
		}
	}
	if positions["prefix"] != 0 || positions["rest"] != 1 {
		t.Errorf("FormatVariadic parameters indexed as %v, want prefix at 0 and rest at 1", positions)
	}
}

// TestNewClassifier_BlankParameter_KeepsThePositionsAfterIt checks that a
// parameter nobody named still occupies its place in the signature.
//
// Every parameter is resolved by counting arguments from the front of the
// call, so a blank one that did not take a position would shift every
// parameter after it onto the wrong argument, and the audit would answer for
// one value while reporting another.
func TestNewClassifier_BlankParameter_KeepsThePositionsAfterIt(t *testing.T) {
	c := newClassifier(loadFixture(t, edgeFixture))

	indexed := map[string]int{}
	for v, ref := range c.params {
		if ref.fn.Name() == "Ignored" {
			indexed[v.Name()] = ref.index
		}
	}

	if indexed["item"] != 1 {
		t.Errorf("Ignored's named parameter is at %d, want 1, after the blank one: %v", indexed["item"], indexed)
	}
}

// TestIndexParams_ParameterTheTypeCheckerDidNotRecord_IsNotIndexed checks the
// guard in front of the index.
//
// The index is keyed by the variable a parameter name defines, so a name with
// no variable behind it has nothing to key on, and recording it would put an
// entry under no variable at all that every later lookup would have to step
// over.
func TestIndexParams_ParameterTheTypeCheckerDidNotRecord_IsNotIndexed(t *testing.T) {
	c := newClassifier(&program{decls: map[*types.Func]*funcDecl{}, callers: map[*types.Func][]callSite{}})
	decl := &ast.FuncDecl{
		Name: ast.NewIdent("Unrecorded"),
		Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{
			{Names: []*ast.Ident{ast.NewIdent("left"), ast.NewIdent("right")}},
			{}, // an unnamed parameter, which takes a position and no name
		}}},
	}
	fn := types.NewFunc(token.NoPos, nil, "Unrecorded", types.NewSignatureType(nil, nil, nil, nil, nil, false))

	c.indexParams(untypedPackage(), fn, decl)

	if len(c.params) != 0 {
		t.Errorf("indexParams recorded %d parameter(s) the type checker gave no variable", len(c.params))
	}
}

// TestNoteFieldWrite_PackageQualifiedTarget_RecordsNothing checks the guard on
// an assignment whose target reads like a field of a variable and is not one.
//
// The base of a qualified name is a package rather than a value, so there is
// no variable whose fields were written, and recording one would be an entry
// against nothing.
func TestNoteFieldWrite_PackageQualifiedTarget_RecordsNothing(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	c := newClassifier(prog)
	pkg := fixturePackage(t, prog, "mdcase")
	qualified := packageQualifiedSelector(t, pkg)
	before := len(c.fieldWritten)

	c.noteFieldWrite(pkg, qualified)

	if len(c.fieldWritten) != before {
		t.Errorf("noteFieldWrite recorded %d write(s) for a package-qualified name, want none",
			len(c.fieldWritten)-before)
	}
}

// packageQualifiedSelector finds a selector in the fixture whose base names an
// imported package rather than a value, such as the strings in strings.Join.
func packageQualifiedSelector(t *testing.T, pkg *packages.Package) *ast.SelectorExpr {
	t.Helper()
	var found *ast.SelectorExpr
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			sel, ok := node.(*ast.SelectorExpr)
			if !ok || found != nil {
				return found == nil
			}
			base, isIdent := sel.X.(*ast.Ident)
			if !isIdent {
				return true
			}
			if _, isPkg := pkg.TypesInfo.Uses[base].(*types.PkgName); isPkg {
				found = sel
			}
			return true
		})
	}
	if found == nil {
		t.Fatal("the fixture names no imported package")
	}
	return found
}

// TestEnvLookup_WithoutBindings_FindsNothing checks both ways an environment
// can carry no binding: no environment at all, and one whose bindings were
// never built. Either has to answer that the parameter is unbound, so that it
// is resolved from every caller rather than from a map that is not there.
func TestEnvLookup_WithoutBindings_FindsNothing(t *testing.T) {
	variable := types.NewVar(token.NoPos, nil, "left", types.Typ[types.String])

	cases := []struct {
		name string
		env  *env
	}{
		{name: "no environment", env: nil},
		{name: "an environment with no bindings", env: &env{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, bound := tc.env.lookup(variable); bound {
				t.Error("lookup reported a binding")
			}
		})
	}
}

// TestCommaOK_Operands_RecognisesTheTwoValueForms checks which right-hand
// sides yield a value the audit can follow and which do not.
func TestCommaOK_Operands_RecognisesTheTwoValueForms(t *testing.T) {
	cases := []struct {
		name string
		expr ast.Expr
		want bool
	}{
		{name: "a map or slice index", expr: &ast.IndexExpr{X: ast.NewIdent("m"), Index: ast.NewIdent("k")}, want: true},
		{name: "a type assertion", expr: &ast.TypeAssertExpr{X: ast.NewIdent("v")}, want: true},
		{name: "a channel receive", expr: &ast.UnaryExpr{Op: token.ARROW, X: ast.NewIdent("ch")}, want: true},
		{name: "an address-of is not one", expr: &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent("v")}, want: false},
		{name: "a call is not one", expr: &ast.CallExpr{Fun: ast.NewIdent("f")}, want: false},
		{name: "a parenthesised index still is", expr: &ast.ParenExpr{X: &ast.IndexExpr{X: ast.NewIdent("m")}}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := commaOK(tc.expr); got != tc.want {
				t.Errorf("commaOK(%T) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

// TestLocalVar_Fixture_SkipsWhatCannotBeFollowed checks the targets the
// classifier refuses to record: the blank identifier, a field, and an index.
func TestLocalVar_Fixture_SkipsWhatCannotBeFollowed(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	c := newClassifier(prog)
	pkg := fixturePackage(t, prog, "mdcase")

	cases := []struct {
		name string
		expr ast.Expr
	}{
		{name: "the blank identifier", expr: ast.NewIdent("_")},
		{name: "a field", expr: &ast.SelectorExpr{X: ast.NewIdent("pair"), Sel: ast.NewIdent("Left")}},
		{name: "an index", expr: &ast.IndexExpr{X: ast.NewIdent("cells")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.localVar(pkg, tc.expr); got != nil {
				t.Errorf("localVar recorded %s as %v", tc.name, got)
			}
		})
	}

	// A field's own identifier is a variable to the type checker, and the one
	// thing that tells it from a variable the audit can follow backwards is
	// that it belongs to no scope. Following it would mean answering for a
	// field with whatever was assigned to a variable of the same name.
	t.Run("the identifier a field is named by", func(t *testing.T) {
		_, expr := holeByExpression(t, prog, "mdcase", "item.Title")
		sel, ok := expr.(*ast.SelectorExpr)
		if !ok {
			t.Fatalf("expected a selector, got %T", expr)
		}
		if got := c.localVar(pkg, sel.Sel); got != nil {
			t.Errorf("localVar recorded the field %s as %v", sel.Sel.Name, got)
		}
	})
}

// TestEscapers_Set_RefusesStripControlBytes checks the one helper that must
// not be read as an answer: it drops the control bytes and leaves both the
// pipe and the angle bracket, so accepting it would pass exactly the values
// this audit exists to find.
func TestEscapers_Set_RefusesStripControlBytes(t *testing.T) {
	if escapers["StripControlBytes"] {
		t.Error("StripControlBytes is accepted as an escaper")
	}
	for _, name := range []string{"EscapeMdTableCell", "EscapeMdHeading", "MdTitleLink"} {
		t.Run(name, func(t *testing.T) {
			if !escapers[name] {
				t.Errorf("%s is not accepted as an escaper", name)
			}
		})
	}
}
