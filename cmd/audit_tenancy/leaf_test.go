package main

import (
	"testing"
)

// TestCheckLeaf_ARegisterTheServerImportsForFree_Passes: the fixture leaf
// imports time alone, which the package standing in for the server imports
// too, and declares only constants.
func TestCheckLeaf_ARegisterTheServerImportsForFree_Passes(t *testing.T) {
	report := fixture{files: map[string]string{"site/site.go": siteHeader}}.run(t)
	assertFindings(t, report, "G12")
}

// TestCheckLeaf_ARegisterThatCostsTheServerSomething_IsAFinding: an import
// the rules do not allow, an allowed import the server does not already
// import, and a package-level variable each fail.
func TestCheckLeaf_ARegisterThatCostsTheServerSomething_IsAFinding(t *testing.T) {
	leafExtra := `package leaf

import (
	"fmt"
	"os"
)

var Loaded = fmt.Sprint(os.Args)
`
	report := fixture{files: map[string]string{"site/site.go": siteHeader, "leaf/extra.go": leafExtra}}.run(t)
	assertFindings(t, report, "G12",
		leafDir+": declares a package-level variable, which is initialization work in every binary that imports the register",
		leafDir+": imports fmt, which "+siteDir+" does not import itself",
		leafDir+": imports os, and the register may import only [errors fmt strings time]",
	)
}

// TestCheckLeaf_AServerThatIsNotLoaded_LinksNothing: when the package named
// as the server is not in the program, every import of the register is one it
// does not link.
func TestCheckLeaf_AServerThatIsNotLoaded_LinksNothing(t *testing.T) {
	r := fixtureRules()
	r.server = "cmd/nowhere"
	report := fixture{files: map[string]string{"site/site.go": siteHeader}, rules: &r}.run(t)
	assertFindings(t, report, "G12", leafDir+": imports time, which cmd/nowhere does not import itself")
}
