package issuediscussions

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Input types.

// ListInput defines parameters for listing issue discussions.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	// OrderBy names the column used to order keyset-paginated results.
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. created_at, updated_at)"`
	// Sort selects the sort direction for ordered results.
	Sort string `json:"sort,omitempty" jsonschema:"Sort direction (asc or desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetInput defines parameters for getting a single issue discussion.
type GetInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID     int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
}

// CreateInput defines parameters for creating an issue discussion.
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	Body      string               `json:"body" jsonschema:"Discussion body (Markdown supported),required"`
	// CreatedAt backdates the discussion's first note; requires admin or project/group owner rights (ISO 8601).
	CreatedAt string `json:"created_at,omitempty" jsonschema:"Backdate the discussion creation time (ISO 8601, e.g. 2025-01-01T00:00:00Z). Requires admin or owner rights"`
}

// AddNoteInput defines parameters for adding a note to an issue discussion.
type AddNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID     int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID to reply to,required"`
	Body         string               `json:"body" jsonschema:"Note body (Markdown supported),required"`
	// CreatedAt backdates the note; requires admin or project/group owner rights (ISO 8601).
	CreatedAt string `json:"created_at,omitempty" jsonschema:"Backdate the note creation time (ISO 8601, e.g. 2025-01-01T00:00:00Z). Requires admin or owner rights"`
}

// UpdateNoteInput defines parameters for updating an issue discussion note.
type UpdateNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID     int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
	NoteID       int64                `json:"note_id" jsonschema:"Note ID to update,required"`
	Body         string               `json:"body" jsonschema:"Updated note body,required"`
	// CreatedAt overrides the note's creation timestamp; requires admin or project/group owner rights (ISO 8601).
	CreatedAt string `json:"created_at,omitempty" jsonschema:"Override the note creation time (ISO 8601, e.g. 2025-01-01T00:00:00Z). Requires admin or owner rights"`
}

// DeleteNoteInput defines parameters for deleting an issue discussion note.
type DeleteNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID     int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
	NoteID       int64                `json:"note_id" jsonschema:"Note ID to delete,required"`
}

// Output types.

// NoteOutput is a note of an issue discussion: a type of its own over
// [toolutil.DiscussionThreadNoteOutput], the note shape the four REST
// discussion packages share, every field GitLab's Note entity sends, the
// author as the UserBasic object it is. Until 2.8.0 this package published a
// six-field note with the author's username as a string. It is a defined type
// rather than an alias because the Markdown registry is keyed by type: while
// the four packages aliased one, the commit discussion card rendered every
// issue thread and note, offering commit discussion actions as their next
// steps (issue 1291).
type NoteOutput toolutil.DiscussionThreadNoteOutput

// Output is an issue discussion thread: a type of its own over
// [toolutil.DiscussionThreadOutput], the thread shape with full note payloads
// and the thread's own resolution state, for the reason [NoteOutput] gives.
type Output toolutil.DiscussionThreadOutput

// ListOutput holds a list of issue discussions.
type ListOutput struct {
	toolutil.HintableOutput
	Discussions []Output                  `json:"discussions"`
	Pagination  toolutil.PaginationOutput `json:"pagination"`
}

// Handlers.

// List lists issue discussions.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := toolutil.RequireProjectItem(ctx, "issue_discussion_list", input.ProjectID, "issue_iid", input.IssueIID); err != nil {
		return ListOutput{}, err
	}
	opts := &gl.ListIssueDiscussionsOptions{
		OrderBy: input.OrderBy, Sort: input.Sort,
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	discussions, resp, err := client.GL().Discussions.ListIssueDiscussions(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("issue_discussion_list", err, http.StatusNotFound,
			"verify project_id and issue_iid with issue.get")
	}
	threads, err := capturedThreads("issue_discussion_list", discussions, captured)
	if err != nil {
		return ListOutput{}, err
	}
	return ListOutput{Discussions: threads, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// Get gets a single issue discussion.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := toolutil.RequireProjectItem(ctx, "issue_discussion_get", input.ProjectID, "issue_iid", input.IssueIID); err != nil {
		return Output{}, err
	}
	if input.DiscussionID == "" {
		return Output{}, errors.New("issue_discussion_get: discussion_id is required")
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	d, _, err := client.GL().Discussions.GetIssueDiscussion(string(input.ProjectID), input.IssueIID, input.DiscussionID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("issue_discussion_get", err, http.StatusNotFound,
			"verify discussion_id with issue.discussion_list")
	}
	return capturedThread("issue_discussion_get", d, captured)
}

// Create creates a new issue discussion thread.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := toolutil.RequireProjectItem(ctx, "issue_discussion_create", input.ProjectID, "issue_iid", input.IssueIID); err != nil {
		return Output{}, err
	}
	createdAt, err := toolutil.ParseTimeField("created_at", input.CreatedAt)
	if err != nil {
		return Output{}, fmt.Errorf("issue_discussion_create: %w", err)
	}
	opts := &gl.CreateIssueDiscussionOptions{
		Body:      new(input.Body),
		CreatedAt: createdAt,
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	d, _, err := client.GL().Discussions.CreateIssueDiscussion(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("issue_discussion_create", err, http.StatusNotFound,
			"verify project_id and issue_iid with issue.get; creating discussions requires Reporter role or higher")
	}
	return capturedThread("issue_discussion_create", d, captured)
}

