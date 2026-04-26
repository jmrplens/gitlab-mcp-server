// Package mrdraftnotes implements MCP tool handlers for GitLab merge request
// draft notes (pending review comments). It supports list, get, create, update,
// delete, publish, and publish-all operations via the MR DraftNotes API.
package mrdraftnotes

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------.

// ListInput defines parameters for listing draft notes in a merge request.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Order by: id (default)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort: asc or desc (default)"`
	toolutil.PaginationInput
}

// GetInput defines parameters for retrieving a single draft note.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
	NoteID    int64                `json:"note_id"    jsonschema:"Draft note ID,required"`
}

// DiffPosition defines the location of an inline diff comment on a merge
// request. Use this to place a draft note on a specific line in the diff.
type DiffPosition struct {
	BaseSHA  string `json:"base_sha"  jsonschema:"Base commit SHA (merge-base),required"`
	StartSHA string `json:"start_sha" jsonschema:"SHA of the first commit in the MR,required"`
	HeadSHA  string `json:"head_sha"  jsonschema:"HEAD commit SHA of the MR source branch,required"`
	OldPath  string `json:"old_path,omitempty"  jsonschema:"File path before the change (for modified/deleted files)"`
	NewPath  string `json:"new_path"            jsonschema:"File path after the change,required"`
	OldLine  int    `json:"old_line,omitempty" jsonschema:"Line in old file. Set ONLY for removed lines. For modified or added lines use new_line instead. Set both old_line and new_line only for unchanged context lines."`
	NewLine  int    `json:"new_line,omitempty" jsonschema:"Line in new file. Set ONLY for added or modified lines. For removed lines use old_line instead. Set both old_line and new_line only for unchanged context lines."`
}

