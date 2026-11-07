// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package mrdraftnotes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------.

// ListInput defines parameters for listing draft notes in a merge request.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Order by: id (default)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort: asc or desc (default)"`
	toolutil.PaginationInput
}

// GetInput defines parameters for retrieving a single draft note.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
	NoteID    int64                `json:"note_id"    jsonschema:"Draft note ID,required"`
}

// DiffPosition defines the location of an inline diff comment on a merge
// request. Use this to place a draft note on a specific line in the diff.
// It mirrors the GitLab client-go PositionOptions (discussions.go) one-to-one.
//
// The four image fields are pointers, as PositionOptions has them, because
// GitLab takes 0 as a value of each: an x or a y of 0 is a point on the
// image's edge, and a plain number could not tell it from a field left out,
// so the coordinate was dropped from the request. x and y are integers, as
// GitLab declares them, where PositionOptions types them as floats: Grape
// coerced a fraction to its whole part without a word, so 12.5 placed the
// note at 12. [toolutil.ImageCoordinate] converts them for the request.
type DiffPosition struct {
	BaseSHA      string         `json:"base_sha"  jsonschema:"Base commit SHA (merge-base),required"`
	StartSHA     string         `json:"start_sha" jsonschema:"SHA of the first commit in the MR,required"`
	HeadSHA      string         `json:"head_sha"  jsonschema:"HEAD commit SHA of the MR source branch,required"`
	OldPath      string         `json:"old_path,omitempty"  jsonschema:"File path before the change (for modified/deleted files). Required for position_type 'file'."`
	NewPath      string         `json:"new_path,omitempty"  jsonschema:"File path after the change. Required for position_type 'file'."`
	PositionType string         `json:"position_type,omitempty" jsonschema:"Position type: 'text' for a line comment (default), 'image' for an image coordinate comment, or 'file' for a whole-file comment. An image position needs width, height, x and y, and a file position both old_path and new_path. Neither takes a line."`
	OldLine      int            `json:"old_line,omitempty" jsonschema:"Line in old file. Set ONLY for removed lines. For modified or added lines use new_line instead. Set both old_line and new_line only for unchanged context lines."`
	NewLine      int            `json:"new_line,omitempty" jsonschema:"Line in new file. Set ONLY for added or modified lines. For removed lines use old_line instead. Set both old_line and new_line only for unchanged context lines."`
	LineRange    *DiffLineRange `json:"line_range,omitempty" jsonschema:"Line range for multi-line text comments. Set start and end line positions to span more than one line."`
	Width        *int64         `json:"width,omitempty" jsonschema:"Image width in pixels. Required for position_type 'image', and only for it."`
	Height       *int64         `json:"height,omitempty" jsonschema:"Image height in pixels. Required for position_type 'image', and only for it."`
	X            *int64         `json:"x,omitempty" jsonschema:"X coordinate of the comment on an image diff, in whole pixels. Required for position_type 'image', and only for it."`
	Y            *int64         `json:"y,omitempty" jsonschema:"Y coordinate of the comment on an image diff, in whole pixels. Required for position_type 'image', and only for it."`
}

// DiffLineRange describes the start and end line positions of a multi-line
// inline draft note. It mirrors the GitLab client-go LineRangeOptions.
type DiffLineRange struct {
	Start *DiffLinePosition `json:"start,omitempty" jsonschema:"Start line position of the range."`
	End   *DiffLinePosition `json:"end,omitempty"   jsonschema:"End line position of the range."`
}

// DiffLinePosition describes a single line position within a line range. It
// mirrors the GitLab client-go LinePositionOptions.
type DiffLinePosition struct {
	LineCode string `json:"line_code,omitempty" jsonschema:"Line code identifying the diff line (sha_oldline_newline)."`
	Type     string `json:"type,omitempty"      jsonschema:"Line type: 'new', 'old', or empty for unchanged context lines."`
	OldLine  int    `json:"old_line,omitempty"  jsonschema:"Line number in the old file for this range boundary."`
	NewLine  int    `json:"new_line,omitempty"  jsonschema:"Line number in the new file for this range boundary."`
}

