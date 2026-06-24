package issuenotes

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// CreateInput defines parameters for adding a comment to an issue.
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID,required"`
	Body      string               `json:"body"       jsonschema:"Note body (Markdown supported),required"`
	Internal  *bool                `json:"internal,omitempty"   jsonschema:"Mark note as internal (visible only to project members)"`
	CreatedAt string               `json:"created_at,omitempty" jsonschema:"Backdate the note to this RFC 3339 timestamp (e.g. 2026-01-15T10:00:00Z). Requires administrator or project/group owner permissions; ignored otherwise."`
}

// Output represents a note (comment) on an issue. Per the 1:1 audit policy it
// mirrors every field of gl.Note, surfacing the full author / resolved_by /
// position sub-objects. Per the locked canonical-key convention the full
// *NoteUserOutput author object is surfaced on the canonical `author` key.
type Output struct {
	toolutil.HintableOutput
	ID           int64               `json:"id"`
	Body         string              `json:"body"`
	Author       *NoteUserOutput     `json:"author,omitempty"`
	Attachment   string              `json:"attachment,omitempty"`
	Title        string              `json:"title,omitempty"`
	FileName     string              `json:"file_name,omitempty"`
	CreatedAt    string              `json:"created_at"`
	UpdatedAt    string              `json:"updated_at"`
	ExpiresAt    string              `json:"expires_at,omitempty"`
	System       bool                `json:"system"`
	Internal     bool                `json:"internal"`
	Resolvable   bool                `json:"resolvable,omitempty"`
	Resolved     bool                `json:"resolved,omitempty"`
	ResolvedAt   string              `json:"resolved_at,omitempty"`
	ResolvedBy   *NoteUserOutput     `json:"resolved_by,omitempty"`
	NoteableType string              `json:"noteable_type,omitempty"`
	NoteableID   int64               `json:"noteable_id,omitempty"`
	NoteableIID  int64               `json:"noteable_iid,omitempty"`
	CommitID     string              `json:"commit_id,omitempty"`
	Type         string              `json:"type,omitempty"`
	Position     *NotePositionOutput `json:"position,omitempty"`
	ProjectID    int64               `json:"project_id,omitempty"`
	Confidential bool                `json:"confidential"`
}

// ListInput defines parameters for listing issue notes.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"         jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"          jsonschema:"Issue internal ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Order by field (created_at, updated_at)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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

// ToOutput converts a GitLab API [gl.Note] to the MCP tool output format. Per
// the locked canonical-key convention it surfaces the full author object on the
// canonical `author` key, while additively surfacing the resolved_by / position
// sub-objects and every other gl.Note field (1:1 audit policy). Timestamps are
// formatted as RFC 3339 strings.
func ToOutput(n *gl.Note) Output {
	out := Output{
		ID:           n.ID,
		Body:         n.Body,
		Author:       noteAuthorOutput(n.Author),
		Attachment:   n.Attachment,
		Title:        n.Title,
		FileName:     n.FileName,
		System:       n.System,
		Internal:     n.Internal,
		Resolvable:   n.Resolvable,
		Resolved:     n.Resolved,
		ResolvedBy:   noteResolvedByOutput(n.ResolvedBy),
		NoteableType: n.NoteableType,
		NoteableID:   n.NoteableID,
		NoteableIID:  n.NoteableIID,
		CommitID:     n.CommitID,
		Type:         string(n.Type),
		Position:     notePositionOutput(n.Position),
		ProjectID:    n.ProjectID,
		Confidential: n.Internal,
	}
	out.CreatedAt = formatTimePtr(n.CreatedAt)
	out.UpdatedAt = formatTimePtr(n.UpdatedAt)
	out.ExpiresAt = formatTimePtr(n.ExpiresAt)
	out.ResolvedAt = formatTimePtr(n.ResolvedAt)
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
	if t := toolutil.ParseOptionalTime(input.CreatedAt); t != nil {
		opts.CreatedAt = t
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
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
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

// GetNote retrieves a single issue note by note ID from the GitLab
// Notes API (GET /projects/:id/issues/:issue_iid/notes/:note_id).
// Returns the note details or an error if the note is not found.
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

// Update replaces the body of an existing issue note via the GitLab
// Notes API (PUT /projects/:id/issues/:issue_iid/notes/:note_id). Only the
// original author can edit a note; system notes cannot be edited.
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

// Delete removes an issue note from a GitLab project via the GitLab
// Notes API (DELETE /projects/:id/issues/:issue_iid/notes/:note_id). Only
// the note author or a project Maintainer can delete a note; system notes
// cannot be deleted.
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