// CreateInput defines parameters for creating a draft note.
type CreateInput struct {
	ProjectID             toolutil.StringOrInt `json:"project_id"                        jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                 int64                `json:"mr_iid"                            jsonschema:"Merge request internal ID,required"`
	Note                  string               `json:"note"                              jsonschema:"Note body text (Markdown),required"`
	CommitID              string               `json:"commit_id,omitempty"               jsonschema:"SHA of the commit to comment on"`
	InReplyToDiscussionID string               `json:"in_reply_to_discussion_id,omitempty" jsonschema:"Discussion ID to reply to"`
	ResolveDiscussion     *bool                `json:"resolve_discussion,omitempty"      jsonschema:"Resolve the discussion when published"`
	Position              *DiffPosition        `json:"position,omitempty"                jsonschema:"Diff position for inline comments on specific lines. Omit for general MR comments."`
}

// UpdateInput defines parameters for updating a draft note.
type UpdateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
	NoteID    int64                `json:"note_id"    jsonschema:"Draft note ID,required"`
	Note      string               `json:"note,omitempty" jsonschema:"Updated note body text (Markdown)"`
	Position  *DiffPosition        `json:"position,omitempty" jsonschema:"Updated diff position for inline comments"`
}

// DeleteInput defines parameters for deleting a draft note.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
	NoteID    int64                `json:"note_id"    jsonschema:"Draft note ID,required"`
}

// PublishInput defines parameters for publishing a single draft note.
type PublishInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
	NoteID    int64                `json:"note_id"    jsonschema:"Draft note ID,required"`
}

// PublishAllInput defines parameters for publishing all draft notes.
type PublishAllInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// PositionOutput represents the diff position of an inline draft note.
type PositionOutput struct {
	BaseSHA  string `json:"base_sha"`
	StartSHA string `json:"start_sha"`
	HeadSHA  string `json:"head_sha"`
	NewPath  string `json:"new_path,omitempty"`
	OldPath  string `json:"old_path,omitempty"`
	NewLine  int64  `json:"new_line,omitempty"`
	OldLine  int64  `json:"old_line,omitempty"`
}

// Output represents a single draft note.
type Output struct {
	toolutil.HintableOutput
	ID                int64           `json:"id"`
	AuthorID          int64           `json:"author_id"`
	MergeRequestID    int64           `json:"merge_request_id"`
	Note              string          `json:"note"`
	CommitID          string          `json:"commit_id,omitempty"`
	DiscussionID      string          `json:"discussion_id,omitempty"`
	ResolveDiscussion bool            `json:"resolve_discussion"`
	Position          *PositionOutput `json:"position,omitempty"`
}

// ListOutput holds a paginated list of draft notes.
type ListOutput struct {
	toolutil.HintableOutput
	DraftNotes []Output                  `json:"draft_notes"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// ToOutput converts a client-go DraftNote to the MCP output type.
func ToOutput(d *gl.DraftNote) Output {
	out := Output{
		ID:                d.ID,
		AuthorID:          d.AuthorID,
		MergeRequestID:    d.MergeRequestID,
		Note:              d.Note,
		CommitID:          d.CommitID,
		DiscussionID:      d.DiscussionID,
		ResolveDiscussion: d.ResolveDiscussion,
	}
	if d.Position != nil {
		out.Position = &PositionOutput{
			BaseSHA:  d.Position.BaseSHA,
			StartSHA: d.Position.StartSHA,
			HeadSHA:  d.Position.HeadSHA,
			NewPath:  d.Position.NewPath,
			OldPath:  d.Position.OldPath,
			NewLine:  d.Position.NewLine,
			OldLine:  d.Position.OldLine,
		}
	}
	return out
}

// toDiffPositionOptions converts a DiffPosition to the GitLab client PositionOptions.
func toDiffPositionOptions(p *DiffPosition) *gl.PositionOptions {
	pos := &gl.PositionOptions{
		BaseSHA:      new(p.BaseSHA),
		StartSHA:     new(p.StartSHA),
		HeadSHA:      new(p.HeadSHA),
		NewPath:      new(p.NewPath),
		PositionType: new("text"),
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
	return pos
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// List retrieves all draft notes for a merge request.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("draftNoteList: project_id is required")
	}
	if input.MRIID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("draftNoteList", "mr_iid")
	}
	opts := &gl.ListDraftNotesOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	notes, resp, err := client.GL().DraftNotes.ListDraftNotes(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("draftNoteList", err, http.StatusNotFound,
			"verify project_id and merge_request_iid with gitlab_merge_request_list; draft notes are visible only to their author until published")
	}
	out := ListOutput{
		DraftNotes: make([]Output, 0, len(notes)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, n := range notes {
		out.DraftNotes = append(out.DraftNotes, ToOutput(n))
	}
	return out, nil
}

// Get retrieves a single draft note by ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("draftNoteGet: project_id is required")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("draftNoteGet", "mr_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("draftNoteGet", "note_id")
	}
	note, _, err := client.GL().DraftNotes.GetDraftNote(string(input.ProjectID), input.MRIID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("draftNoteGet", err, http.StatusNotFound,
			"verify draft_note_id with gitlab_draft_note_list; draft notes are author-private until published")
	}
	return ToOutput(note), nil
}

// Create creates a new draft note on a merge request.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("draftNoteCreate: project_id is required")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("draftNoteCreate", "mr_iid")
	}
	if input.Note == "" {
		return Output{}, errors.New("draftNoteCreate: note is required")
	}
	if input.Position != nil {
		if err := validatePosition(ctx, client, string(input.ProjectID), input.MRIID, input.Position); err != nil {
			return Output{}, fmt.Errorf("draftNoteCreate: %w", err)
		}
	}
	opts := &gl.CreateDraftNoteOptions{
		Note: new(input.Note),
	}
	if input.CommitID != "" {
		opts.CommitID = new(input.CommitID)
	}
	if input.InReplyToDiscussionID != "" {
		opts.InReplyToDiscussionID = new(input.InReplyToDiscussionID)
	}
	if input.ResolveDiscussion != nil {
		opts.ResolveDiscussion = input.ResolveDiscussion
	}
	if input.Position != nil {
		opts.Position = toDiffPositionOptions(input.Position)
	}
	note, _, err := client.GL().DraftNotes.CreateDraftNote(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("draftNoteCreate", err, http.StatusBadRequest,
			"note body is required; for code-line comments include position with base_sha/start_sha/head_sha + new_path/old_path + line_code; in_reply_to_discussion_id requires existing discussion_id; resolve_discussion only valid on existing discussions")
	}
	return ToOutput(note), nil
}

