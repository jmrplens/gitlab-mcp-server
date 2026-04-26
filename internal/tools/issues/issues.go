// Package issues implements GitLab issue operations including create, get, list,
// update, and delete. It exposes typed input/output structs and handler
// functions that interact with the GitLab Issues API v4.
package issues

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	msgNoIssuesFound = "No issues found.\n"
	tblHeaderIssues  = "| IID | Title | State | Author | Labels |\n"
)

// CreateInput defines parameters for creating a new issue.
type CreateInput struct {
	// Basic metadata
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Title       string               `json:"title" jsonschema:"Issue title,required"`
	Description string               `json:"description,omitempty" jsonschema:"Issue description (Markdown supported)"`
	IssueType   string               `json:"issue_type,omitempty" jsonschema:"Issue type (issue, incident, test_case, task)"`

	// Assignment and tracking
	AssigneeID  int64   `json:"assignee_id,omitempty" jsonschema:"Single user ID to assign (use assignee_ids for multiple)"`
	AssigneeIDs []int64 `json:"assignee_ids,omitempty" jsonschema:"User IDs to assign"`
	Labels      string  `json:"labels,omitempty" jsonschema:"Comma-separated labels to apply"`
	MilestoneID int64   `json:"milestone_id,omitempty" jsonschema:"Milestone ID to associate,required"`
	EpicID      int64   `json:"epic_id,omitempty" jsonschema:"Epic ID to associate the issue with"`
	Weight      int64   `json:"weight,omitempty" jsonschema:"Issue weight (0 or higher)"`
	DueDate     string  `json:"due_date,omitempty" jsonschema:"Due date in YYYY-MM-DD format"`

	// Behavior flags
	Confidential *bool  `json:"confidential,omitempty" jsonschema:"Mark issue as confidential"`
	CreatedAt    string `json:"created_at,omitempty" jsonschema:"Creation date override (ISO 8601, requires admin permissions)"`

	// Discussion resolution
	MergeRequestToResolveDiscussionsOf int64  `json:"merge_request_to_resolve_discussions_of,omitempty" jsonschema:"MR IID whose unresolved discussions become issues"`
	DiscussionToResolve                string `json:"discussion_to_resolve,omitempty" jsonschema:"Discussion ID to mark as resolved by this issue"`
}

// Output represents a GitLab issue.
type Output struct {
	toolutil.HintableOutput
	ID                  int64    `json:"id"`
	IID                 int64    `json:"issue_iid"`
	Title               string   `json:"title"`
	Description         string   `json:"description"`
	State               string   `json:"state"`
	Labels              []string `json:"labels"`
	Assignees           []string `json:"assignees"`
	Milestone           string   `json:"milestone"`
	Author              string   `json:"author"`
	ClosedBy            string   `json:"closed_by,omitempty"`
	WebURL              string   `json:"web_url"`
	CreatedAt           string   `json:"created_at"`
	UpdatedAt           string   `json:"updated_at"`
	ClosedAt            string   `json:"closed_at"`
	DueDate             string   `json:"due_date"`
	Confidential        bool     `json:"confidential"`
	DiscussionLocked    bool     `json:"discussion_locked"`
	ProjectID           int64    `json:"project_id"`
	Weight              int64    `json:"weight,omitempty"`
	IssueType           string   `json:"issue_type,omitempty"`
	HealthStatus        string   `json:"health_status,omitempty"`
	References          string   `json:"references,omitempty"`
	MergeRequestCount   int64    `json:"merge_request_count,omitempty"`
	TaskCompletionCount int64    `json:"task_completion_count,omitempty"`
	TaskCompletionTotal int64    `json:"task_completion_total,omitempty"`
	UserNotesCount      int64    `json:"user_notes_count,omitempty"`
	Upvotes             int64    `json:"upvotes,omitempty"`
	Downvotes           int64    `json:"downvotes,omitempty"`
	Subscribed          bool     `json:"subscribed"`
	TimeEstimate        int64    `json:"time_estimate,omitempty"`
	TotalTimeSpent      int64    `json:"total_time_spent,omitempty"`
	MovedToID           int64    `json:"moved_to_id,omitempty"`
	EpicIssueID         int64    `json:"epic_issue_id,omitempty"`
}

// GetInput defines parameters for retrieving a single issue.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue IID (project-scoped internal ID, not 'issue_id'),required"`
}

// ListInput defines filters for listing project issues.
type ListInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id"                  jsonschema:"Project ID or URL-encoded path,required"`
	State            string               `json:"state,omitempty"             jsonschema:"Filter by state (opened, closed, all)"`
	Labels           string               `json:"labels,omitempty"            jsonschema:"Comma-separated label names to filter by"`
	NotLabels        string               `json:"not_labels,omitempty"        jsonschema:"Comma-separated label names to exclude"`
	Milestone        string               `json:"milestone,omitempty"         jsonschema:"Milestone title to filter by"`
	Scope            string               `json:"scope,omitempty"             jsonschema:"Filter by scope (created_by_me, assigned_to_me, all)"`
	Search           string               `json:"search,omitempty"            jsonschema:"Search in title and description"`
	AssigneeUsername string               `json:"assignee_username,omitempty" jsonschema:"Filter by assignee username"`
	AuthorUsername   string               `json:"author_username,omitempty"   jsonschema:"Filter by author username"`
	IIDs             []int64              `json:"iids,omitempty"              jsonschema:"Filter by issue internal IDs"`
	IssueType        string               `json:"issue_type,omitempty"        jsonschema:"Filter by issue type (issue, incident, test_case, task)"`
	Confidential     *bool                `json:"confidential,omitempty"      jsonschema:"Filter by confidential status"`
	CreatedAfter     string               `json:"created_after,omitempty"     jsonschema:"Return issues created after date (ISO 8601 format, e.g. 2025-01-01T00:00:00Z)"`
	CreatedBefore    string               `json:"created_before,omitempty"    jsonschema:"Return issues created before date (ISO 8601 format, e.g. 2025-12-31T23:59:59Z)"`
	UpdatedAfter     string               `json:"updated_after,omitempty"     jsonschema:"Return issues updated after date (ISO 8601 format, e.g. 2025-01-01T00:00:00Z)"`
	UpdatedBefore    string               `json:"updated_before,omitempty"    jsonschema:"Return issues updated before date (ISO 8601 format, e.g. 2025-12-31T23:59:59Z)"`
	OrderBy          string               `json:"order_by,omitempty"          jsonschema:"Order by field (created_at, updated_at, priority, due_date)"`
	Sort             string               `json:"sort,omitempty"              jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
}

// ListOutput holds a paginated list of issues.
type ListOutput struct {
	toolutil.HintableOutput
	Issues     []Output                  `json:"issues"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// UpdateInput defines parameters for updating an existing issue.
type UpdateInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID         int64                `json:"issue_iid"               jsonschema:"Issue IID (project-scoped internal ID, not 'issue_id'),required"`
	Title            string               `json:"title,omitempty"         jsonschema:"New title"`
	Description      string               `json:"description,omitempty"   jsonschema:"New description (Markdown supported)"`
	StateEvent       string               `json:"state_event,omitempty"   jsonschema:"State transition (close, reopen)"`
	AssigneeID       int64                `json:"assignee_id,omitempty"       jsonschema:"Single user ID to assign (use assignee_ids for multiple)"`
	AssigneeIDs      []int64              `json:"assignee_ids,omitempty"  jsonschema:"New assignee user IDs"`
	Labels           string               `json:"labels,omitempty"        jsonschema:"Comma-separated labels to replace all existing"`
	AddLabels        string               `json:"add_labels,omitempty"    jsonschema:"Comma-separated labels to add without removing existing"`
	RemoveLabels     string               `json:"remove_labels,omitempty" jsonschema:"Comma-separated labels to remove"`
	EpicID           int64                `json:"epic_id,omitempty"       jsonschema:"Epic ID to associate (EE only)"`
	MilestoneID      int64                `json:"milestone_id,omitempty"  jsonschema:"New milestone ID (0 to unset),required"`
	DueDate          string               `json:"due_date,omitempty"      jsonschema:"New due date in YYYY-MM-DD format"`
	Confidential     *bool                `json:"confidential,omitempty"  jsonschema:"Update confidential flag"`
	IssueType        string               `json:"issue_type,omitempty"    jsonschema:"Issue type (issue, incident, test_case, task)"`
	Weight           int64                `json:"weight,omitempty"        jsonschema:"Issue weight (0 or higher)"`
	DiscussionLocked *bool                `json:"discussion_locked,omitempty" jsonschema:"Lock discussions on this issue"`
}

// DeleteInput defines parameters for deleting an issue.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue IID (project-scoped internal ID, not 'issue_id'),required"`
}

// ListGroupInput defines parameters for listing issues across a group.
type ListGroupInput struct {
	GroupID        toolutil.StringOrInt `json:"group_id"                jsonschema:"Group ID or URL-encoded path,required"`
	State          string               `json:"state,omitempty"         jsonschema:"Filter by state (opened, closed, all)"`
	Labels         string               `json:"labels,omitempty"        jsonschema:"Comma-separated list of labels to filter by"`
	Milestone      string               `json:"milestone,omitempty"     jsonschema:"Milestone title to filter by"`
	Search         string               `json:"search,omitempty"        jsonschema:"Search in title and description"`
	Scope          string               `json:"scope,omitempty"         jsonschema:"Scope (created_by_me, assigned_to_me, all)"`
	AuthorUsername string               `json:"author_username,omitempty" jsonschema:"Filter by author username"`
	CreatedAfter   string               `json:"created_after,omitempty"  jsonschema:"Return issues created after date (ISO 8601)"`
	CreatedBefore  string               `json:"created_before,omitempty" jsonschema:"Return issues created before date (ISO 8601)"`
	UpdatedAfter   string               `json:"updated_after,omitempty"  jsonschema:"Return issues updated after date (ISO 8601)"`
	UpdatedBefore  string               `json:"updated_before,omitempty" jsonschema:"Return issues updated before date (ISO 8601)"`
	toolutil.PaginationInput
}

