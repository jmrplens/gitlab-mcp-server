package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// gateSource is the fixture's own gate: a failure type, the budget whose spend
// is the lower charge, and the charge helper, the one declaration allowed to
// spend it. The functions under test are appended to it.
const gateSource = siteHeader + `
type gateFailure struct {
	status  int
	code    int
	message string
	header  map[string]string
}

func newHeader(pairs ...string) map[string]string { return map[string]string{} }

type budget struct{}

func (b *budget) spend(key string) {}

type gate struct {
	b    *budget
	pick func() *gateFailure
}

func (g *gate) charge(key string) { g.b.spend(key) }

func (g *gate) rejected(message string) *gateFailure {
	return &gateFailure{status: 401, code: -40100, message: message}
}

func (g *gate) upstream() *gateFailure {
	return &gateFailure{status: 503, code: -50300, message: "Upstream down."}
}

func describe(n int) string { return "" }
`

// resolveSource returns five refusals from five kinds of block, two of them
// charged: a literal, a literal whose text does not fold, and two built by
// constructors, one taking its text as an argument.
const resolveBody = `
func (g *gate) resolve(token string, n int) (int, *gateFailure) {
	if n == 1 {
		return 0, &gateFailure{status: 429, code: -42900, message: "Too many attempts."}
	} else if token == "" {
		(g.charge(token))
		return 0, &gateFailure{status: 401, code: -40100, message: "Authentication required."}
	}
	switch n {
	case 2:
		return 0, &gateFailure{status: 400, code: -32600, message: describe(n)}
	case 3:
		g.charge(token)
		return 0, g.rejected("Rejected token.")
	}
	for i := 0; i < n; i++ {
		if i == 5 {
			return 0, g.upstream()
		}
	}
	return n, nil
}
`

// resolveAt is the failure table's function, charging through the helper.
func resolveAt() tenancy.Site {
	return tenancy.Site{Pkg: siteDir, Name: "gate.resolve", Role: tenancy.Charge, Call: "gate.charge", Count: 2}
}

// resolveFailures are the five rows resolveBody is held to.
func resolveFailures() []tenancy.Failure {
	at := resolveAt()
	return []tenancy.Failure{
		{Kind: "blocked", At: at, Status: 429, Prefix: "Too many attempts."},
		{Kind: "missing", Attributable: true, Charged: true, At: at, Status: 401, Prefix: "Authentication required."},
		{Kind: "invalid", At: at, Status: 400},
		{Kind: "rejected", Attributable: true, Charged: true, At: at, Status: 401, Prefix: "Rejected token."},
		{Kind: "upstream", At: at, Status: 503, Prefix: "Upstream down."},
	}
}

// TestCheckCharges_TheCodeChargesWhatTheTableSays_Passes: every return is
// matched to its row, the two charged ones by the helper call before them in
// their own block, and every charge call is placed.
func TestCheckCharges_TheCodeChargesWhatTheTableSays_Passes(t *testing.T) {
	report := fixture{files: map[string]string{"site/site.go": gateSource + resolveBody}, fails: resolveFailures()}.run(t)
	assertFindings(t, report, "G7")
	if report.Summary.Returns != 5 {
		t.Fatalf("returns read = %d, want 5", report.Summary.Returns)
	}
}

// TestCheckCharges_AChargeMovedBetweenBranches_FailsTwice: the branch the
// charge left is charged in the table and not in the code, and the branch it
// reached the reverse.
func TestCheckCharges_AChargeMovedBetweenBranches_FailsTwice(t *testing.T) {
	moved := strings.Replace(resolveBody, "if n == 1 {\n", "if n == 1 {\n\t\tg.charge(token)\n", 1)
	moved = strings.Replace(moved, "\t\t(g.charge(token))\n", "", 1)
	report := fixture{files: map[string]string{"site/site.go": gateSource + moved}, fails: resolveFailures()}.run(t)
	assertFindings(t, report, "G7",
		siteDir+":gate.resolve blocked: is charged here, and the failure table says uncharged",
		siteDir+":gate.resolve missing: is uncharged here, and the failure table says charged",
	)
}

// TestCheckCharges_WhatTheTableAndTheCodeDoNotShare_IsAFinding: a return no
// row matches, a row no return matches, and a charge in a block no refusal
// return follows (inside a loop, and inside a function literal) each fail.
func TestCheckCharges_WhatTheTableAndTheCodeDoNotShare_IsAFinding(t *testing.T) {
	body := `
func (g *gate) resolve(token string, n int) (int, *gateFailure) {
	for _, c := range token {
		if c == 'x' {
			g.charge(token)
		}
	}
	go func() { g.charge(token) }()
	if n == 7 {
		return 0, &gateFailure{status: 418, message: "Teapot."}
	}
	return 0, &gateFailure{status: 429, code: -42900, message: "Too many attempts."}
}
`
	at := resolveAt()
	report := fixture{
		files: map[string]string{"site/site.go": gateSource + body},
		fails: []tenancy.Failure{
			{Kind: "blocked", At: at, Status: 429, Prefix: "Too many attempts."},
			{Kind: "gone", At: at, Status: 404, Prefix: "Gone."},
		},
	}.run(t)
	assertFindings(t, report, "G7",
		siteDir+":gate.resolve: charges a failure in a block no refusal return follows, so no row of the table can say it is charged",
		siteDir+":gate.resolve: charges a failure in a block no refusal return follows, so no row of the table can say it is charged",
		siteDir+":gate.resolve: returns a 418 refusal beginning \"Teapot.\" that no row of the failure table matches",
		siteDir+":gate.resolve gone: is a 404 refusal beginning \"Gone.\" in the failure table that no return of "+siteDir+":gate.resolve matches",
	)
}

