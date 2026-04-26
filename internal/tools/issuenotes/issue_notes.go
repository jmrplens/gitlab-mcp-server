// Package issuenotes implements GitLab issue note (comment) operations including
// create and list. Notes are comments attached to issues and may be marked
// as internal (visible only to project members) or system-generated.
package issuenotes

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// CreateInput defines parameters for adding a comment to an issue.
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID,required"`
	Body      string               `json:"body"       jsonschema:"Note body (Markdown supported),required"`
	Internal  *bool                `json:"internal,omitempty" jsonschema:"Mark note as internal (visible only to project members)"`
}

// Output represents a note (comment) on an issue.
type Output struct {
	toolutil.HintableOutput
	ID           int64  `json:"id"`
	Body         string `json:"body"`
	Author       string `json:"author"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	System       bool   `json:"system"`
	Internal     bool   `json:"internal"`
	Resolvable   bool   `json:"resolvable,omitempty"`
	Resolved     bool   `json:"resolved,omitempty"`
	NoteableType string `json:"notable_type,omitempty"`
	NoteableID   int64  `json:"notable_id,omitempty"`
	CommitID     string `json:"commit_id,omitempty"`
	Type         string `json:"type,omitempty"`
	NoteableIID  int64  `json:"notable_iid,omitempty"`
	ProjectID    int64  `json:"project_id,omitempty"`
	Confidential bool   `json:"confidential"`
}

// ListInput defines parameters for listing issue notes.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"         jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"          jsonschema:"Issue internal ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Order by field (created_at, updated_at)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
}

// ListOutput holds a paginated list of issue notes.
type ListOutput struct {
	toolutil.HintableOutput
	Notes      []Output                  `json:"notes"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// GetInput defines parameters for getting a single issue note.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID,required"`
	NoteID    int64                `json:"note_id"    jsonschema:"ID of the note to retrieve,required"`
}

// UpdateInput defines parameters for updating an issue note.
type UpdateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID,required"`
	NoteID    int64                `json:"note_id"    jsonschema:"ID of the note to update,required"`
	Body      string               `json:"body"       jsonschema:"Updated note body (Markdown supported),required"`
}

// DeleteInput defines parameters for deleting an issue note.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID,required"`
	NoteID    int64                `json:"note_id"    jsonschema:"ID of the note to delete,required"`
}

// ToOutput converts a GitLab API [gl.Note] to the MCP tool output
// format, extracting the author username and formatting timestamps as
// RFC 3339 strings.
func ToOutput(n *gl.Note) Output {
	out := Output{
		ID:           n.ID,
		Body:         n.Body,
		Author:       n.Author.Username,
		System:       n.System,
		Internal:     n.Internal,
		Resolvable:   n.Resolvable,
		Resolved:     n.Resolved,
		NoteableType: n.NoteableType,
		NoteableID:   n.NoteableID,
		CommitID:     n.CommitID,
		Type:         string(n.Type),
		NoteableIID:  n.NoteableIID,
		ProjectID:    n.ProjectID,
		Confidential: n.Internal,
	}
	if n.CreatedAt != nil {
		out.CreatedAt = n.CreatedAt.Format(time.RFC3339)
	}
	if n.UpdatedAt != nil {
		out.UpdatedAt = n.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// Create adds a new comment to the specified issue in a GitLab
// project. The note body supports Markdown and can optionally be marked as
// internal. Returns the created note or an error if the API call fails.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("issueNoteCreate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.IssueIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issueNoteCreate", "issue_iid")
	}
	opts := &gl.CreateIssueNoteOptions{
		Body: new(toolutil.NormalizeText(input.Body)),
	}
	if input.Internal != nil {
		opts.Internal = input.Internal
	}
	n, _, err := client.GL().Notes.CreateIssueNote(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("issueNoteCreate", err, http.StatusNotFound,
			"verify project_id and issue_iid with gitlab_issue_get; creating notes requires Reporter role or higher")
	}
	return ToOutput(n), nil
}

// List retrieves a paginated list of notes for a specific issue.
// Supports ordering by created_at or updated_at and sorting in ascending
// or descending order. Returns the notes with pagination metadata.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("issueNoteList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.IssueIID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("issueNoteList", "issue_iid")
	}
	opts := &gl.ListIssueNotesOptions{}
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
	notes, resp, err := client.GL().Notes.ListIssueNotes(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("issueNoteList", err, http.StatusNotFound,
			"verify project_id and issue_iid with gitlab_issue_get")
	}
	out := make([]Output, len(notes))
	for i, n := range notes {
		out[i] = ToOutput(n)
	}
	return ListOutput{Notes: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetNote retrieves a single note from an issue by note ID.
func GetNote(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("issueNoteGet: project_id is required")
	}
	if input.IssueIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issueNoteGet", "issue_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issueNoteGet", "note_id")
	}
	n, _, err := client.GL().Notes.GetIssueNote(string(input.ProjectID), input.IssueIID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("issueNoteGet", err, http.StatusNotFound,
			"verify note_id with gitlab_issue_notes_list")
	}
	return ToOutput(n), nil
}

// Update modifies the body of an existing issue note.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("issueNoteUpdate: project_id is required")
	}
	if input.IssueIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issueNoteUpdate", "issue_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issueNoteUpdate", "note_id")
	}
	n, _, err := client.GL().Notes.UpdateIssueNote(string(input.ProjectID), input.IssueIID, input.NoteID, &gl.UpdateIssueNoteOptions{
		Body: new(toolutil.NormalizeText(input.Body)),
	}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("issueNoteUpdate", err, http.StatusForbidden,
			"only the note author can edit a note; system notes cannot be edited")
	}
	return ToOutput(n), nil
}

// Delete removes a note from an issue.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("issueNoteDelete: project_id is required")
	}
	if input.IssueIID <= 0 {
		return toolutil.ErrRequiredInt64("issueNoteDelete", "issue_iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("issueNoteDelete", "note_id")
	}
	_, err := client.GL().Notes.DeleteIssueNote(string(input.ProjectID), input.IssueIID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("issueNoteDelete", err, http.StatusForbidden,
			"only the note author or a Maintainer can delete a note; system notes cannot be deleted")
	}
	return nil
}
