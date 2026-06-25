package awardemoji

import (
	"context"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// hintEmojiOwnerOnly is the hint shared by all emoji delete handlers.
const hintEmojiOwnerOnly = "only the user who awarded the emoji can remove it"

// emojiListQuery captures the shared list-query parameters (offset/keyset
// pagination plus order_by/sort) mirrored from gl.ListAwardEmojiOptions so each
// award emoji list handler builds the underlying gl.ListOptions once. OrderBy
// and Sort are applied after ApplyListOptions so an explicit value always wins.
type emojiListQuery struct {
	pagination toolutil.PaginationInput
	keyset     toolutil.KeysetPaginationInput
	orderBy    string
	sort       string
}

// applyTo populates a gl.ListAwardEmojiOptions from the captured input: offset
// and keyset pagination via ApplyListOptions, then order_by/sort when supplied.
func (q emojiListQuery) applyTo() *gl.ListAwardEmojiOptions {
	opts := &gl.ListAwardEmojiOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, q.pagination, q.keyset)
	if q.orderBy != "" {
		opts.OrderBy = q.orderBy
	}
	if q.sort != "" {
		opts.Sort = q.sort
	}
	return opts
}

// Issue Input types.

// IssueListInput is the input for listing award emoji on an issue.
type IssueListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"issue_iid" jsonschema:"Issue IID (project-scoped internal ID),required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. created_at, updated_at, id). Used with keyset pagination."`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort order: 'asc' or 'desc'."`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// IssueListOnNoteInput is the input for listing award emoji on an issue note.
type IssueListOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"issue_iid" jsonschema:"Issue IID (project-scoped internal ID),required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. created_at, updated_at, id). Used with keyset pagination."`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort order: 'asc' or 'desc'."`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	IID       int64                `json:"merge_request_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. created_at, updated_at, id). Used with keyset pagination."`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort order: 'asc' or 'desc'."`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// MRListOnNoteInput is the input for listing award emoji on a merge request note.
type MRListOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"merge_request_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. created_at, updated_at, id). Used with keyset pagination."`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort order: 'asc' or 'desc'."`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// MRGetInput is the input for getting a single award emoji on a merge request.
type MRGetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"merge_request_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// MRGetOnNoteInput is the input for getting an award emoji on a merge request note.
type MRGetOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"merge_request_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// MRCreateInput is the input for creating an award emoji on a merge request.
type MRCreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"merge_request_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	Name      string               `json:"name" jsonschema:"Emoji name without colons (e.g. thumbsup),required"`
}

// MRCreateOnNoteInput is the input for creating an award emoji on a merge request note.
type MRCreateOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"merge_request_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	Name      string               `json:"name" jsonschema:"Emoji name without colons (e.g. thumbsup),required"`
}

// MRDeleteInput is the input for deleting an award emoji from a merge request.
type MRDeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"merge_request_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// MRDeleteOnNoteInput is the input for deleting an award emoji from a merge request note.
type MRDeleteOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"merge_request_iid" jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	AwardID   int64                `json:"award_id" jsonschema:"Award emoji ID,required"`
}

// Snippet Input types.

// SnippetListInput is the input for listing award emoji on a project snippet.
type SnippetListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. created_at, updated_at, id). Used with keyset pagination."`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort order: 'asc' or 'desc'."`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// SnippetListOnNoteInput is the input for listing award emoji on a snippet note.
type SnippetListOnNoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IID       int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	NoteID    int64                `json:"note_id" jsonschema:"Note ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. created_at, updated_at, id). Used with keyset pagination."`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort order: 'asc' or 'desc'."`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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

// UserOutput mirrors gl.BasicUser (the AwardEmoji.User field): the user who
// awarded the emoji. Per the 1:1 audit policy (full nested objects) it surfaces
// every field of the SDK struct and is replicated here rather than imported
// from a sibling package to preserve the zero-import-cycle constraint
// (C-IMPORTS).
type UserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// Output represents a single award emoji.
type Output struct {
	toolutil.HintableOutput
	ID            int64       `json:"id"`
	Name          string      `json:"name"`
	User          *UserOutput `json:"user,omitempty"`
	CreatedAt     string      `json:"created_at,omitempty"`
	UpdatedAt     string      `json:"updated_at,omitempty"`
	AwardableID   int64       `json:"awardable_id"`
	AwardableType string      `json:"awardable_type"`
}

