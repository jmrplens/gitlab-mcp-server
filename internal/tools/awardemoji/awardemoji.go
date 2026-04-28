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

// hintEmojiOwnerOnly is the hint shared by all emoji delete handlers.
const hintEmojiOwnerOnly = "only the user who awarded the emoji can remove it"

// Issue Input types.

// IssueListInput is the input for listing award emoji on an issue.
type IssueListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"issue_iid" jsonschema:"Issue IID (project-scoped internal ID),required"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination (default 1)"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page (default 20, max 100)"`
}

// IssueListOnNoteInput is the input for listing award emoji on an issue note.
type IssueListOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"issue_iid" jsonschema:"Issue IID (project-scoped internal ID),required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination (default 1)"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page (default 20, max 100)"`
}

// IssueGetInput is the input for getting a single award emoji on an issue.
type IssueGetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"issue_iid" jsonschema:"Issue IID (project-scoped internal ID),required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// IssueGetOnNoteInput is the input for getting an award emoji on an issue note.
type IssueGetOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"issue_iid" jsonschema:"Issue IID (project-scoped internal ID),required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// IssueCreateInput is the input for creating an award emoji on an issue.
type IssueCreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"issue_iid" jsonschema:"Issue IID (project-scoped internal ID),required"`
	Name      string               `json:"name" jsonschema:"Emoji name without colons (e.g. thumbsup),required"`
}

// IssueCreateOnNoteInput is the input for creating an award emoji on an issue note.
type IssueCreateOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"issue_iid" jsonschema:"Issue IID (project-scoped internal ID),required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	Name      string               `json:"name" jsonschema:"Emoji name without colons (e.g. thumbsup),required"`
}

// IssueDeleteInput is the input for deleting an award emoji from an issue.
type IssueDeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"issue_iid" jsonschema:"Issue IID (project-scoped internal ID),required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// IssueDeleteOnNoteInput is the input for deleting an award emoji from an issue note.
type IssueDeleteOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"issue_iid" jsonschema:"Issue IID (project-scoped internal ID),required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// MR Input types.

// MRListInput is the input for listing award emoji on a merge request.
type MRListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"mr_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination (default 1)"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page (default 20, max 100)"`
}

// MRListOnNoteInput is the input for listing award emoji on a merge request note.
type MRListOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"mr_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination (default 1)"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page (default 20, max 100)"`
}

// MRGetInput is the input for getting a single award emoji on a merge request.
type MRGetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"mr_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// MRGetOnNoteInput is the input for getting an award emoji on a merge request note.
type MRGetOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"mr_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// MRCreateInput is the input for creating an award emoji on a merge request.
type MRCreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"mr_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	Name      string               `json:"name" jsonschema:"Emoji name without colons (e.g. thumbsup),required"`
}

// MRCreateOnNoteInput is the input for creating an award emoji on a merge request note.
type MRCreateOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"mr_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	Name      string               `json:"name" jsonschema:"Emoji name without colons (e.g. thumbsup),required"`
}

// MRDeleteInput is the input for deleting an award emoji from a merge request.
type MRDeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"mr_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// MRDeleteOnNoteInput is the input for deleting an award emoji from a merge request note.
type MRDeleteOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"mr_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// Snippet Input types.

// SnippetListInput is the input for listing award emoji on a project snippet.
type SnippetListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination (default 1)"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page (default 20, max 100)"`
}

// SnippetListOnNoteInput is the input for listing award emoji on a snippet note.
type SnippetListOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination (default 1)"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page (default 20, max 100)"`
}

// SnippetGetInput is the input for getting a single award emoji on a snippet.
type SnippetGetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// SnippetGetOnNoteInput is the input for getting an award emoji on a snippet note.
type SnippetGetOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// SnippetCreateInput is the input for creating an award emoji on a snippet.
type SnippetCreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	Name      string               `json:"name" jsonschema:"Emoji name without colons (e.g. thumbsup),required"`
}

// SnippetCreateOnNoteInput is the input for creating an award emoji on a snippet note.
type SnippetCreateOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	Name      string               `json:"name" jsonschema:"Emoji name without colons (e.g. thumbsup),required"`
}

// SnippetDeleteInput is the input for deleting an award emoji from a snippet.
type SnippetDeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// SnippetDeleteOnNoteInput is the input for deleting an award emoji from a snippet note.
type SnippetDeleteOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
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
func ListIssueAwardEmoji(ctx context.Context, client *gitlabclient.Client, input IssueListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("issue_emoji_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("issue_emoji_list", "issue_iid")
	}
	opts := &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage}}
	emojis, resp, err := client.GL().AwardEmoji.ListIssueAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("issue_emoji_list", err, 404,
			"verify the issue exists with gitlab_issue_get (correct project_id and issue_iid)")
	}
	return toListOutput(emojis, resp), nil
}

