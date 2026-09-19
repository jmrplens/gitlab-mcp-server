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

// TestDeclaredAliasMentions_EveryEntry_CarriesAReason holds the second table
// to the same standard, and to one more of its own: an entry has to name a
// dotted spelling, since the site it excuses is a prose token.
func TestDeclaredAliasMentions_EveryEntry_CarriesAReason(t *testing.T) {
	for token, reason := range declaredAliasMentions {
		t.Run(token, func(t *testing.T) {
			if !strings.Contains(token, ".") {
				t.Errorf("%q is not an action-ID-shaped token, so no prose site can name it", token)
			}
			if len(strings.TrimSpace(reason)) < 20 {
				t.Errorf("reason for %q is %q, want a sentence a reviewer can judge", token, reason)
			}
		})
	}
}

// TestExemptAliasMention_UndeclaredToken_IsNotExcused holds that the alias
// table excuses what it names and nothing else, which is what keeps the
// canonical-ID demand a rule rather than a suggestion.
func TestExemptAliasMention_UndeclaredToken_IsNotExcused(t *testing.T) {
	if exemptAliasMention("issue.no_such_alias") {
		t.Error("an undeclared alias mention was excused")
	}
	if exemptAliasMention("") {
		t.Error("the empty token was excused")
	}
}

// TestStaleDeclarations_UsedEntry_IsNotReported holds both directions of the
// stale check, over both tables, since a check that reported everything or
// nothing would pass a test written only one way.
func TestStaleDeclarations_UsedEntry_IsNotReported(t *testing.T) {
	usedExemptions := map[string]struct{}{}
	for token := range proseExemptions {
		usedExemptions[token] = struct{}{}
	}
	usedAliases := map[string]struct{}{}
	for token := range declaredAliasMentions {
		usedAliases[token] = struct{}{}
	}
	if stale := staleDeclarations(usedExemptions, usedAliases); len(stale) != 0 {
		t.Errorf("stale = %v, want none when every entry excused something", stale)
	}

	stale := staleDeclarations(map[string]struct{}{}, map[string]struct{}{})
	if len(stale) != len(proseExemptions)+len(declaredAliasMentions) {
		t.Errorf("stale = %v, want every entry of both tables when none excused anything", stale)
	}
	if !slices.IsSorted(stale) {
		t.Errorf("stale = %v, want a stable order", stale)
	}
	for _, entry := range stale {
		t.Run(entry, func(t *testing.T) {
			if !strings.Contains(entry, "proseExemptions") && !strings.Contains(entry, "declaredAliasMentions") {
				t.Errorf("%q names no table, so a reader is not told which map to open", entry)
			}
		})
	}
}
