package toolutil

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestDiscussionOutputFromGitLab verifies shared REST discussion conversion
// preserves thread metadata, note author names, timestamps, and system flags.
func TestDiscussionOutputFromGitLab(t *testing.T) {
	createdAt := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	discussion := &gl.Discussion{
		ID:             "discussion-1",
		IndividualNote: true,
		Notes: []*gl.Note{
			{ID: 11, Body: "first", Author: gl.NoteAuthor{Username: "alice"}, CreatedAt: &createdAt, UpdatedAt: &updatedAt, System: true},
			{ID: 12, Body: "second"},
		},
	}

	out := DiscussionOutputFromGitLab(discussion)

	if out.ID != "discussion-1" || !out.IndividualNote {
		t.Fatalf("discussion metadata = %q/%t, want discussion-1/true", out.ID, out.IndividualNote)
	}
	if len(out.Notes) != 2 {
		t.Fatalf("notes length = %d, want 2", len(out.Notes))
	}
	first := out.Notes[0]
	if first.ID != 11 || first.Body != "first" || first.Author != "alice" || !first.System {
		t.Fatalf("first note = %+v, want mapped ID/body/author/system", first)
	}
	if first.CreatedAt != createdAt.Format(time.RFC3339) || first.UpdatedAt != updatedAt.Format(time.RFC3339) {
		t.Fatalf("timestamps = %q/%q, want RFC3339 created/updated", first.CreatedAt, first.UpdatedAt)
	}
	if out.Notes[1].CreatedAt != "" || out.Notes[1].UpdatedAt != "" {
		t.Fatalf("zero timestamp note = %+v, want empty timestamps", out.Notes[1])
	}
}

// TestDiscussionOutputsFromGitLabHandlesNil verifies nil API elements do not
// panic and keep slice cardinality so callers can preserve response shape.
func TestDiscussionOutputsFromGitLabHandlesNil(t *testing.T) {
	out := DiscussionOutputsFromGitLab([]*gl.Discussion{nil, {ID: "discussion-2"}})

	if len(out) != 2 {
		t.Fatalf("outputs length = %d, want 2", len(out))
	}
	if out[0].ID != "" || out[1].ID != "discussion-2" {
		t.Fatalf("outputs = %+v, want empty first and mapped second", out)
	}
}

// TestDiscussionNoteOutputFromGitLabNil verifies that a nil note pointer
// produces a zero-value DiscussionNoteOutput without panicking.
func TestDiscussionNoteOutputFromGitLabNil(t *testing.T) {
	out := DiscussionNoteOutputFromGitLab(nil)
	zero := DiscussionNoteOutput{}
	if out.ID != zero.ID || out.Body != zero.Body || out.Author != zero.Author ||
		out.CreatedAt != zero.CreatedAt || out.UpdatedAt != zero.UpdatedAt || out.System != zero.System {
		t.Errorf("nil note mapping = %+v, want zero value", out)
	}
}

// TestDiscussionNoteOutputFromGitLab_EmptyAuthor verifies that a note with an
// empty author username does not populate the Author field on the output.
func TestDiscussionNoteOutputFromGitLab_EmptyAuthor(t *testing.T) {
	out := DiscussionNoteOutputFromGitLab(&gl.Note{ID: 5, Body: "hi"})
	if out.Author != "" {
		t.Errorf("Author = %q, want empty for blank source author", out.Author)
	}
}
