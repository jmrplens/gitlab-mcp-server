// note_shapes_test.go contains unit tests for the shared note-shape
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

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestNewNoteUserOutputFromAuthor pins the additive author conversion
// (always non-nil; every NoteAuthor field, plus the two UserBasic fields the
// captured response adds and the email GitLab does not send left out).
func TestNewNoteUserOutputFromAuthor(t *testing.T) {
	got := NewNoteUserOutputFromAuthor(gl.NoteAuthor{
		ID: 1, Username: "alice", Email: "alice@example.com",
		Name: "Alice", State: "active",
		AvatarURL: "https://example.com/a.png", WebURL: "https://example.com/alice",
	}, NoteUserExtra{PublicEmail: "alice@public.example", Locked: true})
	if got == nil {
		t.Fatal("NewNoteUserOutputFromAuthor returned nil for a populated author")
	}
	want := &NoteUserOutput{
		ID: 1, Username: "alice", PublicEmail: "alice@public.example", Name: "Alice", State: "active", Locked: true,
		AvatarURL: "https://example.com/a.png", WebURL: "https://example.com/alice",
	}
	if *got != *want {
		t.Errorf("NewNoteUserOutputFromAuthor() = %+v, want %+v", got, want)
	}
}

// TestNewNoteUserOutputFromResolvedBy verifies the nil-on-empty contract
// (the resolved_by JSON key must be absent when no user has resolved the
// note, per the locked canonical-key convention) and that the captured
// fields reach a populated resolver.
func TestNewNoteUserOutputFromResolvedBy(t *testing.T) {
	if got := NewNoteUserOutputFromResolvedBy(gl.NoteResolvedBy{}, NoteUserExtra{Locked: true}); got != nil {
		t.Errorf("empty resolved-by must return nil, got %+v", got)
	}
	got := NewNoteUserOutputFromResolvedBy(gl.NoteResolvedBy{ID: 7, Username: "bob"}, NoteUserExtra{PublicEmail: "bob@public.example"})
	if got == nil || got.ID != 7 || got.Username != "bob" || got.PublicEmail != "bob@public.example" {
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
		// Confidential is GitLab's own value, read from the captured response.
		{"Confidential", got.Confidential},
		{"Imported", got.Imported},
		{"ImportedFrom", got.ImportedFrom == "github"},
		{"CommandsChanges", got.CommandsChanges["label"] == "bug"},
		{"Suggestions", len(got.Suggestions) == 1 && got.Suggestions[0].ID == 5 && got.Suggestions[0].ToContent == "fixed"},
	} {
		if !c.ok {
			t.Errorf("NoteOutputFromGitLab: field %s mismatch in %+v", c.name, got)
		}
	}
}

