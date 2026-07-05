package toolutil

import "testing"

// TestDiscussionIDParamGuidance pins the canonical discussion_id guidance:
// the caller-supplied value source is passed through verbatim while the
// semantic role, example binding, and confusion note stay fixed.
func TestDiscussionIDParamGuidance(t *testing.T) {
	src := "Discussion thread id from a prior gitlab_mr_discussion_list response."
	got := DiscussionIDParamGuidance(src)
	if got.SemanticRole != "discussion_id" {
		t.Errorf("SemanticRole = %q, want discussion_id", got.SemanticRole)
	}
	if got.ValueSource != src {
		t.Errorf("ValueSource = %q, want %q", got.ValueSource, src)
	}
	if got.ExampleBinding != `params.discussion_id:"6a9c1750b37d513a43987b574953fceb50b03ce7"` {
		t.Errorf("ExampleBinding = %q", got.ExampleBinding)
	}
	if len(got.CommonConfusions) != 1 || got.CommonConfusions[0] == "" {
		t.Errorf("CommonConfusions = %v, want the fixed thread-vs-note note", got.CommonConfusions)
	}
}

// TestDiscussionNoteIDParamGuidance pins the canonical note_id guidance for
// discussion note actions: pass-through value source plus fixed role,
// example, and confusion note.
func TestDiscussionNoteIDParamGuidance(t *testing.T) {
	src := "Numeric note id within the discussion thread, from a prior discussion response."
	got := DiscussionNoteIDParamGuidance(src)
	if got.SemanticRole != "note_id" {
		t.Errorf("SemanticRole = %q, want note_id", got.SemanticRole)
	}
	if got.ValueSource != src {
		t.Errorf("ValueSource = %q, want %q", got.ValueSource, src)
	}
	if got.ExampleBinding != "params.note_id:300" {
		t.Errorf("ExampleBinding = %q, want params.note_id:300", got.ExampleBinding)
	}
	if len(got.CommonConfusions) != 1 || got.CommonConfusions[0] == "" {
		t.Errorf("CommonConfusions = %v, want the fixed note-vs-thread note", got.CommonConfusions)
	}
}
