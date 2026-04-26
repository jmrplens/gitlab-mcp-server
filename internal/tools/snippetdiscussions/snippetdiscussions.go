// Package snippetdiscussions implements MCP tools for GitLab snippet discussion operations.
package snippetdiscussions

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Input types.

// ListInput defines parameters for listing snippet discussions.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SnippetID int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page"`
}

// GetInput defines parameters for getting a single snippet discussion.
type GetInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SnippetID    int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
}

// CreateInput defines parameters for creating a snippet discussion.
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SnippetID int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	Body      string               `json:"body" jsonschema:"Discussion body (Markdown supported),required"`
}

// AddNoteInput defines parameters for adding a note to a snippet discussion.
type AddNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SnippetID    int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID to reply to,required"`
	Body         string               `json:"body" jsonschema:"Note body (Markdown supported),required"`
}

// UpdateNoteInput defines parameters for updating a snippet discussion note.
type UpdateNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SnippetID    int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
	NoteID       int64                `json:"note_id" jsonschema:"Note ID to update,required"`
	Body         string               `json:"body" jsonschema:"Updated note body,required"`
}

// DeleteNoteInput defines parameters for deleting a snippet discussion note.
type DeleteNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SnippetID    int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
	NoteID       int64                `json:"note_id" jsonschema:"Note ID to delete,required"`
}

// Output types.

// NoteOutput represents a single note within a discussion.
type NoteOutput struct {
	toolutil.HintableOutput
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
	System    bool   `json:"system"`
}

// Output represents a discussion thread.
type Output struct {
	toolutil.HintableOutput
	ID             string       `json:"id"`
	IndividualNote bool         `json:"individual_note"`
	Notes          []NoteOutput `json:"notes"`
}

// ListOutput holds a list of snippet discussions.
type ListOutput struct {
	toolutil.HintableOutput
	Discussions []Output                  `json:"discussions"`
	Pagination  toolutil.PaginationOutput `json:"pagination"`
}

// Handlers.

// List lists snippet discussions.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("snippet_discussion_list: project_id is required")
	}
	if input.SnippetID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("snippet_discussion_list", "snippet_id")
	}
	opts := &gl.ListSnippetDiscussionsOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
	}
	discussions, resp, err := client.GL().Discussions.ListSnippetDiscussions(string(input.ProjectID), input.SnippetID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("snippet_discussion_list", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get and snippet_id with gitlab_project_snippet_list")
	}
	return toListOutput(discussions, resp), nil
}

// Get gets a single snippet discussion.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, errors.New("snippet_discussion_get: project_id is required")
	}
	if input.SnippetID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_discussion_get", "snippet_id")
	}
	d, _, err := client.GL().Discussions.GetSnippetDiscussion(string(input.ProjectID), input.SnippetID, input.DiscussionID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("snippet_discussion_get", err, http.StatusNotFound,
			"verify discussion_id with gitlab_list_snippet_discussions (discussion IDs are 40-char hex strings)")
	}
	return toOutput(d), nil
}

// Create creates a new snippet discussion thread.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, errors.New("snippet_discussion_create: project_id is required")
	}
	if input.SnippetID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_discussion_create", "snippet_id")
	}
	opts := &gl.CreateSnippetDiscussionOptions{
		Body: new(input.Body),
	}
	d, _, err := client.GL().Discussions.CreateSnippetDiscussion(string(input.ProjectID), input.SnippetID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("snippet_discussion_create", err, http.StatusBadRequest,
			"body is required and cannot be empty; commenting requires Reporter role or being the snippet author")
	}
	return toOutput(d), nil
}

// AddNote adds a note to an existing snippet discussion.
func AddNote(ctx context.Context, client *gitlabclient.Client, input AddNoteInput) (NoteOutput, error) {
	if input.ProjectID == "" {
		return NoteOutput{}, errors.New("snippet_discussion_add_note: project_id is required")
	}
	if input.SnippetID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("snippet_discussion_add_note", "snippet_id")
	}
	opts := &gl.AddSnippetDiscussionNoteOptions{
		Body: new(input.Body),
	}
	note, _, err := client.GL().Discussions.AddSnippetDiscussionNote(string(input.ProjectID), input.SnippetID, input.DiscussionID, opts, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("snippet_discussion_add_note", err, http.StatusNotFound,
			"verify discussion_id with gitlab_list_snippet_discussions; the discussion must exist on this snippet")
	}
	return noteToOutput(note), nil
}

// UpdateNote updates an existing snippet discussion note.
func UpdateNote(ctx context.Context, client *gitlabclient.Client, input UpdateNoteInput) (NoteOutput, error) {
	if input.ProjectID == "" {
		return NoteOutput{}, errors.New("snippet_discussion_update_note: project_id is required")
	}
	if input.SnippetID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("snippet_discussion_update_note", "snippet_id")
	}
	if input.NoteID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("snippet_discussion_update_note", "note_id")
	}
	opts := &gl.UpdateSnippetDiscussionNoteOptions{
		Body: new(input.Body),
	}
	note, _, err := client.GL().Discussions.UpdateSnippetDiscussionNote(string(input.ProjectID), input.SnippetID, input.DiscussionID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("snippet_discussion_update_note", err, http.StatusForbidden,
			"updating a note requires being the note author; system notes cannot be modified")
	}
	return noteToOutput(note), nil
}

// DeleteNote deletes a snippet discussion note.
func DeleteNote(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) error {
	if input.ProjectID == "" {
		return errors.New("snippet_discussion_delete_note: project_id is required")
	}
	if input.SnippetID <= 0 {
		return toolutil.ErrRequiredInt64("snippet_discussion_delete_note", "snippet_id")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("snippet_discussion_delete_note", "note_id")
	}
	_, err := client.GL().Discussions.DeleteSnippetDiscussionNote(string(input.ProjectID), input.SnippetID, input.DiscussionID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("snippet_discussion_delete_note", err, http.StatusForbidden,
			"deleting a note requires being the note author or Maintainer role; system notes cannot be deleted")
	}
	return nil
}

// Converters.

// noteToOutput converts the GitLab API response to the tool output format.
func noteToOutput(n *gl.Note) NoteOutput {
	out := NoteOutput{
		ID:     n.ID,
		Body:   n.Body,
		System: n.System,
	}
	if n.Author.Username != "" {
		out.Author = n.Author.Username
	}
	if !n.CreatedAt.IsZero() {
		out.CreatedAt = n.CreatedAt.Format(time.RFC3339)
	}
	if n.UpdatedAt != nil && !n.UpdatedAt.IsZero() {
		out.UpdatedAt = n.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// toOutput converts the GitLab API response to the tool output format.
func toOutput(d *gl.Discussion) Output {
	out := Output{
		ID:             d.ID,
		IndividualNote: d.IndividualNote,
		Notes:          make([]NoteOutput, 0, len(d.Notes)),
	}
	for _, n := range d.Notes {
		out.Notes = append(out.Notes, noteToOutput(n))
	}
	return out
}

// toListOutput converts the GitLab API response to the tool output format.
func toListOutput(discussions []*gl.Discussion, resp *gl.Response) ListOutput {
	out := ListOutput{
		Discussions: make([]Output, 0, len(discussions)),
		Pagination:  toolutil.PaginationFromResponse(resp),
	}
	for _, d := range discussions {
		out.Discussions = append(out.Discussions, toOutput(d))
	}
	return out
}

// Formatters.
