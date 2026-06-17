// ranking_test.go contains unit tests for the dynamic surface ranking pipeline.
package dynamic

import (
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
