package main

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// aliasSite names a declaration of the site package as an Alias of a register
// constant.
func aliasSite(name, reads string) tenancy.Site {
	return tenancy.Site{Pkg: siteDir, Name: name, Role: tenancy.Alias, Reads: reads}
}

// elementSite names one element of a composite literal as an Alias.
func elementSite(name string, index int, reads string) tenancy.Site {
	s := aliasSite(name, reads)
	s.Arg = index
	return s
}

// TestCheckAliases_AReferenceToTheRegister_Passes: a const and a var whose
// initializer is the register constant, a const that reads another declared
// alias of it, and a literal whose every element does, are all aliases.
func TestCheckAliases_AReferenceToTheRegister_Passes(t *testing.T) {
	source := siteHeader + `
const direct = leaf.Limit

const chained = direct

var window = leaf.Window

var ratio float64 = leaf.Ratio

var ladder = [...]int{leaf.Limit, 1: leaf.Limit}
`
	report := fixture{
		files: map[string]string{"site/site.go": source},
		rows: []tenancy.Decision{row("ROW-001",
			aliasSite("direct", "Limit"), aliasSite("chained", "Limit"), aliasSite("window", "Window"),
			aliasSite("ratio", "Ratio"), elementSite("ladder", 0, "Limit"), elementSite("ladder", 1, "Limit"))},
	}.run(t)
	assertFindings(t, report, "G2")
}

// TestCheckAliases_WhatIsNotAnAlias_IsAFinding: a literal, another constant,
// an expression, a changed typedness, a register constant that does not
// exist, a function, and a literal element that is not declared or not an
// alias each fail.
func TestCheckAliases_WhatIsNotAnAlias_IsAFinding(t *testing.T) {
	source := siteHeader + `
const literal = 64

const other = 8

const readsOther = other

const expression = leaf.Limit + 1

const typed int = leaf.Limit

func notAValue() {}

var ladder = [...]int{leaf.Limit, 7}

const otherPackage = time.Second
`
	report := fixture{
		files: map[string]string{"site/site.go": source},
		rows: []tenancy.Decision{row("ROW-001",
			aliasSite("literal", "Limit"), aliasSite("readsOther", "Limit"), aliasSite("expression", "Limit"),
			aliasSite("typed", "Limit"), aliasSite("literal", "Missing"), aliasSite("notAValue", "Limit"),
			aliasSite("gone", "Limit"), elementSite("ladder", 0, "Limit"), elementSite("ladder", 5, "Limit"),
			elementSite("ladder", -1, "Limit"), elementSite("ladder", 2, "Limit"), aliasSite("otherPackage", "Window"))},
	}.run(t)
	assertFindings(t, report, "G2",
		"ROW-001: "+siteDir+":expression is the expression leaf.Limit + 1 rather than a reference to Limit",
		"ROW-001: "+siteDir+":ladder names element -1 of a literal with 2 elements",
		"ROW-001: "+siteDir+":otherPackage reads time.Second rather than Window",
		"ROW-001: "+siteDir+":ladder element 1 is not declared as an alias: every element of an aliased literal is one",
		"ROW-001: "+siteDir+":ladder names element 5 of a literal with 2 elements",
		"ROW-001: "+siteDir+":ladder names element 2 of a literal with 2 elements",
		"ROW-001: "+siteDir+":literal aliases Missing, which is not a constant of the register",
		"ROW-001: "+siteDir+":literal is the literal 64 rather than Limit",
		"ROW-001: "+siteDir+":notAValue is not a const or var with an initializer of its own, so it cannot alias Limit",
		"ROW-001: "+siteDir+":readsOther reads other rather than Limit",
		"ROW-001: "+siteDir+":typed is typed int where Limit is untyped int: an alias keeps the typedness of the literal it replaced",
	)
}

// TestCheckAliases_AnElementThatIsNotAnAlias_IsAFinding: an element holding a
// literal where the register names it an alias.
func TestCheckAliases_AnElementThatIsNotAnAlias_IsAFinding(t *testing.T) {
	source := siteHeader + `
var ladder = [...]int{1, leaf.Limit}
`
	report := fixture{
		files: map[string]string{"site/site.go": source},
		rows:  []tenancy.Decision{row("ROW-001", elementSite("ladder", 0, "Limit"), elementSite("ladder", 1, "Limit"))},
	}.run(t)
	assertFindings(t, report, "G2", "ROW-001: "+siteDir+":ladder element 0 is the literal 1 rather than Limit")
}

// TestCheckAliases_APendingRow_IsDeferred: a row whose values have not moved
// yet is not held to G2, and every other rule still applies to it.
func TestCheckAliases_APendingRow_IsDeferred(t *testing.T) {
	source := siteHeader + `
const literal = 64
`
	report := fixture{
		files:   map[string]string{"site/site.go": source},
		rows:    []tenancy.Decision{row("ROW-001", aliasSite("literal", "Limit"), site("gone", tenancy.Enforce))},
		pending: []string{"ROW-001"},
	}.run(t)
	assertFindings(t, report, "G2")
	assertFindings(t, report, "G1", "ROW-001: names "+siteDir+":gone, which matches nothing: "+siteDir+" declares nothing named gone")
}

// TestIsRegisterValue_NeedsTheRegistersConstant: a register function of the
// right name is not the value, and neither is a constant of another package
// that happens to share its name.
func TestIsRegisterValue_NeedsTheRegistersConstant(t *testing.T) {
	source := siteHeader + `
const Limit = 64

var byName = Limit
`
	p := programOf(t, fixture{files: map[string]string{"site/site.go": source, "leaf/rules.go": "package leaf\n\nfunc Busy() bool { return false }\n"}})
	g := &gate{p: p, reg: register{leaf: leafDir}, aliases: map[string]map[string]bool{}}
	leafPkg := p.byDir[leafDir].Types.Scope()
	if g.isRegisterValue(leafPkg.Lookup("Busy"), "Busy") {
		t.Fatal("a register function counted as a register value")
	}
	if g.isRegisterValue(p.byDir[siteDir].Types.Scope().Lookup("Limit"), "Limit") {
		t.Fatal("a constant of the site package counted as the register's")
	}
	if !g.isRegisterValue(leafPkg.Lookup("Limit"), "Limit") {
		t.Fatal("the register's own constant did not count")
	}
	if g.leafConst("") != nil || g.leafFunc("") != nil {
		t.Fatal("an empty name found a register declaration")
	}
	unloaded := &gate{p: p, reg: register{leaf: "internal/nowhere"}}
	if unloaded.leafConst("Limit") != nil || unloaded.leafFunc("Busy") != nil {
		t.Fatal("a register that is not loaded answered a lookup")
	}
}