// CreateInput defines parameters for creating a draft note.
type CreateInput struct {
	ProjectID             toolutil.StringOrInt `json:"project_id"                        jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                 int64                `json:"merge_request_iid"                            jsonschema:"Merge request internal ID,required"`
	Note                  string               `json:"note"                              jsonschema:"Note body text (Markdown),required"`
	CommitID              string               `json:"commit_id,omitempty"               jsonschema:"SHA of the commit to comment on"`
	InReplyToDiscussionID string               `json:"in_reply_to_discussion_id,omitempty" jsonschema:"Discussion ID to reply to"`
	ResolveDiscussion     *bool                `json:"resolve_discussion,omitempty"      jsonschema:"Resolve the discussion when published"`
	Position              *DiffPosition        `json:"position,omitempty"                jsonschema:"Diff position for inline comments on specific lines. Omit for general MR comments."`
}

// UpdateInput defines parameters for updating a draft note. It takes a new
// body, a new position or both, and keeps whichever of the two it is not
// given ([Update]).
type UpdateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
	NoteID    int64                `json:"note_id"    jsonschema:"Draft note ID,required"`
	Note      string               `json:"note,omitempty" jsonschema:"Updated note body text (Markdown). Omit it to keep the current body. Give note, position or both"`
	Position  *DiffPosition        `json:"position,omitempty" jsonschema:"Updated diff position for inline comments, replacing the current one whole. It needs base_sha, start_sha and head_sha from the merge request's diff_refs and the file's new_path or old_path, and by position_type a new_line or old_line (text, the default), width, height, x and y (image) or both old_path and new_path (file). Omit it to keep the current position. Give note, position or both"`
}

// DeleteInput defines parameters for deleting a draft note.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
	NoteID    int64                `json:"note_id"    jsonschema:"Draft note ID,required"`
}

// PublishInput defines parameters for publishing a single draft note.
type PublishInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
	NoteID    int64                `json:"note_id"    jsonschema:"Draft note ID,required"`
}

// PublishAllInput defines parameters for publishing all draft notes. The
// three optional fields finish the review in the same call: a summary note
// posted after the drafts, whether that note is internal, and the review state
// the caller records as a reviewer.
type PublishAllInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID         int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
	Note          string               `json:"note,omitempty" jsonschema:"Summary comment GitLab posts on the merge request after publishing the drafts. Markdown is supported"`
	Internal      *bool                `json:"internal,omitempty" jsonschema:"Whether the summary note is internal, visible only to members who can see internal notes. Defaults to false and applies only together with note"`
	ReviewerState string               `json:"reviewer_state,omitempty" jsonschema:"Review state to record for the caller as a reviewer after publishing: requested_changes or reviewed. It does not record a formal approval"`
}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// PositionOutput represents the diff position of an inline draft note. It
// mirrors the GitLab client-go NotePosition (notes.go) one-to-one.
type PositionOutput struct {
	BaseSHA      string                    `json:"base_sha"`
	StartSHA     string                    `json:"start_sha"`
	HeadSHA      string                    `json:"head_sha"`
	PositionType string                    `json:"position_type,omitempty"`
	NewPath      string                    `json:"new_path,omitempty"`
	OldPath      string                    `json:"old_path,omitempty"`
	NewLine      int64                     `json:"new_line,omitempty"`
	OldLine      int64                     `json:"old_line,omitempty"`
	LineRange    *toolutil.LineRangeOutput `json:"line_range,omitempty"`
}

// Output represents a single draft note.
type Output struct {
	toolutil.HintableOutput
	ID                int64           `json:"id"`
	AuthorID          int64           `json:"author_id"`
	MergeRequestID    int64           `json:"merge_request_id"`
	Note              string          `json:"note"`
	CommitID          string          `json:"commit_id,omitempty"`
	LineCode          string          `json:"line_code,omitempty"`
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
		LineCode:          d.LineCode,
		DiscussionID:      d.DiscussionID,
		ResolveDiscussion: d.ResolveDiscussion,
	}
	if d.Position != nil {
		out.Position = &PositionOutput{
			BaseSHA:      d.Position.BaseSHA,
			StartSHA:     d.Position.StartSHA,
			HeadSHA:      d.Position.HeadSHA,
			PositionType: d.Position.PositionType,
			NewPath:      d.Position.NewPath,
			OldPath:      d.Position.OldPath,
			NewLine:      d.Position.NewLine,
			OldLine:      d.Position.OldLine,
		}
		if lr := d.Position.LineRange; lr != nil {
			out.Position.LineRange = &toolutil.LineRangeOutput{
				Start: toolutil.NewLinePositionOutput(lr.StartRange),
				End:   toolutil.NewLinePositionOutput(lr.EndRange),
			}
		}
	}
	return out
}

// toDiffPositionOptions converts a DiffPosition to the GitLab client
// PositionOptions, mirroring every field one-to-one. A new_path left out or
// blank is not sent, for the reason [toolutil.PositionNewPath] gives.
func toDiffPositionOptions(p *DiffPosition) *gl.PositionOptions {
	positionType := p.PositionType
	if positionType == "" {
		positionType = toolutil.PositionTypeText
	}
	pos := &gl.PositionOptions{
		BaseSHA:      new(p.BaseSHA),
		StartSHA:     new(p.StartSHA),
		HeadSHA:      new(p.HeadSHA),
		NewPath:      toolutil.PositionNewPath(p.NewPath),
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
	pos.Width, pos.Height = p.Width, p.Height
	pos.X, pos.Y = toolutil.ImageCoordinate(p.X), toolutil.ImageCoordinate(p.Y)
	if lr := p.LineRange; lr != nil {
		pos.LineRange = &gl.LineRangeOptions{
			Start: toLinePositionOptions(lr.Start),
			End:   toLinePositionOptions(lr.End),
		}
	}
	return pos
}

// toLinePositionOptions converts a DiffLinePosition to the GitLab client
// LinePositionOptions, mirroring every field one-to-one.
func toLinePositionOptions(p *DiffLinePosition) *gl.LinePositionOptions {
	if p == nil {
		return nil
	}
	opt := &gl.LinePositionOptions{}
	if p.LineCode != "" {
		opt.LineCode = new(p.LineCode)
	}
	if p.Type != "" {
		opt.Type = new(p.Type)
	}
	if p.OldLine != 0 {
		v := int64(p.OldLine)
		opt.OldLine = &v
	}
	if p.NewLine != 0 {
		v := int64(p.NewLine)
		opt.NewLine = &v
	}
	return opt
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
		return ListOutput{}, toolutil.ErrRequiredInt64("draftNoteList", "merge_request_iid")
	}
	opts := &gl.ListDraftNotesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, toolutil.KeysetPaginationInput{})
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	notes, resp, err := client.GL().DraftNotes.ListDraftNotes(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("draftNoteList", err, http.StatusNotFound,
			"verify project_id and merge_request_iid with merge_request.list; draft notes are visible only to their author until published")
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
		return Output{}, toolutil.ErrRequiredInt64("draftNoteGet", "merge_request_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("draftNoteGet", "note_id")
	}
	note, _, err := client.GL().DraftNotes.GetDraftNote(string(input.ProjectID), input.MRIID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("draftNoteGet", err, http.StatusNotFound,
			"verify draft_note_id with mr_review.draft_note_list; draft notes are author-private until published")
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
		return Output{}, toolutil.ErrRequiredInt64("draftNoteCreate", "merge_request_iid")
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
			"note body is required; an inline position needs base_sha, start_sha and head_sha, and by position_type: text a new_path/old_path with new_line/old_line, image width, height, x and y, file both old_path and new_path; in_reply_to_discussion_id requires existing discussion_id; resolve_discussion only valid on existing discussions")
	}
	return ToOutput(note), nil
}

// The first release whose draft note update keeps a position the request
// leaves out: gitlab-org/gitlab!258695 made the route set the position only
// when one is sent, and it shipped in 19.5.0. docs/development/upstream-bugs.md
// records what an older release does.
const (
	keepsPositionSinceMajor = 19
	keepsPositionSinceMinor = 5
)

// Update changes a pending draft note's body, its position or both.
//
// GitLab's update route writes both at once and asks for the body every
// time: it refuses a request without one with a 400, "Missing params to
// modify", although it declares the body optional, and a release
// before 19.5 stores a position the request leaves out as none, so the
// draft loses its line and is published as a general comment. So a position
// sent alone goes with the draft's current body, and a body sent alone, to
// an instance not known to run 19.5 or later, goes with the draft's current
// position, both read from the draft first ([currentDraft]). A call naming
// neither has nothing to change and is refused before any request.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("draftNoteUpdate: project_id is required")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("draftNoteUpdate", "merge_request_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("draftNoteUpdate", "note_id")
	}
	if input.Note == "" && input.Position == nil {
		return Output{}, errors.New("draftNoteUpdate: note or position is required. Give the new body, the new position or both, and the draft keeps the one left out")
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
	keepPosition := opts.Position == nil && !client.AtLeast(ctx, keepsPositionSinceMajor, keepsPositionSinceMinor)
	if opts.Note == nil || keepPosition {
		//gitlab:request optional: the draft is read only for what the update leaves out, its body when only a position is sent, and its position when only a body is sent to an instance not known to run 19.5 or later
		current, err := currentDraft(ctx, client, input, keepPosition)
		if err != nil {
			return Output{}, err
		}
		if opts.Note == nil {
			opts.Note = new(current.note)
		}
		if keepPosition {
			opts.Position = current.position
		}
	}
	note, _, err := client.GL().DraftNotes.UpdateDraftNote(string(input.ProjectID), input.MRIID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("draftNoteUpdate", err, http.StatusForbidden,
			"only the draft author can update; verify draft_note_id with mr_review.draft_note_list; published notes cannot be updated via this endpoint")
	}
	return ToOutput(note), nil
}

// capturedDraft is what an update decodes off the captured answer to its read
// of the draft: the position, whole and as GitLab wrote it, which
// [currentDraft] reads as a [capturedPosition] when it is sent back. It is
// kept raw here so the capture holds the one key client-go's DraftNote also
// decodes, the position, rather than the keys inside it.
type capturedDraft struct {
	Position json.RawMessage `json:"position"`
}

// capturedPosition is a stored position as GitLab answers it: every key the
// update sends back, read into the options it is sent in, except an image's
// width, height, x and y, which are read as numbers of either JSON form.
// GitLab's position schema (app/validators/json_schemas/position.json) takes
// each of the four as an integer or as a string of up to ten characters, the
// web UI's draft route stores the position JSON it is given with its types
// (DiffPositionableNote#position=), and
// Gitlab::Diff::Formatters::ImageFormatter answers each as stored, so a draft
// can come back with "width":"64". The options type all four as numbers.
// The route declares the four Integer (lib/api/draft_notes.rb) and coerces a
// string to one, so the number sent back is what the string would have been
// stored as.
type capturedPosition struct {
	gl.PositionOptions
	Width  *json.Number `json:"width"`
	Height *json.Number `json:"height"`
	X      *json.Number `json:"x"`
	Y      *json.Number `json:"y"`
}

// options returns the stored position as the options the update sends it
// back in, with the width and height read as whole numbers and x and y as the
// floats the options carry. A measure that is not a number of its kind, a
// width of 64.5 for one, is an error, since the route would refuse it.
func (p *capturedPosition) options() (*gl.PositionOptions, error) {
	opts := p.PositionOptions
	err := errors.Join(
		storedMeasure(&opts.Width, "width", p.Width, json.Number.Int64),
		storedMeasure(&opts.Height, "height", p.Height, json.Number.Int64),
		storedMeasure(&opts.X, "x", p.X, json.Number.Float64),
		storedMeasure(&opts.Y, "y", p.Y, json.Number.Float64),
	)
	if err != nil {
		return nil, err
	}
	return &opts, nil
}

// storedMeasure reads one of a stored image position's measures into *dst
// with parse, and leaves *dst nil when GitLab answered the key as null or
// left it out, so that key is not sent back.
func storedMeasure[T any](dst **T, name string, n *json.Number, parse func(json.Number) (T, error)) error {
	if n == nil {
		return nil
	}
	v, err := parse(*n)
	if err != nil {
		return fmt.Errorf("%s %s: %w", name, n.String(), err)
	}
	*dst = &v
	return nil
}

// storedDraft is what an update reads of the draft it changes: the body, and
// the position as the options the update sends it back in, nil when the
// draft has none GitLab places on the diff.
type storedDraft struct {
	note     string
	position *gl.PositionOptions
}

// currentDraft reads the draft an update names, for the body or the position
// the update leaves out ([Update]). withPosition says whether the position is
// sent back. When it is not, the update sends a position of its own, and the
// stored one is not decoded at all, so a stored position this server could
// not send back does not stop a call that replaces it.
//
// The position is decoded from the captured response ([capturedPosition])
// into the PositionOptions the update sends, whose keys are the ones GitLab
// answers with, because client-go's NotePosition carries no width, height, x
// or y (docs/development/upstream-bugs.md) and an image draft would come back
// without its point. A key GitLab answers as null is left out of the request.
// GitLab answers a draft that has no position with an all-null text position,
// and one with no start or head commit or no file is published as a general
// comment whatever it carries ([toolutil.ValidatePositionAnchor]), so neither
// is sent back. A base commit GitLab holds as null, which it does for the
// diff of a branch with no merge base, is sent as an empty one, since the
// route requires the key and an empty value is what a position this server
// creates sends for it.
func currentDraft(ctx context.Context, client *gitlabclient.Client, input UpdateInput, withPosition bool) (storedDraft, error) {
	captureCtx, captured := gitlabclient.WithResponseCapture(ctx)
	draft, _, err := client.GL().DraftNotes.GetDraftNote(string(input.ProjectID), input.MRIID, input.NoteID, gl.WithContext(captureCtx))
	if err != nil {
		return storedDraft{}, toolutil.WrapErrWithStatusHint("draftNoteUpdate", err, http.StatusNotFound,
			"the draft is read first for what the update leaves out; verify note_id with mr_review.draft_note_list; draft notes are visible only to their author until published")
	}
	current := storedDraft{note: draft.Note}
	if !withPosition {
		return current, nil
	}
	var answer capturedDraft
	var p *capturedPosition
	err = captured.Decode(&answer)
	if err == nil && len(answer.Position) > 0 {
		err = json.Unmarshal(answer.Position, &p)
	}
	if err != nil {
		return storedDraft{}, fmt.Errorf("draftNoteUpdate: read the draft's position: %w", err)
	}
	placed := p != nil && toolutil.ValidatePositionAnchor(deref(p.StartSHA), deref(p.HeadSHA), deref(p.OldPath), deref(p.NewPath)) == nil
	if !placed {
		return current, nil
	}
	if current.position, err = p.options(); err != nil {
		return storedDraft{}, fmt.Errorf("draftNoteUpdate: read the draft's position: %w", err)
	}
	if current.position.BaseSHA == nil {
		current.position.BaseSHA = new("")
	}
	return current, nil
}

// deref returns the string s points to, and the empty string for nil.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
		return toolutil.ErrRequiredInt64("draftNoteDelete", "merge_request_iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("draftNoteDelete", "note_id")
	}
	_, err := client.GL().DraftNotes.DeleteDraftNote(string(input.ProjectID), input.MRIID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("draftNoteDelete", err, http.StatusForbidden,
			"only the draft author can delete; verify draft_note_id with mr_review.draft_note_list; published notes cannot be deleted via this endpoint")
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
		return toolutil.ErrRequiredInt64("draftNotePublish", "merge_request_iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("draftNotePublish", "note_id")
	}
	_, err := client.GL().DraftNotes.PublishDraftNote(string(input.ProjectID), input.MRIID, input.NoteID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("draftNotePublish", err, http.StatusForbidden,
			"only the draft author can publish; verify draft_note_id with mr_review.draft_note_list; once published the note becomes a regular MR note and cannot be unpublished")
	}
	return nil
}

// PublishAll publishes all pending draft notes on a merge request, and with
// them the summary note and reviewer state the caller named, if any.
//
// It calls the options variant because the other binding sends none of the
// three parameters. client-go marks that variant deprecated only because it
// plans to fold the options into PublishAllDraftNotes in v4, and says to use
// it meanwhile whenever the options are needed. The options are always a
// non-nil value, so a call naming none of them sends the empty object `{}`,
// the one empty body Grape parses to the parameter set it means; a nil
// pointer would reach the body as the `null` recorded in
// docs/development/upstream-bugs.md.
func PublishAll(ctx context.Context, client *gitlabclient.Client, input PublishAllInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("draftNotePublishAll: project_id is required")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("draftNotePublishAll", "merge_request_iid")
	}
	opts := &gl.PublishAllDraftNotesOptions{Internal: input.Internal}
	if input.Note != "" {
		opts.Note = new(input.Note)
	}
	if input.ReviewerState != "" {
		opts.ReviewerState = new(input.ReviewerState)
	}
	_, err := client.GL().DraftNotes.PublishAllDraftNotesWithOptions(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx)) //nolint:staticcheck // SA1019: the only binding that carries note, internal and reviewer_state until client-go v4 folds them into PublishAllDraftNotes.
	if err != nil {
		return toolutil.WrapErrWithStatusHint("draftNotePublishAll", err, http.StatusForbidden,
			"publishes all current user's draft notes on the MR. Cannot be undone; verify project_id + merge_request_iid; use mr_review.draft_note_list first to review pending drafts")
	}
	return nil
}

// validatePosition judges a position by the fields GitLab reads for it,
// before the write is sent. Every position, of any type, needs its start and
// head commits and a file path, which [toolutil.ValidatePositionAnchor]
// checks first. A text position, which is what an unnamed type is sent as,
// needs a line, and is then judged against the lines of the merge request's
// diff by [validateTextPosition]. An image position is judged by its width,
// height, x and y, and a file position by its old and new paths, with no
// request, since GitLab places neither by a line of the diff. For a draft
// note that is the only judgement before publication: GitLab saves a draft
// whose position is incomplete, in any of these parts, and publishes it as a
// general comment (DraftNote#on_diff? asks Position#complete?). A
// position_type GitLab does not declare is left to GitLab, which refuses it
// by name.
func validatePosition(ctx context.Context, client *gitlabclient.Client, projectID string, mrIID int64, pos *DiffPosition) error {
	if err := toolutil.ValidatePositionAnchor(pos.StartSHA, pos.HeadSHA, pos.OldPath, pos.NewPath); err != nil {
		return err
	}
	switch pos.PositionType {
	case "", toolutil.PositionTypeText:
		if err := toolutil.ValidateTextPosition(int64(pos.OldLine), int64(pos.NewLine)); err != nil {
			return err
		}
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
// draft notes from being silently lost when published with out-of-range
// positions. On a file the diff renames it also asks for both of the file's
// paths ([toolutil.ValidateRenamedFilePosition]). When the diff cannot be
// listed, or cannot be shown to be the one the position's SHAs name, the
// check is skipped and the write that follows is judged by GitLab, for the
// reasons [toolutil.MergeRequestDiffForPositionCheck] gives.
func validateTextPosition(ctx context.Context, client *gitlabclient.Client, projectID string, mrIID int64, pos *DiffPosition) error {
	// GitLab takes the file from new_path unless it is blank, and from old_path
	// otherwise (new_path.presence || old_path), so a new_path of blanks names
	// the old_path's file, as toolutil.ValidatePositionAnchor already reads it.
	targetPath := pos.NewPath
	if strings.TrimSpace(targetPath) == "" {
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
				"Omit the position parameter to create a general (non-inline) comment instead", targetPath,
		)
	}
	if err := toolutil.ValidateRenamedFilePosition(fileDiff, pos.OldPath, pos.NewPath); err != nil {
		return err
	}

	if fileDiff.Diff == "" {
		return nil
	}
	lines := toolutil.ParseDiffLines(fileDiff.Diff)
	return toolutil.ValidateDiffPosition(lines, pos.NewLine, pos.OldLine)
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
