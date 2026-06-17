// case_fixture_cache_test.go covers the typed fixture output cache that
// de-duplicates expensive GitLab fixture preparation across case attempts.

package evaluator

import "testing"

// TestFixtureOutputCache_ReusesClonedOutputForSameKey verifies that the
// fixture output cache stores one cloned [FixtureOutput] per idempotency key
// and returns defensive copies so caller mutations cannot poison later
// attempts.
//
// The test increments a counter inside the ensure callback and confirms the
// counter only fires once even when two callers ask for the same key. It
// then mutates the first returned map and asserts that a subsequent lookup
// returns the original cloned value, not the mutated one. This protects
// case attempts from accidentally clobbering shared fixture state.
func TestFixtureOutputCache_ReusesClonedOutputForSameKey(t *testing.T) {
	cache := newFixtureOutputCache()
	var calls int
	output, err := cache.ensure("case:key", func() (FixtureOutput, error) {
		calls++
		return FixtureOutput{"project_id": "123"}, nil
	})
	if err != nil {
		t.Fatalf("first ensure error = %v", err)
	}
	output["project_id"] = "mutated"
	second, err := cache.ensure("case:key", func() (FixtureOutput, error) {
		calls++
		return FixtureOutput{"project_id": "456"}, nil
	})
	if err != nil {
		t.Fatalf("second ensure error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("ensure calls = %d, want 1", calls)
	}
	if got := second["project_id"]; got != "123" {
		t.Fatalf("cached project_id = %q, want original cloned value", got)
	}
}
