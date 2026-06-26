// noteshapes_test.go contains unit tests for the shared note-shape
// converters exposed by toolutil. The previous per-package tests were
// retained by deleting the local shapes.go files; this file replaces
// the shared unit-test surface so future regressions in any consumer
// (issuenotes, mrnotes, snippetnotes, mrdiscussions, commitdiscussions)
// are caught from the canonical home.
package toolutil

import (
	"encoding/json"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestNewNoteUserOutputFromAuthor pins the additive author conversion
// (always non-nil; full NoteAuthor field coverage).
func TestNewNoteUserOutputFromAuthor(t *testing.T) {
	got := NewNoteUserOutputFromAuthor(gl.NoteAuthor{
		ID: 1, Username: "alice", Email: "alice@example.com",
		Name: "Alice", State: "active",
		AvatarURL: "https://example.com/a.png", WebURL: "https://example.com/alice",
	})
	if got == nil {
		t.Fatal("NewNoteUserOutputFromAuthor returned nil for a populated author")
	}
	if got.ID != 1 || got.Username != "alice" || got.Email != "alice@example.com" ||
		got.Name != "Alice" || got.State != "active" ||
		got.AvatarURL != "https://example.com/a.png" || got.WebURL != "https://example.com/alice" {
		t.Errorf("NewNoteUserOutputFromAuthor field mismatch: %+v", got)
	}
}

// TestNewNoteUserOutputFromResolvedBy verifies the nil-on-empty contract
// (the resolved_by JSON key must be absent when no user has resolved the
// note, per the locked canonical-key convention).
func TestNewNoteUserOutputFromResolvedBy(t *testing.T) {
	if got := NewNoteUserOutputFromResolvedBy(gl.NoteResolvedBy{}); got != nil {
		t.Errorf("empty resolved-by must return nil, got %+v", got)
	}
	got := NewNoteUserOutputFromResolvedBy(gl.NoteResolvedBy{ID: 7, Username: "bob"})
	if got == nil || got.ID != 7 || got.Username != "bob" {
		t.Errorf("populated resolved-by: %+v", got)
	}
}

// TestNewLinePositionOutput pins the line-position conversion (nil-on-nil).
func TestNewLinePositionOutput(t *testing.T) {
	if got := NewLinePositionOutput(nil); got != nil {
		t.Errorf("nil line position must return nil, got %+v", got)
	}
	got := NewLinePositionOutput(&gl.LinePosition{
		LineCode: "abc123", Type: "new",
		OldLine: 5, NewLine: 6,
	})
	if got == nil || got.LineCode != "abc123" || got.Type != "new" ||
		got.OldLine != 5 || got.NewLine != 6 {
		t.Errorf("populated line position: %+v", got)
	}
}

// TestNewLineRangeOutput pins the line-range conversion (nil on missing
// or both-endpoints-nil).
func TestNewLineRangeOutput(t *testing.T) {
	if got := NewLineRangeOutput(nil); got != nil {
		t.Errorf("nil line range must return nil, got %+v", got)
	}
	if got := NewLineRangeOutput(&gl.LineRange{}); got != nil {
		t.Errorf("empty line range must return nil, got %+v", got)
	}
	got := NewLineRangeOutput(&gl.LineRange{EndRange: &gl.LinePosition{LineCode: "y", NewLine: 5}})
	if got == nil || got.End == nil || got.End.LineCode != "y" {
		t.Errorf("end-only line range: %+v", got)
	}
	if got.Start != nil {
		t.Errorf("end-only line range should have nil start, got %+v", got.Start)
	}
}

// TestNewNotePositionOutput pins the position conversion (nil-on-nil
// and full field coverage when populated).
func TestNewNotePositionOutput(t *testing.T) {
	if got := NewNotePositionOutput(nil); got != nil {
		t.Errorf("nil position must return nil, got %+v", got)
	}
	got := NewNotePositionOutput(&gl.NotePosition{
		BaseSHA: "b", StartSHA: "s", HeadSHA: "h",
		PositionType: "text", NewPath: "n.go", NewLine: 2,
		OldPath: "o.go", OldLine: 1,
		LineRange: &gl.LineRange{StartRange: &gl.LinePosition{LineCode: "s"}},
	})
	if got == nil || got.BaseSHA != "b" || got.StartSHA != "s" || got.HeadSHA != "h" ||
		got.PositionType != "text" || got.NewPath != "n.go" || got.NewLine != 2 ||
		got.OldPath != "o.go" || got.OldLine != 1 {
		t.Errorf("populated position field mismatch: %+v", got)
	}
	if got.LineRange == nil || got.LineRange.Start == nil {
		t.Errorf("position.LineRange.Start should be populated, got %+v", got.LineRange)
	}
}

// TestNoteShapeJSONTags pins the on-wire JSON keys for the shared
// shapes. Any change to the tag set is a breaking change for MCP
// consumers and must be reviewed.
func TestNoteShapeJSONTags(t *testing.T) {
	raw, err := json.Marshal(&NoteUserOutput{
		ID: 1, Username: "u", Name: "n", WebURL: "w",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"id":1`, `"username":"u"`, `"name":"n"`, `"web_url":"w"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("NoteUserOutput JSON missing %q: %s", key, raw)
		}
	}
	for _, key := range []string{`"email"`, `"state"`, `"avatar_url"`} {
		if strings.Contains(string(raw), key) {
			t.Errorf("NoteUserOutput JSON should omit empty %q: %s", key, raw)
		}
	}
	raw, err = json.Marshal(&LinePositionOutput{LineCode: "x"})
	if err != nil {
		t.Fatalf("marshal line position: %v", err)
	}
	if !strings.Contains(string(raw), `"line_code":"x"`) {
		t.Errorf("LinePositionOutput JSON missing line_code: %s", raw)
	}
	if strings.Contains(string(raw), `"old_line"`) {
		t.Errorf("LinePositionOutput should omit empty old_line: %s", raw)
	}
}
