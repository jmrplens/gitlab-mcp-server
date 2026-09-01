package mrnotes

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// CreateInput defines parameters for adding a general comment to a merge request.
type CreateInput struct {
	ProjectID               toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                   int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	Body                    string               `json:"body"       jsonschema:"Comment body (Markdown supported),required"`
	Internal                *bool                `json:"internal,omitempty"   jsonschema:"Mark note as internal (visible only to project members)"`
	CreatedAt               string               `json:"created_at,omitempty" jsonschema:"Backdate the note to this RFC 3339 timestamp (e.g. 2026-01-15T10:00:00Z). Requires administrator or project/group owner permissions. Ignored otherwise."`
	MergeRequestDiffHeadSHA string               `json:"merge_request_diff_head_sha,omitempty" jsonschema:"Required for the deduplication of system notes: SHA referencing the most recent diff version of the merge request."`
}

// Output is an alias of [toolutil.NoteOutput], the canonical REST note shape
// shared between mrnotes and issuenotes. Field layout and JSON tags are
// defined in toolutil; see [toolutil.NoteOutputFromGitLab] for the 1:1 audit
// conversion logic.
type Output = toolutil.NoteOutput

// ListInput defines parameters for listing merge request notes.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"     jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"         jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Order by field (created_at, updated_at)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListOutput holds a list of notes.
type ListOutput struct {
	toolutil.HintableOutput
	Notes      []Output                  `json:"notes"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// UpdateInput defines parameters for editing a note.
type UpdateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	NoteID    int64                `json:"note_id"    jsonschema:"ID of the note to update,required"`
	Body      string               `json:"body"       jsonschema:"Updated comment body,required"`
}

// DeleteInput defines parameters for deleting a note.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	NoteID    int64                `json:"note_id"    jsonschema:"ID of the note to delete,required"`
}

// GetInput defines parameters for getting a single note.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	NoteID    int64                `json:"note_id"    jsonschema:"ID of the note to retrieve,required"`
}

// ToOutput converts a GitLab API [gl.Note] to the MCP tool [Output] shape.
// Delegates to [toolutil.NoteOutputFromGitLab] which owns the 1:1 audit
// field mapping and timestamp formatting.
func ToOutput(n *gl.Note) Output {
	return toolutil.NoteOutputFromGitLab(n)
}

// Create adds a new general comment to a merge request.
// The body is normalized before submission. Returns the created note.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrNoteCreate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrNoteCreate", "merge_request_iid")
	}
	opts := &gl.CreateMergeRequestNoteOptions{
		Body: new(toolutil.NormalizeText(input.Body)),
	}
	if input.Internal != nil {
		opts.Internal = input.Internal
	}
	if t := toolutil.ParseOptionalTime(input.CreatedAt); t != nil {
		opts.CreatedAt = t
	}
	if input.MergeRequestDiffHeadSHA != "" {
		opts.MergeRequestDiffHeadSHA = new(input.MergeRequestDiffHeadSHA)
	}
	n, _, err := client.GL().Notes.CreateMergeRequestNote(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("mrNoteCreate", err, http.StatusNotFound,
			"verify project_id and merge_request_iid with gitlab_mr_get; creating notes requires Reporter role or higher")
	}
	return ToOutput(n), nil
}

// List returns a paginated list of notes for a merge request.
// Results can be ordered by creation or update time and sorted in ascending
// or descending direction.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("mrNotesList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("mrNotesList", "merge_request_iid")
	}
	opts := &gl.ListMergeRequestNotesOptions{}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	notes, resp, err := client.GL().Notes.ListMergeRequestNotes(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("mrNotesList", err, http.StatusNotFound,
			"verify project_id and merge_request_iid with gitlab_mr_get")
	}
	out := make([]Output, len(notes))
	for i, n := range notes {
		out[i] = ToOutput(n)
	}
	return ListOutput{Notes: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// Update modifies the body of an existing note on a merge request.
// Returns the updated note with refreshed timestamps.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrNoteUpdate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrNoteUpdate", "merge_request_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrNoteUpdate", "note_id")
	}
	n, _, err := client.GL().Notes.UpdateMergeRequestNote(string(input.ProjectID), input.MRIID, input.NoteID, &gl.UpdateMergeRequestNoteOptions{
		Body: new(toolutil.NormalizeText(input.Body)),
	}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("mrNoteUpdate", err, http.StatusForbidden,
			"only the note author can edit a note. System notes cannot be edited")
	}
	return ToOutput(n), nil
}

// GetNote retrieves a single note from a merge request by note ID.
func GetNote(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrNoteGet: project_id is required")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrNoteGet", "merge_request_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrNoteGet", "note_id")
	}
	n, _, err := client.GL().Notes.GetMergeRequestNote(string(input.ProjectID), input.MRIID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("mrNoteGet", err, http.StatusNotFound,
			"verify note_id with gitlab_mr_notes_list")
	}
	return ToOutput(n), nil
}

// Delete removes a note from a merge request. Returns an error if the
// note does not exist or the user lacks permission.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("mrNoteDelete: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("mrNoteDelete", "merge_request_iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("mrNoteDelete", "note_id")
	}
	_, err := client.GL().Notes.DeleteMergeRequestNote(string(input.ProjectID), input.MRIID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("mrNoteDelete", err, http.StatusForbidden,
			"only the note author or a Maintainer can delete a note. System notes cannot be deleted")
	}
	return nil
}

// Markdown Formatting.
