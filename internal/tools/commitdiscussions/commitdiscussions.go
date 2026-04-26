// Package commitdiscussions implements MCP tools for GitLab commit discussion operations.
package commitdiscussions

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Input types.

// ListInput defines parameters for listing commit discussions.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CommitSHA string               `json:"commit_sha" jsonschema:"Commit SHA,required"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page"`
}

// GetInput defines parameters for getting a single commit discussion.
type GetInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CommitSHA    string               `json:"commit_sha" jsonschema:"Commit SHA,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
}

// PositionInput defines position attributes for inline commit discussions.
type PositionInput struct {
	BaseSHA      string `json:"base_sha" jsonschema:"Base commit SHA,required"`
	StartSHA     string `json:"start_sha" jsonschema:"Start commit SHA,required"`
	HeadSHA      string `json:"head_sha" jsonschema:"Head commit SHA,required"`
	PositionType string `json:"position_type" jsonschema:"Position type (text or image),required"`
	NewPath      string `json:"new_path,omitempty" jsonschema:"File path after change"`
	NewLine      int64  `json:"new_line,omitempty" jsonschema:"Line number after change"`
	OldPath      string `json:"old_path,omitempty" jsonschema:"File path before change"`
	OldLine      int64  `json:"old_line,omitempty" jsonschema:"Line number before change"`
}

// CreateInput defines parameters for creating a commit discussion.
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CommitSHA string               `json:"commit_sha" jsonschema:"Commit SHA,required"`
	Body      string               `json:"body" jsonschema:"Discussion body (Markdown supported),required"`
	Position  *PositionInput       `json:"position,omitempty" jsonschema:"Position for inline diff comments"`
}

// AddNoteInput defines parameters for adding a note to a commit discussion.
type AddNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CommitSHA    string               `json:"commit_sha" jsonschema:"Commit SHA,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID to reply to,required"`
	Body         string               `json:"body" jsonschema:"Note body (Markdown supported),required"`
}

// UpdateNoteInput defines parameters for updating a commit discussion note.
type UpdateNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CommitSHA    string               `json:"commit_sha" jsonschema:"Commit SHA,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
	NoteID       int64                `json:"note_id" jsonschema:"Note ID to update,required"`
	Body         string               `json:"body" jsonschema:"Updated note body,required"`
}

// DeleteNoteInput defines parameters for deleting a commit discussion note.
type DeleteNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CommitSHA    string               `json:"commit_sha" jsonschema:"Commit SHA,required"`
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

// ListOutput holds a list of commit discussions.
type ListOutput struct {
	toolutil.HintableOutput
	Discussions []Output                  `json:"discussions"`
	Pagination  toolutil.PaginationOutput `json:"pagination"`
}

// Handlers.

// List lists commit discussions.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListCommitDiscussionsOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
	}
	discussions, resp, err := client.GL().Discussions.ListCommitDiscussions(string(input.ProjectID), input.CommitSHA, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("commit_discussion_list", err, http.StatusNotFound,
			"verify project_id and commit_sha with gitlab_commit_get")
	}
	return toListOutput(discussions, resp), nil
}

// Get gets a single commit discussion.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	d, _, err := client.GL().Discussions.GetCommitDiscussion(string(input.ProjectID), input.CommitSHA, input.DiscussionID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("commit_discussion_get", err, http.StatusNotFound,
			"verify discussion_id with gitlab_list_commit_discussions")
	}
	return toOutput(d), nil
}

// Create creates a new commit discussion thread.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	opts := &gl.CreateCommitDiscussionOptions{
		Body: new(input.Body),
	}
	if input.Position != nil {
		opts.Position = &gl.NotePosition{
			BaseSHA:      input.Position.BaseSHA,
			StartSHA:     input.Position.StartSHA,
			HeadSHA:      input.Position.HeadSHA,
			PositionType: input.Position.PositionType,
			NewPath:      input.Position.NewPath,
			NewLine:      input.Position.NewLine,
			OldPath:      input.Position.OldPath,
			OldLine:      input.Position.OldLine,
		}
	}
	d, _, err := client.GL().Discussions.CreateCommitDiscussion(string(input.ProjectID), input.CommitSHA, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("commit_discussion_create", err, http.StatusBadRequest,
			"for inline diff comments, position requires base_sha, head_sha, start_sha, position_type=text, and a valid old_path/new_path with line numbers; verify the commit_sha exists in the project")
	}
	return toOutput(d), nil
}

// AddNote adds a note to an existing commit discussion.
func AddNote(ctx context.Context, client *gitlabclient.Client, input AddNoteInput) (NoteOutput, error) {
	opts := &gl.AddCommitDiscussionNoteOptions{
		Body: new(input.Body),
	}
	note, _, err := client.GL().Discussions.AddCommitDiscussionNote(string(input.ProjectID), input.CommitSHA, input.DiscussionID, opts, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("commit_discussion_add_note", err, http.StatusNotFound,
			"verify discussion_id with gitlab_list_commit_discussions")
	}
	return noteToOutput(note), nil
}

// UpdateNote updates an existing commit discussion note.
func UpdateNote(ctx context.Context, client *gitlabclient.Client, input UpdateNoteInput) (NoteOutput, error) {
	if input.NoteID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("commit_discussion_update_note", "note_id")
	}
	opts := &gl.UpdateCommitDiscussionNoteOptions{
		Body: new(input.Body),
	}
	note, _, err := client.GL().Discussions.UpdateCommitDiscussionNote(string(input.ProjectID), input.CommitSHA, input.DiscussionID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("commit_discussion_update_note", err, http.StatusForbidden,
			"only the note author can edit a discussion note")
	}
	return noteToOutput(note), nil
}

// DeleteNote deletes a commit discussion note.
func DeleteNote(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) error {
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("commit_discussion_delete_note", "note_id")
	}
	_, err := client.GL().Discussions.DeleteCommitDiscussionNote(string(input.ProjectID), input.CommitSHA, input.DiscussionID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("commit_discussion_delete_note", err, http.StatusForbidden,
			"only the note author or a Maintainer can delete a discussion note")
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
