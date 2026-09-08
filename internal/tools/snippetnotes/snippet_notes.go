package snippetnotes

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ListInput defines parameters for listing snippet notes.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SnippetID int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Order by field (created_at, updated_at)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	CreatedAt string               `json:"created_at,omitempty" jsonschema:"Backdate the note to this RFC 3339 timestamp (e.g. 2026-01-15T10:00:00Z). Requires administrator or project/group owner permissions. Ignored otherwise."`
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

// Output is an alias of [toolutil.NoteOutput], the canonical REST note
// shape shared with issuenotes and mrnotes: every field GitLab's Note entity
// sends, the author as the UserBasic object it is. Until 2.8.0 this package
// carried a copy of that shape with `updated_at` omitted when empty.
type Output = toolutil.NoteOutput

// ListOutput holds a paginated list of snippet notes.
type ListOutput struct {
	toolutil.HintableOutput
	Notes      []Output                  `json:"notes"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// toOutput converts a GitLab API [gl.Note], and what the captured response
// adds to it, to the MCP tool output format, through the shared conversion
// in [toolutil.NoteOutputFromGitLab].
func toOutput(n *gl.Note, extra toolutil.NoteExtra) Output {
	return toolutil.NoteOutputFromGitLab(n, extra)
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
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	notes, resp, err := client.GL().Notes.ListSnippetNotes(string(input.ProjectID), input.SnippetID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("snippetNoteList", err, http.StatusNotFound,
			"verify project_id and snippet_id with gitlab_snippet_list; private snippets require Reporter role on the project")
	}
	extras, err := toolutil.CapturedNotes(captured, len(notes))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("snippetNoteList", err)
	}
	out := make([]Output, len(notes))
	for i, n := range notes {
		out[i] = toOutput(n, extras[i])
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	n, _, err := client.GL().Notes.GetSnippetNote(string(input.ProjectID), input.SnippetID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("snippetNoteGet", err, http.StatusNotFound,
			"verify project_id, snippet_id, and note_id with gitlab_snippet_note_list")
	}
	extra, err := toolutil.CapturedNote(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("snippetNoteGet", err)
	}
	return toOutput(n, extra), nil
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
	opts := &gl.CreateSnippetNoteOptions{
		Body: &body,
	}
	if t := toolutil.ParseOptionalTime(input.CreatedAt); t != nil {
		opts.CreatedAt = t
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	n, _, err := client.GL().Notes.CreateSnippetNote(string(input.ProjectID), input.SnippetID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("snippetNoteCreate", err, http.StatusBadRequest,
			"body is required and rendered as GitLab Flavored Markdown (max 1MB); requires Reporter role on the project")
	}
	extra, err := toolutil.CapturedNote(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("snippetNoteCreate", err)
	}
	return toOutput(n, extra), nil
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	n, _, err := client.GL().Notes.UpdateSnippetNote(string(input.ProjectID), input.SnippetID, input.NoteID, &gl.UpdateSnippetNoteOptions{
		Body: &body,
	}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("snippetNoteUpdate", err, http.StatusForbidden,
			"only the note author or a Maintainer/Owner can edit; verify note_id with gitlab_snippet_note_list; system notes cannot be edited")
	}
	extra, err := toolutil.CapturedNote(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("snippetNoteUpdate", err)
	}
	return toOutput(n, extra), nil
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
			"only the note author or a Maintainer/Owner can delete; deletion is irreversible. System notes cannot be removed")
	}
	return nil
}
