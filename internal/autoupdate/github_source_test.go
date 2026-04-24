// github_source_test.go contains unit tests for the GitHub-backed update
// source constructor.

package autoupdate

import "testing"

// TestNewGitHubSource_ReturnsNonNilSource verifies that newGitHubSource constructs a non-nil update source without error.
func TestNewGitHubSource_ReturnsNonNilSource(t *testing.T) {
	src, err := newGitHubSource()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src == nil {
		t.Fatal("expected non-nil source")
	}
}
