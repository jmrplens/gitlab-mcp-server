package issuelinks

import (
	"context"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Field and tool name constants shared by the issuelinks handlers. Centralizing
// them keeps the error messages and parameter validation consistent.
const (
	fieldProjectID      = "project_id"
	fieldIssueIID       = "issue_iid"
	toolListIssueLinks  = "list issue links"
	toolGetIssueLink    = "get issue link"
	toolCreateIssueLink = "create issue link"
	toolDeleteIssueLink = "delete issue link"
)

// ---------------------------------------------------------------------------
// Input / Output types
// ---------------------------------------------------------------------------.

// ListInput holds parameters for listing issue relations.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int                  `json:"issue_iid" jsonschema:"Issue IID,required"`
}

// GetInput holds parameters for getting a specific issue link.
type GetInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID    int                  `json:"issue_iid" jsonschema:"Issue IID,required"`
	IssueLinkID int                  `json:"issue_link_id" jsonschema:"Issue link ID,required"`
}

// CreateInput holds parameters for creating an issue link.
type CreateInput struct {
	ProjectID       toolutil.StringOrInt `json:"project_id" jsonschema:"Source project ID or URL-encoded path,required"`
	IssueIID        int                  `json:"issue_iid" jsonschema:"Source issue IID,required"`
	TargetProjectID string               `json:"target_project_id" jsonschema:"Target project ID or path,required"`
	TargetIssueIID  string               `json:"target_issue_iid" jsonschema:"Target issue IID,required"`
	LinkType        string               `json:"link_type" jsonschema:"Link type: relates_to (default), blocks, or is_blocked_by"`
}

// DeleteInput holds parameters for deleting an issue link.
type DeleteInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID    int                  `json:"issue_iid" jsonschema:"Issue IID,required"`
	IssueLinkID int                  `json:"issue_link_id" jsonschema:"Issue link ID to remove,required"`
}

// Output represents a single issue link. It mirrors the gitlab.IssueLink struct
// (id, source_issue, target_issue, link_type). SourceIssue and TargetIssue
// surface the full SDK issue objects (1:1 audit policy, full nested objects).
type Output struct {
	toolutil.HintableOutput
	ID          int             `json:"id"`
	LinkType    string          `json:"link_type"`
	SourceIssue *IssueRefOutput `json:"source_issue,omitempty"`
	TargetIssue *IssueRefOutput `json:"target_issue,omitempty"`
}

// RelationOutput represents a related issue from the list endpoint. It mirrors
// the full gitlab.IssueRelation struct: author/assignee/assignees/milestone/
// references are surfaced as full nested objects and labels as a []string
// (1:1 audit policy).
//
// The block after the SDK's own fields is what lib/api/entities/related_issue.rb
// sends and client-go's IssueRelation declares on no field, read from the
// captured response beside the SDK's decode (ADR-0021) and described on
// [relationExtra]. Nineteen of them are on every response; `epic`, `epic_iid`,
// `iteration` and `health_status` arrive under their licensed feature, and
// `task_status` under the issue's own task state.
type RelationOutput struct {
	ID             int               `json:"id"`
	IID            int               `json:"iid"`
	State          string            `json:"state"`
	Description    string            `json:"description,omitempty"`
	Confidential   bool              `json:"confidential"`
	Author         *UserOutput       `json:"author,omitempty"`
	Milestone      *MilestoneOutput  `json:"milestone,omitempty"`
	ProjectID      int               `json:"project_id"`
	Assignees      []*UserOutput     `json:"assignees,omitempty"`
	Assignee       *UserOutput       `json:"assignee,omitempty"`
	UpdatedAt      string            `json:"updated_at,omitempty"`
	Title          string            `json:"title"`
	CreatedAt      string            `json:"created_at,omitempty"`
	Labels         []string          `json:"labels,omitempty"`
	DueDate        string            `json:"due_date,omitempty"`
	WebURL         string            `json:"web_url"`
	References     *ReferencesOutput `json:"references,omitempty"`
	Weight         int64             `json:"weight,omitempty" tier:"premium"`
	UserNotesCount int64             `json:"user_notes_count,omitempty"`
	IssueLinkID    int               `json:"issue_link_id"`
	LinkType       string            `json:"link_type"`
	LinkCreatedAt  string            `json:"link_created_at,omitempty"`
	LinkUpdatedAt  string            `json:"link_updated_at,omitempty"`

	Links                *RelationLinksOutput        `json:"_links,omitempty"`
	BlockingIssuesCount  int64                       `json:"blocking_issues_count,omitempty"`
	ClosedAt             string                      `json:"closed_at,omitempty"`
	ClosedBy             *toolutil.UserBasicOutput   `json:"closed_by,omitempty"`
	DiscussionLocked     bool                        `json:"discussion_locked"`
	Downvotes            int64                       `json:"downvotes,omitempty"`
	Epic                 *RelationEpicOutput         `json:"epic,omitempty" tier:"premium"`
	EpicIID              int64                       `json:"epic_iid,omitempty" tier:"premium"`
	HasTasks             bool                        `json:"has_tasks"`
	HealthStatus         string                      `json:"health_status,omitempty" tier:"ultimate"`
	Imported             bool                        `json:"imported"`
	ImportedFrom         string                      `json:"imported_from,omitempty"`
	IssueType            string                      `json:"issue_type,omitempty"`
	Iteration            *IterationOutput            `json:"iteration,omitempty" tier:"premium"`
	MergeRequestsCount   int64                       `json:"merge_requests_count,omitempty"`
	MovedToID            int64                       `json:"moved_to_id,omitempty"`
	ServiceDeskReplyTo   string                      `json:"service_desk_reply_to,omitempty"`
	Severity             string                      `json:"severity,omitempty"`
	StartDate            string                      `json:"start_date,omitempty"`
	TaskCompletionStatus *TaskCompletionStatusOutput `json:"task_completion_status,omitempty"`
	TaskStatus           string                      `json:"task_status,omitempty"`
	TimeStats            *TimeStatsOutput            `json:"time_stats,omitempty"`
	Type                 string                      `json:"type,omitempty"`
	Upvotes              int64                       `json:"upvotes,omitempty"`
}

