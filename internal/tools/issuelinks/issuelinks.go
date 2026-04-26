// Package issuelinks implements MCP tool handlers for GitLab issue link
// operations including list, get, create, and delete. It manages relationships
// between issues (relates_to, blocks, is_blocked_by) via the IssueLinks API.
package issuelinks

import (
	"context"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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

// Output represents a single issue link.
type Output struct {
	toolutil.HintableOutput
	ID              int    `json:"id"`
	SourceIssueIID  int    `json:"source_issue_iid"`
	SourceProjectID int    `json:"source_project_id"`
	TargetIssueIID  int    `json:"target_issue_iid"`
	TargetProjectID int    `json:"target_project_id"`
	LinkType        string `json:"link_type"`
}

// RelationOutput represents a related issue from the list endpoint.
type RelationOutput struct {
	ID          int    `json:"id"`
	IID         int    `json:"iid"`
	Title       string `json:"title"`
	State       string `json:"state"`
	ProjectID   int    `json:"project_id"`
	LinkType    string `json:"link_type"`
	IssueLinkID int    `json:"issue_link_id"`
	WebURL      string `json:"web_url"`
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
		ID:       int(link.ID),
		LinkType: link.LinkType,
	}
	if link.SourceIssue != nil {
		out.SourceIssueIID = int(link.SourceIssue.IID)
		out.SourceProjectID = int(link.SourceIssue.ProjectID)
	}
	if link.TargetIssue != nil {
		out.TargetIssueIID = int(link.TargetIssue.IID)
		out.TargetProjectID = int(link.TargetIssue.ProjectID)
	}
	return out
}

// toRelationOutput converts the GitLab API response to the tool output format.
func toRelationOutput(r *gitlab.IssueRelation) RelationOutput {
	return RelationOutput{
		ID:          int(r.ID),
		IID:         int(r.IID),
		Title:       r.Title,
		State:       r.State,
		ProjectID:   int(r.ProjectID),
		LinkType:    r.LinkType,
		IssueLinkID: int(r.IssueLinkID),
		WebURL:      r.WebURL,
	}
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// List lists resources for the issuelinks package.
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

// Get retrieves resources for the issuelinks package.
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
			"verify issue_link_id with gitlab_issue_links_list; the link must belong to the specified issue")
	}
	return toOutput(link), nil
}

// Create creates resources for the issuelinks package.
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

// Delete deletes resources for the issuelinks package.
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
			"verify issue_link_id with gitlab_issue_links_list; deleting issue links requires Reporter role or higher")
	}
	return nil
}
