package autoupdate

import "testing"

func TestNewGitHubSource_NoToken(t *testing.T) {
	src, err := newGitHubSource("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src == nil {
		t.Fatal("expected non-nil source")
	}
}

func TestNewGitHubSource_WithToken(t *testing.T) {
	src, err := newGitHubSource("ghp_test_token_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src == nil {
		t.Fatal("expected non-nil source")
	}
}