// ListOutput holds a paginated list of award emoji.
type ListOutput struct {
	toolutil.HintableOutput
	AwardEmoji []Output                  `json:"award_emoji"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

type noteEmojiRequest struct {
	ProjectID toolutil.StringOrInt
	IID       int64
	NoteID    int64
	AwardID   int64
	Name      string
	IIDField  string
	Operation string
}

func validateNoteEmojiRequest(req noteEmojiRequest, requireAward bool) error {
	if req.ProjectID == "" {
		return toolutil.WrapErrWithMessage(req.Operation, toolutil.ErrFieldRequired("project_id"))
	}
	if req.IID <= 0 {
		return toolutil.ErrRequiredInt64(req.Operation, req.IIDField)
	}
	if req.NoteID <= 0 {
		return toolutil.ErrRequiredInt64(req.Operation, "note_id")
	}
	if requireAward && req.AwardID <= 0 {
		return toolutil.ErrRequiredInt64(req.Operation, "award_id")
	}
	return nil
}

func createNoteAwardEmoji(ctx context.Context, req noteEmojiRequest, notFoundHint string, create func(any, int64, int64, *gl.CreateAwardEmojiOptions, ...gl.RequestOptionFunc) (*gl.AwardEmoji, *gl.Response, error)) (Output, error) {
	if err := validateNoteEmojiRequest(req, false); err != nil {
		return Output{}, err
	}
	emoji, _, err := create(string(req.ProjectID), req.IID, req.NoteID, &gl.CreateAwardEmojiOptions{Name: req.Name}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint(req.Operation, err, 404, notFoundHint)
	}
	return toOutput(emoji), nil
}

func deleteNoteAwardEmoji(ctx context.Context, req noteEmojiRequest, listToolHint string, remove func(any, int64, int64, int64, ...gl.RequestOptionFunc) (*gl.Response, error)) error {
	if err := validateNoteEmojiRequest(req, true); err != nil {
		return err
	}
	_, err := remove(string(req.ProjectID), req.IID, req.NoteID, req.AwardID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 403) {
			return toolutil.WrapErrWithHint(req.Operation, err, hintEmojiOwnerOnly)
		}
		if toolutil.IsHTTPStatus(err, 404) {
			return toolutil.WrapErrWithHint(req.Operation, err, "award already removed or never existed - list awards with "+listToolHint+" to verify award_id")
		}
		return toolutil.WrapErrWithMessage(req.Operation, err)
	}
	return nil
}

func listNoteAwardEmoji(ctx context.Context, req noteEmojiRequest, query emojiListQuery, notFoundHint string, list func(any, int64, int64, *gl.ListAwardEmojiOptions, ...gl.RequestOptionFunc) ([]*gl.AwardEmoji, *gl.Response, error)) (ListOutput, error) {
	if err := validateNoteEmojiRequest(req, false); err != nil {
		return ListOutput{}, err
	}
	emojis, resp, err := list(string(req.ProjectID), req.IID, req.NoteID, query.applyTo(), gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint(req.Operation, err, 404, notFoundHint)
	}
	return toListOutput(emojis, resp), nil
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
	opts := emojiListQuery{pagination: input.PaginationInput, keyset: input.KeysetPaginationInput, orderBy: input.OrderBy, sort: input.Sort}.applyTo()
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
	return listNoteAwardEmoji(ctx, noteEmojiRequest{ProjectID: input.ProjectID, IID: input.IID, NoteID: input.NoteID, IIDField: "issue_iid", Operation: "issue_note_emoji_list"},
		emojiListQuery{pagination: input.PaginationInput, keyset: input.KeysetPaginationInput, orderBy: input.OrderBy, sort: input.Sort},
		"verify the issue and note exist with gitlab_issue_note_get (correct project_id, issue_iid, note_id)", client.GL().AwardEmoji.ListIssuesAwardEmojiOnNote)
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
	return createNoteAwardEmoji(ctx, noteEmojiRequest{ProjectID: input.ProjectID, IID: input.IID, NoteID: input.NoteID, Name: input.Name, IIDField: "issue_iid", Operation: "issue_note_emoji_create"},
		"verify the issue note exists with gitlab_issue_note_get; emoji name must be a valid shortname without colons (e.g. \"thumbsup\")",
		client.GL().AwardEmoji.CreateIssuesAwardEmojiOnNote)
}

// DeleteIssueNoteAwardEmoji deletes an award emoji from an issue note.
func DeleteIssueNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input IssueDeleteOnNoteInput) error {
	return deleteNoteAwardEmoji(ctx, noteEmojiRequest{ProjectID: input.ProjectID, IID: input.IID, NoteID: input.NoteID, AwardID: input.AwardID, IIDField: "issue_iid", Operation: "issue_note_emoji_delete"},
		"gitlab_issue_note_emoji_list", client.GL().AwardEmoji.DeleteIssuesAwardEmojiOnNote)
}

// MR Award Emoji Handlers.

// ListMRAwardEmoji lists all award emoji on a merge request.
func ListMRAwardEmoji(ctx context.Context, client *gitlabclient.Client, input MRListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("mr_emoji_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("mr_emoji_list", "merge_request_iid")
	}
	opts := emojiListQuery{pagination: input.PaginationInput, keyset: input.KeysetPaginationInput, orderBy: input.OrderBy, sort: input.Sort}.applyTo()
	emojis, resp, err := client.GL().AwardEmoji.ListMergeRequestAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("mr_emoji_list", err, 404,
			"verify the merge request exists with gitlab_mr_get (correct project_id and merge_request_iid)")
	}
	return toListOutput(emojis, resp), nil
}

// GetMRAwardEmoji gets a single award emoji on a merge request.
func GetMRAwardEmoji(ctx context.Context, client *gitlabclient.Client, input MRGetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("mr_emoji_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_emoji_get", "merge_request_iid")
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
		return Output{}, toolutil.ErrRequiredInt64("mr_emoji_create", "merge_request_iid")
	}
	opts := &gl.CreateAwardEmojiOptions{Name: input.Name}
	emoji, _, err := client.GL().AwardEmoji.CreateMergeRequestAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 404) || isDuplicateAwardEmojiError(err) {
			if existing, ok := findExistingMRAwardEmoji(ctx, client, input); ok {
				return existing, nil
			}
		}
		return Output{}, toolutil.WrapErrWithStatusHint("mr_emoji_create", err, 404,
			"verify the merge request exists with gitlab_mr_get; emoji name must be a valid shortname without colons (e.g. \"thumbsup\")")
	}
	return toOutput(emoji), nil
}

func isDuplicateAwardEmojiError(err error) bool {
	return toolutil.ContainsAny(err, "award emoji name has already been taken", "name has already been taken")
}

func findExistingMRAwardEmoji(ctx context.Context, client *gitlabclient.Client, input MRCreateInput) (Output, bool) {
	currentUser, _, err := client.GL().Users.CurrentUser(gl.WithContext(ctx))
	if err != nil {
		return Output{}, false
	}
	opts := &gl.ListAwardEmojiOptions{ListOptions: gl.ListOptions{Page: 1, PerPage: 100}}
	for {
		emojis, resp, listErr := client.GL().AwardEmoji.ListMergeRequestAwardEmoji(string(input.ProjectID), input.IID, opts, gl.WithContext(ctx))
		if listErr != nil {
			return Output{}, false
		}
		for _, emoji := range emojis {
			if strings.EqualFold(emoji.Name, input.Name) && emoji.User.ID == currentUser.ID {
				return toOutput(emoji), true
			}
		}
		if resp == nil || resp.NextPage == 0 {
			return Output{}, false
		}
		opts.Page = resp.NextPage
	}
}

// DeleteMRAwardEmoji deletes an award emoji from a merge request.
func DeleteMRAwardEmoji(ctx context.Context, client *gitlabclient.Client, input MRDeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("mr_emoji_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("mr_emoji_delete", "merge_request_iid")
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
	return listNoteAwardEmoji(ctx, noteEmojiRequest{ProjectID: input.ProjectID, IID: input.IID, NoteID: input.NoteID, IIDField: "merge_request_iid", Operation: "mr_note_emoji_list"},
		emojiListQuery{pagination: input.PaginationInput, keyset: input.KeysetPaginationInput, orderBy: input.OrderBy, sort: input.Sort},
		"verify the MR and note exist with gitlab_mr_note_get (correct project_id, merge_request_iid, note_id)", client.GL().AwardEmoji.ListMergeRequestAwardEmojiOnNote)
}

// GetMRNoteAwardEmoji gets a single award emoji on a merge request note.
func GetMRNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input MRGetOnNoteInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("mr_note_emoji_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mr_note_emoji_get", "merge_request_iid")
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
	return createNoteAwardEmoji(ctx, noteEmojiRequest{ProjectID: input.ProjectID, IID: input.IID, NoteID: input.NoteID, Name: input.Name, IIDField: "merge_request_iid", Operation: "mr_note_emoji_create"},
		"verify the MR note exists with gitlab_mr_note_get; emoji name must be a valid shortname without colons (e.g. \"thumbsup\")",
		client.GL().AwardEmoji.CreateMergeRequestAwardEmojiOnNote)
}

// DeleteMRNoteAwardEmoji deletes an award emoji from a merge request note.
func DeleteMRNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input MRDeleteOnNoteInput) error {
	return deleteNoteAwardEmoji(ctx, noteEmojiRequest{ProjectID: input.ProjectID, IID: input.IID, NoteID: input.NoteID, AwardID: input.AwardID, IIDField: "merge_request_iid", Operation: "mr_note_emoji_delete"},
		"gitlab_mr_note_emoji_list", client.GL().AwardEmoji.DeleteMergeRequestAwardEmojiOnNote)
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
	opts := emojiListQuery{pagination: input.PaginationInput, keyset: input.KeysetPaginationInput, orderBy: input.OrderBy, sort: input.Sort}.applyTo()
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
	return listNoteAwardEmoji(ctx, noteEmojiRequest{ProjectID: input.ProjectID, IID: input.IID, NoteID: input.NoteID, IIDField: "snippet_id", Operation: "snippet_note_emoji_list"},
		emojiListQuery{pagination: input.PaginationInput, keyset: input.KeysetPaginationInput, orderBy: input.OrderBy, sort: input.Sort},
		"verify the snippet and note exist with gitlab_snippet_note_get (correct project_id, snippet_id, note_id)", client.GL().AwardEmoji.ListSnippetAwardEmojiOnNote)
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
	return createNoteAwardEmoji(ctx, noteEmojiRequest{ProjectID: input.ProjectID, IID: input.IID, NoteID: input.NoteID, Name: input.Name, IIDField: "snippet_id", Operation: "snippet_note_emoji_create"},
		"verify the snippet note exists with gitlab_snippet_note_get; emoji name must be a valid shortname without colons (e.g. \"thumbsup\")",
		client.GL().AwardEmoji.CreateSnippetAwardEmojiOnNote)
}

// DeleteSnippetNoteAwardEmoji deletes an award emoji from a snippet note.
func DeleteSnippetNoteAwardEmoji(ctx context.Context, client *gitlabclient.Client, input SnippetDeleteOnNoteInput) error {
	return deleteNoteAwardEmoji(ctx, noteEmojiRequest{ProjectID: input.ProjectID, IID: input.IID, NoteID: input.NoteID, AwardID: input.AwardID, IIDField: "snippet_id", Operation: "snippet_note_emoji_delete"},
		"gitlab_snippet_note_emoji_list", client.GL().AwardEmoji.DeleteSnippetAwardEmojiOnNote)
}

// Converters.

// userOutput converts a gl.BasicUser value (AwardEmoji.User) into the additive
// user object. The user is always present on an award emoji, so this returns a
// pointer to a populated value (never nil) to keep the canonical `user` key
// stable.
func userOutput(u gl.BasicUser) *UserOutput {
	return &UserOutput{
		ID:        u.ID,
		Username:  u.Username,
		Name:      u.Name,
		State:     u.State,
		CreatedAt: formatTimePtr(u.CreatedAt),
		AvatarURL: u.AvatarURL,
		WebURL:    u.WebURL,
	}
}

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// toOutput converts the GitLab API response to the tool output format.
func toOutput(e *gl.AwardEmoji) Output {
	return Output{
		ID:            e.ID,
		Name:          e.Name,
		User:          userOutput(e.User),
		CreatedAt:     formatTimePtr(e.CreatedAt),
		UpdatedAt:     formatTimePtr(e.UpdatedAt),
		AwardableID:   e.AwardableID,
		AwardableType: e.AwardableType,
	}
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
