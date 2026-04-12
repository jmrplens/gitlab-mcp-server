// Package epicnotes implements GitLab epic note (comment) operations including
// list, get, create, update, and delete. Notes are comments attached to group
// epics and may be system-generated or user-created.
package epicnotes

import (
	"context"
	"errors"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput defines parameters for listing epic notes.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID int64                `json:"epic_iid" jsonschema:"Epic internal ID within the group,required"`
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Order by field (created_at, updated_at)"`
	Sort    string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
}

// GetInput defines parameters for getting a single epic note.
type GetInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID int64                `json:"epic_iid" jsonschema:"Epic internal ID within the group,required"`
	NoteID  int64                `json:"note_id"  jsonschema:"ID of the note to retrieve,required"`
}

// CreateInput defines parameters for creating a note on an epic.
type CreateInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID int64                `json:"epic_iid" jsonschema:"Epic internal ID within the group,required"`
	Body    string               `json:"body"     jsonschema:"Note body (Markdown supported),required"`
}

// UpdateInput defines parameters for updating an epic note.
type UpdateInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID int64                `json:"epic_iid" jsonschema:"Epic internal ID within the group,required"`
	NoteID  int64                `json:"note_id"  jsonschema:"ID of the note to update,required"`
	Body    string               `json:"body"     jsonschema:"Updated note body (Markdown supported),required"`
}

// DeleteInput defines parameters for deleting an epic note.
type DeleteInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID int64                `json:"epic_iid" jsonschema:"Epic internal ID within the group,required"`
	NoteID  int64                `json:"note_id"  jsonschema:"ID of the note to delete,required"`
}

// Output represents a note (comment) on an epic.
type Output struct {
	toolutil.HintableOutput
	ID           int64  `json:"id"`
	Body         string `json:"body"`
	Author       string `json:"author"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at,omitempty"`
	System       bool   `json:"system"`
	NoteableType string `json:"notable_type,omitempty"`
	NoteableID   int64  `json:"notable_id,omitempty"`
}

// ListOutput holds a paginated list of epic notes.
type ListOutput struct {
	toolutil.HintableOutput
	Notes      []Output                  `json:"notes"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// toOutput converts a GitLab Note to the MCP tool output format.
func toOutput(n *gl.Note) Output {
	out := Output{
		ID:           n.ID,
		Body:         n.Body,
		System:       n.System,
		NoteableType: n.NoteableType,
		NoteableID:   n.NoteableID,
	}
	if n.Author.Username != "" {
		out.Author = n.Author.Username
	}
	if n.CreatedAt != nil {
		out.CreatedAt = n.CreatedAt.Format(time.RFC3339)
	}
	if n.UpdatedAt != nil {
		out.UpdatedAt = n.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// List retrieves a paginated list of notes on an epic.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, errors.New("epicNoteList: group_id is required. Use gitlab_group_list to find the group ID first")
	}
	if input.EpicIID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("epicNoteList", "epic_iid")
	}
	opts := &gl.ListEpicNotesOptions{}
	if input.OrderBy != "" {
		opts.OrderBy = &input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = &input.Sort
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	notes, resp, err := client.GL().Notes.ListEpicNotes(string(input.GroupID), input.EpicIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("epicNoteList", err)
	}
	out := make([]Output, len(notes))
	for i, n := range notes {
		out[i] = toOutput(n)
	}
	return ListOutput{Notes: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// Get retrieves a single note on an epic.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("epicNoteGet: group_id is required")
	}
	if input.EpicIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicNoteGet", "epic_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicNoteGet", "note_id")
	}
	n, _, err := client.GL().Notes.GetEpicNote(string(input.GroupID), input.EpicIID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("epicNoteGet", err)
	}
	return toOutput(n), nil
}

// Create adds a new note to an epic.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("epicNoteCreate: group_id is required")
	}
	if input.EpicIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicNoteCreate", "epic_iid")
	}
	if input.Body == "" {
		return Output{}, errors.New("epicNoteCreate: body is required")
	}
	body := toolutil.NormalizeText(input.Body)
	n, _, err := client.GL().Notes.CreateEpicNote(string(input.GroupID), input.EpicIID, &gl.CreateEpicNoteOptions{
		Body: &body,
	}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("epicNoteCreate", err)
	}
	return toOutput(n), nil
}

// Update modifies the body of an existing epic note.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("epicNoteUpdate: group_id is required")
	}
	if input.EpicIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicNoteUpdate", "epic_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicNoteUpdate", "note_id")
	}
	body := toolutil.NormalizeText(input.Body)
	n, _, err := client.GL().Notes.UpdateEpicNote(string(input.GroupID), input.EpicIID, input.NoteID, &gl.UpdateEpicNoteOptions{
		Body: &body,
	}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("epicNoteUpdate", err)
	}
	return toOutput(n), nil
}

// Delete removes a note from an epic.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return errors.New("epicNoteDelete: group_id is required")
	}
	if input.EpicIID <= 0 {
		return toolutil.ErrRequiredInt64("epicNoteDelete", "epic_iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("epicNoteDelete", "note_id")
	}
	_, err := client.GL().Notes.DeleteEpicNote(string(input.GroupID), input.EpicIID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("epicNoteDelete", err)
	}
	return nil
}
