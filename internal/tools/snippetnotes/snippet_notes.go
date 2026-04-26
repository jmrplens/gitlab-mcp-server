// Package snippetnotes implements GitLab snippet note (comment) operations including
// list, get, create, update, and delete. Notes are comments attached to project
// snippets and may be system-generated or user-created.
package snippetnotes

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput defines parameters for listing snippet notes.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SnippetID int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Order by field (created_at, updated_at)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
}

// GetInput defines parameters for getting a single snippet note.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	SnippetID int64                `json:"snippet_id"  jsonschema:"Snippet ID,required"`
	NoteID    int64                `json:"note_id"     jsonschema:"ID of the note to retrieve,required"`
}

// CreateInput defines parameters for creating a note on a snippet.
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	SnippetID int64                `json:"snippet_id"  jsonschema:"Snippet ID,required"`
	Body      string               `json:"body"        jsonschema:"Note body (Markdown supported),required"`
}

// UpdateInput defines parameters for updating a snippet note.
type UpdateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	SnippetID int64                `json:"snippet_id"  jsonschema:"Snippet ID,required"`
	NoteID    int64                `json:"note_id"     jsonschema:"ID of the note to update,required"`
	Body      string               `json:"body"        jsonschema:"Updated note body (Markdown supported),required"`
}

// DeleteInput defines parameters for deleting a snippet note.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	SnippetID int64                `json:"snippet_id"  jsonschema:"Snippet ID,required"`
	NoteID    int64                `json:"note_id"     jsonschema:"ID of the note to delete,required"`
}

// Output represents a note (comment) on a snippet.
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

// ListOutput holds a paginated list of snippet notes.
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

// List retrieves a paginated list of notes on a snippet.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("snippetNoteList: project_id is required. Use gitlab_project_list to find the ID first")
	}
	if input.SnippetID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("snippetNoteList", "snippet_id")
	}
	opts := &gl.ListSnippetNotesOptions{}
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
	notes, resp, err := client.GL().Notes.ListSnippetNotes(string(input.ProjectID), input.SnippetID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("snippetNoteList", err, http.StatusNotFound,
			"verify project_id and snippet_id with gitlab_snippet_list; private snippets require Reporter role on the project")
	}
	out := make([]Output, len(notes))
	for i, n := range notes {
		out[i] = toOutput(n)
	}
	return ListOutput{Notes: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// Get retrieves a single note on a snippet.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("snippetNoteGet: project_id is required")
	}
	if input.SnippetID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippetNoteGet", "snippet_id")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippetNoteGet", "note_id")
	}
	n, _, err := client.GL().Notes.GetSnippetNote(string(input.ProjectID), input.SnippetID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("snippetNoteGet", err, http.StatusNotFound,
			"verify project_id, snippet_id, and note_id with gitlab_snippet_note_list")
	}
	return toOutput(n), nil
}

// Create adds a new note to a snippet.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("snippetNoteCreate: project_id is required")
	}
	if input.SnippetID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippetNoteCreate", "snippet_id")
	}
	if input.Body == "" {
		return Output{}, errors.New("snippetNoteCreate: body is required")
	}
	body := toolutil.NormalizeText(input.Body)
	n, _, err := client.GL().Notes.CreateSnippetNote(string(input.ProjectID), input.SnippetID, &gl.CreateSnippetNoteOptions{
		Body: &body,
	}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("snippetNoteCreate", err, http.StatusBadRequest,
			"body is required and rendered as GitLab Flavored Markdown (max 1MB); requires Reporter role on the project")
	}
	return toOutput(n), nil
}

// Update modifies the body of an existing snippet note.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("snippetNoteUpdate: project_id is required")
	}
	if input.SnippetID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippetNoteUpdate", "snippet_id")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippetNoteUpdate", "note_id")
	}
	body := toolutil.NormalizeText(input.Body)
	n, _, err := client.GL().Notes.UpdateSnippetNote(string(input.ProjectID), input.SnippetID, input.NoteID, &gl.UpdateSnippetNoteOptions{
		Body: &body,
	}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("snippetNoteUpdate", err, http.StatusForbidden,
			"only the note author or a Maintainer/Owner can edit; verify note_id with gitlab_snippet_note_list; system notes cannot be edited")
	}
	return toOutput(n), nil
}

// Delete removes a note from a snippet.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("snippetNoteDelete: project_id is required")
	}
	if input.SnippetID <= 0 {
		return toolutil.ErrRequiredInt64("snippetNoteDelete", "snippet_id")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("snippetNoteDelete", "note_id")
	}
	_, err := client.GL().Notes.DeleteSnippetNote(string(input.ProjectID), input.SnippetID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("snippetNoteDelete", err, http.StatusForbidden,
			"only the note author or a Maintainer/Owner can delete; deletion is irreversible \u2014 system notes cannot be removed")
	}
	return nil
}
