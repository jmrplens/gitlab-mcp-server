package toolutil

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// DiscussionNoteOutputFromGitLab maps a GitLab note to the shared REST
// discussion note output shape.
func DiscussionNoteOutputFromGitLab(note *gl.Note) DiscussionNoteOutput {
	if note == nil {
		return DiscussionNoteOutput{}
	}
	out := DiscussionNoteOutput{
		ID:     note.ID,
		Body:   note.Body,
		System: note.System,
	}
	if note.Author.Username != "" {
		out.Author = note.Author.Username
	}
	if note.CreatedAt != nil && !note.CreatedAt.IsZero() {
		out.CreatedAt = note.CreatedAt.Format(time.RFC3339)
	}
	if note.UpdatedAt != nil && !note.UpdatedAt.IsZero() {
		out.UpdatedAt = note.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// DiscussionOutputFromGitLab maps a GitLab discussion to the shared REST
// discussion output shape.
func DiscussionOutputFromGitLab(discussion *gl.Discussion) DiscussionOutput {
	if discussion == nil {
		return DiscussionOutput{}
	}
	out := DiscussionOutput{
		ID:             discussion.ID,
		IndividualNote: discussion.IndividualNote,
		Notes:          make([]DiscussionNoteOutput, 0, len(discussion.Notes)),
	}
	for _, note := range discussion.Notes {
		out.Notes = append(out.Notes, DiscussionNoteOutputFromGitLab(note))
	}
	return out
}

// DiscussionOutputsFromGitLab maps GitLab discussions to the shared REST
// discussion output shape.
func DiscussionOutputsFromGitLab(discussions []*gl.Discussion) []DiscussionOutput {
	out := make([]DiscussionOutput, 0, len(discussions))
	for _, discussion := range discussions {
		out = append(out, DiscussionOutputFromGitLab(discussion))
	}
	return out
}
