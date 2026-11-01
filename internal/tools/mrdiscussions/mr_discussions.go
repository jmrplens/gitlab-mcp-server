// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package mrdiscussions

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// DiffLineRangeInput mirrors gl.LineRangeOptions: the start/end endpoints of a
// multi-line diff comment position.
type DiffLineRangeInput struct {
	Start *DiffLinePositionInput `json:"start,omitempty" jsonschema:"Start endpoint of a multi-line diff comment range."`
	End   *DiffLinePositionInput `json:"end,omitempty"   jsonschema:"End endpoint of a multi-line diff comment range."`
}

// DiffLinePositionInput mirrors gl.LinePositionOptions: one endpoint of a
// multi-line diff comment range.
type DiffLinePositionInput struct {
	LineCode string `json:"line_code,omitempty" jsonschema:"Line code identifying this line in the diff."`
	Type     string `json:"type,omitempty"      jsonschema:"Line type: new for a line added by the change, old otherwise. Omit for an unchanged context line."`
	OldLine  int    `json:"old_line,omitempty"  jsonschema:"Line number in the old file for this endpoint."`
	NewLine  int    `json:"new_line,omitempty"  jsonschema:"Line number in the new file for this endpoint."`
}

// DiffPosition defines the location of an inline diff comment. It mirrors
// gl.PositionOptions in full, supporting both text (line) positions and image
// (coordinate) positions.
//
// The four image fields are pointers, as gl.PositionOptions has them, because
// GitLab takes 0 as a value of each: an x or a y of 0 is a point on the
// image's edge, and a plain number could not tell it from a field left out,
// so the coordinate was dropped from the request.
type DiffPosition struct {
	BaseSHA      string              `json:"base_sha"  jsonschema:"Base commit SHA (merge-base),required"`
	StartSHA     string              `json:"start_sha" jsonschema:"SHA of the first commit in the MR,required"`
	HeadSHA      string              `json:"head_sha"  jsonschema:"HEAD commit SHA of the MR source branch,required"`
	OldPath      string              `json:"old_path,omitempty"  jsonschema:"File path before the change (for modified/deleted files). Required for position_type=file."`
	NewPath      string              `json:"new_path"            jsonschema:"File path after the change,required"`
	OldLine      int                 `json:"old_line,omitempty" jsonschema:"Line in old file. Set ONLY for removed lines. For modified or added lines use new_line instead. Set both old_line and new_line only for unchanged context lines."`
	NewLine      int                 `json:"new_line,omitempty" jsonschema:"Line in new file. Set ONLY for added or modified lines. For removed lines use old_line instead. Set both old_line and new_line only for unchanged context lines."`
	PositionType string              `json:"position_type,omitempty" jsonschema:"Position type: text for a line comment (default), image for an image coordinate comment, or file for a whole-file comment. An image position needs width, height, x and y, and a file position both old_path and new_path. Neither takes a line."`
	LineRange    *DiffLineRangeInput `json:"line_range,omitempty"    jsonschema:"Start/end line range for a multi-line text diff comment."`
	Width        *int64              `json:"width,omitempty"  jsonschema:"Image width in pixels. Required for position_type=image."`
	Height       *int64              `json:"height,omitempty" jsonschema:"Image height in pixels. Required for position_type=image."`
	X            *float64            `json:"x,omitempty"      jsonschema:"X coordinate of the comment on the image. Required for position_type=image."`
	Y            *float64            `json:"y,omitempty"      jsonschema:"Y coordinate of the comment on the image. Required for position_type=image."`
}

// CreateInput defines parameters for creating a discussion (inline or general).
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	Body      string               `json:"body"       jsonschema:"Discussion body,required"`
	Position  *DiffPosition        `json:"position,omitempty" jsonschema:"Diff position for inline comments. Omit for general MR discussions."`
	// CommitID anchors the discussion to a specific commit within the MR.
	CommitID string `json:"commit_id,omitempty" jsonschema:"SHA of the commit to anchor the discussion to (optional)."`
	// CreatedAt backdates the discussion's first note; requires admin or project/group owner rights (ISO 8601).
	CreatedAt string `json:"created_at,omitempty" jsonschema:"Backdate the discussion creation time (ISO 8601, e.g. 2025-01-01T00:00:00Z). Requires admin or owner rights"`
}