// AddNote adds a note to an existing issue discussion.
func AddNote(ctx context.Context, client *gitlabclient.Client, input AddNoteInput) (NoteOutput, error) {
	if err := toolutil.RequireProjectItem(ctx, "issue_discussion_add_note", input.ProjectID, "issue_iid", input.IssueIID); err != nil {
		return NoteOutput{}, err
	}
	if input.DiscussionID == "" {
		return NoteOutput{}, errors.New("issue_discussion_add_note: discussion_id is required")
	}
	createdAt, err := toolutil.ParseTimeField("created_at", input.CreatedAt)
	if err != nil {
		return NoteOutput{}, fmt.Errorf("issue_discussion_add_note: %w", err)
	}
	opts := &gl.AddIssueDiscussionNoteOptions{
		Body:      new(input.Body),
		CreatedAt: createdAt,
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	note, _, err := client.GL().Discussions.AddIssueDiscussionNote(string(input.ProjectID), input.IssueIID, input.DiscussionID, opts, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("issue_discussion_add_note", err, http.StatusNotFound,
			"verify discussion_id with issue.discussion_list")
	}
	return capturedThreadNote("issue_discussion_add_note", note, captured)
}

// UpdateNote updates an existing issue discussion note.
func UpdateNote(ctx context.Context, client *gitlabclient.Client, input UpdateNoteInput) (NoteOutput, error) {
	if err := toolutil.RequireProjectItem(ctx, "issue_discussion_update_note", input.ProjectID, "issue_iid", input.IssueIID); err != nil {
		return NoteOutput{}, err
	}
	if input.DiscussionID == "" {
		return NoteOutput{}, errors.New("issue_discussion_update_note: discussion_id is required")
	}
	if input.NoteID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("issue_discussion_update_note", "note_id")
	}
	createdAt, err := toolutil.ParseTimeField("created_at", input.CreatedAt)
	if err != nil {
		return NoteOutput{}, fmt.Errorf("issue_discussion_update_note: %w", err)
	}
	opts := &gl.UpdateIssueDiscussionNoteOptions{
		Body:      new(input.Body),
		CreatedAt: createdAt,
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	note, _, err := client.GL().Discussions.UpdateIssueDiscussionNote(string(input.ProjectID), input.IssueIID, input.DiscussionID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("issue_discussion_update_note", err, http.StatusForbidden,
			"only the note author can edit a discussion note")
	}
	return capturedThreadNote("issue_discussion_update_note", note, captured)
}

// DeleteNote deletes an issue discussion note.
func DeleteNote(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) error {
	if err := toolutil.RequireProjectItem(ctx, "issue_discussion_delete_note", input.ProjectID, "issue_iid", input.IssueIID); err != nil {
		return err
	}
	if input.DiscussionID == "" {
		return errors.New("issue_discussion_delete_note: discussion_id is required")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("issue_discussion_delete_note", "note_id")
	}
	_, err := client.GL().Discussions.DeleteIssueDiscussionNote(string(input.ProjectID), input.IssueIID, input.DiscussionID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("issue_discussion_delete_note", err, http.StatusForbidden,
			"only the note author or a Maintainer can delete a discussion note")
	}
	return nil
}

// Converters.

// capturedThread, capturedThreads and capturedThreadNote are the shared
// conversions of a captured answer, answering in this package's own types.
var (
	capturedThread     = toolutil.CapturedThread[Output]
	capturedThreads    = toolutil.CapturedThreads[Output]
	capturedThreadNote = toolutil.CapturedThreadNote[NoteOutput]
)

// Formatters.
