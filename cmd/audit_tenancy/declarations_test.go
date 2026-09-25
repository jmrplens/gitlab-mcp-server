package main

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// TestNotADecision_EveryEntryIsReviewable: every exemption names a package
// and a declaration, a category that is defined, and a reason in words.
func TestNotADecision_EveryEntryIsReviewable(t *testing.T) {
	for key, entry := range notADecision {
		t.Run(key, func(t *testing.T) {
			if dir, name, ok := strings.Cut(key, ":"); !ok || dir == "" || name == "" {
				t.Fatalf("key %q is not package:Name", key)
			}
			if _, known := categories[entry.category]; !known {
				t.Fatalf("category %q is not defined", entry.category)
			}
			if strings.TrimSpace(entry.reason) == "" {
				t.Fatal("no reason")
			}
		})
	}
}

// TestCategories_EachSaysWhatItCovers.
func TestCategories_EachSaysWhatItCovers(t *testing.T) {
	for name, covers := range categories {
		t.Run(name, func(t *testing.T) {
			if strings.TrimSpace(covers) == "" {
				t.Fatal("a category that says nothing is an excuse nobody reviewed")
			}
		})
	}
}

// TestPending_IsTheValuedRowsOfTheRegister: the list holds each of the
// register's Valued rows once, and nothing else, which is what "the rows not
// yet migrated" is before the first value layer.
func TestPending_IsTheValuedRowsOfTheRegister(t *testing.T) {
	var valued []string
	for _, d := range tenancy.Decisions() {
		if d.Disposition == tenancy.Valued {
			valued = append(valued, d.ID)
		}
	}
	if len(pending) != len(valued) || !sameSet(pending, valued) {
		t.Fatalf("pending = %v, want the Valued rows %v", sortedPending(), valued)
	}
}

// sameSet reports whether a and b hold the same strings.
func sameSet(a, b []string) bool {
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

// TestCheckPending_HoldsTheListToTheRegister: an entry naming no row fails,
// and so does a pending row whose alias, argument and orphan checks already
// pass, since the layer that moved it forgot to say so; a row still reading a
// literal stays pending.
func TestCheckPending_HoldsTheListToTheRegister(t *testing.T) {
	source := siteHeader + `
const moved = leaf.Limit

const unmoved = 64
`
	report := fixture{
		files: map[string]string{"site/site.go": source},
		rows: []tenancy.Decision{
			row("ROW-001", aliasSite("moved", "Limit")),
			row("ROW-002", aliasSite("unmoved", "Limit")),
		},
		pending: []string{"ROW-001", "ROW-002", "ROW-404"},
	}.run(t)
	assertFindings(t, report, "G1",
		"ROW-001: is pending, and its alias, argument and orphan checks already pass: take it out of pending",
		"ROW-404: is pending, and the register has no such row",
	)
	if strings.Join(report.Pending, ",") != "ROW-001,ROW-002,ROW-404" {
		t.Fatalf("pending = %v, want the list sorted", report.Pending)
	}
}

// TestCheckPending_AnOrphanKeepsARowPending: a pending row whose alias passes
// while a value it owns is still read by nothing stays pending.
func TestCheckPending_AnOrphanKeepsARowPending(t *testing.T) {
	source := siteHeader + `
const moved = leaf.Limit
`
	d := row("ROW-001", aliasSite("moved", "Limit"))
	d.Values = []string{"Limit", "Window", "Missing"}
	report := fixture{files: map[string]string{"site/site.go": source}, rows: []tenancy.Decision{d}, pending: []string{"ROW-001"}}.run(t)
	assertFindings(t, report, "G1")
}

// TestCheckExemptions_HoldsTheTableToWhatItAnswered: an exemption that
// answered nothing this run, and one naming a category nobody defined, each
// fail.
func TestCheckExemptions_HoldsTheTableToWhatItAnswered(t *testing.T) {
	source := siteHeader + `
const maxWidgets = 5

const plain = 6

func Enforce() {}
`
	report := fixture{
		files: map[string]string{"site/site.go": source},
		rows:  []tenancy.Decision{row("ROW-001", site("Enforce", tenancy.Enforce))},
		exempt: map[string]exemption{
			siteDir + ":maxWidgets": {"made-up", "answers the name"},
			siteDir + ":plain":      {categoryParsing, "answers nothing"},
		},
	}.run(t)
	assertFindings(t, report, "G10",
		siteDir+":maxWidgets: is exempted under the category \"made-up\", which is not one of the defined ones",
		siteDir+":plain: is exempted, and this run found nothing limit-shaped there for it to answer",
	)
}

// TestProductionRules_NameWhatTheTreeSpells: the refusal types carry the
// fields the tree's literals have, the gate type is among them, and every
// Retry-After source a row can declare has names to read.
func TestProductionRules_NameWhatTheTreeSpells(t *testing.T) {
	r := productionRules()
	found := false
	for _, rt := range r.refusalTypes {
		if rt.code == "" {
			t.Fatalf("refusal type %s names no code field", rt.name)
		}
		found = found || rt.name == r.gateType
	}
	if !found {
		t.Fatalf("the gate type %s is not a refusal type", r.gateType)
	}
	for _, source := range []tenancy.RetryAfter{tenancy.RetryAfterFixed, tenancy.RetryAfterUpstreamOrFixed, tenancy.RetryAfterLongestBlock} {
		if len(r.retryAfterReads[source]) == 0 {
			t.Fatalf("Retry-After source %d names nothing to read", source)
		}
	}
}