// NoteOutput is a note of a merge request discussion: a type of its own over
// [toolutil.DiscussionThreadNoteOutput], the rich note shape the four REST
// discussion packages share, whose fields and JSON tags (UpdatedAt with
// omitempty, Resolved and Resolvable without) are defined there. It is a
// defined type rather than an alias because the Markdown registry is keyed by
// type: while the four packages aliased one, the commit discussion card
// rendered every merge request thread and note, offering commit discussion
// actions as their next steps, and this package's cards were never served
// (issue 1291).
type NoteOutput toolutil.DiscussionThreadNoteOutput

// Output is a merge request discussion thread: a type of its own over
// [toolutil.DiscussionThreadOutput], the thread shape with full note
// payloads, for the reason [NoteOutput] gives.
type Output toolutil.DiscussionThreadOutput

// ResolveInput defines parameters for resolving/unresolving a discussion.
type ResolveInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"   jsonschema:"Project ID or URL-encoded path,required"`
	MRIID        int64                `json:"merge_request_iid"       jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"ID of the discussion to resolve,required"`
	Resolved     bool                 `json:"resolved"      jsonschema:"True to resolve, false to unresolve,required"`
}

// ReplyInput defines parameters for replying to an existing discussion.
type ReplyInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"    jsonschema:"Project ID or URL-encoded path,required"`
	MRIID        int64                `json:"merge_request_iid"        jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"ID of the discussion to reply to,required"`
	Body         string               `json:"body"          jsonschema:"Reply body,required"`
	// CreatedAt backdates the reply note; requires admin or project/group owner rights (ISO 8601).
	CreatedAt string `json:"created_at,omitempty" jsonschema:"Backdate the note creation time (ISO 8601, e.g. 2025-01-01T00:00:00Z). Requires admin or owner rights"`
}

// ListInput defines parameters for listing discussions.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	// OrderBy names the column used to order results.
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. created_at, updated_at)"`
	// Sort selects the sort direction for ordered results.
	Sort string `json:"sort,omitempty" jsonschema:"Sort direction (asc or desc)"`
	toolutil.PaginationInput
}

// ListOutput holds a list of discussions.
type ListOutput struct {
	toolutil.HintableOutput
	Discussions []Output                  `json:"discussions"`
	Pagination  toolutil.PaginationOutput `json:"pagination"`
}

// NoteToOutput converts a GitLab API [gl.Note], and what the captured
// response adds to it, to a [NoteOutput]: the shared conversion in
// [toolutil.DiscussionThreadNoteOutputFromGitLab], kept under the package's
// name for its callers and tests.
func NoteToOutput(n *gl.Note, extra toolutil.NoteExtra) NoteOutput {
	return NoteOutput(toolutil.DiscussionThreadNoteOutputFromGitLab(n, extra))
}

// ToOutput converts a GitLab API [gl.Discussion], and what the captured
// response adds to it, to an [Output], including all notes within the
// thread.
func ToOutput(d *gl.Discussion, extra toolutil.DiscussionExtra) Output {
	return Output(toolutil.DiscussionThreadOutputFromGitLab(d, extra))
}

// Create creates a new discussion on a merge request. When a
// [DiffPosition] is provided the discussion is attached as an inline comment
// on the specified file and line; otherwise a general discussion is created.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrDiscussionCreate: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrDiscussionCreate", "merge_request_iid")
	}
	// The timestamp is read before the position is checked against the merge
	// request's diff, so a value that does not parse is refused before any
	// request.
	createdAt, err := toolutil.ParseTimeField("created_at", input.CreatedAt)
	if err != nil {
		return Output{}, fmt.Errorf("mrDiscussionCreate: %w", err)
	}
	if input.Position != nil {
		if err = validatePosition(ctx, client, string(input.ProjectID), input.MRIID, input.Position); err != nil {
			return Output{}, fmt.Errorf("mrDiscussionCreate: %w", err)
		}
	}
	opts := &gl.CreateMergeRequestDiscussionOptions{
		Body:      new(toolutil.NormalizeText(input.Body)),
		CreatedAt: createdAt,
	}
	if input.CommitID != "" {
		opts.CommitID = new(input.CommitID)
	}
	if input.Position != nil {
		opts.Position = buildPositionOptions(input.Position)
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	d, _, err := client.GL().Discussions.CreateMergeRequestDiscussion(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("mrDiscussionCreate", err, http.StatusBadRequest,
			"for inline diff comments, position requires base_sha, head_sha and start_sha, and by position_type: text a valid old_path/new_path with line numbers, image width, height, x and y, file both old_path and new_path; use mr_review.changes_get to fetch the diff context")
	}
	extra, err := toolutil.CapturedDiscussion(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("mrDiscussionCreate", err)
	}
	return ToOutput(d, extra), nil
}

