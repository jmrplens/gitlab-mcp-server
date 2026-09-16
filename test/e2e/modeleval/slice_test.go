//go:build e2e

// slice_test.go checks the one decision the individual surface forces: which
// tools a model is shown when it cannot be shown all of them.
//
// Every case here is offline and none of them needs a catalog: the domain of a
// tool is whatever the reader passed in says it is, which is the same seam the
// runner fills with the server's own identifier. What is being checked is the
// selection, not the catalog.

package modeleval

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval/internal/provider"
)

// servedList builds a tool list of several domains plus a few tools that belong
// to none, in the shape the individual surface serves: one tool per action,
// named after its domain.
func servedList(perDomain int, domains ...string) []provider.Tool {
	var tools []provider.Tool
	for _, domain := range domains {
		for index := range perDomain {
			tools = append(tools, provider.Tool{Name: "gitlab_" + domain + "_" + strconv.Itoa(index)})
		}
	}
	tools = append(tools,
		provider.Tool{Name: "gitlab_discover_project"},
		provider.Tool{Name: "gitlab_interactive_issue"},
	)
	return tools
}

// domainReader names the domain of a tool built by [servedList], and nothing
// for the two that belong to no catalog action.
func domainReader(tool string) string {
	name, found := strings.CutPrefix(tool, "gitlab_")
	if !found {
		return ""
	}
	domain, _, numbered := strings.Cut(name, "_")
	if !numbered {
		return ""
	}
	if domain == "discover" || domain == "interactive" {
		return ""
	}
	return domain
}

// names spells a slice's tools, for comparing two selections.
func names(tools []provider.Tool) []string {
	spelled := make([]string, 0, len(tools))
	for _, tool := range tools {
		spelled = append(spelled, tool.Name)
	}
	return spelled
}

// TestSliceTools_ShowsTheCasesOwnDomainsAndFillsTheRestToTheBudget is the whole
// contract in one case: the tools the case could need are all there, the
// standalone tools are all there, the rest is filled to exactly the budget, and
// the filling comes from other domains.
func TestSliceTools_ShowsTheCasesOwnDomainsAndFillsTheRestToTheBudget(t *testing.T) {
	served := servedList(20, "issue", "project", "pipeline", "runner", "wiki")
	const budget = 60

	slice := sliceTools(served, "MT-001", []string{"issue", "wiki"}, budget, domainReader)

	if !slice.Sliced {
		t.Fatalf("Sliced = false for %d tools under a budget of %d", len(served), budget)
	}
	if len(slice.Tools) != budget {
		t.Errorf("the slice carries %d tools, want the budget of %d", len(slice.Tools), budget)
	}
	shown := names(slice.Tools)
	for _, tool := range served {
		domain := domainReader(tool.Name)
		mine := domain == "issue" || domain == "wiki" || domain == ""
		if mine && !slices.Contains(shown, tool.Name) {
			t.Errorf("%s belongs to the case or to no domain and was not shown", tool.Name)
		}
	}
	if slice.Named != 40 {
		t.Errorf("Named = %d, want the 40 tools of the two domains the case touches", slice.Named)
	}
	if slice.Unplaceable != 2 {
		t.Errorf("Unplaceable = %d, want the two tools outside the catalog", slice.Unplaceable)
	}
	if slice.Distractors != budget-42 {
		t.Errorf("Distractors = %d, want %d", slice.Distractors, budget-42)
	}
	if slice.Overflowed {
		t.Errorf("Overflowed = true, and the case's own domains are %d of a budget of %d", 42, budget)
	}
}