// ListOutput represents a list of issue relations.
type ListOutput struct {
	toolutil.HintableOutput
	Relations []RelationOutput `json:"relations"`
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// toOutput converts the GitLab API response to the tool output format.
func toOutput(link *gitlab.IssueLink) Output {
	out := Output{
		ID:          int(link.ID),
		LinkType:    link.LinkType,
		SourceIssue: issueRefOutput(link.SourceIssue),
		TargetIssue: issueRefOutput(link.TargetIssue),
	}
	return out
}

// toRelationOutput converts the GitLab API response to the tool output format,
// mirroring every field of gitlab.IssueRelation (full nested objects for
// author/assignee/assignees/milestone/references; labels as []string), and
// takes beside it the keys the capture read that the SDK struct does not
// model.
func toRelationOutput(r *gitlab.IssueRelation, extra relationExtra) RelationOutput {
	return RelationOutput{
		ID:             int(r.ID),
		IID:            int(r.IID),
		State:          r.State,
		Description:    r.Description,
		Confidential:   r.Confidential,
		Author:         authorOutput(r.Author),
		Milestone:      milestoneOutput(r.Milestone),
		ProjectID:      int(r.ProjectID),
		Assignees:      assigneeOutputs(r.Assignees),
		Assignee:       assigneeOutput(r.Assignee),
		UpdatedAt:      toolutil.FormatTimePtr(r.UpdatedAt),
		Title:          r.Title,
		CreatedAt:      toolutil.FormatTimePtr(r.CreatedAt),
		Labels:         []string(r.Labels),
		DueDate:        toolutil.FormatISOTimePtr(r.DueDate),
		WebURL:         r.WebURL,
		References:     referencesOutput(r.References),
		Weight:         r.Weight,
		UserNotesCount: r.UserNotesCount,
		IssueLinkID:    int(r.IssueLinkID),
		LinkType:       r.LinkType,
		LinkCreatedAt:  toolutil.FormatTimePtr(r.LinkCreatedAt),
		LinkUpdatedAt:  toolutil.FormatTimePtr(r.LinkUpdatedAt),

		Links:                extra.Links,
		BlockingIssuesCount:  extra.BlockingIssuesCount,
		ClosedAt:             toolutil.FormatTimePtr(extra.ClosedAt),
		ClosedBy:             extra.ClosedBy,
		DiscussionLocked:     extra.DiscussionLocked,
		Downvotes:            extra.Downvotes,
		Epic:                 extra.Epic,
		EpicIID:              extra.EpicIID,
		HasTasks:             extra.HasTasks,
		HealthStatus:         extra.HealthStatus,
		Imported:             extra.Imported,
		ImportedFrom:         extra.ImportedFrom,
		IssueType:            extra.IssueType,
		Iteration:            extra.Iteration,
		MergeRequestsCount:   extra.MergeRequestsCount,
		MovedToID:            extra.MovedToID,
		ServiceDeskReplyTo:   extra.ServiceDeskReplyTo,
		Severity:             extra.Severity,
		StartDate:            extra.StartDate,
		TaskCompletionStatus: extra.TaskCompletionStatus,
		TaskStatus:           extra.TaskStatus,
		TimeStats:            extra.TimeStats,
		Type:                 extra.Type,
		Upvotes:              extra.Upvotes,
	}
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// List retrieves the list of issue relations (links) for a given issue
// from the GitLab Issue links API (GET /projects/:id/issues/:issue_iid/links).
// Returns a [ListOutput] with the linked issues or an error if the project
// or issue is not found.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired(fieldProjectID)
	}
	if input.IssueIID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64(toolListIssueLinks, fieldIssueIID)
	}
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage(toolListIssueLinks, err)
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	relations, _, err := client.GL().IssueLinks.ListIssueRelations(string(input.ProjectID), int64(input.IssueIID), gitlab.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint(toolListIssueLinks, err, http.StatusNotFound,
			"verify project_id with gitlab_project_get and issue_iid with gitlab_issue_list")
	}

	extras, err := capturedRelations(captured, len(relations))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr(toolListIssueLinks, err)
	}

	out := ListOutput{
		Relations: make([]RelationOutput, 0, len(relations)),
	}
	for i, r := range relations {
		out.Relations = append(out.Relations, toRelationOutput(r, extras[i]))
	}
	return out, nil
}