// ListGroupOutput holds a paginated list of group issues.
type ListGroupOutput struct {
	toolutil.HintableOutput
	Issues     []Output                  `json:"issues"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ToOutput converts a GitLab API [gl.Issue] to the MCP tool output
// format, extracting author, milestone, assignees, and formatting timestamps
// as RFC 3339 strings. Nil labels are normalized to an empty slice.
func ToOutput(issue *gl.Issue) Output {
	out := Output{
		ID:          issue.ID,
		IID:         issue.IID,
		Title:       issue.Title,
		Description: issue.Description,
		State:       issue.State,
		Labels:      []string(issue.Labels),
		WebURL:      issue.WebURL,
	}
	if out.Labels == nil {
		out.Labels = []string{}
	}
	if issue.Author != nil {
		out.Author = issue.Author.Username
	}
	if issue.Milestone != nil {
		out.Milestone = issue.Milestone.Title
	}
	assignees := make([]string, 0, len(issue.Assignees))
	for _, a := range issue.Assignees {
		assignees = append(assignees, a.Username)
	}
	out.Assignees = assignees
	if issue.CreatedAt != nil {
		out.CreatedAt = issue.CreatedAt.Format(time.RFC3339)
	}
	if issue.UpdatedAt != nil {
		out.UpdatedAt = issue.UpdatedAt.Format(time.RFC3339)
	}
	if issue.ClosedAt != nil {
		out.ClosedAt = issue.ClosedAt.Format(time.RFC3339)
	}
	if issue.DueDate != nil {
		out.DueDate = time.Time(*issue.DueDate).Format("2006-01-02")
	}
	out.Confidential = issue.Confidential
	out.DiscussionLocked = issue.DiscussionLocked
	out.ProjectID = issue.ProjectID
	out.Weight = issue.Weight
	out.HealthStatus = issue.HealthStatus
	out.MergeRequestCount = issue.MergeRequestCount
	if issue.IssueType != nil {
		out.IssueType = *issue.IssueType
	}
	if issue.ClosedBy != nil {
		out.ClosedBy = issue.ClosedBy.Username
	}
	if issue.References != nil {
		out.References = issue.References.Full
	}
	out.UserNotesCount = issue.UserNotesCount
	if issue.TaskCompletionStatus != nil {
		out.TaskCompletionCount = issue.TaskCompletionStatus.CompletedCount
		out.TaskCompletionTotal = issue.TaskCompletionStatus.Count
	}
	out.Upvotes = issue.Upvotes
	out.Downvotes = issue.Downvotes
	out.Subscribed = issue.Subscribed
	out.MovedToID = issue.MovedToID
	if issue.TimeStats != nil {
		out.TimeEstimate = issue.TimeStats.TimeEstimate
		out.TotalTimeSpent = issue.TimeStats.TotalTimeSpent
	}
	out.EpicIssueID = issue.EpicIssueID
	return out
}

// Create creates a new issue in the specified GitLab project.
// It maps all optional fields (description, labels, assignees, milestone,
// due date, confidential) to the GitLab API request.
// Returns the created issue or an error if the API call fails.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("issueCreate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts, err := buildCreateOpts(input)
	if err != nil {
		return Output{}, err
	}
	issue, _, err := client.GL().Issues.CreateIssue(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint("issueCreate", err,
				"verify project_id with gitlab_project_get; the project must exist and your token must have at least Reporter role")
		}
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) {
			return Output{}, toolutil.WrapErrWithHint("issueCreate", err,
				"check that referenced labels, assignee_ids and milestone_id exist in this project (use gitlab_label_list, gitlab_user_get, gitlab_milestone_list)")
		}
		return Output{}, toolutil.WrapErrWithMessage("issueCreate", err)
	}
	return ToOutput(issue), nil
}

// buildCreateOpts maps CreateInput fields to the GitLab API create options.
func buildCreateOpts(input CreateInput) (*gl.CreateIssueOptions, error) {
	opts := &gl.CreateIssueOptions{
		Title: new(input.Title),
	}
	if input.Description != "" {
		opts.Description = new(toolutil.NormalizeText(input.Description))
	}
	if input.AssigneeID != 0 {
		opts.AssigneeID = new(input.AssigneeID)
	}
	if len(input.AssigneeIDs) > 0 {
		opts.AssigneeIDs = &input.AssigneeIDs
	}
	if input.Labels != "" {
		labels := gl.LabelOptions(strings.Split(input.Labels, ","))
		opts.Labels = &labels
	}
	if input.MilestoneID > 0 {
		opts.MilestoneID = new(input.MilestoneID)
	}
	if input.DueDate != "" {
		d, err := parseDueDate(input.DueDate)
		if err != nil {
			return nil, err
		}
		opts.DueDate = d
	}
	applyCreateExtraOpts(opts, input)
	if input.CreatedAt != "" {
		t, err := time.Parse(time.RFC3339, input.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("issueCreate: invalid created_at format (expected ISO 8601/RFC 3339): %w", err)
		}
		opts.CreatedAt = &t
	}
	if input.MergeRequestToResolveDiscussionsOf > 0 {
		opts.MergeRequestToResolveDiscussionsOf = new(input.MergeRequestToResolveDiscussionsOf)
	}
	if input.DiscussionToResolve != "" {
		opts.DiscussionToResolve = new(input.DiscussionToResolve)
	}
	return opts, nil
}

// applyCreateExtraOpts sets optional non-error-returning fields on the create options.
func applyCreateExtraOpts(opts *gl.CreateIssueOptions, input CreateInput) {
	if input.Confidential != nil {
		opts.Confidential = input.Confidential
	}
	if input.IssueType != "" {
		opts.IssueType = new(input.IssueType)
	}
	if input.Weight > 0 {
		opts.Weight = new(input.Weight)
	}
	if input.EpicID > 0 {
		opts.EpicID = new(input.EpicID)
	}
}

// Get retrieves a single issue by its internal ID from a GitLab project.
// Returns the issue details or an error if the issue is not found.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("issueGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.IssueIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issueGet", "issue_iid")
	}
	issue, _, err := client.GL().Issues.GetIssue(string(input.ProjectID), input.IssueIID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint("issueGet", err,
				"verify project_id and issue_iid; use gitlab_issue_list to see existing issues in the project")
		}
		return Output{}, toolutil.WrapErrWithMessage("issueGet", err)
	}
	return ToOutput(issue), nil
}

// List retrieves a paginated list of issues for a GitLab project.
// Supports filtering by state, labels, milestone, search text, assignee,
// author, and sorting options. Returns the issues with pagination metadata.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("issueList: project_id is required. Use gitlab_project_list to find the project ID first, then pass it as project_id")
	}
	opts := &gl.ListProjectIssuesOptions{}
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.Labels != "" {
		labels := gl.LabelOptions(strings.Split(input.Labels, ","))
		opts.Labels = &labels
	}
	if input.Milestone != "" {
		opts.Milestone = new(input.Milestone)
	}
	if input.Scope != "" {
		opts.Scope = new(input.Scope)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.AssigneeUsername != "" {
		opts.AssigneeUsername = new(input.AssigneeUsername)
	}
	if input.AuthorUsername != "" {
		opts.AuthorUsername = new(input.AuthorUsername)
	}
	opts.CreatedAfter = toolutil.ParseOptionalTime(input.CreatedAfter)
	opts.CreatedBefore = toolutil.ParseOptionalTime(input.CreatedBefore)
	opts.UpdatedAfter = toolutil.ParseOptionalTime(input.UpdatedAfter)
	opts.UpdatedBefore = toolutil.ParseOptionalTime(input.UpdatedBefore)
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Confidential != nil {
		opts.Confidential = input.Confidential
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	issues, resp, err := client.GL().Issues.ListProjectIssues(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("issueList", err)
	}
	out := make([]Output, len(issues))
	for i, issue := range issues {
		out[i] = ToOutput(issue)
	}
	return ListOutput{Issues: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// buildUpdateOpts maps UpdateInput fields to the GitLab API update
// options, applying only non-zero values. Returns an error if due_date parsing fails.
func buildUpdateOpts(input UpdateInput) (*gl.UpdateIssueOptions, error) {
	opts := &gl.UpdateIssueOptions{}
	if input.Title != "" {
		opts.Title = new(input.Title)
	}
	if input.Description != "" {
		opts.Description = new(toolutil.NormalizeText(input.Description))
	}
	if input.StateEvent != "" {
		opts.StateEvent = new(input.StateEvent)
	}
	if input.AssigneeID != 0 {
		opts.AssigneeID = new(input.AssigneeID)
	}
	if len(input.AssigneeIDs) > 0 {
		opts.AssigneeIDs = &input.AssigneeIDs
	}
	if input.Labels != "" {
		labels := gl.LabelOptions(strings.Split(input.Labels, ","))
		opts.Labels = &labels
	}
	if input.AddLabels != "" {
		labels := gl.LabelOptions(strings.Split(input.AddLabels, ","))
		opts.AddLabels = &labels
	}
	if input.RemoveLabels != "" {
		labels := gl.LabelOptions(strings.Split(input.RemoveLabels, ","))
		opts.RemoveLabels = &labels
	}
	if input.MilestoneID > 0 {
		opts.MilestoneID = new(input.MilestoneID)
	}
	if input.DueDate != "" {
		d, err := parseDueDate(input.DueDate)
		if err != nil {
			return nil, err
		}
		opts.DueDate = d
	}
	if input.Confidential != nil {
		opts.Confidential = input.Confidential
	}
	if input.IssueType != "" {
		opts.IssueType = new(input.IssueType)
	}
	if input.Weight > 0 {
		opts.Weight = new(input.Weight)
	}
	if input.EpicID > 0 {
		opts.EpicID = new(input.EpicID)
	}
	if input.DiscussionLocked != nil {
		opts.DiscussionLocked = input.DiscussionLocked
	}
	return opts, nil
}

// Update modifies an existing issue in a GitLab project.
// Only non-zero fields in the input are applied. Supports changing title,
// description, state, assignees, labels (replace, add, remove), milestone,
// due date, and confidential flag. Returns the updated issue.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("issueUpdate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.IssueIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issueUpdate", "issue_iid")
	}
	opts, err := buildUpdateOpts(input)
	if err != nil {
		return Output{}, err
	}
	issue, _, err := client.GL().Issues.UpdateIssue(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint("issueUpdate", err,
				"verify project_id and issue_iid. Use gitlab_issue_list to check available issues")
		}
		return Output{}, toolutil.WrapErrWithMessage("issueUpdate", err)
	}
	return ToOutput(issue), nil
}

// Delete permanently removes an issue from a GitLab project.
// Returns an error if the issue does not exist or the user lacks permission.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("issueDelete: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.IssueIID <= 0 {
		return toolutil.ErrRequiredInt64("issueDelete", "issue_iid")
	}
	_, err := client.GL().Issues.DeleteIssue(string(input.ProjectID), input.IssueIID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("issueDelete", err,
				"only project owners or administrators can delete issues. Use gitlab_issue_update with state_event='close' instead")
		}
		return toolutil.WrapErrWithMessage("issueDelete", err)
	}
	return nil
}

// parseDueDate converts a YYYY-MM-DD string to *gl.ISOTime.
func parseDueDate(s string) (*gl.ISOTime, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, fmt.Errorf("invalid due_date format (expected YYYY-MM-DD): %w", err)
	}
	d := gl.ISOTime(t)
	return &d, nil
}

// ListGroup retrieves a paginated list of issues across all projects in a group.
func ListGroup(ctx context.Context, client *gitlabclient.Client, input ListGroupInput) (ListGroupOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListGroupOutput{}, err
	}
	if input.GroupID == "" {
		return ListGroupOutput{}, errors.New("issueListGroup: group_id is required. Use gitlab_group_list to find the ID first, then pass it as group_id")
	}

	opts := &gl.ListGroupIssuesOptions{}
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.Labels != "" {
		labels := gl.LabelOptions(strings.Split(input.Labels, ","))
		opts.Labels = &labels
	}
	if input.Milestone != "" {
		opts.Milestone = new(input.Milestone)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.Scope != "" {
		opts.Scope = new(input.Scope)
	}
	if input.AuthorUsername != "" {
		opts.AuthorUsername = new(input.AuthorUsername)
	}
	opts.CreatedAfter = toolutil.ParseOptionalTime(input.CreatedAfter)
	opts.CreatedBefore = toolutil.ParseOptionalTime(input.CreatedBefore)
	opts.UpdatedAfter = toolutil.ParseOptionalTime(input.UpdatedAfter)
	opts.UpdatedBefore = toolutil.ParseOptionalTime(input.UpdatedBefore)
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	issues, resp, err := client.GL().Issues.ListGroupIssues(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListGroupOutput{}, toolutil.WrapErrWithMessage("issueListGroup", err)
	}

	out := make([]Output, len(issues))
	for i, issue := range issues {
		out[i] = ToOutput(issue)
	}
	return ListGroupOutput{Issues: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ListAllInput defines parameters for the global ListIssues endpoint (no project scope).
type ListAllInput struct {
	State            string `json:"state,omitempty"             jsonschema:"Filter by state (opened, closed, all)"`
	Labels           string `json:"labels,omitempty"            jsonschema:"Comma-separated label names to filter by"`
	Milestone        string `json:"milestone,omitempty"         jsonschema:"Milestone title to filter by"`
	Scope            string `json:"scope,omitempty"             jsonschema:"Filter by scope (created_by_me, assigned_to_me, all)"`
	Search           string `json:"search,omitempty"            jsonschema:"Search in title and description"`
	AssigneeUsername string `json:"assignee_username,omitempty" jsonschema:"Filter by assignee username"`
	AuthorUsername   string `json:"author_username,omitempty"   jsonschema:"Filter by author username"`
	OrderBy          string `json:"order_by,omitempty"          jsonschema:"Order by field (created_at, updated_at, priority, due_date)"`
	Sort             string `json:"sort,omitempty"              jsonschema:"Sort direction (asc, desc)"`
	CreatedAfter     string `json:"created_after,omitempty"     jsonschema:"Return issues created after date (ISO 8601)"`
	CreatedBefore    string `json:"created_before,omitempty"    jsonschema:"Return issues created before date (ISO 8601)"`
	UpdatedAfter     string `json:"updated_after,omitempty"     jsonschema:"Return issues updated after date (ISO 8601)"`
	UpdatedBefore    string `json:"updated_before,omitempty"    jsonschema:"Return issues updated before date (ISO 8601)"`
	Confidential     *bool  `json:"confidential,omitempty"      jsonschema:"Filter by confidential status"`
	toolutil.PaginationInput
}

// ListAll retrieves a paginated list of issues visible to the authenticated user
// across all projects (global scope).
func ListAll(ctx context.Context, client *gitlabclient.Client, input ListAllInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}

	opts := &gl.ListIssuesOptions{}
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.Labels != "" {
		labels := gl.LabelOptions(strings.Split(input.Labels, ","))
		opts.Labels = &labels
	}
	if input.Milestone != "" {
		opts.Milestone = new(input.Milestone)
	}
	if input.Scope != "" {
		opts.Scope = new(input.Scope)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.AssigneeUsername != "" {
		opts.AssigneeUsername = new(input.AssigneeUsername)
	}
	if input.AuthorUsername != "" {
		opts.AuthorUsername = new(input.AuthorUsername)
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Confidential != nil {
		opts.Confidential = input.Confidential
	}
	opts.CreatedAfter = toolutil.ParseOptionalTime(input.CreatedAfter)
	opts.CreatedBefore = toolutil.ParseOptionalTime(input.CreatedBefore)
	opts.UpdatedAfter = toolutil.ParseOptionalTime(input.UpdatedAfter)
	opts.UpdatedBefore = toolutil.ParseOptionalTime(input.UpdatedBefore)
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	result, resp, err := client.GL().Issues.ListIssues(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("issueListAll", err)
	}

	out := make([]Output, len(result))
	for i, issue := range result {
		out[i] = ToOutput(issue)
	}
	return ListOutput{Issues: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetByIDInput defines parameters for retrieving an issue by its global ID.
type GetByIDInput struct {
	IssueID int64 `json:"issue_id" jsonschema:"The global issue ID (not the project-scoped IID),required"`
}

// GetByID retrieves a single issue by its global numeric ID.
func GetByID(ctx context.Context, client *gitlabclient.Client, input GetByIDInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.IssueID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issueGetByID", "issue_id")
	}
	issue, _, err := client.GL().Issues.GetIssueByID(input.IssueID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("issueGetByID", err)
	}
	return ToOutput(issue), nil
}

// ReorderInput defines parameters for reordering an issue.
type ReorderInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID     int64                `json:"issue_iid"               jsonschema:"Issue internal ID,required"`
	MoveAfterID  *int64               `json:"move_after_id,omitempty"  jsonschema:"ID of issue to position after"`
	MoveBeforeID *int64               `json:"move_before_id,omitempty" jsonschema:"ID of issue to position before"`
}

// Reorder changes the position of an issue relative to other issues.
func Reorder(ctx context.Context, client *gitlabclient.Client, input ReorderInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("issueReorder: project_id is required")
	}
	if input.IssueIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issueReorder", "issue_iid")
	}
	opts := &gl.ReorderIssueOptions{
		MoveAfterID:  input.MoveAfterID,
		MoveBeforeID: input.MoveBeforeID,
	}
	issue, _, err := client.GL().Issues.ReorderIssue(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("issueReorder", err)
	}
	return ToOutput(issue), nil
}

// MoveInput defines parameters for moving an issue to another project.
type MoveInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"     jsonschema:"Source project ID or URL-encoded path,required"`
	IssueIID    int64                `json:"issue_iid"      jsonschema:"Issue internal ID,required"`
	ToProjectID int64                `json:"to_project_id"  jsonschema:"Target project ID to move issue to,required"`
}

