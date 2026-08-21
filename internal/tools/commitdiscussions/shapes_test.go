// shapes_test.go contains unit tests for the commit discussion output
// converters and position-input builders, exercising the full 1:1 field
// mapping (author, resolved_by, position, line range) plus nil-guard paths.
// Shared note shapes (NoteUserOutput, LinePositionOutput, LineRangeOutput,
// NotePositionOutput) live in internal/toolutil since DEDUP-001 wave 2;
// the canonical converter unit tests are in toolutil/noteshapes_test.go.
package commitdiscussions

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestNoteToOutput_FullMapping verifies that NoteToOutput mirrors every gl.Note
// field, including the author, resolved_by, and position sub-objects with a
// multi-line line range, and formats timestamps as RFC 3339.
func TestNoteToOutput_FullMapping(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	expires := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	resolved := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	n := &gl.Note{
		ID:           42,
		Type:         gl.NoteTypeValue("DiffNote"),
		Body:         "body",
		Attachment:   "file.png",
		Title:        "title",
		FileName:     "f.go",
		Author:       gl.NoteAuthor{ID: 1, Username: "alice", Email: "a@x.io", Name: "Alice", State: "active", AvatarURL: "av", WebURL: "web"},
		System:       true,
		CreatedAt:    &created,
		UpdatedAt:    &updated,
		ExpiresAt:    &expires,
		CommitID:     "abc",
		NoteableID:   7,
		NoteableType: "Commit",
		ProjectID:    9,
		NoteableIID:  3,
		Resolvable:   true,
		Resolved:     true,
		ResolvedAt:   &resolved,
		ResolvedBy:   gl.NoteResolvedBy{ID: 2, Username: "bob", Name: "Bob", State: "active"},
		Internal:     true,
		Confidential: true, //nolint:staticcheck // deprecated SDK field/API is exposed deliberately: the 1:1 parity policy mirrors the full surface while upstream keeps it
		Position: &gl.NotePosition{
			BaseSHA: "base", StartSHA: "start", HeadSHA: "head",
			PositionType: "text", NewPath: "new.go", NewLine: 10,
			OldPath: "old.go", OldLine: 5,
			LineRange: &gl.LineRange{
				StartRange: &gl.LinePosition{LineCode: "lc1", Type: "new", OldLine: 1, NewLine: 2},
				EndRange:   &gl.LinePosition{LineCode: "lc2", Type: "old", OldLine: 3, NewLine: 4},
			},
		},
	}

	out := NoteToOutput(n)
	assertNoteScalars(t, out)
	assertNoteUsers(t, out)
	assertNoteTimestamps(t, out)
	assertNotePosition(t, out)
}

// assertNoteScalars checks the scalar and flag fields of a fully-mapped note.
func assertNoteScalars(t *testing.T, out NoteOutput) {
	t.Helper()
	if out.ID != 42 || out.Body != "body" || out.Type != "DiffNote" {
		t.Fatalf("scalar mapping wrong: %+v", out)
	}
	if !out.Internal || !out.Confidential || !out.System || !out.Resolved || !out.Resolvable {
		t.Fatalf("flag mapping wrong: %+v", out)
	}
}

// assertNoteUsers checks the author and resolved_by sub-objects.
func assertNoteUsers(t *testing.T, out NoteOutput) {
	t.Helper()
	if out.Author == nil || out.Author.Username != "alice" || out.Author.Email != "a@x.io" {
		t.Fatalf("author mapping wrong: %+v", out.Author)
	}
	if out.ResolvedBy == nil || out.ResolvedBy.Username != "bob" {
		t.Fatalf("resolved_by mapping wrong: %+v", out.ResolvedBy)
	}
}

// assertNoteTimestamps checks the RFC 3339 timestamp formatting.
func assertNoteTimestamps(t *testing.T, out NoteOutput) {
	t.Helper()
	if out.CreatedAt != "2026-01-01T00:00:00Z" || out.UpdatedAt != "2026-01-02T00:00:00Z" ||
		out.ExpiresAt != "2026-02-01T00:00:00Z" || out.ResolvedAt != "2026-01-03T00:00:00Z" {
		t.Fatalf("timestamp mapping wrong: %+v", out)
	}
}

// assertNotePosition checks the position and multi-line line range sub-objects.
func assertNotePosition(t *testing.T, out NoteOutput) {
	t.Helper()
	if out.Position == nil || out.Position.BaseSHA != "base" || out.Position.NewLine != 10 {
		t.Fatalf("position mapping wrong: %+v", out.Position)
	}
	if out.Position.LineRange == nil || out.Position.LineRange.Start == nil || out.Position.LineRange.Start.LineCode != "lc1" {
		t.Fatalf("line range start wrong: %+v", out.Position.LineRange)
	}
	if out.Position.LineRange.End == nil || out.Position.LineRange.End.NewLine != 4 {
		t.Fatalf("line range end wrong: %+v", out.Position.LineRange)
	}
}

// TestNoteToOutput_Minimal verifies the nil-guard paths: a nil note returns a
// zero value, a note with no resolved_by user and no position omits those
// sub-objects, and nil timestamps render as empty strings.
func TestNoteToOutput_Minimal(t *testing.T) {
	if got := NoteToOutput(nil); got.ID != 0 || got.Author != nil {
		t.Fatalf("nil note should produce zero value, got: %+v", got)
	}
	out := NoteToOutput(&gl.Note{ID: 1, Body: "hi"})
	if out.ResolvedBy != nil {
		t.Errorf("expected nil resolved_by, got %+v", out.ResolvedBy)
	}
	if out.Position != nil {
		t.Errorf("expected nil position, got %+v", out.Position)
	}
	if out.CreatedAt != "" {
		t.Errorf("expected empty created_at, got %q", out.CreatedAt)
	}
}