// GetIssueAwardEmoji gets a single award emoji on an issue.
func GetIssueAwardEmoji(ctx context.Context, client *gitlabclient.Client, input IssueGetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("issue_emoji_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_emoji_get", "issue_iid")
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
func CreateIssueAwardEmoji(ctx context.Context, client *gitlabclient.Client, input IssueCreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("issue_emoji_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_emoji_create", "issue_iid")
	}
	opts := &gl.CreateAwardEmojiOptions{Name: input.Name}
	emoji, _, err := client.GL().AwardEmoji.CreateIssueAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("issue_emoji_create", err, 404,
			"verify the issue exists with gitlab_issue_get; emoji name must be a valid GitLab shortname without colons (e.g. \"thumbsup\")")
	}
	return toOutput(emoji), nil
}

// DeleteIssueAwardEmoji deletes an award emoji from an issue.
func DeleteIssueAwardEmoji(ctx context.Context, client *gitlabclient.Client, input IssueDeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("issue_emoji_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("issue_emoji_delete", "issue_iid")
	}
	if input.AwardID <= 0 {
		return toolutil.ErrRequiredInt64("issue_emoji_delete", "award_id")
	}
	_, err := client.GL().AwardEmoji.DeleteIssueAwardEmoji(string(input.ProjectID), input.IID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 403) {
			return toolutil.WrapErrWithHint("issue_emoji_delete", err, hintEmojiOwnerOnly)
		}
		if toolutil.IsHTTPStatus(err, 404) {
			return toolutil.WrapErrWithHint("issue_emoji_delete", err, "award already removed or never existed \u2014 list awards with gitlab_issue_emoji_list to verify award_id")
		}
		return toolutil.WrapErrWithMessage("issue_emoji_delete", err)
	}
	return nil
}

// Issue Note Award Emoji Handlers.

// ListIssueNoteAwardEmoji lists all award emoji on an issue note.
func ListIssueNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input IssueListOnNoteInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("issue_note_emoji_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("issue_note_emoji_list", "issue_iid")
	}
	if input.NoteID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("issue_note_emoji_list", "note_id")
	}
	opts := &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage}}
	emojis, resp, err := client.GL().AwardEmoji.ListIssuesAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("issue_note_emoji_list", err, 404,
			"verify the issue and note exist with gitlab_issue_note_get (correct project_id, issue_iid, note_id)")
	}
	return toListOutput(emojis, resp), nil
}

// GetIssueNoteAwardEmoji gets a single award emoji on an issue note.
func GetIssueNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input IssueGetOnNoteInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("issue_note_emoji_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_note_emoji_get", "issue_iid")
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
func CreateIssueNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input IssueCreateOnNoteInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("issue_note_emoji_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_note_emoji_create", "issue_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issue_note_emoji_create", "note_id")
	}
	opts := &gl.CreateAwardEmojiOptions{Name: input.Name}
	emoji, _, err := client.GL().AwardEmoji.CreateIssuesAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("issue_note_emoji_create", err, 404,
			"verify the issue note exists with gitlab_issue_note_get; emoji name must be a valid shortname without colons (e.g. \"thumbsup\")")
	}
	return toOutput(emoji), nil
}

// DeleteIssueNoteAwardEmoji deletes an award emoji from an issue note.
func DeleteIssueNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input IssueDeleteOnNoteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("issue_note_emoji_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("issue_note_emoji_delete", "issue_iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("issue_note_emoji_delete", "note_id")
	}
	if input.AwardID <= 0 {
		return toolutil.ErrRequiredInt64("issue_note_emoji_delete", "award_id")
	}
	_, err := client.GL().AwardEmoji.DeleteIssuesAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 403) {
			return toolutil.WrapErrWithHint("issue_note_emoji_delete", err, hintEmojiOwnerOnly)
		}
		if toolutil.IsHTTPStatus(err, 404) {
			return toolutil.WrapErrWithHint("issue_note_emoji_delete", err, "award already removed or never existed \u2014 list awards with gitlab_issue_note_emoji_list to verify award_id")
		}
		return toolutil.WrapErrWithMessage("issue_note_emoji_delete", err)
	}
	return nil
}

// MR Award Emoji Handlers.

// ListMRAwardEmoji lists all award emoji on a merge request.
func ListMRAwardEmoji(ctx context.Context, client *gitlabclient.Client, input MRListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("mr_emoji_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("mr_emoji_list", "mr_iid")
	}
	opts := &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage}}
	emojis, resp, err := client.GL().AwardEmoji.ListMergeRequestAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("mr_emoji_list", err, 404,
			"verify the merge request exists with gitlab_mr_get (correct project_id and mr_iid)")
	}
	return toListOutput(emojis, resp), nil
}

