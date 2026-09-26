package docgen

import (
	"errors"
	"io/fs"
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

// TestComputeReplacedSection_Output_KeepsTheOrderOfTheFile verifies the exact
// text the splice produces: what preceded the start marker stays in front of
// the new content, and what follows the end marker stays behind it.
//
// The assertion is the whole string because a Contains check passes whichever
// way round the two halves are joined, which is how a file's order came to be
// unasserted while every piece of it was.
func TestComputeReplacedSection_Output_KeepsTheOrderOfTheFile(t *testing.T) {
	got, err := ComputeReplacedSection("intro\n<!-- START -->\nold\n<!-- END -->\ntail\n", "<!-- START -->", "<!-- END -->", "NEW")
	if err != nil {
		t.Fatalf("ComputeReplacedSection() error = %v", err)
	}
	if want := "intro\n<!-- START -->\n\nNEW\n<!-- END -->\ntail\n"; got != want {
		t.Errorf("ComputeReplacedSection() = %q, want %q", got, want)
	}
}

// TestComputeReplacedSection_EmptySection_IsFilledRatherThanRefused verifies a
// managed region whose end marker sits immediately after its start marker is
// filled, since that is the shape a document carries before its section has
// ever been generated.
func TestComputeReplacedSection_EmptySection_IsFilledRatherThanRefused(t *testing.T) {
	got, err := ComputeReplacedSection("<!-- START --><!-- END -->\n", "<!-- START -->", "<!-- END -->", "NEW")
	if err != nil {
		t.Fatalf("ComputeReplacedSection() error = %v", err)
	}
	if want := "<!-- START -->\n\nNEW\n<!-- END -->\n"; got != want {
		t.Errorf("ComputeReplacedSection() = %q, want %q", got, want)
	}
}

// TestComputeReplacedSection_Refusals_NameTheMarkerThatIsMissing verifies each
// refusal quotes the marker it looked for and not the other one, so a
// generator pointed at the wrong document says which end of the region it
// could not find rather than naming whichever marker came to hand.
func TestComputeReplacedSection_Refusals_NameTheMarkerThatIsMissing(t *testing.T) {
	const start, end = "<!-- START -->", "<!-- END -->"

	tests := []struct {
		name      string
		text      string
		wantName  string
		otherName string
	}{
		{name: "start marker absent", text: "nothing managed here\n", wantName: start, otherName: end},
		{name: "end marker absent", text: start + "\nbody\n", wantName: end, otherName: start},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ComputeReplacedSection(tt.text, start, end, "NEW")
			if err == nil {
				t.Fatal("ComputeReplacedSection() error = nil, want a refusal")
			}
			if !strings.Contains(err.Error(), tt.wantName) {
				t.Errorf("error = %q, want it to name %q", err, tt.wantName)
			}
			if strings.Contains(err.Error(), tt.otherName) {
				t.Errorf("error = %q, want it not to name %q", err, tt.otherName)
			}
		})
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

// TestReplaceSection_MissingFileErrors verifies a read failure is wrapped: the
// error names the document and still answers errors.Is for fs.ErrNotExist, so
// a generator can tell a document that is not there from one it could not
// splice.
func TestReplaceSection_MissingFileErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.md")

	err := ReplaceSection(path, "<!-- S -->", "<!-- E -->", "x")
	if err == nil {
		t.Fatal("ReplaceSection on missing file: error = nil, want read error")
	}
	if !strings.HasPrefix(err.Error(), "reading "+path+": ") {
		t.Errorf("ReplaceSection() error = %q, want it to begin with %q", err, "reading "+path+": ")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("errors.Is(%v, fs.ErrNotExist) = false, want the read failure wrapped", err)
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
