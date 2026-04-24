// github_source_test.go contains unit tests for the GitHub-backed update
// source constructor.
package autoupdate

import "testing"

func TestNewGitHubSource_ReturnsNonNilSource(t *testing.T) {
	src, err := newGitHubSource()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src == nil {
		t.Fatal("expected non-nil source")
	}
}
