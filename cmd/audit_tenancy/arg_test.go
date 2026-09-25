package main

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// argSite names the index'th argument of count calls to call inside the site
// package's function name.
func argSite(name, call string, index, count int, reads string) tenancy.Site {
	return tenancy.Site{Pkg: siteDir, Name: name, Role: tenancy.Arg, Call: call, Arg: index, Count: count, Reads: reads}
}

// argSource has the two shapes an Arg site takes: a package function, and a
// method named with its receiver's type.
const argSource = siteHeader + `
func parse(s string, fallback float64) float64 { return fallback }

type loader struct{}

func (loader) parse(s string, fallback float64) float64 { return fallback }

func Load() {
	through := func(s string) float64 { return 0 }
	_ = through("value")
	_ = parse("a", leaf.Ratio)
	_ = parse("b", leaf.Ratio)
}

func Method(l loader) {
	_ = l.parse("a", leaf.Ratio)
}

func Literal() {
	_ = parse("a", 0)
}

func scale(ratio float64, s string) float64 { return ratio }

func First() {
	_ = scale(leaf.Ratio, "x")
}

var notAFunction = 1
`

// TestCheckArgs_TheConstantAtTheIndexTheRightNumberOfTimes_Passes, the first
// argument and the last alike.
func TestCheckArgs_TheConstantAtTheIndexTheRightNumberOfTimes_Passes(t *testing.T) {
	report := fixture{
		files: map[string]string{"site/site.go": argSource},
		rows: []tenancy.Decision{row("ROW-001",
			argSite("Load", "parse", 1, 2, "Ratio"), argSite("Method", "loader.parse", 1, 1, "Ratio"),
			argSite("First", "scale", 0, 1, "Ratio"))},
	}.run(t)
	assertFindings(t, report, "G3")
}

// TestCheckArgs_WhatIsNotTheRegistersArgument_IsAFinding: a literal in the
// position, a different number of calls, an index past the arguments, a
// register constant that does not exist and a site that is not a function
// each fail.
func TestCheckArgs_WhatIsNotTheRegistersArgument_IsAFinding(t *testing.T) {
	report := fixture{
		files: map[string]string{"site/site.go": argSource},
		rows: []tenancy.Decision{row("ROW-001",
			argSite("Literal", "parse", 1, 1, "Ratio"),
			argSite("Load", "parse", 1, 1, "Ratio"),
			argSite("Method", "loader.parse", 4, 1, "Ratio"),
			argSite("Method", "loader.parse", -1, 1, "Ratio"),
			argSite("Method", "loader.parse", 2, 1, "Ratio"),
			argSite("Load", "parse", 1, 2, "Missing"),
			argSite("notAFunction", "parse", 1, 1, "Ratio"),
			argSite("gone", "parse", 1, 1, "Ratio"))},
	}.run(t)
	assertFindings(t, report, "G3",
		"ROW-001: "+siteDir+":Literal argument 1 of parse is the literal 0 rather than Ratio",
		"ROW-001: "+siteDir+":Load calls parse 2 times, and the register says 1",
		"ROW-001: "+siteDir+":Load passes Missing, which is not a constant of the register",
		"ROW-001: "+siteDir+":Method calls loader.parse with 2 arguments, so it has none at index 4",
		"ROW-001: "+siteDir+":Method calls loader.parse with 2 arguments, so it has none at index -1",
		"ROW-001: "+siteDir+":Method calls loader.parse with 2 arguments, so it has none at index 2",
		"ROW-001: "+siteDir+":notAFunction is not a function, so it has no call to parse",
	)
}

// TestCheckArgs_APendingRow_IsDeferred.
func TestCheckArgs_APendingRow_IsDeferred(t *testing.T) {
	report := fixture{
		files:   map[string]string{"site/site.go": argSource},
		rows:    []tenancy.Decision{row("ROW-001", argSite("Literal", "parse", 1, 1, "Ratio"))},
		pending: []string{"ROW-001"},
	}.run(t)
	assertFindings(t, report, "G3")
}
