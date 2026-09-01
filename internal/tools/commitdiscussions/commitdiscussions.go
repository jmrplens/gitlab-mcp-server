package commitdiscussions

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Input types.

// ListInput defines parameters for listing commit discussions. It mirrors
// gl.ListCommitDiscussionsOptions (which embeds gl.ListOptions), exposing
// keyset pagination (order_by, sort, pagination, page_token) in addition to
// offset pagination (1:1 audit P3).
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CommitSHA string               `json:"commit_sha" jsonschema:"Commit SHA,required"`
	// OrderBy names the column used to order keyset-paginated results.
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. created_at, updated_at)"`
	// Sort selects the sort direction for ordered results.
	Sort string `json:"sort,omitempty" jsonschema:"Sort direction (asc or desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetInput defines parameters for getting a single commit discussion.
type GetInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CommitSHA    string               `json:"commit_sha" jsonschema:"Commit SHA,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
}

// LinePositionInput mirrors gl.LinePosition: one endpoint (start or end) of a
// multi-line diff comment range.
type LinePositionInput struct {
	LineCode string `json:"line_code,omitempty" jsonschema:"Line code identifying this line in the diff."`
	Type     string `json:"type,omitempty"      jsonschema:"Line type (new, old, or expanded)."`
	OldLine  int64  `json:"old_line,omitempty"  jsonschema:"Line number in the old file for this endpoint."`
	NewLine  int64  `json:"new_line,omitempty"  jsonschema:"Line number in the new file for this endpoint."`
}

// LineRangeInput mirrors gl.LineRange: the start/end endpoints of a multi-line
// diff comment position.
type LineRangeInput struct {
	Start *LinePositionInput `json:"start,omitempty" jsonschema:"Start endpoint of a multi-line diff comment range."`
	End   *LinePositionInput `json:"end,omitempty"   jsonschema:"End endpoint of a multi-line diff comment range."`
}

// PositionInput defines position attributes for inline commit discussions. It
// mirrors gl.NotePosition in full — the position struct accepted by
// gl.CreateCommitDiscussionOptions.Position. Commit-thread positions are text
// (line) positions only; gl.NotePosition has no image (width/height/x/y) fields,
// so none are surfaced here (1:1 audit: no invented fields).
type PositionInput struct {
	BaseSHA      string          `json:"base_sha" jsonschema:"Base commit SHA,required"`
	StartSHA     string          `json:"start_sha" jsonschema:"Start commit SHA,required"`
	HeadSHA      string          `json:"head_sha" jsonschema:"Head commit SHA,required"`
	PositionType string          `json:"position_type" jsonschema:"Position type (text or image),required"`
	NewPath      string          `json:"new_path,omitempty" jsonschema:"File path after change"`
	NewLine      int64           `json:"new_line,omitempty" jsonschema:"Line number after change"`
	OldPath      string          `json:"old_path,omitempty" jsonschema:"File path before change"`
	OldLine      int64           `json:"old_line,omitempty" jsonschema:"Line number before change"`
	LineRange    *LineRangeInput `json:"line_range,omitempty" jsonschema:"Start/end line range for a multi-line text diff comment."`
}

// CreateInput defines parameters for creating a commit discussion. It mirrors
// gl.CreateCommitDiscussionOptions (body, position, created_at).
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CommitSHA string               `json:"commit_sha" jsonschema:"Commit SHA,required"`
	Body      string               `json:"body" jsonschema:"Discussion body (Markdown supported),required"`
	Position  *PositionInput       `json:"position,omitempty" jsonschema:"Position for inline diff comments"`
	// CreatedAt backdates the discussion's first note; requires admin or project/group owner rights (ISO 8601).
	CreatedAt string `json:"created_at,omitempty" jsonschema:"Backdate the discussion creation time (ISO 8601, e.g. 2025-01-01T00:00:00Z). Requires admin or owner rights"`
}

// AddNoteInput defines parameters for adding a note to a commit discussion. It
// mirrors gl.AddCommitDiscussionNoteOptions (body, created_at).
type AddNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CommitSHA    string               `json:"commit_sha" jsonschema:"Commit SHA,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID to reply to,required"`
	Body         string               `json:"body" jsonschema:"Note body (Markdown supported),required"`
	// CreatedAt backdates the note; requires admin or project/group owner rights (ISO 8601).
	CreatedAt string `json:"created_at,omitempty" jsonschema:"Backdate the note creation time (ISO 8601, e.g. 2025-01-01T00:00:00Z). Requires admin or owner rights"`
}

// UpdateNoteInput defines parameters for updating a commit discussion note. It
// mirrors gl.UpdateCommitDiscussionNoteOptions (body, created_at).
type UpdateNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CommitSHA    string               `json:"commit_sha" jsonschema:"Commit SHA,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
	NoteID       int64                `json:"note_id" jsonschema:"Note ID to update,required"`
	Body         string               `json:"body" jsonschema:"Updated note body,required"`
	// CreatedAt overrides the note's creation timestamp; requires admin or project/group owner rights (ISO 8601).
	CreatedAt string `json:"created_at,omitempty" jsonschema:"Override the note creation time (ISO 8601, e.g. 2025-01-01T00:00:00Z). Requires admin or owner rights"`
}

// DeleteNoteInput defines parameters for deleting a commit discussion note.
type DeleteNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CommitSHA    string               `json:"commit_sha" jsonschema:"Commit SHA,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"Discussion ID,required"`
	NoteID       int64                `json:"note_id" jsonschema:"Note ID to delete,required"`
}

// Output types.

