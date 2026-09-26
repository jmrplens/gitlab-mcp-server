package main

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// orphanRules holds the fixture leaf's value, code and rule files to a
// reader, which the other fixtures leave off.
func orphanRules() *rules {
	r := fixtureRules()
	r.valueFiles = []string{"values.go", "codes.go"}
	r.ruleFiles = []string{"rules.go"}
	return &r
}

// orphanLeafRules has an exported rule, an unexported helper and an exported
// method, of which only the first is held to a row.
const orphanLeafRules = `package leaf

func Busy() bool { return helper() }

func helper() bool { return false }

type Holdings struct{}

func (Holdings) Held() bool { return false }

var _ = 1
`

// orphanSource reads Limit and Ratio, and holds an Alias of Window that reads
// something else, so Window is declared and still unread.
const orphanSource = siteHeader + `
const limit = leaf.Limit

var ratio = leaf.Ratio

const window = 30 * time.Second

func Parse(fallback float64) float64 { return fallback }

func Load() { _ = Parse(leaf.Ratio) }
`

// TestCheckOrphans_EveryRegisterValueHasAReader: a constant an Alias reads,
// one only an Arg reads, and a rule function a row names are not orphans.
func TestCheckOrphans_EveryRegisterValueHasAReader(t *testing.T) {
	d := row("ROW-001", aliasSite("limit", "Limit"), argSite("Load", "Parse", 0, 1, "Ratio"),
		aliasSite("window", "Window"))
	d.Functions = []string{"Busy"}
	report := fixture{
		files: map[string]string{
			"site/site.go":  orphanSource + "\nconst busy = leaf.CodeBusy\n",
			"leaf/rules.go": orphanLeafRules,
		},
		rows:  []tenancy.Decision{d, row("ROW-002", aliasSite("busy", "CodeBusy"))},
		rules: orphanRules(),
	}.run(t)
	assertFindings(t, report, "G6",
		leafDir+":Window: is a register value nothing outside the register reads",
	)
}

// orphanLeafCodes is a value file that also holds a function, which is not a
// value and is not held to a reader through it.
const orphanLeafCodes = leafCodes + `
func codeName() string { return "busy" }
`

// TestCheckOrphans_ARegisterValueNothingReads_IsAFinding: a constant no site
// is declared to read, including one only a row's Values name, and a rule
// function no row names, each fail.
func TestCheckOrphans_ARegisterValueNothingReads_IsAFinding(t *testing.T) {
	report := fixture{
		files: map[string]string{"site/site.go": orphanSource, "leaf/rules.go": orphanLeafRules, "leaf/codes.go": orphanLeafCodes},
		rows: []tenancy.Decision{
			row("ROW-001", aliasSite("limit", "Limit")),
			{ID: "ROW-002", Values: []string{"Window"}},
		},
		rules: orphanRules(),
	}.run(t)
	assertFindings(t, report, "G6",
		leafDir+":Busy: is a register function no row names in Functions",
		leafDir+":CodeBusy: is a register value no Alias or Arg site reads",
		leafDir+":Ratio: is a register value no Alias or Arg site reads",
		leafDir+":Window: is a register value no Alias or Arg site reads",
	)
}

// TestCheckOrphans_NoRegisterLoaded_JudgesNothing: a run whose register is not
// in the program has no values to hold, and G12 is what says so.
func TestCheckOrphans_NoRegisterLoaded_JudgesNothing(t *testing.T) {
	cfg := fixture{files: map[string]string{"site/site.go": orphanSource}, rules: orphanRules()}.config(t)
	cfg.register.leaf = "internal/nowhere"
	report, err := audit(cfg)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	assertFindings(t, report, "G6")
	assertFindings(t, report, "G12", "internal/nowhere: the register package is not in the loaded program")
}
