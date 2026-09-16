//go:build e2e

// slice.go decides which of a session's tools a model is shown.
//
// On two of the three surfaces it decides nothing: dynamic publishes two tools
// and meta a few dozen, and both fit in any request. The individual surface
// publishes one tool per action, and its whole list is 682,878 tokens at
// Ultimate, 648,852 at Premium and 539,274 at Free/CE
// (docs/development/token-footprint.md), which is over the context window of at
// least one of the four providers outright and leaves no room for a
// conversation on the others. That figure is committed in this repository, so
// nothing here has to spend a request rediscovering it.
//
// So a model on the individual surface is shown a slice: every tool of the
// domains the case's key touches, every tool that belongs to no catalog domain,
// and distractors from other domains filled to a budget in an order seeded by
// the case ID. That is what a client which filters its tool list does, and it
// measures tool choice within a slice, which is a different question from
// choice across a whole catalog. It is published as a different question too:
// an individual row carries its slice size and is a comparison class of its
// own.
//
// Two things this is not. It is not a narrowing of the server: the session
// serves its whole catalog and a call to a tool outside the slice is dispatched
// like any other, so what the slice bounds is what the model was shown and
// never what it was allowed to do. And it is not a hint about the step: a case
// that touches issues is shown every issue tool, the right one among them, in
// an order that mixes them with the distractors so that position says nothing.

package modeleval

import (
	"hash/fnv"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval/internal/provider"
)

// toolSlice is what one attempt was shown, and the counts that say how it was
// arrived at.
//
// The counts are kept rather than computed away because the slice is the one
// thing about an individual row a reader has to be able to interrogate: a row
// that completed nothing is a different finding depending on whether the case's
// own tools were 12 of the 128 shown or 128 of them.
type toolSlice struct {
	// Tools are the tools the model is shown, in the order it is shown them.
	Tools []provider.Tool
	// Budget is what was asked for, which is what the row publishes. It is the
	// setting and not the length: a case whose own domains overflow it is shown
	// more than the budget rather than less than its case needs, and a case on
	// a session serving fewer tools than the budget is shown all of them.
	Budget int
	// Named is how many of the shown tools belong to a domain the case's key
	// touches.
	Named int
	// Unplaceable is how many belong to no catalog domain at all: the
	// standalone tools, which are registered outside the catalog under one name
	// on every surface and are always shown.
	Unplaceable int
	// Distractors is how many tools of other domains were added to reach the
	// budget.
	Distractors int
	// Sliced says a choice was made. A served list no larger than the budget is
	// shown whole, in the order the server listed it, and that is not a slice.
	Sliced bool
	// Overflowed says the tools that must be shown outnumbered the budget, so
	// the slice is larger than it was asked to be.
	//
	// Keeping them is the only honest reading. Dropping a tool the case needs
	// to hold a number would leave an attempt that could not have succeeded and
	// a row that does not say so, which is the fold this whole rebuild exists
	// to stop.
	Overflowed bool
}

// Summary says in one line what the model was shown, for the run's own log.
func (s toolSlice) Summary(served int) string {
	if !s.Sliced {
		return strconv.Itoa(served) + " tool(s), the whole served list, which fits the budget of " +
			strconv.Itoa(s.Budget)
	}
	parts := []string{
		strconv.Itoa(len(s.Tools)) + " of " + strconv.Itoa(served) + " tool(s)",
		strconv.Itoa(s.Named) + " from the case's own domains",
		strconv.Itoa(s.Unplaceable) + " outside the catalog",
		strconv.Itoa(s.Distractors) + " distractor(s)",
	}
	if s.Overflowed {
		parts = append(parts, "over the budget of "+strconv.Itoa(s.Budget)+
			", which the tools that must be shown exceed between them")
	}
	return strings.Join(parts, ", ")
}

