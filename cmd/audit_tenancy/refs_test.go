package main

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// readingSite is an Enforce site whose body must read a register constant.
func readingSite(name, reads string) tenancy.Site {
	s := site(name, tenancy.Enforce)
	s.Reads = reads
	return s
}

// refsLeafRules is a register function a row can name, standing in for the
// promoted rules.
const refsLeafRules = `package leaf

func Busy(n int) bool { return n > Limit }

func Unused() bool { return false }
`

// refsSource reads the register directly, through a declared alias, and not
// at all, and calls the register function from one place.
const refsSource = siteHeader + `
const limit = leaf.Limit

func Direct(n int) bool { return n > leaf.Limit }

func Aliased(n int) bool { return n > limit }

func Neither(n int) bool { return n > 3 }

func Promoted(n int) bool { return leaf.Busy(n) }

type Holder struct{}
`

// TestCheckRefs_AnEnforcingSiteThatReadsItsValue_Passes: directly, and
// through an alias the register declares; and a register function a row
// names is called from one of its Enforce sites.
func TestCheckRefs_AnEnforcingSiteThatReadsItsValue_Passes(t *testing.T) {
	d := row("ROW-001", aliasSite("limit", "Limit"), readingSite("Direct", "Limit"), readingSite("Aliased", "Limit"),
		site("Holder", tenancy.Enforce), site("gone", tenancy.Enforce), readingSite("alsoGone", "Limit"), site("Promoted", tenancy.Enforce))
	d.Functions = []string{"Busy"}
	report := fixture{
		files: map[string]string{"site/site.go": refsSource, "leaf/rules.go": refsLeafRules},
		rows:  []tenancy.Decision{d},
	}.run(t)
	assertFindings(t, report, "G5")
}

// TestCheckRefs_AnAnswerNobodyConsults_IsAFinding: an Enforce site that does
// not read its value, one naming a value the register lacks, one with no body
// to read, a function no Enforce site calls and a function the register lacks
// each fail.
func TestCheckRefs_AnAnswerNobodyConsults_IsAFinding(t *testing.T) {
	d := row("ROW-001", readingSite("Neither", "Limit"), readingSite("Direct", "Missing"), readingSite("Holder", "Limit"),
		site("Direct", tenancy.Enforce))
	d.Functions = []string{"Unused", "Absent"}
	report := fixture{
		files: map[string]string{"site/site.go": refsSource, "leaf/rules.go": refsLeafRules},
		rows:  []tenancy.Decision{d},
	}.run(t)
	assertFindings(t, report, "G5",
		"ROW-001: "+siteDir+":Direct is declared to read Missing, which is not a constant of the register",
		"ROW-001: "+siteDir+":Holder does not refer to Limit, or to a declared alias of it",
		"ROW-001: "+siteDir+":Neither does not refer to Limit, or to a declared alias of it",
		"ROW-001: names Absent, which is not a function of the register",
		"ROW-001: names Unused, and none of its Enforce sites calls it",
	)
}
