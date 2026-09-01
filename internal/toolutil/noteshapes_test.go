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
	"time"

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

// assertNoteOutputScalars checks all scalar fields of a NoteOutput using a
// table-driven loop. Extracted from TestNoteOutputFromGitLab_FieldMapping to
// keep the test function's cyclomatic complexity below the linter threshold.
func assertNoteOutputScalars(t *testing.T, got NoteOutput) {
	t.Helper()
	for _, c := range []struct {
		name string
		ok   bool
	}{
		{"ID==42", got.ID == 42},
		{"Body", got.Body == "hello"},
		{"Attachment", got.Attachment == "file.txt"},
		{"Title", got.Title == "T"},
		{"FileName", got.FileName == "f.txt"},
		{"System", got.System},
		{"Internal", got.Internal},
		{"Resolvable", got.Resolvable},
		{"Resolved", got.Resolved},
		{"NoteableType", got.NoteableType == "MergeRequest"},
		{"NoteableID==7", got.NoteableID == 7},
		{"NoteableIID==3", got.NoteableIID == 3},
		{"CommitID", got.CommitID == "abc"},
		{"Type", got.Type == "DiffNote"},
		{"ProjectID==99", got.ProjectID == 99},
		// Confidential must mirror Internal (1:1 audit policy for MR/issue notes).
		{"Confidential==Internal", got.Confidential},
	} {
		if !c.ok {
			t.Errorf("NoteOutputFromGitLab: field %s mismatch in %+v", c.name, got)
		}
	}
}

// TestNoteOutputFromGitLab_FieldMapping verifies that all gl.Note fields are
// mapped to their canonical output keys by NoteOutputFromGitLab, including the
// dual Confidential=Internal mapping and RFC 3339 timestamp formatting.
func TestNoteOutputFromGitLab_FieldMapping(t *testing.T) {
	ts := testTimePtr("2026-01-15T10:00:00Z")
	n := &gl.Note{
		ID:           42,
		Body:         "hello",
		Attachment:   "file.txt",
		Title:        "T",
		FileName:     "f.txt",
		CreatedAt:    ts,
		UpdatedAt:    ts,
		ExpiresAt:    ts,
		System:       true,
		Internal:     true,
		Resolvable:   true,
		Resolved:     true,
		ResolvedAt:   ts,
		NoteableType: "MergeRequest",
		NoteableID:   7,
		NoteableIID:  3,
		CommitID:     "abc",
		Type:         "DiffNote",
		ProjectID:    99,
		Author:       gl.NoteAuthor{ID: 1, Username: "alice", Name: "Alice"},
		ResolvedBy:   gl.NoteResolvedBy{ID: 2, Username: "bob"},
	}
	got := NoteOutputFromGitLab(n)
	assertNoteOutputScalars(t, got)
	if got.Author == nil || got.Author.Username != "alice" {
		t.Errorf("NoteOutputFromGitLab: Author missing or wrong: %+v", got.Author)
	}
	if got.ResolvedBy == nil || got.ResolvedBy.Username != "bob" {
		t.Errorf("NoteOutputFromGitLab: ResolvedBy missing or wrong: %+v", got.ResolvedBy)
	}
	if got.CreatedAt == "" || got.UpdatedAt == "" || got.ExpiresAt == "" || got.ResolvedAt == "" {
		t.Errorf("NoteOutputFromGitLab: zero timestamp: created=%q updated=%q expires=%q resolved=%q",
			got.CreatedAt, got.UpdatedAt, got.ExpiresAt, got.ResolvedAt)
	}
}

// TestNoteOutputFromGitLab_ZeroNote verifies that NoteOutputFromGitLab
// returns a valid zero-value NoteOutput (no panic) and leaves optional
// sub-objects nil when the note has no author ID, no resolver, no position.
func TestNoteOutputFromGitLab_ZeroNote(t *testing.T) {
	got := NoteOutputFromGitLab(&gl.Note{})
	if got.ID != 0 || got.Body != "" {
		t.Errorf("NoteOutputFromGitLab zero-note: unexpected non-zero fields: %+v", got)
	}
	// Empty NoteAuthor (zero ID, empty Username) still produces a non-nil
	// Author pointer because NoteAuthor is always present on a note (non-pointer).
	if got.Author == nil {
		t.Error("NoteOutputFromGitLab zero-note: Author should be non-nil (always present)")
	}
	// Empty NoteResolvedBy → nil.
	if got.ResolvedBy != nil {
		t.Errorf("NoteOutputFromGitLab zero-note: ResolvedBy should be nil, got %+v", got.ResolvedBy)
	}
	// No diff position → nil.
	if got.Position != nil {
		t.Errorf("NoteOutputFromGitLab zero-note: Position should be nil, got %+v", got.Position)
	}
}

