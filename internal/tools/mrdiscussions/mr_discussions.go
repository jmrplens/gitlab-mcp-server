// mr_discussions.go implements GitLab merge request discussion operations
// including create (general and inline), resolve/unresolve, reply, and list.
// It exposes typed input/output structs and handler functions registered as
// MCP tools.

package mrdiscussions

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// DiffPosition defines the location of an inline diff comment.
type DiffPosition struct {
	BaseSHA  string `json:"base_sha"  jsonschema:"Base commit SHA (merge-base),required"`
	StartSHA string `json:"start_sha" jsonschema:"SHA of the first commit in the MR,required"`
	HeadSHA  string `json:"head_sha"  jsonschema:"HEAD commit SHA of the MR source branch,required"`
	OldPath  string `json:"old_path,omitempty"  jsonschema:"File path before the change (for modified/deleted files)"`
	NewPath  string `json:"new_path"            jsonschema:"File path after the change,required"`
	OldLine  int    `json:"old_line,omitempty" jsonschema:"Line in old file. Set ONLY for removed lines. For modified or added lines use new_line instead. Set both old_line and new_line only for unchanged context lines."`
	NewLine  int    `json:"new_line,omitempty" jsonschema:"Line in new file. Set ONLY for added or modified lines. For removed lines use old_line instead. Set both old_line and new_line only for unchanged context lines."`
}

// CreateInput defines parameters for creating a discussion (inline or general).
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	Body      string               `json:"body"       jsonschema:"Discussion body,required"`
	Position  *DiffPosition        `json:"position,omitempty" jsonschema:"Diff position for inline comments. Omit for general MR discussions."`
}

// NoteOutput represents a single note within a discussion.
type NoteOutput struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	Body        string `json:"body"`
	Author      string `json:"author"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	Resolved    bool   `json:"resolved"`
	Resolvable  bool   `json:"resolvable"`
	System      bool   `json:"system"`
	Internal    bool   `json:"internal"`
	Type        string `json:"type,omitempty"`
	NoteableID  int64  `json:"notable_id,omitempty"`
	NoteableIID int64  `json:"notable_iid,omitempty"`
	CommitID    string `json:"commit_id,omitempty"`
	ProjectID   int64  `json:"project_id,omitempty"`
}

// Output represents a discussion thread.
type Output struct {
	toolutil.HintableOutput
	ID             string       `json:"id"`
	IndividualNote bool         `json:"individual_note"`
	Notes          []NoteOutput `json:"notes"`
}

// ResolveInput defines parameters for resolving/unresolving a discussion.
type ResolveInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"   jsonschema:"Project ID or URL-encoded path,required"`
	MRIID        int64                `json:"mr_iid"       jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"ID of the discussion to resolve,required"`
	Resolved     bool                 `json:"resolved"      jsonschema:"True to resolve, false to unresolve,required"`
}

// ReplyInput defines parameters for replying to an existing discussion.
type ReplyInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"    jsonschema:"Project ID or URL-encoded path,required"`
	MRIID        int64                `json:"mr_iid"        jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"ID of the discussion to reply to,required"`
	Body         string               `json:"body"          jsonschema:"Reply body,required"`
}

// ListInput defines parameters for listing discussions.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	toolutil.PaginationInput
}

// ListOutput holds a list of discussions.
type ListOutput struct {
	toolutil.HintableOutput
	Discussions []Output                  `json:"discussions"`
	Pagination  toolutil.PaginationOutput `json:"pagination"`
}

// NoteToOutput converts a GitLab API [gl.Note] to a
// [NoteOutput], formatting the creation timestamp as RFC 3339.
func NoteToOutput(n *gl.Note) NoteOutput {
	out := NoteOutput{
		ID:          n.ID,
		Body:        n.Body,
		Author:      n.Author.Username,
		Resolved:    n.Resolved,
		Resolvable:  n.Resolvable,
		System:      n.System,
		Internal:    n.Internal,
		Type:        n.NoteableType,
		NoteableID:  n.NoteableID,
		NoteableIID: n.NoteableIID,
		CommitID:    n.CommitID,
		ProjectID:   n.ProjectID,
	}
	if n.CreatedAt != nil {
		out.CreatedAt = n.CreatedAt.Format(time.RFC3339)
	}
	if n.UpdatedAt != nil {
		out.UpdatedAt = n.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// ToOutput converts a GitLab API [gl.Discussion] to an
// [Output], including all notes within the thread.
func ToOutput(d *gl.Discussion) Output {
	notes := make([]NoteOutput, len(d.Notes))
	for i, n := range d.Notes {
		notes[i] = NoteToOutput(n)
	}
	return Output{
		ID:             d.ID,
		IndividualNote: d.IndividualNote,
		Notes:          notes,
	}
}

// Create creates a new discussion on a merge request. When a
// [DiffPosition] is provided the discussion is attached as an inline comment
// on the specified file and line; otherwise a general discussion is created.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrDiscussionCreate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrDiscussionCreate", "mr_iid")
	}
	if input.Position != nil {
		if err := validatePosition(ctx, client, string(input.ProjectID), input.MRIID, input.Position); err != nil {
			return Output{}, fmt.Errorf("mrDiscussionCreate: %w", err)
		}
	}
	opts := &gl.CreateMergeRequestDiscussionOptions{
		Body: new(toolutil.NormalizeText(input.Body)),
	}
	if input.Position != nil {
		p := input.Position
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
		opts.Position = pos
	}
	d, _, err := client.GL().Discussions.CreateMergeRequestDiscussion(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("mrDiscussionCreate", err, http.StatusBadRequest,
			"for inline diff comments, position requires base_sha, head_sha, start_sha, position_type=text, and a valid old_path/new_path with line numbers; use gitlab_mr_changes to fetch the diff context")
	}
	return ToOutput(d), nil
}

