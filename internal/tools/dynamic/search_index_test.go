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
			scored, err := registry.scoredMatches(t.Context(), terms, scoreEntryWithExplanation)
			if err != nil {
				t.Fatalf("scoredMatches(%q) error = %v", query, err)
			}
			indexed := sortAndLimitMatches(scored, 5)
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

// TestSearchIndex_CandidateEntryIndexesNarrowsToTheMatchingPostings verifies
// that a term with an index bucket narrows the candidate set to that bucket
// instead of handing the full catalog back. Both answers rank identically,
// because the index only decides which entries are scored, so nothing about a
// search result would notice a narrowing that quietly stopped narrowing: what
// it costs is a scoring pass over every action on every query.
func TestSearchIndex_CandidateEntryIndexesNarrowsToTheMatchingPostings(t *testing.T) {
	index := searchIndex{
		byToken:  map[string][]int{"pipeline": {1}},
		byAlias:  map[string][]int{},
		byDomain: map[string][]int{},
		byAction: map[string][]int{},
		all:      []int{0, 1, 2},
	}

	got := index.candidateEntryIndexes(normalizeSearchTerms("pipeline"))
	if joined := strings.Join(intsToStrings(got), ","); joined != "1" {
		t.Errorf("candidateEntryIndexes(pipeline) = %v, want only the indexed posting 1", got)
	}
}

// TestSearchDocumentIndexTokens_ReadsEveryField verifies that the token index
// carries a word from every searchable field of a document, in field order and
// without repeats. A field dropped here cannot be searched for at all: the
// index decides which entries a term even reaches, and the scorers below it
// never see an entry the index left out.
func TestSearchDocumentIndexTokens_ReadsEveryField(t *testing.T) {
	document := searchDocument{
		Backend:          "backendword",
		Capability:       "capabilityword",
		Resource:         "resourceword",
		Operation:        "operationword",
		Scope:            "scopeword",
		CanonicalID:      "canonical.idword",
		Tool:             "toolword",
		Domain:           "domainword",
		Action:           "actionword",
		FlatText:         "flat textword",
		IDWords:          []string{"idwordone"},
		DomainWords:      []string{"domainwordone"},
		ActionWords:      []string{"actionwordone"},
		Aliases:          []string{"alias_wordone"},
		Tags:             []string{"tagword", "backendword"},
		RequiredParams:   []string{"required_param"},
		SchemaProperties: []string{"schema_property"},
	}

	want := []string{
		"backendword", "capabilityword", "resourceword", "operationword", "scopeword",
		"canonical", "idword", "toolword", "domainword", "actionword", "flat", "textword",
		"idwordone", "domainwordone", "actionwordone", "alias", "wordone", "tagword",
		"required", "param", "schema", "property",
	}
	if got := searchDocumentIndexTokens(document); !slices.Equal(got, want) {
		t.Errorf("searchDocumentIndexTokens() = %v, want %v", got, want)
	}
}

func intsToStrings(values []int) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.Itoa(value))
	}
	return out
}
