// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package commitdiscussions

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

// ListInput defines parameters for listing commit discussions. It mirrors
// gl.ListCommitDiscussionsOptions (which embeds gl.ListOptions), exposing
// its order_by and sort beside offset pagination (1:1 audit P3). It offers no
// keyset pagination, since GitLab pages this list by offset alone.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CommitSHA string               `json:"commit_sha" jsonschema:"Commit SHA,required"`
	// OrderBy names the column used to order results.
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. created_at, updated_at)"`
	// Sort selects the sort direction for ordered results.
	Sort string `json:"sort,omitempty" jsonschema:"Sort direction (asc or desc)"`
	toolutil.PaginationInput
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
	Type     string `json:"type,omitempty"      jsonschema:"Line type: new for a line added by the change, old otherwise. Omit for an unchanged context line."`
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
// (line) and file positions; gl.NotePosition has no image (width/height/x/y)
// fields, so none are surfaced here (1:1 audit: no invented fields), and an
// image position, which GitLab cannot complete without them, is refused
// before the request.
type PositionInput struct {
	BaseSHA      string          `json:"base_sha" jsonschema:"Base commit SHA,required"`
	StartSHA     string          `json:"start_sha" jsonschema:"Start commit SHA,required"`
	HeadSHA      string          `json:"head_sha" jsonschema:"Head commit SHA,required"`
	PositionType string          `json:"position_type" jsonschema:"Position type: text for a line comment, which needs new_line or old_line, or file for a whole-file comment, which needs both old_path and new_path and no line. GitLab also declares image, which needs width, height, x and y, and this action cannot send them,required"`
	NewPath      string          `json:"new_path,omitempty" jsonschema:"File path after change, from the file's entry in repository.commit_diff. Send it with old_path"`
	NewLine      int64           `json:"new_line,omitempty" jsonschema:"Line number after change"`
	OldPath      string          `json:"old_path,omitempty" jsonschema:"File path before change, from the file's entry in repository.commit_diff, which repeats new_path for a file that was not renamed. Send it with new_path"`
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
// applying offset pagination through toolutil.ApplyListOptions.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	opts := &gl.ListCommitDiscussionsOptions{
		OrderBy: input.OrderBy, Sort: input.Sort,
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, toolutil.KeysetPaginationInput{})
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	discussions, resp, err := client.GL().Discussions.ListCommitDiscussions(string(input.ProjectID), input.CommitSHA, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("commit_discussion_list", err, http.StatusNotFound,
			"verify project_id and commit_sha with repository.commit_get")
	}
	extras, err := toolutil.CapturedDiscussions(captured, len(discussions))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("commit_discussion_list", err)
	}
	return toListOutput(discussions, extras, resp), nil
}

// Get gets a single commit discussion.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	d, _, err := client.GL().Discussions.GetCommitDiscussion(string(input.ProjectID), input.CommitSHA, input.DiscussionID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("commit_discussion_get", err, http.StatusNotFound,
			"verify discussion_id with repository.commit_discussion_list")
	}
	extra, err := toolutil.CapturedDiscussion(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("commit_discussion_get", err)
	}
	return ToOutput(d, extra), nil
}

// Create creates a new commit discussion thread.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	createdAt, err := toolutil.ParseTimeField("created_at", input.CreatedAt)
	if err != nil {
		return Output{}, fmt.Errorf("commit_discussion_create: %w", err)
	}
	if input.Position != nil {
		if err = validatePosition(input.Position); err != nil {
			return Output{}, fmt.Errorf("commit_discussion_create: %w", err)
		}
	}
	opts := &gl.CreateCommitDiscussionOptions{
		Body:      new(toolutil.NormalizeText(input.Body)),
		CreatedAt: createdAt,
	}
	if input.Position != nil {
		opts.Position = buildNotePosition(input.Position)
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	d, _, err := client.GL().Discussions.CreateCommitDiscussion(string(input.ProjectID), input.CommitSHA, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("commit_discussion_create", err, http.StatusBadRequest,
			"for inline diff comments, position requires base_sha, start_sha and head_sha, and by position_type: text both old_path and new_path with new_line or old_line on a line of the commit's diff, file both old_path and new_path; verify the commit_sha exists in the project")
	}
	extra, err := toolutil.CapturedDiscussion(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("commit_discussion_create", err)
	}
	return ToOutput(d, extra), nil
}

