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
	for name, source := range map[string]tenancy.RetryAfter{
		"fixed":             tenancy.RetryAfterFixed,
		"upstream or fixed": tenancy.RetryAfterUpstreamOrFixed,
		"longest block":     tenancy.RetryAfterLongestBlock,
	} {
		t.Run(name, func(t *testing.T) {
			if len(r.retryAfterReads[source]) == 0 {
				t.Fatalf("Retry-After source %d names nothing to read", source)
			}
		})
	}
}
