package toolutil

// DiscussionIDParamGuidance returns the canonical parameter guidance for the
// discussion_id parameter used by discussion-scoped actions across the
// discussion domains (MR, issue, snippet, commit, epic). valueSource is the
// domain-specific description of where the thread id comes from; the semantic
// role, example binding, and thread-vs-note confusion note are identical for
// every domain.
func DiscussionIDParamGuidance(valueSource string) ParameterGuidance {
	return ParameterGuidance{
		SemanticRole:     "discussion_id",
		ValueSource:      valueSource,
		ExampleBinding:   `params.discussion_id:"6a9c1750b37d513a43987b574953fceb50b03ce7"`,
		CommonConfusions: []string{"The discussion_id is the thread hash, not a note id. Pass note_id separately for note actions."},
	}
}

// DiscussionNoteIDParamGuidance returns the canonical parameter guidance for
// the note_id parameter of discussion note update/delete actions. valueSource
// is the domain-specific description of where the note id comes from; the
// semantic role, example binding, and note-vs-thread confusion note are
// identical for every discussion domain.
func DiscussionNoteIDParamGuidance(valueSource string) ParameterGuidance {
	return ParameterGuidance{
		SemanticRole:     "note_id",
		ValueSource:      valueSource,
		ExampleBinding:   "params.note_id:300",
		CommonConfusions: []string{"note_id is the numeric id of a single note. discussion_id is the thread hash."},
	}
}