// TestCheckCharges_EveryKindOfBlock_IsWalked: a refusal returned from a
// switch, a type switch, a select, a labeled loop and a bare block is found,
// and a charge before it in its own block marks it.
func TestCheckCharges_EveryKindOfBlock_IsWalked(t *testing.T) {
	body := `
func (g *gate) resolve(token string, v any, ch chan int) *gateFailure {
	<-ch
	switch v.(type) {
	case int:
		g.charge(token)
		return &gateFailure{status: 401, code: -40100, message: "Typed."}
	}
	select {
	case <-ch:
		return &gateFailure{status: 400, code: -32600, message: "Selected."}
	}
outer:
	for {
		{
			return &gateFailure{status: 429, code: -42900, message: "Labeled."}
		}
		break outer
	}
	return nil
}
`
	at := tenancy.Site{Pkg: siteDir, Name: "gate.resolve", Role: tenancy.Charge, Call: "gate.charge", Count: 1}
	report := fixture{
		files: map[string]string{"site/site.go": gateSource + body},
		fails: []tenancy.Failure{
			{Kind: "typed", Charged: true, Attributable: true, At: at, Status: 401, Prefix: "Typed."},
			{Kind: "selected", At: at, Status: 400, Prefix: "Selected."},
			{Kind: "labeled", At: at, Status: 429, Prefix: "Labeled."},
		},
	}.run(t)
	assertFindings(t, report, "G7")
	if report.Summary.Returns != 3 {
		t.Fatalf("returns read = %d, want 3", report.Summary.Returns)
	}
}

// TestCheckCharges_ARefusalTheGateCannotRead_IsAFinding: a refusal from a
// function value, from a variable, from a call that returns every result at
// once, and from a constructor that builds two, cannot be matched to a row,
// and saying so is the answer rather than passing over them.
func TestCheckCharges_ARefusalTheGateCannotRead_IsAFinding(t *testing.T) {
	body := `
func (g *gate) either(n int) *gateFailure {
	if n > 0 {
		return &gateFailure{status: 400}
	}
	return &gateFailure{status: 401}
}

func pair() (int, *gateFailure) { return 0, nil }

func (g *gate) resolve(n int, failure *gateFailure, failures chan *gateFailure) (int, *gateFailure) {
	switch n {
	case 1:
		return 0, g.pick()
	case 2:
		return 0, failure
	case 3:
		return pair()
	case 4:
		return 0, g.either(n)
	case 5:
		return 0, <-failures
	}
	return 0, nil
}
`
	report := fixture{
		files: map[string]string{"site/site.go": gateSource + body},
		fails: []tenancy.Failure{{Kind: "none", At: resolveAt(), Status: 400}},
	}.run(t)
	assertFindings(t, report, "G7",
		siteDir+":gate.resolve: returns a refusal from a call the gate cannot follow",
		siteDir+":gate.resolve: returns a refusal the gate cannot read",
		siteDir+":gate.resolve: returns a refusal the gate cannot read",
		siteDir+":gate.resolve: returns what the gate cannot split into its results",
		siteDir+":gate.resolve: returns a refusal from "+siteDir+":gate.either, which builds 2 gate refusals rather than one",
		siteDir+":gate.resolve none: is a 400 refusal beginning \"\" in the failure table that no return of "+siteDir+":gate.resolve matches",
	)
}

// TestCheckCharges_AConstructorGivenNoTextOfItsOwn_ReadsNone: a constructor
// handed a text that does not fold, and holding none of its own, returns a
// refusal with no text, which only a row naming no prefix matches.
func TestCheckCharges_AConstructorGivenNoTextOfItsOwn_ReadsNone(t *testing.T) {
	body := `
func (g *gate) resolve(n int) *gateFailure {
	return g.rejected(describe(n))
}
`
	at := tenancy.Site{Pkg: siteDir, Name: "gate.resolve", Role: tenancy.Charge, Call: "gate.charge"}
	report := fixture{
		files: map[string]string{"site/site.go": gateSource + body},
		fails: []tenancy.Failure{{Kind: "rejected", At: at, Status: 401}},
	}.run(t)
	assertFindings(t, report, "G7")
	if report.Summary.Returns != 1 {
		t.Fatalf("returns read = %d, want 1", report.Summary.Returns)
	}
}