// Resolve resolves or unresolves a discussion thread on a merge
// request, depending on the Resolved flag in the input.
func Resolve(ctx context.Context, client *gitlabclient.Client, input ResolveInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrDiscussionResolve: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrDiscussionResolve", "mr_iid")
	}
	d, _, err := client.GL().Discussions.ResolveMergeRequestDiscussion(string(input.ProjectID), input.MRIID, input.DiscussionID, &gl.ResolveMergeRequestDiscussionOptions{
		Resolved: new(input.Resolved),
	}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("mrDiscussionResolve", err, http.StatusNotFound,
			"verify discussion_id with gitlab_mr_discussions_list; only thread (resolvable) discussions can be resolved")
	}
	return ToOutput(d), nil
}

// Reply adds a reply note to an existing discussion thread on a
// merge request. Returns the newly created note.
func Reply(ctx context.Context, client *gitlabclient.Client, input ReplyInput) (NoteOutput, error) {
	if err := ctx.Err(); err != nil {
		return NoteOutput{}, err
	}
	if input.ProjectID == "" {
		return NoteOutput{}, errors.New("mrDiscussionReply: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("mrDiscussionReply", "mr_iid")
	}
	n, _, err := client.GL().Discussions.AddMergeRequestDiscussionNote(string(input.ProjectID), input.MRIID, input.DiscussionID, &gl.AddMergeRequestDiscussionNoteOptions{
		Body: new(toolutil.NormalizeText(input.Body)),
	}, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("mrDiscussionReply", err, http.StatusNotFound,
			"verify discussion_id with gitlab_mr_discussions_list")
	}
	return NoteToOutput(n), nil
}

// List returns a paginated list of discussion threads for a merge
// request, including all notes within each thread.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("mrDiscussionList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("mrDiscussionList", "mr_iid")
	}
	opts := &gl.ListMergeRequestDiscussionsOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	discussions, resp, err := client.GL().Discussions.ListMergeRequestDiscussions(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("mrDiscussionList", err, http.StatusNotFound,
			"verify project_id and mr_iid with gitlab_merge_request_get")
	}
	out := make([]Output, len(discussions))
	for i, d := range discussions {
		out[i] = ToOutput(d)
	}
	return ListOutput{Discussions: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetInput defines parameters for getting a single discussion.
type GetInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"    jsonschema:"Project ID or URL-encoded path,required"`
	MRIID        int64                `json:"mr_iid"        jsonschema:"Merge request internal ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"ID of the discussion,required"`
}

// UpdateNoteInput defines parameters for updating a discussion note.
type UpdateNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"    jsonschema:"Project ID or URL-encoded path,required"`
	MRIID        int64                `json:"mr_iid"        jsonschema:"Merge request internal ID,required"`
	DiscussionID string               `json:"discussion_id" jsonschema:"ID of the discussion containing the note,required"`
	NoteID       int64                `json:"note_id"       jsonschema:"ID of the note to update,required"`
	Body         string               `json:"body,omitempty"     jsonschema:"New body text (Markdown). Leave empty to keep current body."`
	Resolved     *bool                `json:"resolved,omitempty" jsonschema:"Set to true to resolve, false to unresolve. Omit to leave unchanged."`
}

// DeleteNoteInput defines parameters for deleting a discussion note.
type DeleteNoteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"    jsonschema:"Project ID or URL-encoded path,required"`
	MRIID        int64                `json:"mr_iid"        jsonschema:"Merge request internal ID,required"`
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
		return Output{}, toolutil.ErrRequiredInt64("mrDiscussionGet", "mr_iid")
	}
	d, _, err := client.GL().Discussions.GetMergeRequestDiscussion(string(input.ProjectID), input.MRIID, input.DiscussionID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("mrDiscussionGet", err, http.StatusNotFound,
			"verify discussion_id with gitlab_mr_discussions_list")
	}
	return ToOutput(d), nil
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
		return NoteOutput{}, toolutil.ErrRequiredInt64("mrDiscussionNoteUpdate", "mr_iid")
	}
	if input.NoteID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("mrDiscussionNoteUpdate", "note_id")
	}
	opts := &gl.UpdateMergeRequestDiscussionNoteOptions{}
	if input.Body != "" {
		opts.Body = new(toolutil.NormalizeText(input.Body))
	}
	if input.Resolved != nil {
		opts.Resolved = input.Resolved
	}
	n, _, err := client.GL().Discussions.UpdateMergeRequestDiscussionNote(string(input.ProjectID), input.MRIID, input.DiscussionID, input.NoteID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return NoteOutput{}, toolutil.WrapErrWithHint("mrDiscussionNoteUpdate", err,
				"only the note author can edit body; only Maintainers can change the resolved flag on resolvable threads")
		}
		return NoteOutput{}, toolutil.WrapErrWithStatusHint("mrDiscussionNoteUpdate", err, http.StatusNotFound,
			"verify note_id with gitlab_mr_discussions_get")
	}
	return NoteToOutput(n), nil
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
		return toolutil.ErrRequiredInt64("mrDiscussionNoteDelete", "mr_iid")
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

// validatePosition fetches the MR diff and validates that the given position
// refers to a line actually present in the diff. This prevents 400/500 errors
// from GitLab when commenting on out-of-range lines and provides actionable
// error messages explaining what went wrong.
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
				"Omit the position parameter to create a general (non-inline) discussion instead", targetPath)
	}

	lines := toolutil.ParseDiffLines(fileDiff)
	return toolutil.ValidateDiffPosition(lines, pos.NewLine, pos.OldLine)
}

// Markdown Formatting.
