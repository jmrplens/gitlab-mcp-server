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

// TestReplaceSection_AbsentMarkerErrors verifies a file that carries neither
// marker is refused rather than rewritten, so a generator pointed at the wrong
// document fails instead of replacing somebody's prose.
func TestReplaceSection_AbsentMarkerErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unmarked.md")
	if err := os.WriteFile(path, []byte("# Nothing managed here\n"), 0o600); err != nil {
		t.Fatalf("write unmarked file: %v", err)
	}

	err := ReplaceSection(path, "<!-- S -->", "<!-- E -->", "NEW")
	if err == nil || !strings.Contains(err.Error(), "start marker") {
		t.Fatalf("ReplaceSection() error = %v, want the missing start marker", err)
	}
	got, readErr := os.ReadFile(path) //#nosec G304 -- a path this test built
	if readErr != nil || string(got) != "# Nothing managed here\n" {
		t.Errorf("file = %q, %v; want it untouched", got, readErr)
	}
}