// ListOutput holds a list of commit discussions.
type ListOutput struct {
	toolutil.HintableOutput
	Discussions []Output                  `json:"discussions"`
	Pagination  toolutil.PaginationOutput `json:"pagination"`
}

// Handlers.

// List lists commit discussions. It mirrors gl.ListCommitDiscussionsOptions,
// applying offset and keyset pagination through toolutil.ApplyListOptions.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	opts := &gl.ListCommitDiscussionsOptions{
		OrderBy: input.OrderBy, Sort: input.Sort,
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	discussions, resp, err := client.GL().Discussions.ListCommitDiscussions(string(input.ProjectID), input.CommitSHA, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("commit_discussion_list", err, http.StatusNotFound,
			"verify project_id and commit_sha with gitlab_commit_get")
	}
	return toListOutput(discussions, resp), nil
}

// Get gets a single commit discussion.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	d, _, err := client.GL().Discussions.GetCommitDiscussion(string(input.ProjectID), input.CommitSHA, input.DiscussionID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("commit_discussion_get", err, http.StatusNotFound,
			"verify discussion_id with gitlab_list_commit_discussions")
	}
	return ToOutput(d), nil
}

// Create creates a new commit discussion thread.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	opts := &gl.CreateCommitDiscussionOptions{
		Body:      new(toolutil.NormalizeText(input.Body)),
		CreatedAt: toolutil.ParseOptionalTime(input.CreatedAt),
	}
	if input.Position != nil {
		opts.Position = buildNotePosition(input.Position)
	}
	d, _, err := client.GL().Discussions.CreateCommitDiscussion(string(input.ProjectID), input.CommitSHA, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("commit_discussion_create", err, http.StatusBadRequest,
			"for inline diff comments, position requires base_sha, head_sha, start_sha, position_type=text, and a valid old_path/new_path with line numbers; verify the commit_sha exists in the project")
	}
	return ToOutput(d), nil
}

// AddNote adds a note to an existing commit discussion.
func AddNote(ctx context.Context, client *gitlabclient.Client, input AddNoteInput) (NoteOutput, error) {
	if err := ctx.Err(); err != nil {
		return NoteOutput{}, err
	}
	opts := &gl.AddCommitDiscussionNoteOptions{
		Body:      new(toolutil.NormalizeText(input.Body)),
		CreatedAt: toolutil.ParseOptionalTime(input.CreatedAt),
	}
	note, _, err := client.GL().Discussions.AddCommitDiscussionNote(string(input.ProjectID), input.CommitSHA, input.DiscussionID, opts, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("commit_discussion_add_note", err, http.StatusNotFound,
			"verify discussion_id with gitlab_list_commit_discussions")
	}
	return NoteToOutput(note), nil
}

// UpdateNote updates an existing commit discussion note.
func UpdateNote(ctx context.Context, client *gitlabclient.Client, input UpdateNoteInput) (NoteOutput, error) {
	if err := ctx.Err(); err != nil {
		return NoteOutput{}, err
	}
	if input.NoteID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("commit_discussion_update_note", "note_id")
	}
	opts := &gl.UpdateCommitDiscussionNoteOptions{
		Body:      new(toolutil.NormalizeText(input.Body)),
		CreatedAt: toolutil.ParseOptionalTime(input.CreatedAt),
	}
	note, _, err := client.GL().Discussions.UpdateCommitDiscussionNote(string(input.ProjectID), input.CommitSHA, input.DiscussionID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("commit_discussion_update_note", err, http.StatusForbidden,
			"only the note author can edit a discussion note")
	}
	return NoteToOutput(note), nil
}

// DeleteNote deletes a commit discussion note.
func DeleteNote(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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

// toListOutput converts the GitLab API response to the tool output format.
func toListOutput(discussions []*gl.Discussion, resp *gl.Response) ListOutput {
	out := make([]Output, len(discussions))
	for i, d := range discussions {
		out[i] = ToOutput(d)
	}
	return ListOutput{
		Discussions: out,
		Pagination:  toolutil.PaginationFromResponse(resp),
	}
}

// buildNotePosition maps the full [PositionInput] onto the SDK's
// gl.NotePosition, mirroring every field (base/start/head SHAs, position type,
// old/new path and line, and a multi-line line range). When position_type is
// unset it defaults to "text" to preserve line-comment behavior.
func buildNotePosition(p *PositionInput) *gl.NotePosition {
	positionType := p.PositionType
	if positionType == "" {
		positionType = "text"
	}
	return &gl.NotePosition{
		BaseSHA:      p.BaseSHA,
		StartSHA:     p.StartSHA,
		HeadSHA:      p.HeadSHA,
		PositionType: positionType,
		NewPath:      p.NewPath,
		NewLine:      p.NewLine,
		OldPath:      p.OldPath,
		OldLine:      p.OldLine,
		LineRange:    buildLineRange(p.LineRange),
	}
}

// buildLineRange maps a [LineRangeInput] onto gl.LineRange, returning nil when
// both endpoints are absent.
func buildLineRange(lr *LineRangeInput) *gl.LineRange {
	if lr == nil {
		return nil
	}
	start := buildLinePosition(lr.Start)
	end := buildLinePosition(lr.End)
	if start == nil && end == nil {
		return nil
	}
	return &gl.LineRange{StartRange: start, EndRange: end}
}

// buildLinePosition maps a [LinePositionInput] onto gl.LinePosition, returning
// nil when the endpoint is absent.
func buildLinePosition(p *LinePositionInput) *gl.LinePosition {
	if p == nil {
		return nil
	}
	return &gl.LinePosition{
		LineCode: p.LineCode,
		Type:     p.Type,
		OldLine:  p.OldLine,
		NewLine:  p.NewLine,
	}
}

// Formatters.