// TestNoteOutputJSONTags pins the on-wire JSON keys for NoteOutput. The
// UpdatedAt field must NOT carry omitempty (always present) while
// Resolvable and Resolved MUST carry omitempty (absent for non-resolvable
// notes). Any change is a breaking wire-format change.
func TestNoteOutputJSONTags(t *testing.T) {
	// UpdatedAt without omitempty: must appear even when empty.
	n := NoteOutput{ID: 1}
	raw, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal NoteOutput: %v", err)
	}
	if !strings.Contains(string(raw), `"updated_at":""`) {
		t.Errorf("NoteOutput.UpdatedAt must appear even when empty (no omitempty); got %s", raw)
	}
	// Resolvable/Resolved with omitempty: must be absent when false.
	if strings.Contains(string(raw), `"resolvable"`) {
		t.Errorf("NoteOutput.Resolvable should be omitted when false; got %s", raw)
	}
	if strings.Contains(string(raw), `"resolved"`) {
		t.Errorf("NoteOutput.Resolved should be omitted when false; got %s", raw)
	}
	// System, Internal, Confidential always present (no omitempty).
	if !strings.Contains(string(raw), `"system":false`) {
		t.Errorf("NoteOutput.System must always appear; got %s", raw)
	}
	if !strings.Contains(string(raw), `"internal":false`) {
		t.Errorf("NoteOutput.Internal must always appear; got %s", raw)
	}
	if !strings.Contains(string(raw), `"confidential":false`) {
		t.Errorf("NoteOutput.Confidential must always appear; got %s", raw)
	}
}

// TestDiscussionThreadNoteOutputJSONTags pins the on-wire JSON keys for
// DiscussionThreadNoteOutput. The UpdatedAt field MUST carry omitempty
// (absent for new threads), while Resolved and Resolvable must NOT carry
// omitempty (always present in thread context). Any change is a breaking
// wire-format change.
func TestDiscussionThreadNoteOutputJSONTags(t *testing.T) {
	n := DiscussionThreadNoteOutput{ID: 1}
	raw, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal DiscussionThreadNoteOutput: %v", err)
	}
	// UpdatedAt with omitempty: must be absent when empty.
	if strings.Contains(string(raw), `"updated_at"`) {
		t.Errorf("DiscussionThreadNoteOutput.UpdatedAt must be omitted when empty; got %s", raw)
	}
	// Resolved/Resolvable without omitempty: must appear even when false.
	if !strings.Contains(string(raw), `"resolved":false`) {
		t.Errorf("DiscussionThreadNoteOutput.Resolved must appear even when false; got %s", raw)
	}
	if !strings.Contains(string(raw), `"resolvable":false`) {
		t.Errorf("DiscussionThreadNoteOutput.Resolvable must appear even when false; got %s", raw)
	}
}

// TestDiscussionThreadOutputJSONTags pins the on-wire JSON keys for
// DiscussionThreadOutput and verifies the Notes slice key.
func TestDiscussionThreadOutputJSONTags(t *testing.T) {
	d := DiscussionThreadOutput{ID: "abc123", IndividualNote: true}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal DiscussionThreadOutput: %v", err)
	}
	if !strings.Contains(string(raw), `"id":"abc123"`) {
		t.Errorf("DiscussionThreadOutput.ID tag wrong; got %s", raw)
	}
	if !strings.Contains(string(raw), `"individual_note":true`) {
		t.Errorf("DiscussionThreadOutput.IndividualNote tag wrong; got %s", raw)
	}
	if !strings.Contains(string(raw), `"notes":null`) {
		t.Errorf("DiscussionThreadOutput.Notes must appear; got %s", raw)
	}
}

// testTimePtr parses an RFC 3339 string into a *time.Time for test fixtures.
func testTimePtr(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic("testTimePtr: " + err.Error())
	}
	return &t
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
		t.Run(key, func(t *testing.T) {
			if !strings.Contains(string(raw), key) {
				t.Errorf("NoteUserOutput JSON missing %q: %s", key, raw)
			}
		})
	}
	for _, key := range []string{`"email"`, `"state"`, `"avatar_url"`} {
		t.Run(key, func(t *testing.T) {
			if strings.Contains(string(raw), key) {
				t.Errorf("NoteUserOutput JSON should omit empty %q: %s", key, raw)
			}
		})
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
