// Package awardemoji implements MCP tools for GitLab award emoji operations
// on issues, merge requests, snippets, and their notes.
package awardemoji

import (
	"context"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Common Input/Output types.

// ListInput is the common input for listing award emoji on a resource.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"iid" jsonschema:"Issue IID or MR IID or Snippet ID,required"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination (default 1)"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page (default 20, max 100)"`
}

// ListOnNoteInput is the input for listing award emoji on a note.
type ListOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"iid" jsonschema:"Issue IID or MR IID or Snippet ID,required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination (default 1)"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page (default 20, max 100)"`
}

// GetInput is the common input for getting a single award emoji.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"iid" jsonschema:"Issue IID or MR IID or Snippet ID,required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// GetOnNoteInput is the input for getting an award emoji on a note.
type GetOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"iid" jsonschema:"Issue IID or MR IID or Snippet ID,required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// CreateInput is the common input for creating an award emoji.
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"iid" jsonschema:"Issue IID or MR IID or Snippet ID,required"`
	Name      string               `json:"name" jsonschema:"Emoji name without colons (e.g. thumbsup),required"`
}

// CreateOnNoteInput is the input for creating an award emoji on a note.
type CreateOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"iid" jsonschema:"Issue IID or MR IID or Snippet ID,required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	Name      string               `json:"name" jsonschema:"Emoji name without colons (e.g. thumbsup),required"`
}

// DeleteInput is the common input for deleting an award emoji.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"iid" jsonschema:"Issue IID or MR IID or Snippet ID,required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// DeleteOnNoteInput is the input for deleting an award emoji on a note.
type DeleteOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"iid" jsonschema:"Issue IID or MR IID or Snippet ID,required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// Output represents a single award emoji.
type Output struct {
	toolutil.HintableOutput
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	UserID        int64  `json:"user_id"`
	Username      string `json:"username"`
	CreatedAt     string `json:"created_at,omitempty"`
	AwardableID   int64  `json:"awardable_id"`
	AwardableType string `json:"awardable_type"`
}

// ListOutput holds a paginated list of award emoji.
type ListOutput struct {
	toolutil.HintableOutput
	AwardEmoji []Output                  `json:"award_emoji"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Issue Award Emoji Handlers.

// ListIssueAwardEmoji lists all award emoji on an issue.
func ListIssueAwardEmoji(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("issue_emoji_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("issue_emoji_list", "iid")
	}
	opts := &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage}}
	emojis, resp, err := client.GL().AwardEmoji.ListIssueAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("issue_emoji_list", err)
	}
	return toListOutput(emojis, resp), nil
}

// GetIssueAwardEmoji gets a single award emoji on an issue.
func GetIssueAwardEmoji(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("issue_emoji_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_emoji_get", "iid")
	}
	if input.AwardID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_emoji_get", "award_id")
	}
	emoji, _, err := client.GL().AwardEmoji.GetIssueAwardEmoji(string(input.ProjectID), input.IID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("issue_emoji_get", err)
	}
	return toOutput(emoji), nil
}

// CreateIssueAwardEmoji creates an award emoji on an issue.
func CreateIssueAwardEmoji(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("issue_emoji_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_emoji_create", "iid")
	}
	opts := &gl.CreateAwardEmojiOptions{Name: input.Name}
	emoji, _, err := client.GL().AwardEmoji.CreateIssueAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("issue_emoji_create", err)
	}
	return toOutput(emoji), nil
}

// DeleteIssueAwardEmoji deletes an award emoji from an issue.
func DeleteIssueAwardEmoji(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("issue_emoji_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("issue_emoji_delete", "iid")
	}
	if input.AwardID <= 0 {
		return toolutil.ErrRequiredInt64("issue_emoji_delete", "award_id")
	}
	_, err := client.GL().AwardEmoji.DeleteIssueAwardEmoji(string(input.ProjectID), input.IID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("issue_emoji_delete", err)
	}
	return nil
}

// Issue Note Award Emoji Handlers.

// ListIssueNoteAwardEmoji lists all award emoji on an issue note.
func ListIssueNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input ListOnNoteInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("issue_note_emoji_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("issue_note_emoji_list", "iid")
	}
	if input.NoteID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("issue_note_emoji_list", "note_id")
	}
	opts := &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage}}
	emojis, resp, err := client.GL().AwardEmoji.ListIssuesAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("issue_note_emoji_list", err)
	}
	return toListOutput(emojis, resp), nil
}

// GetIssueNoteAwardEmoji gets a single award emoji on an issue note.
func GetIssueNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input GetOnNoteInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("issue_note_emoji_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_note_emoji_get", "iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_note_emoji_get", "note_id")
	}
	if input.AwardID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_note_emoji_get", "award_id")
	}
	emoji, _, err := client.GL().AwardEmoji.GetIssuesAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("issue_note_emoji_get", err)
	}
	return toOutput(emoji), nil
}

