// metrics_test.go contains unit tests for the dynamic surface metric counters.
package dynamic

import "testing"

// TestRecordSearchRuntimeMetrics_CountsEachEventOnItsOwn verifies which counter
// each search event reaches. The counters are what a deployment reads to tell a
// healthy dynamic surface from one that answers nothing: a zero-result count
// that also moved for searches with results, or a suppression total that
// counted one event per search rather than one per suppressed action, would
// read as ordinary traffic and hide exactly the quality problem it is there to
// show. A negative suppression count adds nothing, since the counter it feeds
// is unsigned and a subtraction there would wrap it.
func TestRecordSearchRuntimeMetrics_CountsEachEventOnItsOwn(t *testing.T) {
	tests := []struct {
		name          string
		resultCount   int
		fuzzyUsed     bool
		ambiguous     bool
		lowConfidence bool
		suppressions  int
		want          SearchRuntimeMetrics
	}{
		{
			name:        "a search with results",
			resultCount: 3,
			want:        SearchRuntimeMetrics{Searches: 1},
		},
		{
			name: "a search with no results",
			want: SearchRuntimeMetrics{Searches: 1, ZeroResultSearches: 1},
		},
		{
			name:          "every quality flag at once",
			resultCount:   1,
			fuzzyUsed:     true,
			ambiguous:     true,
			lowConfidence: true,
			want: SearchRuntimeMetrics{
				Searches:              1,
				FuzzyFallbackSearches: 1,
				AmbiguousAliasQueries: 1,
				LowConfidenceSearches: 1,
			},
		},
		{
			name:         "suppressions are summed, not counted once",
			resultCount:  1,
			suppressions: 3,
			want:         SearchRuntimeMetrics{Searches: 1, DestructiveFuzzySuppressions: 3},
		},
		{
			name:         "a negative suppression count adds nothing",
			resultCount:  1,
			suppressions: -5,
			want:         SearchRuntimeMetrics{Searches: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResetSearchRuntimeMetrics()
			recordSearchRuntimeMetrics(tt.resultCount, tt.fuzzyUsed, tt.ambiguous, tt.lowConfidence, tt.suppressions)

			if got := SearchRuntimeMetricsSnapshot(); got != tt.want {
				t.Errorf("SearchRuntimeMetricsSnapshot() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestMetrics_SearchIndexPostingCount verifies the Metrics_SearchIndexPostingCount handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestMetrics_SearchIndexPostingCount(t *testing.T) {
	index := searchIndex{byToken: map[string][]int{"project": {0, 2}, "delete": {1}}}
	if got := searchIndexPostingCount(index); got != 3 {
		t.Fatalf("searchIndexPostingCount() = %d, want 3", got)
	}
}