// fullNote is a note carrying every field the two converters read, on the
// SDK side and on the captured side.
func fullNote() (*gl.Note, NoteExtra) {
	ts := testTimePtr("2026-01-15T10:00:00Z")
	note := &gl.Note{
		ID:           42,
		Body:         "hello",
		CreatedAt:    ts,
		UpdatedAt:    ts,
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
	extra := NoteExtra{
		Confidential:    true,
		Imported:        true,
		ImportedFrom:    "github",
		CommandsChanges: map[string]any{"label": "bug"},
		Suggestions:     []SuggestionOutput{{ID: 5, FromLine: 1, ToLine: 2, Appliable: true, FromContent: "broken", ToContent: "fixed"}},
		Author:          NoteUserExtra{PublicEmail: "alice@public.example", Locked: true},
		ResolvedBy:      NoteUserExtra{Locked: false},
	}
	return note, extra
}

// TestNoteOutputFromGitLab_FieldMapping verifies that every gl.Note field
// and every captured field is mapped to its canonical output key by
// NoteOutputFromGitLab, with RFC 3339 timestamp formatting.
func TestNoteOutputFromGitLab_FieldMapping(t *testing.T) {
	n, extra := fullNote()

	got := NoteOutputFromGitLab(n, extra)

	assertNoteOutputScalars(t, got)
	if got.Author == nil || got.Author.Username != "alice" || !got.Author.Locked || got.Author.PublicEmail != "alice@public.example" {
		t.Errorf("NoteOutputFromGitLab: Author missing or wrong: %+v", got.Author)
	}
	if got.ResolvedBy == nil || got.ResolvedBy.Username != "bob" || got.ResolvedBy.Locked {
		t.Errorf("NoteOutputFromGitLab: ResolvedBy missing or wrong: %+v", got.ResolvedBy)
	}
	if got.CreatedAt == "" || got.UpdatedAt == "" || got.ResolvedAt == "" {
		t.Errorf("NoteOutputFromGitLab: zero timestamp: created=%q updated=%q resolved=%q",
			got.CreatedAt, got.UpdatedAt, got.ResolvedAt)
	}
}

// TestDiscussionThreadNoteOutputFromGitLab_FieldMapping verifies the thread
// note converter reads what client-go decoded: the scalars, the timestamps
// and the two users.
func TestDiscussionThreadNoteOutputFromGitLab_FieldMapping(t *testing.T) {
	n, extra := fullNote()

	got := DiscussionThreadNoteOutputFromGitLab(n, extra)

	if got.ID != 42 || got.Body != "hello" || !got.Resolved || !got.Resolvable || got.Type != "DiffNote" || got.ProjectID != 99 ||
		got.CreatedAt == "" || got.UpdatedAt == "" || got.ResolvedAt == "" ||
		got.Author == nil || got.Author.Username != "alice" || got.ResolvedBy == nil || got.ResolvedBy.Username != "bob" {
		t.Errorf("DiscussionThreadNoteOutputFromGitLab() = %+v, want the SDK's half of the note", got)
	}
}

// TestDiscussionThreadNoteOutputFromGitLab_CapturedHalf verifies the same
// converter reads what the capture decoded beside the SDK: the fields
// client-go does not model, on the note and on its author.
func TestDiscussionThreadNoteOutputFromGitLab_CapturedHalf(t *testing.T) {
	n, extra := fullNote()

	got := DiscussionThreadNoteOutputFromGitLab(n, extra)

	if !got.Confidential || !got.Imported || got.ImportedFrom != "github" || got.CommandsChanges["label"] != "bug" ||
		len(got.Suggestions) != 1 || got.Author == nil || !got.Author.Locked {
		t.Errorf("DiscussionThreadNoteOutputFromGitLab() = %+v, want the captured half of the note", got)
	}
}

// TestDiscussionThreadNoteOutputFromGitLab_NilNote verifies a nil note, which
// the SDK's []*gl.Note can hold, converts to the zero value rather than a
// dereference.
func TestDiscussionThreadNoteOutputFromGitLab_NilNote(t *testing.T) {
	_, extra := fullNote()

	zero := DiscussionThreadNoteOutputFromGitLab(nil, extra)

	if zero.ID != 0 || zero.Author != nil {
		t.Errorf("a nil note converted to %+v, want the zero value", zero)
	}
}

// TestDiscussionThreadOutputFromGitLab_ReadsBothHalves verifies the thread
// converter: the thread's own resolution state from the capture, one extra
// per note by position, a note beyond what the capture holds converted with
// none, a nil discussion to the zero value, and the list form keeping the
// pairing by position with a short list of extras.
func TestDiscussionThreadOutputFromGitLab_ReadsBothHalves(t *testing.T) {
	n, extra := fullNote()
	discussion := &gl.Discussion{ID: "d1", IndividualNote: true, Notes: []*gl.Note{n, {ID: 43}, nil}}
	threadExtra := DiscussionExtra{Resolvable: true, Resolved: true, Notes: []NoteExtra{extra, {Imported: true}}}

	got := DiscussionThreadOutputFromGitLab(discussion, threadExtra)

	if got.ID != "d1" || !got.IndividualNote || !got.Resolvable || !got.Resolved || len(got.Notes) != 3 {
		t.Fatalf("DiscussionThreadOutputFromGitLab() = %+v, want the thread with three notes", got)
	}
	if got.Notes[0].ID != 42 || !got.Notes[0].Confidential || got.Notes[1].ID != 43 || !got.Notes[1].Imported || got.Notes[2].ID != 0 {
		t.Errorf("notes = %+v, %+v, %+v; want each paired with its extra by position", got.Notes[0], got.Notes[1], got.Notes[2])
	}
	if zero := DiscussionThreadOutputFromGitLab(nil, threadExtra); zero.ID != "" || zero.Notes != nil {
		t.Errorf("a nil discussion converted to %+v, want the zero value", zero)
	}

	list := DiscussionThreadOutputsFromGitLab([]*gl.Discussion{discussion, {ID: "d2"}}, []DiscussionExtra{threadExtra})
	if len(list) != 2 || !list[0].Resolvable || list[1].ID != "d2" || list[1].Resolvable {
		t.Errorf("DiscussionThreadOutputsFromGitLab() = %+v, want the first paired and the second without extras", list)
	}
}

// TestDiscussionThreadOutput_Markdown verifies the view models: the author
// is named by username, an absent author by nothing, and a nil note in a
// thread is left out rather than dereferenced.
func TestDiscussionThreadOutput_Markdown(t *testing.T) {
	thread := DiscussionThreadOutput{ID: "d1", Notes: []*DiscussionThreadNoteOutput{
		{ID: 1, Body: "first", Author: &NoteUserOutput{Username: "alice"}, CreatedAt: "2026-01-15T10:00:00Z"},
		{ID: 2, Body: "second"},
		nil,
	}}

	got := DiscussionThreadOutputMarkdowns([]DiscussionThreadOutput{thread})

	want := []DiscussionMarkdown{{ID: "d1", Notes: []DiscussionNoteMarkdown{
		{ID: 1, Body: "first", Author: "alice", CreatedAt: "2026-01-15T10:00:00Z"},
		{ID: 2, Body: "second"},
	}}}
	if len(got) != 1 || got[0].ID != want[0].ID || len(got[0].Notes) != 2 || got[0].Notes[0] != want[0].Notes[0] || got[0].Notes[1] != want[0].Notes[1] {
		t.Errorf("DiscussionThreadOutputMarkdowns() = %+v, want %+v", got, want)
	}
}

// TestNoteOutputFromGitLab_ZeroNote verifies that NoteOutputFromGitLab
// returns a valid zero-value NoteOutput (no panic) and leaves optional
// sub-objects nil when the note has no author ID, no resolver, no position.
func TestNoteOutputFromGitLab_ZeroNote(t *testing.T) {
	got := NoteOutputFromGitLab(&gl.Note{}, NoteExtra{})
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
