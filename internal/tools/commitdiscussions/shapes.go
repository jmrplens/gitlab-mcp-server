package commitdiscussions

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical output sub-objects mirrored from client-go's note structs. The
// shared note shapes (NoteUserOutput, LinePositionOutput, LineRangeOutput,
// NotePositionOutput) and their converters live in internal/toolutil as of
// DEDUP-001 wave 2. The NoteOutput and Output types below are package-local
// because they include a Notes slice (C-IMPORTS) and so cannot be shared.

// NoteOutput represents a single note within a commit discussion. Per the 1:1
// audit policy it mirrors every field of gl.Note, surfacing the full author /
// resolved_by / position sub-objects on their canonical keys. Timestamps are
// formatted as RFC 3339 strings.
type NoteOutput struct {
	toolutil.HintableOutput
	ID           int64                        `json:"id"`
	Body         string                       `json:"body"`
	Author       *toolutil.NoteUserOutput     `json:"author,omitempty"`
	Attachment   string                       `json:"attachment,omitempty"`
	Title        string                       `json:"title,omitempty"`
	FileName     string                       `json:"file_name,omitempty"`
	CreatedAt    string                       `json:"created_at"`
	UpdatedAt    string                       `json:"updated_at,omitempty"`
	ExpiresAt    string                       `json:"expires_at,omitempty"`
	Resolved     bool                         `json:"resolved"`
	Resolvable   bool                         `json:"resolvable"`
	ResolvedAt   string                       `json:"resolved_at,omitempty"`
	ResolvedBy   *toolutil.NoteUserOutput     `json:"resolved_by,omitempty"`
	System       bool                         `json:"system"`
	Internal     bool                         `json:"internal"`
	Confidential bool                         `json:"confidential"`
	Type         string                       `json:"type,omitempty"`
	NoteableType string                       `json:"noteable_type,omitempty"`
	NoteableID   int64                        `json:"noteable_id,omitempty"`
	NoteableIID  int64                        `json:"noteable_iid,omitempty"`
	CommitID     string                       `json:"commit_id,omitempty"`
	Position     *toolutil.NotePositionOutput `json:"position,omitempty"`
	ProjectID    int64                        `json:"project_id,omitempty"`
}

// Output represents a commit discussion thread. Its Notes field is a local
// []*NoteOutput mirror of the SDK's []*gl.Note (C-IMPORTS replication; the
// auditor flags the local-mirror type, which is the intended behavior).
type Output struct {
	toolutil.HintableOutput
	ID             string        `json:"id"`
	IndividualNote bool          `json:"individual_note"`
	Notes          []*NoteOutput `json:"notes"`
}

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