// Get retrieves a single issue link by its ID from the GitLab Issue links
// API (GET /projects/:id/issues/:issue_iid/links/:issue_link_id). Returns the
// link details including source and target issue metadata.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired(fieldProjectID)
	}
	if input.IssueIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64(toolGetIssueLink, fieldIssueIID)
	}
	if input.IssueLinkID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64(toolGetIssueLink, "issue_link_id")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolGetIssueLink, err)
	}

	link, _, err := client.GL().IssueLinks.GetIssueLink(string(input.ProjectID), int64(input.IssueIID), int64(input.IssueLinkID), gitlab.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint(toolGetIssueLink, err, http.StatusNotFound,
			"verify issue_link_id with gitlab_issue_link_list; the link must belong to the specified issue")
	}
	return toOutput(link), nil
}

// Create creates a new issue link between a source issue and a target issue
// via the GitLab Issue links API (POST /projects/:id/issues/:issue_iid/links).
// The link may be of type "relates_to" (default), "blocks", or "is_blocked_by"
// and may target an issue in a different project.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired(fieldProjectID)
	}
	if input.IssueIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64(toolCreateIssueLink, fieldIssueIID)
	}
	if input.TargetProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("target_project_id")
	}
	if input.TargetIssueIID == "" {
		return Output{}, toolutil.ErrFieldRequired("target_issue_iid")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolCreateIssueLink, err)
	}

	opts := &gitlab.CreateIssueLinkOptions{
		TargetProjectID: &input.TargetProjectID,
		TargetIssueIID:  &input.TargetIssueIID,
	}
	if input.LinkType != "" {
		opts.LinkType = &input.LinkType
	}

	link, _, err := client.GL().IssueLinks.CreateIssueLink(string(input.ProjectID), int64(input.IssueIID), opts, gitlab.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint(toolCreateIssueLink, err, http.StatusBadRequest,
			"link_type must be one of {relates_to, blocks, is_blocked_by}; verify target_project_id and target_issue_iid; cannot link issue to itself or create duplicate links")
	}
	return toOutput(link), nil
}

// Delete removes an existing issue link from a GitLab project via the
// GitLab Issue links API (DELETE /projects/:id/issues/:issue_iid/links/:issue_link_id).
// Returns an error if the link is not found or the caller lacks Reporter
// role or higher.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired(fieldProjectID)
	}
	if input.IssueIID <= 0 {
		return toolutil.ErrRequiredInt64(toolDeleteIssueLink, fieldIssueIID)
	}
	if input.IssueLinkID <= 0 {
		return toolutil.ErrRequiredInt64(toolDeleteIssueLink, "issue_link_id")
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolDeleteIssueLink, err)
	}

	_, _, err := client.GL().IssueLinks.DeleteIssueLink(string(input.ProjectID), int64(input.IssueIID), int64(input.IssueLinkID), gitlab.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint(toolDeleteIssueLink, err, http.StatusNotFound,
			"verify issue_link_id with gitlab_issue_link_list; deleting issue links requires Reporter role or higher")
	}
	return nil
}
