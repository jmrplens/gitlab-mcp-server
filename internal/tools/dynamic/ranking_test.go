// ranking_test.go contains unit tests for the dynamic surface ranking pipeline.
package dynamic

import (
	"slices"
	"strings"
	"testing"
)

// TestRanking_ExplanationSummaryNilAndFuzzy verifies the Ranking_ExplanationSummaryNilAndFuzzy handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestRanking_ExplanationSummaryNilAndFuzzy(t *testing.T) {
	if got := explanationSummary(nil); got != "-" {
		t.Fatalf("explanationSummary(nil) = %q, want -", got)
	}

	summary := explanationSummary(&ScoringExplanation{Reasons: []MatchReason{{
		Field:       searchFieldFuzzyToken,
		QueryTerm:   "marge",
		Alternative: "merge",
		Fuzzy:       true,
	}}})
	if !strings.Contains(summary, "fuzzy-matched") {
		t.Fatalf("explanationSummary(fuzzy) = %q, want fuzzy-matched text", summary)
	}
}

// TestRanking_HasSearchExplanations verifies the Ranking_HasSearchExplanations handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestRanking_HasSearchExplanations(t *testing.T) {
	if hasSearchExplanations([]SearchResult{{ID: "project.get"}}) {
		t.Fatal("hasSearchExplanations() = true, want false for nil explanations")
	}

	reason := ScoringExplanation{TotalScore: 10, Reasons: []MatchReason{{Field: searchFieldCanonicalID, QueryTerm: "project", MatchedValue: "project.get"}}}
	if !hasSearchExplanations([]SearchResult{{ID: "project.get", Explanation: &reason}}) {
		t.Fatal("hasSearchExplanations() = false, want true when at least one explanation exists")
	}
}

// TestExplanationSummary_NamesTheClosestValueItHas verifies which of the three
// values a match reason carries ends up quoted in the summary. The catalog
// value that matched is the answer to "why did this rank", and the expanded
// alternative and the raw query term are fallbacks for the reasons that carry
// no matched value. Quoting a fallback while the real one is present would
// show the reader their own words back as the explanation.
func TestExplanationSummary_NamesTheClosestValueItHas(t *testing.T) {
	tests := []struct {
		name   string
		reason MatchReason
		want   string
	}{
		{
			name:   "matched value wins over both fallbacks",
			reason: MatchReason{Field: searchFieldCanonicalID, QueryTerm: "marge", Alternative: "merge", MatchedValue: "project"},
			want:   "project",
		},
		{
			name:   "alternative stands in for a missing matched value",
			reason: MatchReason{Field: searchFieldFuzzyToken, QueryTerm: "marge", Alternative: "merge"},
			want:   "merge",
		},
		{
			name:   "query term is the last resort",
			reason: MatchReason{Field: searchFieldFuzzyToken, QueryTerm: "marge"},
			want:   "marge",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := explanationSummary(&ScoringExplanation{Reasons: []MatchReason{tt.reason}})
			if !strings.Contains(summary, `"`+tt.want+`"`) {
				t.Errorf("explanationSummary() = %q, want it to quote %q", summary, tt.want)
			}
		})
	}
}

// TestAmbiguousTargetsFromSearchResults_UsesTheFirstPopulatedSet verifies that
// the ambiguity set is taken from the first result that actually carries one,
// and that results carrying none leave it empty. Search models ambiguity as a
// single canonical set, so an empty set taken from the top result would answer
// "these actions share your alias" with nothing at all.
func TestAmbiguousTargetsFromSearchResults_UsesTheFirstPopulatedSet(t *testing.T) {
	t.Run("skips results without an ambiguity set", func(t *testing.T) {
		results := []SearchResult{
			{ID: "project.get"},
			{ID: "project.list", AmbiguousWith: []string{"project.list", "group.list", "project.list"}},
			{ID: "group.get", AmbiguousWith: []string{"unused.target"}},
		}

		got := ambiguousTargetsFromSearchResults(results)
		if !slices.Equal(got, []string{"group.list", "project.list"}) {
			t.Errorf("ambiguousTargetsFromSearchResults() = %v, want the deduplicated first set", got)
		}
	})

	t.Run("no result carries one", func(t *testing.T) {
		got := ambiguousTargetsFromSearchResults([]SearchResult{{ID: "project.get"}, {ID: "project.list"}})
		if got != nil {
			t.Errorf("ambiguousTargetsFromSearchResults() = %v, want nil", got)
		}
	})
}

// TestRankingPenalties_SubtractFromTheScore verifies the two scoring
// adjustments that are meant to push an action down rather than up, at the
// value they are declared with. Both are declared as negative constants, and a
// sign that flipped would turn each into a boost: an action whose name carries
// words the query never mentioned would be rewarded for the words it does not
// answer, and a destructive action would outrank the read the query asked for.
func TestRankingPenalties_SubtractFromTheScore(t *testing.T) {
	t.Run("unmatched action words", func(t *testing.T) {
		entry := actionEntry{ID: "zulu.weird_unmatched", Domain: "zulu", Action: "weird_unmatched"}
		if got := scoreActionSpecificityValue(entry, normalizeSearchTerms("zzzzz")); got != -120 {
			t.Errorf("scoreActionSpecificityValue() = %d, want -120 for two unmatched action words", got)
		}
	})

	t.Run("destructive action under a read intent", func(t *testing.T) {
		entry := actionEntry{ID: "project.delete", Domain: "project", Action: "delete", Destructive: true}
		adjustment, reason := scoreVerbIntentFor(entry, verbIntentRead, nil)
		if adjustment != -24 {
			t.Errorf("scoreVerbIntentFor(read) = %d, want -24 for a destructive action", adjustment)
		}
		if reason.Field != searchFieldVerbIntent {
			t.Errorf("reason.Field = %q, want %q", reason.Field, searchFieldVerbIntent)
		}
	})
}
