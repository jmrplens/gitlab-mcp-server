// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"slices"
	"testing"
)

// TestApplyIdentifierDeclarations_EachDeclaration_IsHeldToWhatItsCategoryRequires
// verifies the table's own checks: a declaration answers a narrowed or an
// undescribed argument read from its input and carries its category, issue
// and reason onto it, and leaves every other argument as it was; one written
// twice, one of a category the table does not define, one with no reason, a
// tracked one with no issue and a gitlab-accepts-id-only one with no evidence
// are stale, and so is one that answers nothing, the one written twice
// answering only once.
func TestApplyIdentifierDeclarations_EachDeclaration_IsHeldToWhatItsCategoryRequires(t *testing.T) {
	arguments := []identifierArgument{
		{Input: "a.In.Narrowed", Status: identifierNarrowed},
		{Input: "a.In.Undescribed", Status: identifierUndescribed},
		{Input: "a.In.IDOnly", Status: identifierIDOnly},
		{Input: "a.In.Unplaced", Status: identifierUnplaced},
		{Input: "a.In.Twice", Status: identifierNarrowed},
	}
	declared := []identifierDeclaration{
		{Input: "a.In.Narrowed", Category: identifierTracked, Issue: 3, Reason: "widened later"},
		{Input: "a.In.Undescribed", Category: identifierAcceptsIDOnly, Evidence: "Finder.find(params[:id])", Reason: "an ID is all it reads"},
		{Input: "a.In.IDOnly", Category: identifierTracked, Issue: 4, Reason: "nothing to answer"},
		{Input: "a.In.Unplaced", Category: identifierTracked, Issue: 5, Reason: "a declaration does not place a route"},
		{Input: "a.In.Twice", Category: identifierTracked, Issue: 6, Reason: "first"},
		{Input: "a.In.Twice", Category: identifierTracked, Issue: 6, Reason: "second"},
		{Input: "b.In.Category", Category: "decided-not-to", Reason: "not a category"},
		{Input: "b.In.Reason", Category: identifierTracked, Issue: 1},
		{Input: "b.In.Issue", Category: identifierTracked, Reason: "no issue"},
		{Input: "b.In.Evidence", Category: identifierAcceptsIDOnly, Reason: "no evidence"},
	}
	stale := applyIdentifierDeclarations(arguments, declared)
	wantStatuses := []string{identifierDeclared, identifierDeclared, identifierIDOnly, identifierUnplaced, identifierDeclared}
	for i, argument := range arguments {
		t.Run(argument.Input, func(t *testing.T) {
			if argument.Status != wantStatuses[i] {
				t.Errorf("%s is %s, want %s", argument.Input, argument.Status, wantStatuses[i])
			}
		})
	}
	if first := arguments[0]; first.Category != identifierTracked || first.Issue != 3 || first.Reason != "widened later" {
		t.Errorf("the declared argument carries %q, %d and %q, want the declaration's", first.Category, first.Issue, first.Reason)
	}
	if twice := arguments[4]; twice.Reason != "first" {
		t.Errorf("the argument declared twice carries %q, want the first declaration's reason", twice.Reason)
	}
	const answersNothing = " answers no integer handed where the record does not say an ID alone is taken"
	want := []string{
		"identifier declaration a.In.Twice is declared twice",
		`identifier declaration b.In.Category: category "decided-not-to" is not one the table defines`,
		"identifier declaration b.In.Reason: it gives no reason",
		"identifier declaration b.In.Issue: a tracked declaration needs the issue that tracks it",
		"identifier declaration b.In.Evidence: a gitlab-accepts-id-only declaration needs the evidence of GitLab's code",
		"identifier declaration a.In.IDOnly" + answersNothing,
		"identifier declaration a.In.Unplaced" + answersNothing,
		"identifier declaration b.In.Category" + answersNothing,
		"identifier declaration b.In.Reason" + answersNothing,
		"identifier declaration b.In.Issue" + answersNothing,
		"identifier declaration b.In.Evidence" + answersNothing,
	}
	if !slices.Equal(stale, want) {
		t.Errorf("stale =\n%q\nwant\n%q", stale, want)
	}
}

// TestIdentifierDeclarations_EveryEntry_NeedsNothing verifies the committed
// table on its own: every declaration names a category the table defines,
// a reason, and what its category requires, and no input is declared
// twice. That each answers an argument is the repository gate's to say.
func TestIdentifierDeclarations_EveryEntry_NeedsNothing(t *testing.T) {
	seen := map[string]bool{}
	for _, declaration := range identifierDeclarations {
		t.Run(declaration.Input, func(t *testing.T) {
			if lacks := identifierNeeds(declaration); lacks != "" {
				t.Errorf("%s: %s", declaration.Input, lacks)
			}
			if seen[declaration.Input] {
				t.Errorf("%s is declared twice", declaration.Input)
			}
			seen[declaration.Input] = true
		})
	}
}