// Update updates an existing draft note.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("draftNoteUpdate: project_id is required")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("draftNoteUpdate", "mr_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("draftNoteUpdate", "note_id")
	}
	if input.Position != nil {
		if err := validatePosition(ctx, client, string(input.ProjectID), input.MRIID, input.Position); err != nil {
			return Output{}, fmt.Errorf("draftNoteUpdate: %w", err)
		}
	}
	opts := &gl.UpdateDraftNoteOptions{}
	if input.Note != "" {
		opts.Note = new(input.Note)
	}
	if input.Position != nil {
		opts.Position = toDiffPositionOptions(input.Position)
	}
	note, _, err := client.GL().DraftNotes.UpdateDraftNote(string(input.ProjectID), input.MRIID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("draftNoteUpdate", err, http.StatusForbidden,
			"only the draft author can update; verify draft_note_id with gitlab_draft_note_list; published notes cannot be updated via this endpoint")
	}
	return ToOutput(note), nil
}

// Delete deletes a draft note from a merge request.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("draftNoteDelete: project_id is required")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("draftNoteDelete", "mr_iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("draftNoteDelete", "note_id")
	}
	_, err := client.GL().DraftNotes.DeleteDraftNote(string(input.ProjectID), input.MRIID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("draftNoteDelete", err, http.StatusForbidden,
			"only the draft author can delete; verify draft_note_id with gitlab_draft_note_list; published notes cannot be deleted via this endpoint")
	}
	return nil
}

// Publish publishes a single draft note, making it visible to all.
func Publish(ctx context.Context, client *gitlabclient.Client, input PublishInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("draftNotePublish: project_id is required")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("draftNotePublish", "mr_iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("draftNotePublish", "note_id")
	}
	_, err := client.GL().DraftNotes.PublishDraftNote(string(input.ProjectID), input.MRIID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("draftNotePublish", err, http.StatusForbidden,
			"only the draft author can publish; verify draft_note_id with gitlab_draft_note_list; once published the note becomes a regular MR note and cannot be unpublished")
	}
	return nil
}

// PublishAll publishes all pending draft notes on a merge request.
func PublishAll(ctx context.Context, client *gitlabclient.Client, input PublishAllInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("draftNotePublishAll: project_id is required")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("draftNotePublishAll", "mr_iid")
	}
	_, err := client.GL().DraftNotes.PublishAllDraftNotes(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("draftNotePublishAll", err, http.StatusForbidden,
			"publishes all current user's draft notes on the MR \u2014 cannot be undone; verify project_id + merge_request_iid; use gitlab_draft_note_list first to review pending drafts")
	}
	return nil
}

// validatePosition fetches the MR diff and validates that the given position
// refers to a line actually present in the diff. This prevents draft notes
// from being silently lost when published with out-of-range positions.
func validatePosition(ctx context.Context, client *gitlabclient.Client, projectID string, mrIID int64, pos *DiffPosition) error {
	diffs, _, err := client.GL().MergeRequests.ListMergeRequestDiffs(projectID, mrIID, &gl.ListMergeRequestDiffsOptions{
		ListOptions: gl.ListOptions{PerPage: 100},
	}, gl.WithContext(ctx))
	if err != nil {
		return nil //nolint:nilerr // Best-effort: if we can't fetch diffs, skip validation
	}

	targetPath := pos.NewPath
	if targetPath == "" {
		targetPath = pos.OldPath
	}
	var fileDiff string
	for _, d := range diffs {
		if d.NewPath == targetPath || d.OldPath == targetPath {
			fileDiff = d.Diff
			break
		}
	}
	if fileDiff == "" {
		return fmt.Errorf(
			"file %q is not in the merge request diff — inline comments can only be placed on changed files. "+
				"Omit the position parameter to create a general (non-inline) comment instead", targetPath)
	}

	lines := toolutil.ParseDiffLines(fileDiff)
	return toolutil.ValidateDiffPosition(lines, pos.NewLine, pos.OldLine)
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