// Resolve resolves or unresolves a discussion thread on a merge
// request, depending on the Resolved flag in the input.
func Resolve(ctx context.Context, client *gitlabclient.Client, input ResolveInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrDiscussionResolve: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrDiscussionResolve", "merge_request_iid")
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	d, _, err := client.GL().Discussions.ResolveMergeRequestDiscussion(string(input.ProjectID), input.MRIID, input.DiscussionID, &gl.ResolveMergeRequestDiscussionOptions{
		Resolved: new(input.Resolved),
	}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("mrDiscussionResolve", err, http.StatusNotFound,
			"verify discussion_id with mr_review.discussion_list; only thread (resolvable) discussions can be resolved")
	}
	extra, err := toolutil.CapturedDiscussion(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("mrDiscussionResolve", err)
	}
	return ToOutput(d, extra), nil
}

// Reply adds a reply note to an existing discussion thread on a
// merge request. Returns the newly created note.
func Reply(ctx context.Context, client *gitlabclient.Client, input ReplyInput) (NoteOutput, error) {
	if err := ctx.Err(); err != nil {
		return NoteOutput{}, err
	}
	if input.ProjectID == "" {
		return NoteOutput{}, errors.New("mrDiscussionReply: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("mrDiscussionReply", "merge_request_iid")
	}
	createdAt, err := toolutil.ParseTimeField("created_at", input.CreatedAt)
	if err != nil {
		return NoteOutput{}, fmt.Errorf("mrDiscussionReply: %w", err)
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	n, _, err := client.GL().Discussions.AddMergeRequestDiscussionNote(string(input.ProjectID), input.MRIID, input.DiscussionID, &gl.AddMergeRequestDiscussionNoteOptions{
		Body:      new(toolutil.NormalizeText(input.Body)),
		CreatedAt: createdAt,
	}, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("mrDiscussionReply", err, http.StatusNotFound,
			"verify discussion_id with mr_review.discussion_list")
	}
	extra, err := toolutil.CapturedNote(captured)
	if err != nil {
		return NoteOutput{}, toolutil.WrapErr("mrDiscussionReply", err)
	}
	return NoteToOutput(n, extra), nil
}

// List returns a paginated list of discussion threads for a merge
// request, including all notes within each thread.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("mrDiscussionList: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("mrDiscussionList", "merge_request_iid")
	}
	opts := &gl.ListMergeRequestDiscussionsOptions{
		OrderBy: input.OrderBy, Sort: input.Sort,
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, toolutil.KeysetPaginationInput{})
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	discussions, resp, err := client.GL().Discussions.ListMergeRequestDiscussions(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("mrDiscussionList", err, http.StatusNotFound,
			"verify project_id and merge_request_iid with merge_request.get")
	}
	extras, err := toolutil.CapturedDiscussions(captured, len(discussions))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("mrDiscussionList", err)
	}
	return ListOutput{Discussions: toolutil.DiscussionThreadOutputsFromGitLab[Output](discussions, extras), Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetInput defines parameters for getting a single discussion.
type GetInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"    jsonschema:"Project ID or URL-encoded path,required"`
	MRIID        int64                `json:"merge_request_iid"        jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"ID of the discussion,required"`
}

// UpdateNoteInput defines parameters for updating a discussion note.
type UpdateNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"    jsonschema:"Project ID or URL-encoded path,required"`
	MRIID        int64                `json:"merge_request_iid"        jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"ID of the discussion containing the note,required"`
	NoteID       int64                `json:"note_id"       jsonschema:"ID of the note to update,required"`
	Body         string               `json:"body,omitempty"     jsonschema:"New body text (Markdown). Leave empty to keep current body."`
	Resolved     *bool                `json:"resolved,omitempty" jsonschema:"Set to true to resolve, false to unresolve. Omit to leave unchanged."`
	// CreatedAt overrides the note's creation timestamp; requires admin or project/group owner rights (ISO 8601).
	CreatedAt string `json:"created_at,omitempty" jsonschema:"Override the note creation time (ISO 8601, e.g. 2025-01-01T00:00:00Z). Requires admin or owner rights"`
}