// Move moves an issue from one project to another.
func Move(ctx context.Context, client *gitlabclient.Client, input MoveInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("issueMove: project_id is required")
	}
	if input.IssueIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issueMove", "issue_iid")
	}
	if input.ToProjectID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issueMove", "to_project_id")
	}
	opts := &gl.MoveIssueOptions{
		ToProjectID: new(input.ToProjectID),
	}
	issue, _, err := client.GL().Issues.MoveIssue(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("issueMove", err,
				"target project not found or you lack access. Use gitlab_project_list to verify the target project")
		}
		return Output{}, toolutil.WrapErrWithMessage("issueMove", err)
	}
	return ToOutput(issue), nil
}

// SubscribeInput defines parameters for subscribing to an issue.
type SubscribeInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID,required"`
}

// Subscribe subscribes the authenticated user to an issue for notifications.
func Subscribe(ctx context.Context, client *gitlabclient.Client, input SubscribeInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("issueSubscribe: project_id is required")
	}
	if input.IssueIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issueSubscribe", "issue_iid")
	}
	issue, _, err := client.GL().Issues.SubscribeToIssue(string(input.ProjectID), input.IssueIID, gl.WithContext(ctx))
	if err != nil {
		// GitLab returns 304 Not Modified with empty body when already subscribed,
		// which causes EOF during JSON decode. Fall back to Get.
		if errors.Is(err, io.EOF) || toolutil.IsHTTPStatus(err, http.StatusNotModified) {
			return Get(ctx, client, GetInput(input))
		}
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint("issueSubscribe", err,
				"verify project_id and issue_iid; use gitlab_issue_get to confirm the issue exists")
		}
		return Output{}, toolutil.WrapErrWithMessage("issueSubscribe", err)
	}
	return ToOutput(issue), nil
}

