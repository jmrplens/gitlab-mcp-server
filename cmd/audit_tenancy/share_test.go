package main

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// shareSource describes numbers in the words G13 reads: a share word in a
// declaration's own doc, one in the doc of the block around another, and a
// word that only contains one.
const shareSource = siteHeader + `
// perEntry bounds fairness between callers.
const perEntry = 64

// The ceilings below keep every caller's quota.
const (
	inBlock = 10
)

// affair is not a share word, and neither is unfairly.
const affair = 1

// Enforce applies them.
func Enforce() {}
`

// shareRow is an allowance row on key with the sites named.
func shareRow(id string, kind tenancy.Kind, key tenancy.Key, sites ...tenancy.Site) tenancy.Decision {
	return tenancy.Decision{ID: id, Kind: kind, Key: key, Sites: sites}
}

// TestCheckShareWords_ANumberOnAMintableKeyCalledAShare_IsAFinding: a share
// word in a site's doc or in its block's, on an allowance keyed on a key a
// caller can mint, fails without a finding recorded for INV-003. Pin and
// Reason sites are read as well, each declaration once.
func TestCheckShareWords_ANumberOnAMintableKeyCalledAShare_IsAFinding(t *testing.T) {
	d := shareRow("HLD-001", tenancy.Ceiling, tenancy.KeyEntry,
		aliasSite("perEntry", "Limit"), aliasSite("inBlock", "Limit"), site("Enforce", tenancy.Enforce), site("gone", tenancy.Enforce),
		site("affair", tenancy.Pin), site("Enforce", tenancy.Reason), site("Enforce", tenancy.Charge))
	d.ReasonAt = site("perEntry", tenancy.Reason)
	report := fixture{files: map[string]string{"site/site.go": shareSource}, rows: []tenancy.Decision{d}}.run(t)
	assertFindings(t, report, "G13",
		"HLD-001: "+siteDir+":inBlock describes a number on the entry key, which a caller can mint, as \"quota\" (INV-003)",
		"HLD-001: "+siteDir+":perEntry describes a number on the entry key, which a caller can mint, as \"fairness\" (INV-003)",
	)
}

// TestCheckShareWords_AFindingAnswersItAndMustStillBeNeeded: the finding that
// records the INV-003 departure excuses the word, and a row carrying it while
// no comment uses a share word has a finding that no longer describes the
// tree.
func TestCheckShareWords_AFindingAnswersItAndMustStillBeNeeded(t *testing.T) {
	excused := shareRow("HLD-001", tenancy.Ceiling, tenancy.KeyEntry, aliasSite("perEntry", "Limit"))
	excused.Findings = []string{"F-04"}
	stale := shareRow("HLD-002", tenancy.Ceiling, tenancy.KeyEntry, aliasSite("affair", "Limit"))
	stale.Findings = []string{"F-04"}
	report := fixture{files: map[string]string{"site/site.go": shareSource}, rows: []tenancy.Decision{excused, stale}}.run(t)
	assertFindings(t, report, "G13",
		"HLD-002: carries a finding recorded for INV-003, and no comment of its sites uses a share word")
}

// TestCheckShareWords_OnlyAllowancesOnMintableKeysAreRead: a rule, and a
// ceiling keyed on the process, are not described as a share whatever their
// comments say.
func TestCheckShareWords_OnlyAllowancesOnMintableKeysAreRead(t *testing.T) {
	report := fixture{
		files: map[string]string{"site/site.go": shareSource},
		rows: []tenancy.Decision{
			shareRow("ROW-001", tenancy.Rule, tenancy.KeyEntry, aliasSite("perEntry", "Limit")),
			shareRow("ROW-002", tenancy.Ceiling, tenancy.KeyProcess, aliasSite("perEntry", "Limit")),
		},
	}.run(t)
	assertFindings(t, report, "G13")
}

// TestWordsOf_SplitsOnEveryNonLetter: whole lower-case words, so "unfairly"
// is one word and not "fair".
func TestWordsOf_SplitsOnEveryNonLetter(t *testing.T) {
	if got := wordsOf("Bounds Fairness, un-fair; unfairly."); !slices.Equal(got, []string{"bounds", "fairness", "un", "fair", "unfairly"}) {
		t.Fatalf("wordsOf = %q", got)
	}
}
