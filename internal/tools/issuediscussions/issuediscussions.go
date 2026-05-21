package issuediscussions

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Input types.

// ListInput defines parameters for listing issue discussions.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page"`
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
}

// AddNoteInput defines parameters for adding a note to an issue discussion.
type AddNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID     int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID to reply to,required"`
	Body         string               `json:"body" jsonschema:"Note body (Markdown supported),required"`
}

// UpdateNoteInput defines parameters for updating an issue discussion note.
type UpdateNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID     int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
	NoteID       int64                `json:"note_id" jsonschema:"Note ID to update,required"`
	Body         string               `json:"body" jsonschema:"Updated note body,required"`
}

// DeleteNoteInput defines parameters for deleting an issue discussion note.
type DeleteNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID     int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
	NoteID       int64                `json:"note_id" jsonschema:"Note ID to delete,required"`
}

// Output types.

// NoteOutput represents a single note within a discussion.
type NoteOutput = toolutil.DiscussionNoteOutput

// Output represents a discussion thread.
type Output = toolutil.DiscussionOutput

// ListOutput holds a list of issue discussions.
type ListOutput struct {
	toolutil.HintableOutput
	Discussions []Output                  `json:"discussions"`
	Pagination  toolutil.PaginationOutput `json:"pagination"`
}

// Handlers.

// List lists issue discussions.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("issue_discussion_list: project_id is required")
	}
	if input.IssueIID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("issue_discussion_list", "issue_iid")
	}
	opts := &gl.ListIssueDiscussionsOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
	}
	discussions, resp, err := client.GL().Discussions.ListIssueDiscussions(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("issue_discussion_list", err, http.StatusNotFound,
			"verify project_id and issue_iid with gitlab_issue_get")
	}
	return toListOutput(discussions, resp), nil
}

// Get gets a single issue discussion.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("issue_discussion_get: project_id is required")
	}
	if input.IssueIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_discussion_get", "issue_iid")
	}
	if input.DiscussionID == "" {
		return Output{}, errors.New("issue_discussion_get: discussion_id is required")
	}
	d, _, err := client.GL().Discussions.GetIssueDiscussion(string(input.ProjectID), input.IssueIID, input.DiscussionID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("issue_discussion_get", err, http.StatusNotFound,
			"verify discussion_id with gitlab_list_issue_discussions")
	}
	return toolutil.DiscussionOutputFromGitLab(d), nil
}

// Create creates a new issue discussion thread.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("issue_discussion_create: project_id is required")
	}
	if input.IssueIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_discussion_create", "issue_iid")
	}
	opts := &gl.CreateIssueDiscussionOptions{
		Body: new(input.Body),
	}
	d, _, err := client.GL().Discussions.CreateIssueDiscussion(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("issue_discussion_create", err, http.StatusNotFound,
			"verify project_id and issue_iid with gitlab_issue_get; creating discussions requires Reporter role or higher")
	}
	return toolutil.DiscussionOutputFromGitLab(d), nil
}

// AddNote adds a note to an existing issue discussion.
func AddNote(ctx context.Context, client *gitlabclient.Client, input AddNoteInput) (NoteOutput, error) {
	if err := ctx.Err(); err != nil {
		return NoteOutput{}, err
	}
	if input.ProjectID == "" {
		return NoteOutput{}, errors.New("issue_discussion_add_note: project_id is required")
	}
	if input.IssueIID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("issue_discussion_add_note", "issue_iid")
	}
	if input.DiscussionID == "" {
		return NoteOutput{}, errors.New("issue_discussion_add_note: discussion_id is required")
	}
	opts := &gl.AddIssueDiscussionNoteOptions{
		Body: new(input.Body),
	}
	note, _, err := client.GL().Discussions.AddIssueDiscussionNote(string(input.ProjectID), input.IssueIID, input.DiscussionID, opts, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("issue_discussion_add_note", err, http.StatusNotFound,
			"verify discussion_id with gitlab_list_issue_discussions")
	}
	return toolutil.DiscussionNoteOutputFromGitLab(note), nil
}

// UpdateNote updates an existing issue discussion note.
func UpdateNote(ctx context.Context, client *gitlabclient.Client, input UpdateNoteInput) (NoteOutput, error) {
	if err := ctx.Err(); err != nil {
		return NoteOutput{}, err
	}
	if input.ProjectID == "" {
		return NoteOutput{}, errors.New("issue_discussion_update_note: project_id is required")
	}
	if input.IssueIID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("issue_discussion_update_note", "issue_iid")
	}
	if input.DiscussionID == "" {
		return NoteOutput{}, errors.New("issue_discussion_update_note: discussion_id is required")
	}
	if input.NoteID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("issue_discussion_update_note", "note_id")
	}
	opts := &gl.UpdateIssueDiscussionNoteOptions{
		Body: new(input.Body),
	}
	note, _, err := client.GL().Discussions.UpdateIssueDiscussionNote(string(input.ProjectID), input.IssueIID, input.DiscussionID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("issue_discussion_update_note", err, http.StatusForbidden,
			"only the note author can edit a discussion note")
	}
	return toolutil.DiscussionNoteOutputFromGitLab(note), nil
}

// DeleteNote deletes an issue discussion note.
func DeleteNote(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("issue_discussion_delete_note: project_id is required")
	}
	if input.IssueIID <= 0 {
		return toolutil.ErrRequiredInt64("issue_discussion_delete_note", "issue_iid")
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

// toListOutput converts the GitLab API response to the tool output format.
func toListOutput(discussions []*gl.Discussion, resp *gl.Response) ListOutput {
	return ListOutput{
		Discussions: toolutil.DiscussionOutputsFromGitLab(discussions),
		Pagination:  toolutil.PaginationFromResponse(resp),
	}
}

// Formatters.