// UnsubscribeInput defines parameters for unsubscribing from an issue.
type UnsubscribeInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID,required"`
}

// Unsubscribe removes the authenticated user's subscription from an issue.
func Unsubscribe(ctx context.Context, client *gitlabclient.Client, input UnsubscribeInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("issueUnsubscribe: project_id is required")
	}
	if input.IssueIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("issueUnsubscribe", "issue_iid")
	}
	issue, _, err := client.GL().Issues.UnsubscribeFromIssue(string(input.ProjectID), input.IssueIID, gl.WithContext(ctx))
	if err != nil {
		if errors.Is(err, io.EOF) || toolutil.IsHTTPStatus(err, http.StatusNotModified) {
			return Get(ctx, client, GetInput(input))
		}
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint("issueUnsubscribe", err,
				"verify project_id and issue_iid; use gitlab_issue_get to confirm the issue exists")
		}
		return Output{}, toolutil.WrapErrWithMessage("issueUnsubscribe", err)
	}
	return ToOutput(issue), nil
}

// CreateTodoInput defines parameters for creating a to-do for an issue.
type CreateTodoInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID,required"`
}

// TodoOutput represents a to-do item created from an issue.
type TodoOutput struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	ActionName  string `json:"action_name"`
	TargetType  string `json:"target_type"`
	TargetTitle string `json:"target_title"`
	TargetURL   string `json:"target_url"`
	Body        string `json:"body,omitempty"`
	State       string `json:"state"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// CreateTodo creates a to-do item for the authenticated user on the specified issue.
func CreateTodo(ctx context.Context, client *gitlabclient.Client, input CreateTodoInput) (TodoOutput, error) {
	if err := ctx.Err(); err != nil {
		return TodoOutput{}, err
	}
	if input.ProjectID == "" {
		return TodoOutput{}, errors.New("issueCreateTodo: project_id is required")
	}
	if input.IssueIID <= 0 {
		return TodoOutput{}, toolutil.ErrRequiredInt64("issueCreateTodo", "issue_iid")
	}
	todo, _, err := client.GL().Issues.CreateTodo(string(input.ProjectID), input.IssueIID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return TodoOutput{}, toolutil.WrapErrWithHint("issueCreateTodo", err,
				"verify project_id and issue_iid; use gitlab_issue_get to confirm the issue exists")
		}
		return TodoOutput{}, toolutil.WrapErrWithMessage("issueCreateTodo", err)
	}
	out := TodoOutput{
		ID:         todo.ID,
		ActionName: string(todo.ActionName),
		TargetType: string(todo.TargetType),
		Body:       todo.Body,
		State:      todo.State,
	}
	if todo.Target != nil {
		out.TargetTitle = todo.Target.Title
		out.TargetURL = todo.Target.WebURL
	}
	if todo.CreatedAt != nil {
		out.CreatedAt = todo.CreatedAt.Format(time.RFC3339)
	}
	return out, nil
}

// Time tracking & related types.

// TimeStatsOutput represents time tracking statistics for an issue.
type TimeStatsOutput struct {
	toolutil.HintableOutput
	HumanTimeEstimate   string `json:"human_time_estimate"`
	HumanTotalTimeSpent string `json:"human_total_time_spent"`
	TimeEstimate        int64  `json:"time_estimate"`
	TotalTimeSpent      int64  `json:"total_time_spent"`
}

// timeStatsToOutput converts the GitLab API response to the tool output format.
func timeStatsToOutput(ts *gl.TimeStats) TimeStatsOutput {
	if ts == nil {
		return TimeStatsOutput{}
	}
	return TimeStatsOutput{
		HumanTimeEstimate:   ts.HumanTimeEstimate,
		HumanTotalTimeSpent: ts.HumanTotalTimeSpent,
		TimeEstimate:        ts.TimeEstimate,
		TotalTimeSpent:      ts.TotalTimeSpent,
	}
}

// SetTimeEstimateInput defines parameters for setting a time estimate on an issue.
type SetTimeEstimateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID,required"`
	Duration  string               `json:"duration"   jsonschema:"Human-readable duration (e.g. 3h30m, 1w2d),required"`
}

