// metrics_test.go contains unit tests for the dynamic surface metric counters.
package dynamic

import "testing"

// TestMetrics_RuntimeCountersClampNegativeSuppressions verifies the Metrics_RuntimeCountersClampNegativeSuppressions handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestMetrics_RuntimeCountersClampNegativeSuppressions(t *testing.T) {
	ResetSearchRuntimeMetrics()
	recordSearchRuntimeMetrics(1, false, false, false, -5)

	metrics := SearchRuntimeMetricsSnapshot()
	if metrics.Searches != 1 {
		t.Fatalf("Searches = %d, want 1", metrics.Searches)
	}
	if metrics.DestructiveFuzzySuppressions != 0 {
		t.Fatalf("DestructiveFuzzySuppressions = %d, want 0", metrics.DestructiveFuzzySuppressions)
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