// CreateIssueNoteAwardEmoji creates an award emoji on an issue note.
func CreateIssueNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input CreateOnNoteInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("issue_note_emoji_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_note_emoji_create", "iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_note_emoji_create", "note_id")
	}
	opts := &gl.CreateAwardEmojiOptions{Name: input.Name}
	emoji, _, err := client.GL().AwardEmoji.CreateIssuesAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("issue_note_emoji_create", err)
	}
	return toOutput(emoji), nil
}

// DeleteIssueNoteAwardEmoji deletes an award emoji from an issue note.
func DeleteIssueNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input DeleteOnNoteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("issue_note_emoji_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("issue_note_emoji_delete", "iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("issue_note_emoji_delete", "note_id")
	}
	if input.AwardID <= 0 {
		return toolutil.ErrRequiredInt64("issue_note_emoji_delete", "award_id")
	}
	_, err := client.GL().AwardEmoji.DeleteIssuesAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("issue_note_emoji_delete", err)
	}
	return nil
}

// MR Award Emoji Handlers.

// ListMRAwardEmoji lists all award emoji on a merge request.
func ListMRAwardEmoji(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("mr_emoji_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("mr_emoji_list", "iid")
	}
	opts := &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage}}
	emojis, resp, err := client.GL().AwardEmoji.ListMergeRequestAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("mr_emoji_list", err)
	}
	return toListOutput(emojis, resp), nil
}

// GetMRAwardEmoji gets a single award emoji on a merge request.
func GetMRAwardEmoji(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("mr_emoji_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_emoji_get", "iid")
	}
	if input.AwardID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_emoji_get", "award_id")
	}
	emoji, _, err := client.GL().AwardEmoji.GetMergeRequestAwardEmoji(string(input.ProjectID), input.IID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("mr_emoji_get", err)
	}
	return toOutput(emoji), nil
}

// CreateMRAwardEmoji creates an award emoji on a merge request.
func CreateMRAwardEmoji(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("mr_emoji_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_emoji_create", "iid")
	}
	opts := &gl.CreateAwardEmojiOptions{Name: input.Name}
	emoji, _, err := client.GL().AwardEmoji.CreateMergeRequestAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("mr_emoji_create", err)
	}
	return toOutput(emoji), nil
}

// DeleteMRAwardEmoji deletes an award emoji from a merge request.
func DeleteMRAwardEmoji(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("mr_emoji_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("mr_emoji_delete", "iid")
	}
	if input.AwardID <= 0 {
		return toolutil.ErrRequiredInt64("mr_emoji_delete", "award_id")
	}
	_, err := client.GL().AwardEmoji.DeleteMergeRequestAwardEmoji(string(input.ProjectID), input.IID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("mr_emoji_delete", err)
	}
	return nil
}

// MR Note Award Emoji Handlers.

// ListMRNoteAwardEmoji lists all award emoji on a merge request note.
func ListMRNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input ListOnNoteInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("mr_note_emoji_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("mr_note_emoji_list", "iid")
	}
	if input.NoteID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("mr_note_emoji_list", "note_id")
	}
	opts := &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage}}
	emojis, resp, err := client.GL().AwardEmoji.ListMergeRequestAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("mr_note_emoji_list", err)
	}
	return toListOutput(emojis, resp), nil
}

// GetMRNoteAwardEmoji gets a single award emoji on a merge request note.
func GetMRNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input GetOnNoteInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("mr_note_emoji_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_note_emoji_get", "iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_note_emoji_get", "note_id")
	}
	if input.AwardID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_note_emoji_get", "award_id")
	}
	emoji, _, err := client.GL().AwardEmoji.GetMergeRequestAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("mr_note_emoji_get", err)
	}
	return toOutput(emoji), nil
}

// CreateMRNoteAwardEmoji creates an award emoji on a merge request note.
func CreateMRNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input CreateOnNoteInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("mr_note_emoji_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_note_emoji_create", "iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_note_emoji_create", "note_id")
	}
	opts := &gl.CreateAwardEmojiOptions{Name: input.Name}
	emoji, _, err := client.GL().AwardEmoji.CreateMergeRequestAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("mr_note_emoji_create", err)
	}
	return toOutput(emoji), nil
}

// DeleteMRNoteAwardEmoji deletes an award emoji from a merge request note.
func DeleteMRNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input DeleteOnNoteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("mr_note_emoji_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("mr_note_emoji_delete", "iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("mr_note_emoji_delete", "note_id")
	}
	if input.AwardID <= 0 {
		return toolutil.ErrRequiredInt64("mr_note_emoji_delete", "award_id")
	}
	_, err := client.GL().AwardEmoji.DeleteMergeRequestAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("mr_note_emoji_delete", err)
	}
	return nil
}