// SetTimeEstimate sets the time estimate for an issue.
func SetTimeEstimate(ctx context.Context, client *gitlabclient.Client, input SetTimeEstimateInput) (TimeStatsOutput, error) {
	if err := ctx.Err(); err != nil {
		return TimeStatsOutput{}, err
	}
	if input.ProjectID == "" {
		return TimeStatsOutput{}, errors.New("issueSetTimeEstimate: project_id is required")
	}
	if input.IssueIID <= 0 {
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("issueSetTimeEstimate", "issue_iid")
	}
	if input.Duration == "" {
		return TimeStatsOutput{}, errors.New("issueSetTimeEstimate: duration is required")
	}
	ts, _, err := client.GL().Issues.SetTimeEstimate(string(input.ProjectID), input.IssueIID,
		&gl.SetTimeEstimateOptions{Duration: new(input.Duration)}, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return TimeStatsOutput{}, toolutil.WrapErrWithHint("issueSetTimeEstimate", err,
				"duration must be in human-readable format like '3h30m', '1w2d', or '45m'; bare numbers are rejected")
		}
		return TimeStatsOutput{}, toolutil.WrapErrWithMessage("issueSetTimeEstimate", err)
	}
	return timeStatsToOutput(ts), nil
}

// ResetTimeEstimate resets the time estimate for an issue back to zero.
func ResetTimeEstimate(ctx context.Context, client *gitlabclient.Client, input GetInput) (TimeStatsOutput, error) {
	if err := ctx.Err(); err != nil {
		return TimeStatsOutput{}, err
	}
	if input.ProjectID == "" {
		return TimeStatsOutput{}, errors.New("issueResetTimeEstimate: project_id is required")
	}
	if input.IssueIID <= 0 {
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("issueResetTimeEstimate", "issue_iid")
	}
	ts, _, err := client.GL().Issues.ResetTimeEstimate(string(input.ProjectID), input.IssueIID, gl.WithContext(ctx))
	if err != nil {
		return TimeStatsOutput{}, toolutil.WrapErrWithMessage("issueResetTimeEstimate", err)
	}
	return timeStatsToOutput(ts), nil
}

