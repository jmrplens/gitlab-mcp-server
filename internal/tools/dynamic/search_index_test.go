// search_index_test covers lightweight candidate-index behavior in package
// dynamic, including top-result parity vs full scan and defensive/fallback
// branches using table-driven subtests and tiny in-memory fixtures.
package dynamic

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestSearchIndex_CandidateGenerationPreservesFullScanTopResults verifies that
// the lightweight index narrows candidate entries without changing the top
// lexical results for the baseline query set.
func TestSearchIndex_CandidateGenerationPreservesFullScanTopResults(t *testing.T) {
	registry := realCatalogRegistry(t)

	queries := []string{
		"merge request list open author project",
		"list open issues",
		"pipeline run trigger",
		"ci variable secret",
		"project delete",
		"discover project from remote",
	}

	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			terms := normalizeSearchTerms(query)
			indexed := sortAndLimitMatches(registry.scoredMatches(terms, scoreEntryWithExplanation), 5)
			fullScan := sortAndLimitMatches(fullScanScoredMatches(registry.entries, terms, scoreEntryWithExplanation), 5)
			if len(fullScan) == 0 {
				t.Fatalf("full scan returned no lexical matches for %q", query)
			}
			if !slices.Equal(scoredActionIDs(indexed), scoredActionIDs(fullScan)) {
				t.Fatalf("indexed matches = %v, full scan = %v", scoredActionIDs(indexed), scoredActionIDs(fullScan))
			}
		})
	}
}

// TestSearchIndex_DefensiveBranches verifies empty-index, empty-term, duplicate
// token, and fallback candidate behavior for the lightweight dynamic search
// index. It uses a tiny hand-built index fixture.
func TestSearchIndex_DefensiveBranches(t *testing.T) {
	var emptyIndex searchIndex
	if got := emptyIndex.candidateEntryIndexes(nil); got != nil {
		t.Fatalf("candidateEntryIndexes(empty index) = %v, want nil", got)
	}

	index := searchIndex{
		byToken: map[string][]int{},
		all:     []int{0, 1},
	}
	if got := index.candidateEntryIndexes(nil); strings.Join(intsToStrings(got), ",") != "0,1" {
		t.Fatalf("candidateEntryIndexes(empty terms) = %v, want all indexes", got)
	}
	index.addValues(index.byToken, []string{"", "project.delete", "project.delete"}, 0)
	if got := index.byToken["project.delete"]; len(got) != 1 || got[0] != 0 {
		t.Fatalf("byToken[project.delete] = %v, want single posting 0", got)
	}
	if got := index.candidateEntryIndexes(normalizeSearchTerms("unknown")); strings.Join(intsToStrings(got), ",") != "0,1" {
		t.Fatalf("candidateEntryIndexes(no candidates) = %v, want full fallback", got)
	}
}

func intsToStrings(values []int) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.Itoa(value))
	}
	return out
}
