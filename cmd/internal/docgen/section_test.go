package docgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestComputeReplacedSection_ReplacesBetweenMarkers verifies the splice swaps
// content between the markers and preserves both markers and the trailing text.
func TestComputeReplacedSection_ReplacesBetweenMarkers(t *testing.T) {
	text := "<!-- START -->\nold content\n<!-- END -->\ntail"
	got, err := ComputeReplacedSection(text, "<!-- START -->", "<!-- END -->", "NEW")
	if err != nil {
		t.Fatalf("ComputeReplacedSection: %v", err)
	}
	if !strings.Contains(got, "<!-- START -->") || !strings.Contains(got, "<!-- END -->") {
		t.Fatalf("markers not preserved:\n%s", got)
	}
	if strings.Contains(got, "old content") {
		t.Fatalf("old content not replaced:\n%s", got)
	}
	if !strings.Contains(got, "NEW") || !strings.Contains(got, "tail") {
		t.Fatalf("new content or tail missing:\n%s", got)
	}
}

// TestComputeReplacedSection_MissingMarkersFailFast verifies a missing start or
// end marker returns a descriptive error rather than silently succeeding.
func TestComputeReplacedSection_MissingMarkersFailFast(t *testing.T) {
	if _, err := ComputeReplacedSection("no markers here", "<!-- START -->", "<!-- END -->", "NEW"); err == nil {
		t.Fatal("missing start marker: error = nil, want descriptive error")
	}
	if _, err := ComputeReplacedSection("<!-- START -->\nbody\n", "<!-- START -->", "<!-- END -->", "NEW"); err == nil {
		t.Fatal("missing end marker: error = nil, want descriptive error")
	}
}

// TestReplaceSection_RewritesFile verifies the file round-trip: read, splice, write.
func TestReplaceSection_RewritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(path, []byte("pre\n<!-- S -->\nold\n<!-- E -->\npost"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceSection(path, "<!-- S -->", "<!-- E -->", "NEW"); err != nil {
		t.Fatalf("ReplaceSection: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, "old") || !strings.Contains(got, "NEW") || !strings.Contains(got, "post") {
		t.Fatalf("file not spliced as expected:\n%s", got)
	}
}

// TestReplaceSection_MissingFileErrors verifies a read failure is wrapped.
func TestReplaceSection_MissingFileErrors(t *testing.T) {
	if err := ReplaceSection(filepath.Join(t.TempDir(), "nope.md"), "<!-- S -->", "<!-- E -->", "x"); err == nil {
		t.Fatal("ReplaceSection on missing file: error = nil, want read error")
	}
}