// GetMRAwardEmoji gets a single award emoji on a merge request.
func GetMRAwardEmoji(ctx context.Context, client *gitlabclient.Client, input MRGetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("mr_emoji_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_emoji_get", "mr_iid")
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
func CreateMRAwardEmoji(ctx context.Context, client *gitlabclient.Client, input MRCreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("mr_emoji_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_emoji_create", "mr_iid")
	}
	opts := &gl.CreateAwardEmojiOptions{Name: input.Name}
	emoji, _, err := client.GL().AwardEmoji.CreateMergeRequestAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("mr_emoji_create", err, 404,
			"verify the merge request exists with gitlab_mr_get; emoji name must be a valid shortname without colons (e.g. \"thumbsup\")")
	}
	return toOutput(emoji), nil
}

// DeleteMRAwardEmoji deletes an award emoji from a merge request.
func DeleteMRAwardEmoji(ctx context.Context, client *gitlabclient.Client, input MRDeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("mr_emoji_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("mr_emoji_delete", "mr_iid")
	}
	if input.AwardID <= 0 {
		return toolutil.ErrRequiredInt64("mr_emoji_delete", "award_id")
	}
	_, err := client.GL().AwardEmoji.DeleteMergeRequestAwardEmoji(string(input.ProjectID), input.IID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 403) {
			return toolutil.WrapErrWithHint("mr_emoji_delete", err, hintEmojiOwnerOnly)
		}
		if toolutil.IsHTTPStatus(err, 404) {
			return toolutil.WrapErrWithHint("mr_emoji_delete", err, "award already removed or never existed \u2014 list awards with gitlab_mr_emoji_list to verify award_id")
		}
		return toolutil.WrapErrWithMessage("mr_emoji_delete", err)
	}
	return nil
}

// MR Note Award Emoji Handlers.

// ListMRNoteAwardEmoji lists all award emoji on a merge request note.
func ListMRNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input MRListOnNoteInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("mr_note_emoji_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("mr_note_emoji_list", "mr_iid")
	}
	if input.NoteID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("mr_note_emoji_list", "note_id")
	}
	opts := &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage}}
	emojis, resp, err := client.GL().AwardEmoji.ListMergeRequestAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("mr_note_emoji_list", err, 404,
			"verify the MR and note exist with gitlab_mr_note_get (correct project_id, mr_iid, note_id)")
	}
	return toListOutput(emojis, resp), nil
}

// GetMRNoteAwardEmoji gets a single award emoji on a merge request note.
func GetMRNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input MRGetOnNoteInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("mr_note_emoji_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_note_emoji_get", "mr_iid")
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
func CreateMRNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input MRCreateOnNoteInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("mr_note_emoji_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_note_emoji_create", "mr_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_note_emoji_create", "note_id")
	}
	opts := &gl.CreateAwardEmojiOptions{Name: input.Name}
	emoji, _, err := client.GL().AwardEmoji.CreateMergeRequestAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("mr_note_emoji_create", err, 404,
			"verify the MR note exists with gitlab_mr_note_get; emoji name must be a valid shortname without colons (e.g. \"thumbsup\")")
	}
	return toOutput(emoji), nil
}

// DeleteMRNoteAwardEmoji deletes an award emoji from a merge request note.
func DeleteMRNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input MRDeleteOnNoteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("mr_note_emoji_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("mr_note_emoji_delete", "mr_iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("mr_note_emoji_delete", "note_id")
	}
	if input.AwardID <= 0 {
		return toolutil.ErrRequiredInt64("mr_note_emoji_delete", "award_id")
	}
	_, err := client.GL().AwardEmoji.DeleteMergeRequestAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 403) {
			return toolutil.WrapErrWithHint("mr_note_emoji_delete", err, hintEmojiOwnerOnly)
		}
		if toolutil.IsHTTPStatus(err, 404) {
			return toolutil.WrapErrWithHint("mr_note_emoji_delete", err, "award already removed or never existed \u2014 list awards with gitlab_mr_note_emoji_list to verify award_id")
		}
		return toolutil.WrapErrWithMessage("mr_note_emoji_delete", err)
	}
	return nil
}

// Snippet Award Emoji Handlers.

