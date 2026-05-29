package evaluator

import "testing"

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