// DeleteNoteInput defines parameters for deleting a discussion note.
type DeleteNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"    jsonschema:"Project ID or URL-encoded path,required"`
	MRIID        int64                `json:"merge_request_iid"        jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"ID of the discussion containing the note,required"`
	NoteID       int64                `json:"note_id"       jsonschema:"ID of the note to delete,required"`
}

// Get retrieves a single discussion thread from a merge request.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrDiscussionGet: project_id is required")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrDiscussionGet", "merge_request_iid")
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	d, _, err := client.GL().Discussions.GetMergeRequestDiscussion(string(input.ProjectID), input.MRIID, input.DiscussionID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("mrDiscussionGet", err, http.StatusNotFound,
			"verify discussion_id with mr_review.discussion_list")
	}
	extra, err := toolutil.CapturedDiscussion(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("mrDiscussionGet", err)
	}
	return ToOutput(d, extra), nil
}

// UpdateNote modifies an existing note within a discussion thread.
func UpdateNote(ctx context.Context, client *gitlabclient.Client, input UpdateNoteInput) (NoteOutput, error) {
	if err := ctx.Err(); err != nil {
		return NoteOutput{}, err
	}
	if input.ProjectID == "" {
		return NoteOutput{}, errors.New("mrDiscussionNoteUpdate: project_id is required")
	}
	if input.MRIID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("mrDiscussionNoteUpdate", "merge_request_iid")
	}
	if input.NoteID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("mrDiscussionNoteUpdate", "note_id")
	}
	createdAt, err := toolutil.ParseTimeField("created_at", input.CreatedAt)
	if err != nil {
		return NoteOutput{}, fmt.Errorf("mrDiscussionNoteUpdate: %w", err)
	}
	opts := &gl.UpdateMergeRequestDiscussionNoteOptions{
		CreatedAt: createdAt,
	}
	if input.Body != "" {
		opts.Body = new(toolutil.NormalizeText(input.Body))
	}
	if input.Resolved != nil {
		opts.Resolved = input.Resolved
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	n, _, err := client.GL().Discussions.UpdateMergeRequestDiscussionNote(string(input.ProjectID), input.MRIID, input.DiscussionID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return NoteOutput{}, toolutil.WrapErrWithHint("mrDiscussionNoteUpdate", err,
				"only the note author can edit body; only Maintainers can change the resolved flag on resolvable threads")
		}
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("mrDiscussionNoteUpdate", err, http.StatusNotFound,
			"verify note_id with mr_review.discussion_get")
	}
	extra, err := toolutil.CapturedNote(captured)
	if err != nil {
		return NoteOutput{}, toolutil.WrapErr("mrDiscussionNoteUpdate", err)
	}
	return NoteToOutput(n, extra), nil
}

// DeleteNote removes a note from a discussion thread.
func DeleteNote(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("mrDiscussionNoteDelete: project_id is required")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("mrDiscussionNoteDelete", "merge_request_iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("mrDiscussionNoteDelete", "note_id")
	}
	_, err := client.GL().Discussions.DeleteMergeRequestDiscussionNote(string(input.ProjectID), input.MRIID, input.DiscussionID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("mrDiscussionNoteDelete", err, http.StatusForbidden,
			"only the note author or a Maintainer can delete a discussion note")
	}
	return nil
}

// buildPositionOptions maps the full [DiffPosition] input onto the SDK's
// gl.PositionOptions, mirroring every field (text line positions, multi-line
// ranges, and image coordinate positions). When position_type is unset it
// defaults to "text" to preserve the historical line-comment behavior.
func buildPositionOptions(p *DiffPosition) *gl.PositionOptions {
	positionType := p.PositionType
	if positionType == "" {
		positionType = toolutil.PositionTypeText
	}
	pos := &gl.PositionOptions{
		BaseSHA:      new(p.BaseSHA),
		StartSHA:     new(p.StartSHA),
		HeadSHA:      new(p.HeadSHA),
		NewPath:      new(p.NewPath),
		PositionType: new(positionType),
	}
	if p.OldPath != "" {
		pos.OldPath = new(p.OldPath)
	}
	if p.NewLine != 0 {
		v := int64(p.NewLine)
		pos.NewLine = &v
	}
	if p.OldLine != 0 {
		v := int64(p.OldLine)
		pos.OldLine = &v
	}
	if p.LineRange != nil {
		pos.LineRange = buildLineRangeOptions(p.LineRange)
	}
	pos.Width, pos.Height, pos.X, pos.Y = p.Width, p.Height, p.X, p.Y
	return pos
}

