package main

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// pinSite names a declaration of the site package pinned to a register
// constant.
func pinSite(name, reads string) tenancy.Site {
	return tenancy.Site{Pkg: siteDir, Name: name, Role: tenancy.Pin, Reads: reads}
}

// pinSource states values a second time, the way the pool's fallbacks state
// the configuration's defaults again.
const pinSource = siteHeader + `
const sameLimit = 64

const sameWindow = 30 * time.Second

var sameVar = 30 * time.Second

const otherLimit = 65

const typedLimit int = 64

const name = "x"

var notConstant = time.Now()

var noValue int

func notAValue() {}
`

// TestCheckPins_AnEqualValueOfEqualType_Passes, a const and a var.
func TestCheckPins_AnEqualValueOfEqualType_Passes(t *testing.T) {
	report := fixture{
		files: map[string]string{"site/site.go": pinSource},
		rows: []tenancy.Decision{row("ROW-001",
			pinSite("sameLimit", "Limit"), pinSite("sameWindow", "Window"), pinSite("sameVar", "Window"))},
	}.run(t)
	assertFindings(t, report, "G4")
}

// TestCheckPins_Drift_IsAFinding: a different value, a different type, a
// different kind of constant, a value that does not fold, a register constant
// that does not exist and a site holding no value each fail.
func TestCheckPins_Drift_IsAFinding(t *testing.T) {
	report := fixture{
		files: map[string]string{"site/site.go": pinSource},
		rows: []tenancy.Decision{row("ROW-001",
			pinSite("otherLimit", "Limit"), pinSite("typedLimit", "Limit"), pinSite("name", "Limit"),
			pinSite("notConstant", "Window"), pinSite("noValue", "Limit"), pinSite("notAValue", "Limit"),
			pinSite("sameLimit", "Missing"), pinSite("gone", "Limit"))},
	}.run(t)
	assertFindings(t, report, "G4",
		"ROW-001: "+siteDir+":name is \"x\" where Limit is 64",
		"ROW-001: "+siteDir+":noValue does not fold to a constant, so it cannot be held equal to Limit",
		"ROW-001: "+siteDir+":notAValue does not fold to a constant, so it cannot be held equal to Limit",
		"ROW-001: "+siteDir+":notConstant does not fold to a constant, so it cannot be held equal to Window",
		"ROW-001: "+siteDir+":otherLimit is 65 where Limit is 64",
		"ROW-001: "+siteDir+":sameLimit is pinned to Missing, which is not a constant of the register",
		"ROW-001: "+siteDir+":typedLimit is typed int where Limit is untyped int",
	)
}