// AddSpentTimeInput defines parameters for adding spent time to an issue.
type AddSpentTimeInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID,required"`
	Duration  string               `json:"duration"   jsonschema:"Human-readable duration (e.g. 1h, 30m, 1w2d),required"`
	Summary   string               `json:"summary,omitempty" jsonschema:"Optional summary of work done"`
}

// AddSpentTime adds spent time for an issue.
func AddSpentTime(ctx context.Context, client *gitlabclient.Client, input AddSpentTimeInput) (TimeStatsOutput, error) {
	if err := ctx.Err(); err != nil {
		return TimeStatsOutput{}, err
	}
	if input.ProjectID == "" {
		return TimeStatsOutput{}, errors.New("issueAddSpentTime: project_id is required")
	}
	if input.IssueIID <= 0 {
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("issueAddSpentTime", "issue_iid")
	}
	if input.Duration == "" {
		return TimeStatsOutput{}, errors.New("issueAddSpentTime: duration is required")
	}
	opts := &gl.AddSpentTimeOptions{Duration: new(input.Duration)}
	if input.Summary != "" {
		opts.Summary = new(input.Summary)
	}
	ts, _, err := client.GL().Issues.AddSpentTime(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return TimeStatsOutput{}, toolutil.WrapErrWithHint("issueAddSpentTime", err,
				"duration must be in human-readable format like '1h', '30m', or '1w2d'; bare numbers are rejected")
		}
		return TimeStatsOutput{}, toolutil.WrapErrWithMessage("issueAddSpentTime", err)
	}
	return timeStatsToOutput(ts), nil
}

// ResetSpentTime resets the total spent time for an issue.
func ResetSpentTime(ctx context.Context, client *gitlabclient.Client, input GetInput) (TimeStatsOutput, error) {
	if err := ctx.Err(); err != nil {
		return TimeStatsOutput{}, err
	}
	if input.ProjectID == "" {
		return TimeStatsOutput{}, errors.New("issueResetSpentTime: project_id is required")
	}
	if input.IssueIID <= 0 {
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("issueResetSpentTime", "issue_iid")
	}
	ts, _, err := client.GL().Issues.ResetSpentTime(string(input.ProjectID), input.IssueIID, gl.WithContext(ctx))
	if err != nil {
		return TimeStatsOutput{}, toolutil.WrapErrWithMessage("issueResetSpentTime", err)
	}
	return timeStatsToOutput(ts), nil
}

// GetTimeStats retrieves total time tracking statistics for an issue.
func GetTimeStats(ctx context.Context, client *gitlabclient.Client, input GetInput) (TimeStatsOutput, error) {
	if err := ctx.Err(); err != nil {
		return TimeStatsOutput{}, err
	}
	if input.ProjectID == "" {
		return TimeStatsOutput{}, errors.New("issueGetTimeStats: project_id is required")
	}
	if input.IssueIID <= 0 {
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("issueGetTimeStats", "issue_iid")
	}
	ts, _, err := client.GL().Issues.GetTimeSpent(string(input.ProjectID), input.IssueIID, gl.WithContext(ctx))
	if err != nil {
		return TimeStatsOutput{}, toolutil.WrapErrWithMessage("issueGetTimeStats", err)
	}
	return timeStatsToOutput(ts), nil
}

