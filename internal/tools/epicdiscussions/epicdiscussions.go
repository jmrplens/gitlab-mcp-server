// Package epicdiscussions implements MCP tools for GitLab epic discussion operations.
package epicdiscussions

import (
	"context"
	"errors"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Input types.

// ListInput defines parameters for listing epic discussions.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicID  int64                `json:"epic_id" jsonschema:"Epic ID,required"`
	Page    int64                `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int64                `json:"per_page,omitempty" jsonschema:"Number of items per page"`
}

// GetInput defines parameters for getting a single epic discussion.
type GetInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicID       int64                `json:"epic_id" jsonschema:"Epic ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
}

// CreateInput defines parameters for creating an epic discussion.
type CreateInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicID  int64                `json:"epic_id" jsonschema:"Epic ID,required"`
	Body    string               `json:"body" jsonschema:"Discussion body (Markdown supported),required"`
}

// AddNoteInput defines parameters for adding a note to an epic discussion.
type AddNoteInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicID       int64                `json:"epic_id" jsonschema:"Epic ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID to reply to,required"`
	Body         string               `json:"body" jsonschema:"Note body (Markdown supported),required"`
}

// UpdateNoteInput defines parameters for updating an epic discussion note.
type UpdateNoteInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicID       int64                `json:"epic_id" jsonschema:"Epic ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
	NoteID       int64                `json:"note_id" jsonschema:"Note ID to update,required"`
	Body         string               `json:"body" jsonschema:"Updated note body,required"`
}

// DeleteNoteInput defines parameters for deleting an epic discussion note.
type DeleteNoteInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicID       int64                `json:"epic_id" jsonschema:"Epic ID,required"`
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

// ListOutput holds a list of epic discussions.
type ListOutput struct {
	toolutil.HintableOutput
	Discussions []Output                  `json:"discussions"`
	Pagination  toolutil.PaginationOutput `json:"pagination"`
}

// Handlers.

// List lists epic discussions.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, errors.New("epic_discussion_list: group_id is required")
	}
	if input.EpicID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("epic_discussion_list", "epic_id")
	}
	opts := &gl.ListGroupEpicDiscussionsOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
	}
	discussions, resp, err := client.GL().Discussions.ListGroupEpicDiscussions(string(input.GroupID), input.EpicID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("epic_discussion_list", err)
	}
	return toListOutput(discussions, resp), nil
}

// Get gets a single epic discussion.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, errors.New("epic_discussion_get: group_id is required")
	}
	if input.EpicID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epic_discussion_get", "epic_id")
	}
	d, _, err := client.GL().Discussions.GetEpicDiscussion(string(input.GroupID), input.EpicID, input.DiscussionID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("epic_discussion_get", err)
	}
	return toOutput(d), nil
}

// Create creates a new epic discussion thread.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, errors.New("epic_discussion_create: group_id is required")
	}
	if input.EpicID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epic_discussion_create", "epic_id")
	}
	opts := &gl.CreateEpicDiscussionOptions{
		Body: new(input.Body),
	}
	d, _, err := client.GL().Discussions.CreateEpicDiscussion(string(input.GroupID), input.EpicID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("epic_discussion_create", err)
	}
	return toOutput(d), nil
}

// AddNote adds a note to an existing epic discussion.
func AddNote(ctx context.Context, client *gitlabclient.Client, input AddNoteInput) (NoteOutput, error) {
	if input.GroupID == "" {
		return NoteOutput{}, errors.New("epic_discussion_add_note: group_id is required")
	}
	if input.EpicID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("epic_discussion_add_note", "epic_id")
	}
	opts := &gl.AddEpicDiscussionNoteOptions{
		Body: new(input.Body),
	}
	note, _, err := client.GL().Discussions.AddEpicDiscussionNote(string(input.GroupID), input.EpicID, input.DiscussionID, opts, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithMessage("epic_discussion_add_note", err)
	}
	return noteToOutput(note), nil
}

// UpdateNote updates an existing epic discussion note.
func UpdateNote(ctx context.Context, client *gitlabclient.Client, input UpdateNoteInput) (NoteOutput, error) {
	if input.GroupID == "" {
		return NoteOutput{}, errors.New("epic_discussion_update_note: group_id is required")
	}
	if input.EpicID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("epic_discussion_update_note", "epic_id")
	}
	if input.NoteID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("epic_discussion_update_note", "note_id")
	}
	opts := &gl.UpdateEpicDiscussionNoteOptions{
		Body: new(input.Body),
	}
	note, _, err := client.GL().Discussions.UpdateEpicDiscussionNote(string(input.GroupID), input.EpicID, input.DiscussionID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithMessage("epic_discussion_update_note", err)
	}
	return noteToOutput(note), nil
}

// DeleteNote deletes an epic discussion note.
func DeleteNote(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) error {
	if input.GroupID == "" {
		return errors.New("epic_discussion_delete_note: group_id is required")
	}
	if input.EpicID <= 0 {
		return toolutil.ErrRequiredInt64("epic_discussion_delete_note", "epic_id")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("epic_discussion_delete_note", "note_id")
	}
	_, err := client.GL().Discussions.DeleteEpicDiscussionNote(string(input.GroupID), input.EpicID, input.DiscussionID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("epic_discussion_delete_note", err)
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