// AddNote adds a note to an existing commit discussion.
func AddNote(ctx context.Context, client *gitlabclient.Client, input AddNoteInput) (NoteOutput, error) {
	if err := ctx.Err(); err != nil {
		return NoteOutput{}, err
	}
	createdAt, err := toolutil.ParseTimeField("created_at", input.CreatedAt)
	if err != nil {
		return NoteOutput{}, fmt.Errorf("commit_discussion_add_note: %w", err)
	}
	opts := &gl.AddCommitDiscussionNoteOptions{
		Body:      new(toolutil.NormalizeText(input.Body)),
		CreatedAt: createdAt,
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	note, _, err := client.GL().Discussions.AddCommitDiscussionNote(string(input.ProjectID), input.CommitSHA, input.DiscussionID, opts, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("commit_discussion_add_note", err, http.StatusNotFound,
			"verify discussion_id with repository.commit_discussion_list")
	}
	extra, err := toolutil.CapturedNote(captured)
	if err != nil {
		return NoteOutput{}, toolutil.WrapErr("commit_discussion_add_note", err)
	}
	return NoteToOutput(note, extra), nil
}

// UpdateNote updates an existing commit discussion note.
func UpdateNote(ctx context.Context, client *gitlabclient.Client, input UpdateNoteInput) (NoteOutput, error) {
	if err := ctx.Err(); err != nil {
		return NoteOutput{}, err
	}
	if input.NoteID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("commit_discussion_update_note", "note_id")
	}
	createdAt, err := toolutil.ParseTimeField("created_at", input.CreatedAt)
	if err != nil {
		return NoteOutput{}, fmt.Errorf("commit_discussion_update_note: %w", err)
	}
	opts := &gl.UpdateCommitDiscussionNoteOptions{
		Body:      new(toolutil.NormalizeText(input.Body)),
		CreatedAt: createdAt,
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	note, _, err := client.GL().Discussions.UpdateCommitDiscussionNote(string(input.ProjectID), input.CommitSHA, input.DiscussionID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("commit_discussion_update_note", err, http.StatusForbidden,
			"only the note author can edit a discussion note")
	}
	extra, err := toolutil.CapturedNote(captured)
	if err != nil {
		return NoteOutput{}, toolutil.WrapErr("commit_discussion_update_note", err)
	}
	return NoteToOutput(note, extra), nil
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

// toListOutput converts the GitLab API response, and what the captured
// answer adds to each discussion, to the tool output format.
func toListOutput(discussions []*gl.Discussion, extras []toolutil.DiscussionExtra, resp *gl.Response) ListOutput {
	return ListOutput{
		Discussions: toolutil.DiscussionThreadOutputsFromGitLab[Output](discussions, extras),
		Pagination:  toolutil.PaginationFromResponse(resp),
	}
}

// validatePosition judges a commit discussion's position by the fields GitLab
// reads for it, before the write is sent and with no request, since the
// commit's diff is not read here. Every position needs the start and head
// commits and a file path, which [toolutil.ValidatePositionAnchor] checks
// first; a text position, which is what an unnamed type is sent as, needs a
// line, and a file position both of its paths. A position_type GitLab does
// not declare is left to GitLab, which refuses it by name.
//
// An image position is refused. GitLab completes one only with its width,
// height, x and y (ImageFormatter#complete?), and this action cannot send
// them: the position client-go's CreateCommitDiscussionOptions carries is a
// gl.NotePosition, which models no image field. The gap is recorded in
// docs/development/upstream-bugs.md, and the refusal retires when the commit
// discussion's position carries the four fields.
//
// GitLab would otherwise refuse each of these with a 400 that names no field
// (DiffNote#positions_complete).
func validatePosition(pos *PositionInput) error {
	if err := toolutil.ValidatePositionAnchor(pos.StartSHA, pos.HeadSHA, pos.OldPath, pos.NewPath); err != nil {
		return err
	}
	switch pos.PositionType {
	case "", toolutil.PositionTypeText:
		return toolutil.ValidateTextPosition(pos.OldLine, pos.NewLine)
	case toolutil.PositionTypeImage:
		return errors.New(
			"an image position cannot be placed through repository.commit_discussion_create: GitLab places an image " +
				"comment by the image's width, height, x and y, and this action sends none of them; use position_type " +
				"file to comment on the image as a whole, or omit position for a general commit discussion",
		)
	case toolutil.PositionTypeFile:
		return toolutil.ValidateFilePosition(pos.OldPath, pos.NewPath)
	}
	return nil
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