// ListSnippetAwardEmoji lists all award emoji on a snippet.
func ListSnippetAwardEmoji(ctx context.Context, client *gitlabclient.Client, input SnippetListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("snippet_emoji_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("snippet_emoji_list", "snippet_id")
	}
	opts := &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage}}
	emojis, resp, err := client.GL().AwardEmoji.ListSnippetAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("snippet_emoji_list", err, 404,
			"verify the snippet exists with gitlab_project_snippet_get (correct project_id and snippet_id)")
	}
	return toListOutput(emojis, resp), nil
}

// GetSnippetAwardEmoji gets a single award emoji on a snippet.
func GetSnippetAwardEmoji(ctx context.Context, client *gitlabclient.Client, input SnippetGetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("snippet_emoji_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_emoji_get", "snippet_id")
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
func CreateSnippetAwardEmoji(ctx context.Context, client *gitlabclient.Client, input SnippetCreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("snippet_emoji_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_emoji_create", "snippet_id")
	}
	opts := &gl.CreateAwardEmojiOptions{Name: input.Name}
	emoji, _, err := client.GL().AwardEmoji.CreateSnippetAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("snippet_emoji_create", err, 404,
			"verify the snippet exists with gitlab_project_snippet_get; emoji name must be a valid shortname without colons (e.g. \"thumbsup\")")
	}
	return toOutput(emoji), nil
}

// DeleteSnippetAwardEmoji deletes an award emoji from a snippet.
func DeleteSnippetAwardEmoji(ctx context.Context, client *gitlabclient.Client, input SnippetDeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("snippet_emoji_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("snippet_emoji_delete", "snippet_id")
	}
	if input.AwardID <= 0 {
		return toolutil.ErrRequiredInt64("snippet_emoji_delete", "award_id")
	}
	_, err := client.GL().AwardEmoji.DeleteSnippetAwardEmoji(string(input.ProjectID), input.IID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 403) {
			return toolutil.WrapErrWithHint("snippet_emoji_delete", err, hintEmojiOwnerOnly)
		}
		if toolutil.IsHTTPStatus(err, 404) {
			return toolutil.WrapErrWithHint("snippet_emoji_delete", err, "award already removed or never existed \u2014 list awards with gitlab_snippet_emoji_list to verify award_id")
		}
		return toolutil.WrapErrWithMessage("snippet_emoji_delete", err)
	}
	return nil
}

// Snippet Note Award Emoji Handlers.

// ListSnippetNoteAwardEmoji lists all award emoji on a snippet note.
func ListSnippetNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input SnippetListOnNoteInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("snippet_note_emoji_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("snippet_note_emoji_list", "snippet_id")
	}
	if input.NoteID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("snippet_note_emoji_list", "note_id")
	}
	opts := &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage}}
	emojis, resp, err := client.GL().AwardEmoji.ListSnippetAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("snippet_note_emoji_list", err, 404,
			"verify the snippet and note exist with gitlab_snippet_note_get (correct project_id, snippet_id, note_id)")
	}
	return toListOutput(emojis, resp), nil
}

// GetSnippetNoteAwardEmoji gets a single award emoji on a snippet note.
func GetSnippetNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input SnippetGetOnNoteInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("snippet_note_emoji_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_note_emoji_get", "snippet_id")
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
func CreateSnippetNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input SnippetCreateOnNoteInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("snippet_note_emoji_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_note_emoji_create", "snippet_id")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("snippet_note_emoji_create", "note_id")
	}
	opts := &gl.CreateAwardEmojiOptions{Name: input.Name}
	emoji, _, err := client.GL().AwardEmoji.CreateSnippetAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("snippet_note_emoji_create", err, 404,
			"verify the snippet note exists with gitlab_snippet_note_get; emoji name must be a valid shortname without colons (e.g. \"thumbsup\")")
	}
	return toOutput(emoji), nil
}

// DeleteSnippetNoteAwardEmoji deletes an award emoji from a snippet note.
func DeleteSnippetNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input SnippetDeleteOnNoteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("snippet_note_emoji_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("snippet_note_emoji_delete", "snippet_id")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("snippet_note_emoji_delete", "note_id")
	}
	if input.AwardID <= 0 {
		return toolutil.ErrRequiredInt64("snippet_note_emoji_delete", "award_id")
	}
	_, err := client.GL().AwardEmoji.DeleteSnippetAwardEmojiOnNote(string(input.ProjectID), input.IID, input.NoteID, input.AwardID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 403) {
			return toolutil.WrapErrWithHint("snippet_note_emoji_delete", err, hintEmojiOwnerOnly)
		}
		if toolutil.IsHTTPStatus(err, 404) {
			return toolutil.WrapErrWithHint("snippet_note_emoji_delete", err, "award already removed or never existed \u2014 list awards with gitlab_snippet_note_emoji_list to verify award_id")
		}
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
