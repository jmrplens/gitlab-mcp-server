package main

import (
	"slices"
	"strings"
	"testing"
)

// TestProseExemptions_EveryEntry_CarriesAReason holds the table to what makes
// an exemption reviewable. A token with no reason beside it is one a later
// reader cannot tell from an oversight, and this repository has written down
// what an unreadable exemption list costs.
func TestProseExemptions_EveryEntry_CarriesAReason(t *testing.T) {
	for token, reason := range proseExemptions {
		t.Run(token, func(t *testing.T) {
			if strings.TrimSpace(token) == "" {
				t.Error("an exemption with no token excuses everything")
			}
			if len(strings.TrimSpace(reason)) < 20 {
				t.Errorf("reason for %q is %q, want a sentence a reviewer can judge", token, reason)
			}
		})
	}
}

// TestProseExemptions_TheTable_StaysShort holds the intent written beside it.
// What keeps the list short is the domain test in the prose rule, so a table
// that has grown is a sign the rule stopped doing its job rather than a sign
// the tree grew.
func TestProseExemptions_TheTable_StaysShort(t *testing.T) {
	if len(proseExemptions) > 5 {
		t.Errorf("prose exemptions = %d; past a handful the rule that produced them is what to fix",
			len(proseExemptions))
	}
}

// TestExemptProse_UndeclaredToken_IsNotExcused holds that the table excuses
// what it names and nothing else.
func TestExemptProse_UndeclaredToken_IsNotExcused(t *testing.T) {
	if exemptProse("project.no_such_thing") {
		t.Error("an undeclared token was excused")
	}
	if exemptProse("") {
		t.Error("the empty token was excused")
	}
}

// TestStaleProseExemptions_UsedEntry_IsNotReported holds both directions of
// the stale check, since a check that reported everything or nothing would
// pass a test written only one way.
func TestStaleProseExemptions_UsedEntry_IsNotReported(t *testing.T) {
	used := map[string]struct{}{}
	for token := range proseExemptions {
		used[token] = struct{}{}
	}
	if stale := staleProseExemptions(used); len(stale) != 0 {
		t.Errorf("stale = %v, want none when every entry excused something", stale)
	}

	stale := staleProseExemptions(map[string]struct{}{})
	if len(stale) != len(proseExemptions) {
		t.Errorf("stale = %v, want every entry when none excused anything", stale)
	}
	if !slices.IsSorted(stale) {
		t.Errorf("stale = %v, want a stable order", stale)
	}
}