// budgetFor is how many tools a model may be shown on one surface: the
// configured slice on individual, and no bound on the other two.
//
// It is the one place that decides, and both readers go through it: the slice a
// model is shown, and the size the session line records and the row publishes.
// Two readings of "is this the individual surface" is how a row comes to say it
// was measured in a slice of 128 while the model was shown a thousand tools.
func budgetFor(surface harness.Surface, configured int) int {
	if surface != harness.SurfaceIndividual {
		return 0
	}
	return configured
}

// shownTools returns what a model is shown on one surface out of what the
// session serves.
func shownTools(
	served []provider.Tool,
	surface harness.Surface,
	caseID string,
	domains []string,
	configured int,
	domainOf func(tool string) string,
) toolSlice {
	return sliceTools(served, caseID, domains, budgetFor(surface, configured), domainOf)
}

// sliceTools returns the tools one case is shown out of what a session serves.
//
// domainOf names the catalog domain a served tool belongs to, and the empty
// string for a tool that belongs to none. It is passed in rather than resolved
// here because the answer is the catalog's, read through the same identifier
// the runner resolves a model's call with: an individual tool's name is
// declared in its ActionSpec and cannot be derived from a formula, so a
// function that read the name would be inventing a mapping the server does not
// use.
//
// A budget of nothing is no budget rather than a budget of nothing: the whole
// served list is shown, which is what the two surfaces that fit a request ask
// for and what [budgetFor] hands them.
//
// The result is a pure function of the served list, the case ID, the domains
// and the budget, so two runs of one case on one catalog show one list. That is
// what lets a row name a slice size rather than a slice: the size, the corpus
// digest and the tool-schema digest together say exactly what each attempt saw.
func sliceTools(
	served []provider.Tool,
	caseID string,
	domains []string,
	budget int,
	domainOf func(tool string) string,
) toolSlice {
	if budget < 1 || len(served) <= budget {
		return toolSlice{Tools: slices.Clone(served), Budget: budget}
	}

	slice := toolSlice{Budget: budget, Sliced: true}
	var distractors []provider.Tool
	for _, tool := range served {
		switch domain := domainOf(tool.Name); {
		case domain == "":
			slice.Tools = append(slice.Tools, tool)
			slice.Unplaceable++
		case slices.Contains(domains, domain):
			slice.Tools = append(slice.Tools, tool)
			slice.Named++
		default:
			distractors = append(distractors, tool)
		}
	}

	if room := budget - len(slice.Tools); room > 0 {
		sortBySeed(distractors, caseID)
		slice.Distractors = min(room, len(distractors))
		slice.Tools = append(slice.Tools, distractors[:slice.Distractors]...)
	} else {
		slice.Overflowed = room < 0
	}
	sortBySeed(slice.Tools, caseID)
	return slice
}

// sortBySeed puts a list of tools in an order that depends on the case and on
// nothing else.
//
// It is used twice and for two different jobs, and both need the same property.
// Choosing which distractors to show is a deterministic subset, so that one
// case's slice is the same list every run and two cases are shown different
// distractors. Ordering the slice that results mixes the case's own tools among
// them, so that a model cannot learn that what it needs is at the front, which
// is a hint it would carry from one case to the next.
func sortBySeed(tools []provider.Tool, caseID string) {
	slices.SortStableFunc(tools, func(a, b provider.Tool) int {
		left, right := seedOrder(caseID, a.Name), seedOrder(caseID, b.Name)
		switch {
		case left < right:
			return -1
		case left > right:
			return 1
		default:
			return strings.Compare(a.Name, b.Name)
		}
	})
}

// seedOrder is one tool's position under one case's seed.
//
// FNV-1a over the case ID and the tool name: a hash rather than a shuffle
// because a shuffle needs a generator whose sequence is part of the answer, and
// a hash of the two names is stable across platforms, across Go versions and
// across a change to the served list, so adding a tool to the catalog moves one
// tool's position rather than every tool's.
func seedOrder(caseID, tool string) uint64 {
	sum := fnv.New64a()
	_, _ = sum.Write([]byte(caseID))
	_, _ = sum.Write([]byte{0})
	_, _ = sum.Write([]byte(tool))
	return sum.Sum64()
}