// ParticipantOutput represents a participant in an issue.
type ParticipantOutput struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	WebURL   string `json:"web_url"`
}

// ParticipantsOutput holds a list of issue participants.
type ParticipantsOutput struct {
	toolutil.HintableOutput
	Participants []ParticipantOutput `json:"participants"`
}

// GetParticipants retrieves the list of participants in an issue.
func GetParticipants(ctx context.Context, client *gitlabclient.Client, input GetInput) (ParticipantsOutput, error) {
	if err := ctx.Err(); err != nil {
		return ParticipantsOutput{}, err
	}
	if input.ProjectID == "" {
		return ParticipantsOutput{}, errors.New("issueGetParticipants: project_id is required")
	}
	if input.IssueIID <= 0 {
		return ParticipantsOutput{}, toolutil.ErrRequiredInt64("issueGetParticipants", "issue_iid")
	}
	users, _, err := client.GL().Issues.GetParticipants(string(input.ProjectID), input.IssueIID, gl.WithContext(ctx))
	if err != nil {
		return ParticipantsOutput{}, toolutil.WrapErrWithMessage("issueGetParticipants", err)
	}
	out := make([]ParticipantOutput, len(users))
	for i, u := range users {
		out[i] = ParticipantOutput{
			ID:       u.ID,
			Username: u.Username,
			Name:     u.Name,
			WebURL:   u.WebURL,
		}
	}
	return ParticipantsOutput{Participants: out}, nil
}

// RelatedMROutput represents a basic merge request linked to an issue.
type RelatedMROutput struct {
	ID           int64  `json:"id"`
	IID          int64  `json:"mr_iid"`
	Title        string `json:"title"`
	State        string `json:"state"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	Author       string `json:"author"`
	WebURL       string `json:"web_url"`
}

// RelatedMRsOutput holds a paginated list of merge requests related to an issue.
type RelatedMRsOutput struct {
	toolutil.HintableOutput
	MergeRequests []RelatedMROutput         `json:"merge_requests"`
	Pagination    toolutil.PaginationOutput `json:"pagination"`
}

// basicMRToOutput converts the GitLab API response to the tool output format.
func basicMRToOutput(mr *gl.BasicMergeRequest) RelatedMROutput {
	out := RelatedMROutput{
		ID:           mr.ID,
		IID:          mr.IID,
		Title:        mr.Title,
		State:        mr.State,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		WebURL:       mr.WebURL,
	}
	if mr.Author != nil {
		out.Author = mr.Author.Username
	}
	return out
}

// ListMRsClosingInput defines parameters for listing MRs that close an issue on merge.
type ListMRsClosingInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID,required"`
	toolutil.PaginationInput
}

// ListMRsClosing retrieves merge requests that will close this issue on merge.
func ListMRsClosing(ctx context.Context, client *gitlabclient.Client, input ListMRsClosingInput) (RelatedMRsOutput, error) {
	if err := ctx.Err(); err != nil {
		return RelatedMRsOutput{}, err
	}
	if input.ProjectID == "" {
		return RelatedMRsOutput{}, errors.New("issueListMRsClosing: project_id is required")
	}
	if input.IssueIID <= 0 {
		return RelatedMRsOutput{}, toolutil.ErrRequiredInt64("issueListMRsClosing", "issue_iid")
	}
	opts := &gl.ListMergeRequestsClosingIssueOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	mrs, resp, err := client.GL().Issues.ListMergeRequestsClosingIssue(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		return RelatedMRsOutput{}, toolutil.WrapErrWithMessage("issueListMRsClosing", err)
	}
	out := make([]RelatedMROutput, len(mrs))
	for i, mr := range mrs {
		out[i] = basicMRToOutput(mr)
	}
	return RelatedMRsOutput{MergeRequests: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ListMRsRelatedInput defines parameters for listing MRs related to an issue.
type ListMRsRelatedInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID,required"`
	toolutil.PaginationInput
}

// ListMRsRelated retrieves merge requests related to this issue.
func ListMRsRelated(ctx context.Context, client *gitlabclient.Client, input ListMRsRelatedInput) (RelatedMRsOutput, error) {
	if err := ctx.Err(); err != nil {
		return RelatedMRsOutput{}, err
	}
	if input.ProjectID == "" {
		return RelatedMRsOutput{}, errors.New("issueListMRsRelated: project_id is required")
	}
	if input.IssueIID <= 0 {
		return RelatedMRsOutput{}, toolutil.ErrRequiredInt64("issueListMRsRelated", "issue_iid")
	}
	opts := &gl.ListMergeRequestsRelatedToIssueOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	mrs, resp, err := client.GL().Issues.ListMergeRequestsRelatedToIssue(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		return RelatedMRsOutput{}, toolutil.WrapErrWithMessage("issueListMRsRelated", err)
	}
	out := make([]RelatedMROutput, len(mrs))
	for i, mr := range mrs {
		out[i] = basicMRToOutput(mr)
	}
	return RelatedMRsOutput{MergeRequests: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// Markdown formatting.

// prefixAt adds '@' before each username for Markdown @mention formatting.
func prefixAt(usernames []string) []string {
	result := make([]string, len(usernames))
	for i, u := range usernames {
		result[i] = "@" + u
	}
	return result
}
