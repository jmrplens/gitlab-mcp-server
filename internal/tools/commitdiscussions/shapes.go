package commitdiscussions

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// NoteOutput is an alias of [toolutil.DiscussionThreadNoteOutput], the rich
// note shape used within discussion threads, shared with mrdiscussions. Field
// layout and JSON tags (including UpdatedAt with omitempty and
// Resolved/Resolvable without omitempty) are defined in toolutil.
type NoteOutput = toolutil.DiscussionThreadNoteOutput

// Output is an alias of [toolutil.DiscussionThreadOutput], the discussion
// thread shape with full note payloads, shared with mrdiscussions.
type Output = toolutil.DiscussionThreadOutput

// NoteToOutput converts a GitLab API [gl.Note] to a [NoteOutput]. Per the 1:1
// audit policy it surfaces the full author object on the canonical `author`
// key, additively surfaces the resolved_by / position sub-objects, and mirrors
// every other gl.Note field. Timestamps are formatted as RFC 3339 strings.
func NoteToOutput(n *gl.Note) NoteOutput {
	if n == nil {
		return NoteOutput{}
	}
	return NoteOutput{
		ID:           n.ID,
		Body:         n.Body,
		Author:       toolutil.NewNoteUserOutputFromAuthor(n.Author),
		Attachment:   n.Attachment,
		Title:        n.Title,
		FileName:     n.FileName,
		CreatedAt:    toolutil.FormatTimePtr(n.CreatedAt),
		UpdatedAt:    toolutil.FormatTimePtr(n.UpdatedAt),
		ExpiresAt:    toolutil.FormatTimePtr(n.ExpiresAt),
		Resolved:     n.Resolved,
		Resolvable:   n.Resolvable,
		ResolvedAt:   toolutil.FormatTimePtr(n.ResolvedAt),
		ResolvedBy:   toolutil.NewNoteUserOutputFromResolvedBy(n.ResolvedBy),
		System:       n.System,
		Internal:     n.Internal,
		Confidential: n.Confidential, //nolint:staticcheck // 1:1 audit: mirror the SDK's deprecated Confidential field verbatim.
		Type:         string(n.Type),
		NoteableType: n.NoteableType,
		NoteableID:   n.NoteableID,
		NoteableIID:  n.NoteableIID,
		CommitID:     n.CommitID,
		Position:     toolutil.NewNotePositionOutput(n.Position),
		ProjectID:    n.ProjectID,
	}
}

// ToOutput converts a GitLab API [gl.Discussion] to an [Output], including all
// notes within the thread.
func ToOutput(d *gl.Discussion) Output {
	if d == nil {
		return Output{}
	}
	notes := make([]*NoteOutput, len(d.Notes))
	for i, n := range d.Notes {
		note := NoteToOutput(n)
		notes[i] = &note
	}
	return Output{
		ID:             d.ID,
		IndividualNote: d.IndividualNote,
		Notes:          notes,
	}
}