// TestSliceTools_IsTheSameListEveryRunAndADifferentOneForAnotherCase checks the
// two halves of the seed's job.
//
// The same case must produce the same list, or two runs of one row measured
// different things and the row cannot say which. Another case must produce
// another list, or the distractors are a constant and the measurement is of one
// arbitrary subset of the catalog rather than of the catalog.
func TestSliceTools_IsTheSameListEveryRunAndADifferentOneForAnotherCase(t *testing.T) {
	served := servedList(30, "issue", "project", "pipeline", "runner", "wiki", "tag")
	domains := []string{"issue"}
	const budget = 80

	first := sliceTools(served, "MT-001", domains, budget, domainReader)
	again := sliceTools(served, "MT-001", domains, budget, domainReader)
	if !slices.Equal(names(first.Tools), names(again.Tools)) {
		t.Errorf("two slices of one case differ:\n%v\n%v", names(first.Tools), names(again.Tools))
	}

	other := sliceTools(served, "MT-002", domains, budget, domainReader)
	if slices.Equal(names(first.Tools), names(other.Tools)) {
		t.Errorf("two cases were shown the same %d tools, so the seed is not reaching the choice", budget)
	}
	for _, tool := range first.Tools {
		if domainReader(tool.Name) == "issue" && !slices.Contains(names(other.Tools), tool.Name) {
			t.Errorf("%s is the case's own domain and the other case did not see it: "+
				"the seed may only move the distractors", tool.Name)
		}
	}
}

// TestSliceTools_MixesTheCasesOwnToolsAmongTheDistractors checks that position
// says nothing.
//
// A slice whose first tools are the ones the case needs would teach a model
// across cases that the answer is at the front, and every row after the first
// would be measuring that lesson rather than tool choice.
func TestSliceTools_MixesTheCasesOwnToolsAmongTheDistractors(t *testing.T) {
	served := servedList(25, "issue", "project", "pipeline", "runner")
	const budget = 60

	slice := sliceTools(served, "MT-003", []string{"issue"}, budget, domainReader)

	var last int
	for index, tool := range slice.Tools {
		if domainReader(tool.Name) == "issue" {
			last = index
		}
	}
	if last < slice.Named {
		t.Errorf("the last tool of the case's own domain is at %d of %d, so the %d of them sit at the front",
			last, len(slice.Tools), slice.Named)
	}
}

// TestSliceTools_ShowsTheWholeListWhenItFits checks that a list that fits is not
// a slice: it is shown in the order the server listed it, and the row says
// nothing was chosen.
func TestSliceTools_ShowsTheWholeListWhenItFits(t *testing.T) {
	served := servedList(4, "issue", "project")

	slice := sliceTools(served, "MT-004", []string{"issue"}, 128, domainReader)

	if slice.Sliced {
		t.Errorf("Sliced = true for %d tools under a budget of 128", len(served))
	}
	if !slices.Equal(names(slice.Tools), names(served)) {
		t.Errorf("the served list was reordered:\n%v\n%v", names(slice.Tools), names(served))
	}
}

// TestSliceTools_KeepsTheCasesOwnToolsWhenTheyOverflowTheBudget is the rule that
// a number is never held by dropping a tool the case needs.
//
// An attempt shown a slice its own action is missing from could not have
// succeeded, and a row that did not say so would report a model failing at
// something it was never given.
func TestSliceTools_KeepsTheCasesOwnToolsWhenTheyOverflowTheBudget(t *testing.T) {
	served := servedList(40, "issue", "project")
	const budget = 20

	slice := sliceTools(served, "MT-005", []string{"issue", "project"}, budget, domainReader)

	if !slice.Overflowed {
		t.Errorf("Overflowed = false with 82 tools that must be shown under a budget of %d", budget)
	}
	if len(slice.Tools) != len(served) {
		t.Errorf("the slice carries %d tools, want all %d that must be shown", len(slice.Tools), len(served))
	}
	if slice.Distractors != 0 {
		t.Errorf("Distractors = %d, and there was no room for one", slice.Distractors)
	}
	if !strings.Contains(slice.Summary(len(served)), "over the budget") {
		t.Errorf("Summary = %q, which does not say the budget was exceeded", slice.Summary(len(served)))
	}
}

// TestSliceTools_WithNoBudgetShowsEverything checks the reading of a budget of
// none, which is what every surface but individual asks for.
func TestSliceTools_WithNoBudgetShowsEverything(t *testing.T) {
	served := servedList(10, "issue", "project")

	slice := sliceTools(served, "MT-006", nil, 0, domainReader)

	if slice.Sliced || len(slice.Tools) != len(served) {
		t.Errorf("a budget of none showed %d of %d tools, Sliced = %t",
			len(slice.Tools), len(served), slice.Sliced)
	}
}