// TestCheckCharges_ADelegatedAnswer_IsNotARefusalOfItsOwn: check returning
// classify's answer, where the table names classify, is classify's refusal
// and matches none of check's rows.
func TestCheckCharges_ADelegatedAnswer_IsNotARefusalOfItsOwn(t *testing.T) {
	body := `
func (g *gate) classify() *gateFailure {
	return &gateFailure{status: 503, code: -50300, message: "Upstream down."}
}

func (g *gate) check(n int) *gateFailure {
	if n > 0 {
		return g.classify()
	}
	return &gateFailure{status: 400, code: -32600, message: "Bad."}
}
`
	check := tenancy.Site{Pkg: siteDir, Name: "gate.check", Role: tenancy.Charge, Call: "gate.charge"}
	classify := tenancy.Site{Pkg: siteDir, Name: "gate.classify", Role: tenancy.Charge, Call: "gate.charge"}
	report := fixture{
		files: map[string]string{"site/site.go": gateSource + body},
		fails: []tenancy.Failure{
			{Kind: "bad", At: check, Status: 400, Prefix: "Bad."},
			{Kind: "upstream", At: classify, Status: 503, Prefix: "Upstream down."},
		},
	}.run(t)
	assertFindings(t, report, "G7")
}

// TestCheckCharges_ATableFunctionThatCannotRefuse_IsAFinding: a function the
// table names that is a value, whose charge helper does not exist, or that
// returns no gate refusal, fails before any return is read; one that does
// not exist is G1's.
func TestCheckCharges_ATableFunctionThatCannotRefuse_IsAFinding(t *testing.T) {
	body := `
var notAFunction = 1

func (g *gate) plain() int { return 0 }
`
	report := fixture{
		files: map[string]string{"site/site.go": gateSource + body},
		fails: []tenancy.Failure{
			{Kind: "a", At: tenancy.Site{Pkg: siteDir, Name: "notAFunction", Call: "gate.charge"}},
			{Kind: "b", At: tenancy.Site{Pkg: siteDir, Name: "gate.rejected", Call: "gate.missing"}},
			{Kind: "c", At: tenancy.Site{Pkg: siteDir, Name: "gate.plain", Call: "gate.charge"}},
			{Kind: "d", At: tenancy.Site{Pkg: siteDir, Name: "gate.gone", Call: "gate.charge"}},
			{Kind: "e", At: tenancy.Site{Pkg: siteDir, Name: "gate.upstream", Call: "notAFunction"}},
		},
	}.run(t)
	assertFindings(t, report, "G7",
		siteDir+":gate.plain: returns no gate refusal",
		siteDir+":gate.rejected: names the charge helper gate.missing, which is not a function of "+siteDir,
		siteDir+":gate.upstream: names the charge helper notAFunction, which is not a function of "+siteDir,
		siteDir+":notAFunction: is not a function, so it returns no refusal",
	)
}

// TestCheckCharges_ALiteralOfATypeTheRulesDoNotRead_IsUnreadable: a gate
// type the rules do not list among the refusal types is a literal G7 cannot
// read, which it reports rather than skips.
func TestCheckCharges_ALiteralOfATypeTheRulesDoNotRead_IsUnreadable(t *testing.T) {
	r := fixtureRules()
	r.refusalTypes = r.refusalTypes[:2]
	report := fixture{files: map[string]string{"site/site.go": gateSource + resolveBody}, fails: resolveFailures(), rules: &r}.run(t)
	got := findings(report, "G7")
	for _, want := range []string{
		siteDir + ":gate.resolve: returns a refusal the gate cannot read",
		siteDir + ":gate.resolve: returns a refusal from " + siteDir + ":gate.rejected, which builds 0 gate refusals rather than one",
	} {
		if !slices.Contains(got, want) {
			t.Fatalf("G7 findings = %q, want %q among them", got, want)
		}
	}
}

// TestPlumbing_ALowerChargeOutsideTheHelpers_IsAFinding: the lower charge is
// held to the declared helpers, whatever the table says.
func TestPlumbing_ALowerChargeOutsideTheHelpers_IsAFinding(t *testing.T) {
	body := `
func (g *gate) sneak(key string) { g.b.spend(key) }
`
	report := fixture{files: map[string]string{"site/site.go": gateSource + body}}.run(t)
	assertFindings(t, report, "G7",
		siteDir+":gate.sneak: spends an authentication budget through "+sitePath+".budget.spend outside the declared charge helpers")
}

// TestMatchScore_PrefersTheRowThatNamesTheReturn: a row naming the return's
// text beats a row that names none, and a row naming other text or another
// status does not match.
func TestMatchScore_PrefersTheRowThatNamesTheReturn(t *testing.T) {
	ret := refusalReturn{status: 401, text: "Rejected token. Check it."}
	tests := []struct {
		name string
		row  tenancy.Failure
		want int
	}{
		{"names the text", tenancy.Failure{Status: 401, Prefix: "Rejected token."}, 2},
		{"names no text", tenancy.Failure{Status: 401}, 1},
		{"names other text", tenancy.Failure{Status: 401, Prefix: "Missing."}, 0},
		{"another status", tenancy.Failure{Status: 403, Prefix: "Rejected token."}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchScore(tt.row, ret); got != tt.want {
				t.Fatalf("matchScore = %d, want %d", got, tt.want)
			}
		})
	}
}
