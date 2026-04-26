// mr_notes.go implements GitLab merge request note (comment) operations
// including create, list, update, and delete. It exposes typed input/output
// structs and handler functions registered as MCP tools.

package mrnotes

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// CreateInput defines parameters for adding a general comment to a merge request.
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	Body      string               `json:"body"       jsonschema:"Comment body (Markdown supported),required"`
}

// Output represents a note (comment) on a merge request.
type Output struct {
	toolutil.HintableOutput
	ID           int64  `json:"id"`
	Body         string `json:"body"`
	Author       string `json:"author"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	System       bool   `json:"system"`
	Resolvable   bool   `json:"resolvable,omitempty"`
	Resolved     bool   `json:"resolved,omitempty"`
	ResolvedBy   string `json:"resolved_by,omitempty"`
	Internal     bool   `json:"internal,omitempty"`
	NoteableType string `json:"notable_type,omitempty"`
	NoteableID   int64  `json:"notable_id,omitempty"`
	NoteableIID  int64  `json:"notable_iid,omitempty"`
	CommitID     string `json:"commit_id,omitempty"`
	Type         string `json:"type,omitempty"`
	ProjectID    int64  `json:"project_id,omitempty"`
	Confidential bool   `json:"confidential"`
	ResolvedAt   string `json:"resolved_at,omitempty"`
	Attachment   string `json:"attachment,omitempty"`
	FileName     string `json:"file_name,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

// ListInput defines parameters for listing merge request notes.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"     jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"         jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Order by field (created_at, updated_at)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
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
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	NoteID    int64                `json:"note_id"    jsonschema:"ID of the note to update,required"`
	Body      string               `json:"body"       jsonschema:"Updated comment body,required"`
}

// DeleteInput defines parameters for deleting a note.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	NoteID    int64                `json:"note_id"    jsonschema:"ID of the note to delete,required"`
}

// GetInput defines parameters for getting a single note.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	NoteID    int64                `json:"note_id"    jsonschema:"ID of the note to retrieve,required"`
}

// ToOutput converts a GitLab API [gl.Note] to an [Output],
// formatting creation and update timestamps as RFC 3339.
func ToOutput(n *gl.Note) Output {
	out := Output{
		ID:           n.ID,
		Body:         n.Body,
		Author:       n.Author.Username,
		System:       n.System,
		Resolvable:   n.Resolvable,
		Resolved:     n.Resolved,
		Internal:     n.Internal,
		NoteableType: n.NoteableType,
		NoteableID:   n.NoteableID,
		NoteableIID:  n.NoteableIID,
		CommitID:     n.CommitID,
		Type:         string(n.Type),
		ProjectID:    n.ProjectID,
		Confidential: n.Internal,
	}
	if n.ResolvedBy.Username != "" {
		out.ResolvedBy = n.ResolvedBy.Username
	}
	if n.ResolvedAt != nil {
		out.ResolvedAt = n.ResolvedAt.Format(time.RFC3339)
	}
	if n.CreatedAt != nil {
		out.CreatedAt = n.CreatedAt.Format(time.RFC3339)
	}
	if n.UpdatedAt != nil {
		out.UpdatedAt = n.UpdatedAt.Format(time.RFC3339)
	}
	out.Attachment = n.Attachment
	out.FileName = n.FileName
	if n.ExpiresAt != nil {
		out.ExpiresAt = n.ExpiresAt.Format(time.RFC3339)
	}
	return out
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
		return Output{}, toolutil.ErrRequiredInt64("mrNoteCreate", "mr_iid")
	}
	n, _, err := client.GL().Notes.CreateMergeRequestNote(string(input.ProjectID), input.MRIID, &gl.CreateMergeRequestNoteOptions{
		Body: new(toolutil.NormalizeText(input.Body)),
	}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("mrNoteCreate", err, http.StatusNotFound,
			"verify project_id and mr_iid with gitlab_mr_get; creating notes requires Reporter role or higher")
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
		return ListOutput{}, toolutil.ErrRequiredInt64("mrNotesList", "mr_iid")
	}
	opts := &gl.ListMergeRequestNotesOptions{}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	notes, resp, err := client.GL().Notes.ListMergeRequestNotes(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("mrNotesList", err, http.StatusNotFound,
			"verify project_id and mr_iid with gitlab_mr_get")
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
		return Output{}, toolutil.ErrRequiredInt64("mrNoteUpdate", "mr_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrNoteUpdate", "note_id")
	}
	n, _, err := client.GL().Notes.UpdateMergeRequestNote(string(input.ProjectID), input.MRIID, input.NoteID, &gl.UpdateMergeRequestNoteOptions{
		Body: new(toolutil.NormalizeText(input.Body)),
	}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("mrNoteUpdate", err, http.StatusForbidden,
			"only the note author can edit a note; system notes cannot be edited")
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
		return Output{}, toolutil.ErrRequiredInt64("mrNoteGet", "mr_iid")
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
		return toolutil.ErrRequiredInt64("mrNoteDelete", "mr_iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("mrNoteDelete", "note_id")
	}
	_, err := client.GL().Notes.DeleteMergeRequestNote(string(input.ProjectID), input.MRIID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("mrNoteDelete", err, http.StatusForbidden,
			"only the note author or a Maintainer can delete a note; system notes cannot be deleted")
	}
	return nil
}

// Markdown Formatting.