// buildLineRangeOptions maps a [DiffLineRangeInput] onto gl.LineRangeOptions,
// returning nil when both endpoints are absent.
func buildLineRangeOptions(lr *DiffLineRangeInput) *gl.LineRangeOptions {
	start := buildLinePositionOptions(lr.Start)
	end := buildLinePositionOptions(lr.End)
	if start == nil && end == nil {
		return nil
	}
	return &gl.LineRangeOptions{Start: start, End: end}
}

// buildLinePositionOptions maps a [DiffLinePositionInput] onto
// gl.LinePositionOptions, returning nil when the endpoint is absent.
func buildLinePositionOptions(p *DiffLinePositionInput) *gl.LinePositionOptions {
	if p == nil {
		return nil
	}
	out := &gl.LinePositionOptions{}
	if p.LineCode != "" {
		out.LineCode = new(p.LineCode)
	}
	if p.Type != "" {
		out.Type = new(p.Type)
	}
	if p.OldLine != 0 {
		v := int64(p.OldLine)
		out.OldLine = &v
	}
	if p.NewLine != 0 {
		v := int64(p.NewLine)
		out.NewLine = &v
	}
	return out
}

// validatePosition judges a position by the fields GitLab reads for its
// position_type, before the write is sent. A text position, which is what an
// unnamed type is sent as, is judged against the lines of the merge
// request's diff by [validateTextPosition]. An image position is judged by
// its width, height, x and y, and a file position by its old and new paths,
// with no request, since GitLab places neither by a line of the diff. A
// position_type GitLab does not declare is left to GitLab, which refuses it
// by name.
func validatePosition(ctx context.Context, client *gitlabclient.Client, projectID string, mrIID int64, pos *DiffPosition) error {
	switch pos.PositionType {
	case "", toolutil.PositionTypeText:
		return validateTextPosition(ctx, client, projectID, mrIID, pos)
	case toolutil.PositionTypeImage:
		return toolutil.ValidateImagePosition(pos.Width, pos.Height, pos.X, pos.Y)
	case toolutil.PositionTypeFile:
		return toolutil.ValidateFilePosition(pos.OldPath, pos.NewPath)
	}
	return nil
}

// validateTextPosition fetches the MR diff and validates that the given
// position refers to a line actually present in the diff. This prevents
// 400/500 errors from GitLab when commenting on out-of-range lines and
// provides actionable error messages explaining what went wrong. When the
// diff cannot be listed, or cannot be shown to be the one the position's
// SHAs name, the check is skipped and the write that follows is judged by
// GitLab, for the reasons [toolutil.MergeRequestDiffForPositionCheck] gives.
func validateTextPosition(ctx context.Context, client *gitlabclient.Client, projectID string, mrIID int64, pos *DiffPosition) error {
	targetPath := pos.NewPath
	if targetPath == "" {
		targetPath = pos.OldPath
	}
	fileDiff, ok := toolutil.MergeRequestDiffForPositionCheck(ctx, client, projectID, mrIID, targetPath, gl.MergeRequestDiffRefs{
		BaseSha: pos.BaseSHA, StartSha: pos.StartSHA, HeadSha: pos.HeadSHA,
	})
	if !ok {
		return nil
	}
	if fileDiff == nil {
		return fmt.Errorf(
			"file %q is not in the merge request diff. Inline comments can only be placed on changed files. "+
				"Omit the position parameter to create a general (non-inline) discussion instead", targetPath,
		)
	}

	if fileDiff.Diff == "" {
		return nil
	}
	lines := toolutil.ParseDiffLines(fileDiff.Diff)
	return toolutil.ValidateDiffPosition(lines, pos.NewLine, pos.OldLine)
}

// Markdown Formatting.
