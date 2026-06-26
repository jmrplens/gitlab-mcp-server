package issuelinks

import (
	"context"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
// author/assignee/assignees/milestone/references; labels as []string).
func toRelationOutput(r *gitlab.IssueRelation) RelationOutput {
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
		UpdatedAt:      formatTimePtr(r.UpdatedAt),
		Title:          r.Title,
		CreatedAt:      formatTimePtr(r.CreatedAt),
		Labels:         []string(r.Labels),
		DueDate:        formatISOTimePtr(r.DueDate),
		WebURL:         r.WebURL,
		References:     referencesOutput(r.References),
		Weight:         r.Weight,
		UserNotesCount: r.UserNotesCount,
		IssueLinkID:    int(r.IssueLinkID),
		LinkType:       r.LinkType,
		LinkCreatedAt:  formatTimePtr(r.LinkCreatedAt),
		LinkUpdatedAt:  formatTimePtr(r.LinkUpdatedAt),
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

	relations, _, err := client.GL().IssueLinks.ListIssueRelations(string(input.ProjectID), int64(input.IssueIID), gitlab.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint(toolListIssueLinks, err, http.StatusNotFound,
			"verify project_id with gitlab_project_get and issue_iid with gitlab_issue_list")
	}

	out := ListOutput{
		Relations: make([]RelationOutput, 0, len(relations)),
	}
	for _, r := range relations {
		out.Relations = append(out.Relations, toRelationOutput(r))
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