// Snippet Award Emoji Handlers.

// ListSnippetAwardEmoji lists all award emoji on a snippet.
func ListSnippetAwardEmoji(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("snippet_emoji_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("snippet_emoji_list", "iid")
	}
	opts := &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage}}
	emojis, resp, err := client.GL().AwardEmoji.ListSnippetAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("snippet_emoji_list", err)
	}
	return toListOutput(emojis, resp), nil
}

// GetSnippetAwardEmoji gets a single award emoji on a snippet.
func GetSnippetAwardEmoji(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("snippet_emoji_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_emoji_get", "iid")
	}
	if input.AwardID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_emoji_get", "award_id")
	}
	emoji, _, err := client.GL().AwardEmoji.GetSnippetAwardEmoji(string(input.ProjectID), input.IID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("snippet_emoji_get", err)
	}
	return toOutput(emoji), nil
}

// CreateSnippetAwardEmoji creates an award emoji on a snippet.
func CreateSnippetAwardEmoji(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("snippet_emoji_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_emoji_create", "iid")
	}
	opts := &gl.CreateAwardEmojiOptions{Name: input.Name}
	emoji, _, err := client.GL().AwardEmoji.CreateSnippetAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("snippet_emoji_create", err)
	}
	return toOutput(emoji), nil
}

// DeleteSnippetAwardEmoji deletes an award emoji from a snippet.
func DeleteSnippetAwardEmoji(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("snippet_emoji_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("snippet_emoji_delete", "iid")
	}
	if input.AwardID <= 0 {
		return toolutil.ErrRequiredInt64("snippet_emoji_delete", "award_id")
	}
	_, err := client.GL().AwardEmoji.DeleteSnippetAwardEmoji(string(input.ProjectID), input.IID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("snippet_emoji_delete", err)
	}
	return nil
}

// Snippet Note Award Emoji Handlers.

// ListSnippetNoteAwardEmoji lists all award emoji on a snippet note.
func ListSnippetNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input ListOnNoteInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("snippet_note_emoji_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("snippet_note_emoji_list", "iid")
	}
	if input.NoteID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("snippet_note_emoji_list", "note_id")
	}
	opts := &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage}}
	emojis, resp, err := client.GL().AwardEmoji.ListSnippetAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("snippet_note_emoji_list", err)
	}
	return toListOutput(emojis, resp), nil
}

// GetSnippetNoteAwardEmoji gets a single award emoji on a snippet note.
func GetSnippetNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input GetOnNoteInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("snippet_note_emoji_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_note_emoji_get", "iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_note_emoji_get", "note_id")
	}
	if input.AwardID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_note_emoji_get", "award_id")
	}
	emoji, _, err := client.GL().AwardEmoji.GetSnippetAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("snippet_note_emoji_get", err)
	}
	return toOutput(emoji), nil
}

// CreateSnippetNoteAwardEmoji creates an award emoji on a snippet note.
func CreateSnippetNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input CreateOnNoteInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("snippet_note_emoji_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_note_emoji_create", "iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_note_emoji_create", "note_id")
	}
	opts := &gl.CreateAwardEmojiOptions{Name: input.Name}
	emoji, _, err := client.GL().AwardEmoji.CreateSnippetAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("snippet_note_emoji_create", err)
	}
	return toOutput(emoji), nil
}

// DeleteSnippetNoteAwardEmoji deletes an award emoji from a snippet note.
func DeleteSnippetNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input DeleteOnNoteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("snippet_note_emoji_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("snippet_note_emoji_delete", "iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("snippet_note_emoji_delete", "note_id")
	}
	if input.AwardID <= 0 {
		return toolutil.ErrRequiredInt64("snippet_note_emoji_delete", "award_id")
	}
	_, err := client.GL().AwardEmoji.DeleteSnippetAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("snippet_note_emoji_delete", err)
	}
	return nil
}

// Converters.

// toOutput converts the GitLab API response to the tool output format.
func toOutput(e *gl.AwardEmoji) Output {
	out := Output{
		ID:            e.ID,
		Name:          e.Name,
		UserID:        e.User.ID,
		Username:      e.User.Username,
		AwardableID:   e.AwardableID,
		AwardableType: e.AwardableType,
	}
	if e.CreatedAt != nil {
		out.CreatedAt = e.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// toListOutput converts the GitLab API response to the tool output format.
func toListOutput(emojis []*gl.AwardEmoji, resp *gl.Response) ListOutput {
	out := ListOutput{
		AwardEmoji: make([]Output, 0, len(emojis)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, e := range emojis {
		out.AwardEmoji = append(out.AwardEmoji, toOutput(e))
	}
	return out
}

// Formatters.