// TestBudgetFor_BoundsTheIndividualSurfaceAndNoOther checks the one place that
// decides whether a surface is sliced, which the shown list and the recorded
// row both read.
func TestBudgetFor_BoundsTheIndividualSurfaceAndNoOther(t *testing.T) {
	tests := []struct {
		name    string
		surface harness.Surface
		want    int
	}{
		{name: "dynamic publishes two tools", surface: harness.SurfaceDynamic},
		{name: "meta publishes a few dozen", surface: harness.SurfaceMeta},
		{name: "individual publishes one per action", surface: harness.SurfaceIndividual, want: defaultSlice},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := budgetFor(testCase.surface, defaultSlice); got != testCase.want {
				t.Errorf("budgetFor(%s) = %d, want %d", testCase.surface, got, testCase.want)
			}
		})
	}
}

// TestShownTools_SlicesOnlyTheIndividualSurface checks the two answers together
// against one served list, which is what an attempt on each surface gets.
func TestShownTools_SlicesOnlyTheIndividualSurface(t *testing.T) {
	served := servedList(50, "issue", "project", "pipeline")

	whole := shownTools(served, harness.SurfaceMeta, "MT-007", []string{"issue"}, defaultSlice, domainReader)
	if whole.Sliced || len(whole.Tools) != len(served) {
		t.Errorf("the meta surface was shown %d of %d tools", len(whole.Tools), len(served))
	}

	sliced := shownTools(served, harness.SurfaceIndividual, "MT-007", []string{"issue"}, defaultSlice, domainReader)
	if !sliced.Sliced || len(sliced.Tools) != defaultSlice {
		t.Errorf("the individual surface was shown %d of %d tools, want the budget of %d",
			len(sliced.Tools), len(served), defaultSlice)
	}
}

// TestSortBySeed_OrdersATieByNameAndKeepsTheRestStable covers the comparator's
// last branch, which two tools of one name are the only way to reach.
//
// It matters for a reason that is not the coverage: a comparator that returned
// an arbitrary answer for a pair it considers equal would make the sort
// unstable, and the slice would differ between two runs of one case on one
// catalog, which is the property every row of the individual surface rests on.
func TestSortBySeed_OrdersATieByNameAndKeepsTheRestStable(t *testing.T) {
	tools := []provider.Tool{
		{Name: "gitlab_issue_1", Description: "first"},
		{Name: "gitlab_issue_0"},
		{Name: "gitlab_issue_1", Description: "second"},
	}

	sortBySeed(tools, "MT-009")

	var tied []string
	for _, tool := range tools {
		if tool.Name == "gitlab_issue_1" {
			tied = append(tied, tool.Description)
		}
	}
	if !slices.Equal(tied, []string{"first", "second"}) {
		t.Errorf("two tools of one name came back as %v, want the order they were given", tied)
	}
}

// TestToolSlice_Summary_SaysWhatWasShownAndOutOfWhat checks the one line the run
// logs, since it is what a maintainer reading a failed attempt sees first.
func TestToolSlice_Summary_SaysWhatWasShownAndOutOfWhat(t *testing.T) {
	served := servedList(30, "issue", "project", "pipeline")
	slice := sliceTools(served, "MT-008", []string{"issue"}, 64, domainReader)

	summary := slice.Summary(len(served))
	for _, want := range []string{
		"64 of 92 tool(s)", "30 from the case's own domains",
		"2 outside the catalog", "32 distractor(s)",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(summary, want) {
				t.Errorf("Summary = %q, which does not say %q", summary, want)
			}
		})
	}

	whole := sliceTools(served, "MT-008", []string{"issue"}, 0, domainReader)
	if got := whole.Summary(len(served)); !strings.Contains(got, "the whole served list") {
		t.Errorf("Summary of an unsliced list = %q", got)
	}
}
