package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"testing"
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