// TestNotePositionOutput_LineRangeEmpty verifies that a line range whose
// endpoints are both nil collapses to a nil LineRange on the output.
func TestNotePositionOutput_LineRangeEmpty(t *testing.T) {
	out := toolutil.NewNotePositionOutput(&gl.NotePosition{BaseSHA: "b", LineRange: &gl.LineRange{}})
	if out == nil {
		t.Fatal("expected non-nil position output")
	}
	if out.LineRange != nil {
		t.Errorf("expected nil line range when both endpoints nil, got %+v", out.LineRange)
	}
	if toolutil.NewNotePositionOutput(nil) != nil {
		t.Error("expected nil for nil position")
	}
	if toolutil.NewLineRangeOutput(nil) != nil {
		t.Error("expected nil for nil line range")
	}
	if toolutil.NewLinePositionOutput(nil) != nil {
		t.Error("expected nil for nil line position")
	}
}

// TestToOutput verifies discussion conversion, including the nil-discussion
// guard and per-note conversion.
func TestToOutput(t *testing.T) {
	if got := ToOutput(nil); got.ID != "" || got.Notes != nil {
		t.Fatalf("nil discussion should produce zero value, got: %+v", got)
	}
	d := &gl.Discussion{
		ID:             "d1",
		IndividualNote: true,
		Notes:          []*gl.Note{{ID: 1, Author: gl.NoteAuthor{Username: "alice"}}},
	}
	out := ToOutput(d)
	if out.ID != "d1" || !out.IndividualNote || len(out.Notes) != 1 {
		t.Fatalf("discussion mapping wrong: %+v", out)
	}
	if out.Notes[0].Author == nil || out.Notes[0].Author.Username != "alice" {
		t.Fatalf("note author wrong: %+v", out.Notes[0])
	}
}

// TestBuildNotePosition_Full verifies that buildNotePosition maps a full
// PositionInput (including a multi-line line range) onto gl.NotePosition.
func TestBuildNotePosition_Full(t *testing.T) {
	in := &PositionInput{
		BaseSHA: "b", StartSHA: "s", HeadSHA: "h",
		PositionType: "text", NewPath: "new.go", NewLine: 10,
		OldPath: "old.go", OldLine: 5,
		LineRange: &LineRangeInput{
			Start: &LinePositionInput{LineCode: "lc1", Type: "new", OldLine: 1, NewLine: 2},
			End:   &LinePositionInput{LineCode: "lc2", Type: "old", OldLine: 3, NewLine: 4},
		},
	}
	pos := buildNotePosition(in)
	if pos.BaseSHA != "b" || pos.PositionType != "text" || pos.NewLine != 10 || pos.OldLine != 5 {
		t.Fatalf("position scalars wrong: %+v", pos)
	}
	if pos.LineRange == nil || pos.LineRange.StartRange == nil || pos.LineRange.StartRange.LineCode != "lc1" {
		t.Fatalf("line range start wrong: %+v", pos.LineRange)
	}
	if pos.LineRange.EndRange == nil || pos.LineRange.EndRange.NewLine != 4 {
		t.Fatalf("line range end wrong: %+v", pos.LineRange)
	}
}

// TestBuildNotePosition_Defaults verifies that an empty position_type defaults
// to "text" and that an empty/absent line range collapses to nil.
func TestBuildNotePosition_Defaults(t *testing.T) {
	pos := buildNotePosition(&PositionInput{BaseSHA: "b"})
	if pos.PositionType != "text" {
		t.Errorf("expected default position_type=text, got %q", pos.PositionType)
	}
	if pos.LineRange != nil {
		t.Errorf("expected nil line range, got %+v", pos.LineRange)
	}
	// Line range present but both endpoints nil collapses to nil.
	pos = buildNotePosition(&PositionInput{BaseSHA: "b", PositionType: "image", LineRange: &LineRangeInput{}})
	if pos.PositionType != "image" {
		t.Errorf("expected position_type=image, got %q", pos.PositionType)
	}
	if pos.LineRange != nil {
		t.Errorf("expected nil line range for empty endpoints, got %+v", pos.LineRange)
	}
	if buildLineRange(nil) != nil {
		t.Error("expected nil for nil line range input")
	}
	if buildLinePosition(nil) != nil {
		t.Error("expected nil for nil line position input")
	}
}

// TestFirstNoteAuthor verifies the firstNoteAuthor and noteAuthorUsername
// nil-guard paths used by the list/note Markdown formatters.
func TestFirstNoteAuthor(t *testing.T) {
	if got := firstNoteAuthor(Output{}); got != "" {
		t.Errorf("expected empty author for no notes, got %q", got)
	}
	if got := firstNoteAuthor(Output{Notes: []*NoteOutput{nil}}); got != "" {
		t.Errorf("expected empty author for nil note, got %q", got)
	}
	if got := noteAuthorUsername(NoteOutput{}); got != "" {
		t.Errorf("expected empty author for nil author, got %q", got)
	}
	d := Output{Notes: []*NoteOutput{{Author: &toolutil.NoteUserOutput{Username: "alice"}}}}
	if got := firstNoteAuthor(d); got != "alice" {
		t.Errorf("expected alice, got %q", got)
	}
}
