package issues

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Shared hints used by error wrappers (kept as constants to satisfy S1192).
const (
	hintVerifyIssue        = "verify project_id and issue_iid with gitlab_issue_get"
	hintConfirmIssueExists = "verify project_id and issue_iid; use gitlab_issue_get to confirm the issue exists"
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
	IID         int64                `json:"iid,omitempty" jsonschema:"Explicit issue IID to assign (requires admin permissions. Normally GitLab assigns the next IID automatically)"`
	Description string               `json:"description,omitempty" jsonschema:"Issue description (Markdown supported)"`
	IssueType   string               `json:"issue_type,omitempty" jsonschema:"Issue type (issue, incident, test_case, task)"`

	// Assignment and tracking
	AssigneeID  int64    `json:"assignee_id,omitempty" jsonschema:"Single user ID to assign (use assignee_ids for multiple)"`
	AssigneeIDs []int64  `json:"assignee_ids,omitempty" jsonschema:"User IDs to assign"`
	Labels      []string `json:"labels,omitempty" jsonschema:"Label names to apply"`
	MilestoneID int64    `json:"milestone_id,omitempty" jsonschema:"Milestone ID to associate"`
	EpicID      int64    `json:"epic_id,omitempty" tier:"premium" jsonschema:"Epic ID to associate the issue with"`
	Weight      int64    `json:"weight,omitempty" tier:"premium" jsonschema:"Issue weight, 0 or higher"`
	DueDate     string   `json:"due_date,omitempty" jsonschema:"Due date in YYYY-MM-DD format"`

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
	ID int64 `json:"id"`
	// IID is the issue's own project-scoped internal ID, mirroring the SDK
	// json key `iid`. Distinct from the global `id`.
	IID         int64    `json:"iid" jsonschema:"Project-scoped internal ID (the #N shown in GitLab). Pass this value as issue_iid to other issue operations. Distinct from the global id."`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	State       string   `json:"state"`
	Labels      []string `json:"labels"`
	// Author is the full author object mirrored from the SDK (1:1 fidelity).
	Author *toolutil.IssueUserOutput `json:"author,omitempty"`
	// Assignees is the list of full assignee objects mirrored from the SDK.
	Assignees []*toolutil.IssueUserOutput `json:"assignees"`
	// Assignee is the singular (deprecated upstream) assignee object.
	Assignee *toolutil.IssueUserOutput `json:"assignee,omitempty"`
	// Milestone is the full milestone object mirrored from the SDK.
	Milestone *toolutil.MRMilestoneOutput `json:"milestone,omitempty"`
	// ClosedBy is the full closer object mirrored from the SDK.
	ClosedBy          *toolutil.IssueUserOutput `json:"closed_by,omitempty"`
	WebURL            string                    `json:"web_url"`
	CreatedAt         string                    `json:"created_at"`
	UpdatedAt         string                    `json:"updated_at"`
	ClosedAt          string                    `json:"closed_at"`
	DueDate           string                    `json:"due_date"`
	Confidential      bool                      `json:"confidential"`
	DiscussionLocked  bool                      `json:"discussion_locked"`
	ProjectID         int64                     `json:"project_id"`
	Weight            int64                     `json:"weight,omitempty" tier:"premium"`
	IssueType         string                    `json:"issue_type,omitempty"`
	HealthStatus      string                    `json:"health_status,omitempty" tier:"ultimate"`
	MergeRequestCount int64                     `json:"merge_requests_count,omitempty"`
	UserNotesCount    int64                     `json:"user_notes_count,omitempty"`
	Upvotes           int64                     `json:"upvotes,omitempty"`
	Downvotes         int64                     `json:"downvotes,omitempty"`
	Subscribed        bool                      `json:"subscribed"`
	MovedToID         int64                     `json:"moved_to_id,omitempty"`
	EpicIssueID       int64                     `json:"epic_issue_id,omitempty" tier:"premium"`
	// Additive 1:1 fields surfaced from the SDK Issue (full sub-objects and
	// scalars not previously exposed).
	ExternalID           string                               `json:"external_id,omitempty"`
	IssueLinkID          int64                                `json:"issue_link_id,omitempty"`
	ServiceDeskReplyTo   string                               `json:"service_desk_reply_to,omitempty"`
	References           *toolutil.ReferencesOutput           `json:"references,omitempty"`
	Epic                 *toolutil.EpicOutput                 `json:"epic,omitempty" tier:"premium"`
	LabelDetails         []*toolutil.LabelDetailsOutput       `json:"label_details,omitempty"`
	Iteration            *toolutil.IterationOutput            `json:"iteration,omitempty" tier:"premium"`
	Links                *toolutil.IssueLinksOutput           `json:"_links,omitempty"`
	TimeStats            *TimeStatsOutput                     `json:"time_stats,omitempty"`
	TaskCompletionStatus *toolutil.TaskCompletionStatusOutput `json:"task_completion_status,omitempty"`
}

// GetInput defines parameters for retrieving a single issue.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue IID (project-scoped internal ID, not 'issue_id'),required"`
}

// ListInput defines filters for listing project issues.
type ListInput struct {
	ProjectID           toolutil.StringOrInt `json:"project_id"                     jsonschema:"Project ID or URL-encoded path,required"`
	State               string               `json:"state,omitempty"                jsonschema:"Filter by state (opened, closed, all)"`
	Labels              []string             `json:"labels,omitempty"               jsonschema:"Label names to filter by"`
	NotLabels           []string             `json:"not_labels,omitempty"           jsonschema:"Label names to exclude"`
	WithLabelsDetails   *bool                `json:"with_labels_details,omitempty"  jsonschema:"Return label objects with full details (id, name, color, description) instead of just names"`
	Milestone           string               `json:"milestone,omitempty"            jsonschema:"Milestone title to filter by"`
	NotMilestone        string               `json:"not_milestone,omitempty"        jsonschema:"Milestone title to exclude"`
	Scope               string               `json:"scope,omitempty"                jsonschema:"Filter by scope (created_by_me, assigned_to_me, all)"`
	Search              string               `json:"search,omitempty"               jsonschema:"Search in title and description"`
	In                  string               `json:"in,omitempty"                   jsonschema:"Fields the search query applies to (title, description, or title,description)"`
	NotIn               string               `json:"not_in,omitempty"               jsonschema:"Fields the negated search query applies to (title, description)"`
	AssigneeID          *int64               `json:"assignee_id,omitempty"          jsonschema:"Filter by assignee user ID"`
	NotAssigneeID       *int64               `json:"not_assignee_id,omitempty"      jsonschema:"Exclude issues assigned to this user ID"`
	AssigneeUsername    string               `json:"assignee_username,omitempty"    jsonschema:"Filter by assignee username"`
	NotAssigneeUsername string               `json:"not_assignee_username,omitempty" jsonschema:"Exclude issues assigned to this username"`
	AuthorID            *int64               `json:"author_id,omitempty"            jsonschema:"Filter by author user ID"`
	NotAuthorID         *int64               `json:"not_author_id,omitempty"        jsonschema:"Exclude issues authored by this user ID"`
	AuthorUsername      string               `json:"author_username,omitempty"      jsonschema:"Filter by author username"`
	NotAuthorUsername   string               `json:"not_author_username,omitempty"  jsonschema:"Exclude issues authored by this username"`
	MyReactionEmoji     string               `json:"my_reaction_emoji,omitempty"    jsonschema:"Filter by issues you reacted to with this emoji (e.g. thumbsup, or None/Any)"`
	NotMyReactionEmoji  string               `json:"not_my_reaction_emoji,omitempty" jsonschema:"Exclude issues you reacted to with this emoji"`
	IIDs                []int64              `json:"iids,omitempty"                 jsonschema:"Filter by issue internal IDs"`
	IssueType           string               `json:"issue_type,omitempty"           jsonschema:"Filter by issue type (issue, incident, test_case, task)"`
	IterationID         *int64               `json:"iteration_id,omitempty"         tier:"premium" jsonschema:"Filter by iteration ID"`
	Confidential        *bool                `json:"confidential,omitempty"         jsonschema:"Filter by confidential status"`
	DueDate             string               `json:"due_date,omitempty"             jsonschema:"Filter by due date (0=no due date, overdue, week, month, next_month_and_previous_two_weeks)"`
	CreatedAfter        string               `json:"created_after,omitempty"        jsonschema:"Return issues created after date (ISO 8601 format, e.g. 2025-01-01T00:00:00Z)"`
	CreatedBefore       string               `json:"created_before,omitempty"       jsonschema:"Return issues created before date (ISO 8601 format, e.g. 2025-12-31T23:59:59Z)"`
	UpdatedAfter        string               `json:"updated_after,omitempty"        jsonschema:"Return issues updated after date (ISO 8601 format, e.g. 2025-01-01T00:00:00Z)"`
	UpdatedBefore       string               `json:"updated_before,omitempty"       jsonschema:"Return issues updated before date (ISO 8601 format, e.g. 2025-12-31T23:59:59Z)"`
	OrderBy             string               `json:"order_by,omitempty"             jsonschema:"Order by field (created_at, updated_at, priority, due_date, relative_position, label_priority, milestone_due, popularity, weight)"`
	Sort                string               `json:"sort,omitempty"                 jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	Labels           []string             `json:"labels,omitempty"        jsonschema:"Label names to replace all existing"`
	AddLabels        []string             `json:"add_labels,omitempty"    jsonschema:"Label names to add without removing existing"`
	RemoveLabels     []string             `json:"remove_labels,omitempty" jsonschema:"Label names to remove"`
	EpicID           int64                `json:"epic_id,omitempty"       tier:"premium" jsonschema:"Epic ID to associate"`
	MilestoneID      *int64               `json:"milestone_id,omitempty"  jsonschema:"New milestone ID (0 to unset. Omit to leave unchanged)"`
	DueDate          string               `json:"due_date,omitempty"      jsonschema:"New due date in YYYY-MM-DD format"`
	Confidential     *bool                `json:"confidential,omitempty"  jsonschema:"Update confidential flag"`
	IssueType        string               `json:"issue_type,omitempty"    jsonschema:"Issue type (issue, incident, test_case, task)"`
	Weight           int64                `json:"weight,omitempty"        tier:"premium" jsonschema:"Issue weight, 0 or higher"`
	DiscussionLocked *bool                `json:"discussion_locked,omitempty" jsonschema:"Lock discussions on this issue"`
	UpdatedAt        string               `json:"updated_at,omitempty"    jsonschema:"Update timestamp override (ISO 8601/RFC 3339, requires admin permissions)"`
}

// DeleteInput defines parameters for deleting an issue.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue IID (project-scoped internal ID, not 'issue_id'),required"`
}

// ListGroupInput defines parameters for listing issues across a group.
type ListGroupInput struct {
	GroupID             toolutil.StringOrInt `json:"group_id"                       jsonschema:"Group ID or URL-encoded path,required"`
	State               string               `json:"state,omitempty"                jsonschema:"Filter by state (opened, closed, all)"`
	Labels              []string             `json:"labels,omitempty"               jsonschema:"Label names to filter by"`
	NotLabels           []string             `json:"not_labels,omitempty"           jsonschema:"Label names to exclude"`
	WithLabelsDetails   *bool                `json:"with_labels_details,omitempty"  jsonschema:"Return label objects with full details instead of just names"`
	Milestone           string               `json:"milestone,omitempty"            jsonschema:"Milestone title to filter by"`
	NotMilestone        string               `json:"not_milestone,omitempty"        jsonschema:"Milestone title to exclude"`
	Scope               string               `json:"scope,omitempty"                jsonschema:"Scope (created_by_me, assigned_to_me, all)"`
	Search              string               `json:"search,omitempty"               jsonschema:"Search in title and description"`
	NotSearch           string               `json:"not_search,omitempty"           jsonschema:"Exclude issues matching this search text"`
	In                  string               `json:"in,omitempty"                   jsonschema:"Fields the search query applies to (title, description, or title,description)"`
	NotIn               string               `json:"not_in,omitempty"               jsonschema:"Fields the negated search query applies to (title, description)"`
	AssigneeID          *int64               `json:"assignee_id,omitempty"          jsonschema:"Filter by assignee user ID"`
	NotAssigneeID       *int64               `json:"not_assignee_id,omitempty"      jsonschema:"Exclude issues assigned to this user ID"`
	AssigneeUsername    string               `json:"assignee_username,omitempty"    jsonschema:"Filter by assignee username"`
	NotAssigneeUsername string               `json:"not_assignee_username,omitempty" jsonschema:"Exclude issues assigned to this username"`
	AuthorID            *int64               `json:"author_id,omitempty"            jsonschema:"Filter by author user ID"`
	NotAuthorID         *int64               `json:"not_author_id,omitempty"        jsonschema:"Exclude issues authored by this user ID"`
	AuthorUsername      string               `json:"author_username,omitempty"      jsonschema:"Filter by author username"`
	NotAuthorUsername   string               `json:"not_author_username,omitempty"  jsonschema:"Exclude issues authored by this username"`
	MyReactionEmoji     string               `json:"my_reaction_emoji,omitempty"    jsonschema:"Filter by issues you reacted to with this emoji (or None/Any)"`
	NotMyReactionEmoji  string               `json:"not_my_reaction_emoji,omitempty" jsonschema:"Exclude issues you reacted to with this emoji"`
	IIDs                []int64              `json:"iids,omitempty"                 jsonschema:"Filter by issue internal IDs"`
	IssueType           string               `json:"issue_type,omitempty"           jsonschema:"Filter by issue type (issue, incident, test_case, task)"`
	IterationID         *int64               `json:"iteration_id,omitempty"         tier:"premium" jsonschema:"Filter by iteration ID"`
	Confidential        *bool                `json:"confidential,omitempty"         jsonschema:"Filter by confidential status"`
	DueDate             string               `json:"due_date,omitempty"             jsonschema:"Filter by due date (0, overdue, week, month, next_month_and_previous_two_weeks)"`
	OrderBy             string               `json:"order_by,omitempty"             jsonschema:"Order by field (created_at, updated_at, priority, due_date, relative_position, label_priority, milestone_due, popularity, weight)"`
	Sort                string               `json:"sort,omitempty"                 jsonschema:"Sort direction (asc, desc)"`
	CreatedAfter        string               `json:"created_after,omitempty"        jsonschema:"Return issues created after date (ISO 8601)"`
	CreatedBefore       string               `json:"created_before,omitempty"       jsonschema:"Return issues created before date (ISO 8601)"`
	UpdatedAfter        string               `json:"updated_after,omitempty"        jsonschema:"Return issues updated after date (ISO 8601)"`
	UpdatedBefore       string               `json:"updated_before,omitempty"       jsonschema:"Return issues updated before date (ISO 8601)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	out.Author = toolutil.NewIssueUserOutputFromIssueAuthor(issue.Author)
	out.Milestone = mrMilestoneOutputPtr(issue.Milestone)
	out.Assignees = toolutil.NewIssueUserOutputsFromIssueAssignees(issue.Assignees)
	if out.Assignees == nil {
		out.Assignees = []*toolutil.IssueUserOutput{}
	}
	// The singular Assignee field is deprecated upstream in favor of Assignees,
	// but the 1:1 audit requires surfacing every SDK field, so it is mirrored
	// verbatim.
	out.Assignee = toolutil.NewIssueUserOutputFromIssueAssignee(issue.Assignee) //nolint:staticcheck // SA1019: surfaced for 1:1 SDK fidelity
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
	out.ClosedBy = toolutil.NewIssueUserOutputFromIssueCloser(issue.ClosedBy)
	out.References = toolutil.NewReferencesOutput(issue.References)
	out.UserNotesCount = issue.UserNotesCount
	out.Upvotes = issue.Upvotes
	out.Downvotes = issue.Downvotes
	out.Subscribed = issue.Subscribed
	out.MovedToID = issue.MovedToID
	out.EpicIssueID = issue.EpicIssueID
	out.ExternalID = issue.ExternalID
	out.IssueLinkID = issue.IssueLinkID
	out.ServiceDeskReplyTo = issue.ServiceDeskReplyTo
	out.Epic = toolutil.NewEpicOutput(issue.Epic)
	out.LabelDetails = toolutil.NewLabelDetailsOutputs(issue.LabelDetails)
	out.Iteration = toolutil.NewIterationOutputFromGroupIteration(issue.Iteration)
	out.Links = toolutil.NewIssueLinksOutput(issue.Links)
	out.TimeStats = timeStatsPtr(issue.TimeStats)
	out.TaskCompletionStatus = toolutil.NewTaskCompletionStatusOutput(issue.TaskCompletionStatus)
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
				"check that referenced labels, assignee_ids and milestone_id exist in this project (use gitlab_label_list, gitlab_get_user, gitlab_milestone_list)")
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
	if input.IID != 0 {
		opts.IID = new(input.IID)
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
	opts.Labels = labelOptions(input.Labels)
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

// optStr returns a pointer to s, or nil when s is empty, so optional string
// filters are omitted from the query rather than sent as an empty value.
func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// labelOptions converts a label-name slice into a *gl.LabelOptions, or nil when
// empty.
func labelOptions(values []string) *gl.LabelOptions {
	if len(values) == 0 {
		return nil
	}
	labels := gl.LabelOptions(values)
	return &labels
}

// optIIDs returns a pointer to the IID slice, or nil when empty.
func optIIDs(iids []int64) *[]int64 {
	if len(iids) == 0 {
		return nil
	}
	return &iids
}

// optStrings returns a pointer to a string slice, or nil when empty.
func optStrings(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	return &values
}

// optAssignee builds a *gl.AssigneeIDValue from an optional user ID, or nil.
func optAssignee(id *int64) *gl.AssigneeIDValue {
	if id == nil {
		return nil
	}
	return gl.AssigneeID(*id)
}

// List retrieves a paginated list of issues for a GitLab project.
// Supports filtering by state, labels, milestone, search text, assignee,
// author, negation filters, iteration, and sorting. Returns the issues with
// pagination metadata.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("issueList: project_id is required. Use gitlab_project_list to find the project ID first, then pass it as project_id")
	}
	opts := &gl.ListProjectIssuesOptions{
		State:               optStr(input.State),
		Labels:              labelOptions(input.Labels),
		NotLabels:           labelOptions(input.NotLabels),
		WithLabelDetails:    input.WithLabelsDetails,
		Milestone:           optStr(input.Milestone),
		NotMilestone:        optStr(input.NotMilestone),
		Scope:               optStr(input.Scope),
		Search:              optStr(input.Search),
		In:                  optStr(input.In),
		NotIn:               optStr(input.NotIn),
		AssigneeID:          optAssignee(input.AssigneeID),
		NotAssigneeID:       input.NotAssigneeID,
		AssigneeUsername:    optStr(input.AssigneeUsername),
		NotAssigneeUsername: optStr(input.NotAssigneeUsername),
		AuthorID:            input.AuthorID,
		NotAuthorID:         input.NotAuthorID,
		AuthorUsername:      optStr(input.AuthorUsername),
		NotAuthorUsername:   optStr(input.NotAuthorUsername),
		MyReactionEmoji:     optStr(input.MyReactionEmoji),
		NotMyReactionEmoji:  optStr(input.NotMyReactionEmoji),
		IIDs:                optIIDs(input.IIDs),
		IssueType:           optStr(input.IssueType),
		IterationID:         input.IterationID,
		Confidential:        input.Confidential,
		DueDate:             optStr(input.DueDate),
		OrderBy:             optStr(input.OrderBy),
		Sort:                optStr(input.Sort),
		CreatedAfter:        toolutil.ParseOptionalTime(input.CreatedAfter),
		CreatedBefore:       toolutil.ParseOptionalTime(input.CreatedBefore),
		UpdatedAfter:        toolutil.ParseOptionalTime(input.UpdatedAfter),
		UpdatedBefore:       toolutil.ParseOptionalTime(input.UpdatedBefore),
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	issues, resp, err := client.GL().Issues.ListProjectIssues(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("issueList", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get \u2014 issues must be enabled in project settings")
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
	opts.Labels = labelOptions(input.Labels)
	opts.AddLabels = labelOptions(input.AddLabels)
	opts.RemoveLabels = labelOptions(input.RemoveLabels)
	if input.MilestoneID != nil {
		opts.MilestoneID = input.MilestoneID
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
	if input.UpdatedAt != "" {
		t, err := time.Parse(time.RFC3339, input.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("issueUpdate: invalid updated_at format (expected ISO 8601/RFC 3339): %w", err)
		}
		opts.UpdatedAt = &t
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

	opts := &gl.ListGroupIssuesOptions{
		State:               optStr(input.State),
		Labels:              labelOptions(input.Labels),
		NotLabels:           labelOptions(input.NotLabels),
		WithLabelDetails:    input.WithLabelsDetails,
		Milestone:           optStr(input.Milestone),
		NotMilestone:        optStr(input.NotMilestone),
		Scope:               optStr(input.Scope),
		Search:              optStr(input.Search),
		NotSearch:           optStr(input.NotSearch),
		In:                  optStr(input.In),
		NotIn:               optStr(input.NotIn),
		AssigneeID:          optAssignee(input.AssigneeID),
		NotAssigneeID:       input.NotAssigneeID,
		AssigneeUsername:    optStr(input.AssigneeUsername),
		NotAssigneeUsername: optStr(input.NotAssigneeUsername),
		AuthorID:            input.AuthorID,
		NotAuthorID:         input.NotAuthorID,
		AuthorUsername:      optStr(input.AuthorUsername),
		NotAuthorUsername:   optStr(input.NotAuthorUsername),
		MyReactionEmoji:     optStr(input.MyReactionEmoji),
		NotMyReactionEmoji:  optStr(input.NotMyReactionEmoji),
		IIDs:                optIIDs(input.IIDs),
		IssueType:           optStr(input.IssueType),
		IterationID:         input.IterationID,
		Confidential:        input.Confidential,
		DueDate:             optStr(input.DueDate),
		OrderBy:             optStr(input.OrderBy),
		Sort:                optStr(input.Sort),
		CreatedAfter:        toolutil.ParseOptionalTime(input.CreatedAfter),
		CreatedBefore:       toolutil.ParseOptionalTime(input.CreatedBefore),
		UpdatedAfter:        toolutil.ParseOptionalTime(input.UpdatedAfter),
		UpdatedBefore:       toolutil.ParseOptionalTime(input.UpdatedBefore),
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)

	issues, resp, err := client.GL().Issues.ListGroupIssues(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListGroupOutput{}, toolutil.WrapErrWithStatusHint("issueListGroup", err, http.StatusNotFound,
			"verify the group exists with gitlab_group_get \u2014 use full_path or numeric ID")
	}

	out := make([]Output, len(issues))
	for i, issue := range issues {
		out[i] = ToOutput(issue)
	}
	return ListGroupOutput{Issues: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ListAllInput defines parameters for the global ListIssues endpoint (no project scope).
type ListAllInput struct {
	State               string   `json:"state,omitempty"                jsonschema:"Filter by state (opened, closed, all)"`
	Labels              []string `json:"labels,omitempty"               jsonschema:"Label names to filter by"`
	NotLabels           []string `json:"not_labels,omitempty"           jsonschema:"Label names to exclude"`
	WithLabelsDetails   *bool    `json:"with_labels_details,omitempty"  jsonschema:"Return label objects with full details instead of just names"`
	Milestone           string   `json:"milestone,omitempty"            jsonschema:"Milestone title to filter by"`
	NotMilestone        string   `json:"not_milestone,omitempty"        jsonschema:"Milestone title to exclude"`
	Scope               string   `json:"scope,omitempty"                jsonschema:"Filter by scope (created_by_me, assigned_to_me, all)"`
	Search              string   `json:"search,omitempty"               jsonschema:"Search in title and description"`
	NotSearch           string   `json:"not_search,omitempty"           jsonschema:"Exclude issues matching this search text"`
	In                  string   `json:"in,omitempty"                   jsonschema:"Fields the search query applies to (title, description, or title,description)"`
	NotIn               string   `json:"not_in,omitempty"               jsonschema:"Fields the negated search query applies to (title, description)"`
	AssigneeID          *int64   `json:"assignee_id,omitempty"          jsonschema:"Filter by assignee user ID"`
	NotAssigneeID       []int64  `json:"not_assignee_id,omitempty"      jsonschema:"Exclude issues assigned to these user IDs"`
	AssigneeUsername    string   `json:"assignee_username,omitempty"    jsonschema:"Filter by assignee username"`
	NotAssigneeUsername string   `json:"not_assignee_username,omitempty" jsonschema:"Exclude issues assigned to this username"`
	AuthorID            *int64   `json:"author_id,omitempty"            jsonschema:"Filter by author user ID"`
	NotAuthorID         []int64  `json:"not_author_id,omitempty"        jsonschema:"Exclude issues authored by these user IDs"`
	AuthorUsername      string   `json:"author_username,omitempty"      jsonschema:"Filter by author username"`
	NotAuthorUsername   string   `json:"not_author_username,omitempty"  jsonschema:"Exclude issues authored by this username"`
	MyReactionEmoji     string   `json:"my_reaction_emoji,omitempty"    jsonschema:"Filter by issues you reacted to with this emoji (or None/Any)"`
	NotMyReactionEmoji  []string `json:"not_my_reaction_emoji,omitempty" jsonschema:"Exclude issues you reacted to with these emoji"`
	IIDs                []int64  `json:"iids,omitempty"                 jsonschema:"Filter by issue internal IDs"`
	IssueType           string   `json:"issue_type,omitempty"           jsonschema:"Filter by issue type (issue, incident, test_case, task)"`
	IterationID         *int64   `json:"iteration_id,omitempty"         tier:"premium" jsonschema:"Filter by iteration ID"`
	Confidential        *bool    `json:"confidential,omitempty"         jsonschema:"Filter by confidential status"`
	DueDate             string   `json:"due_date,omitempty"             jsonschema:"Filter by due date (0, overdue, week, month, next_month_and_previous_two_weeks)"`
	OrderBy             string   `json:"order_by,omitempty"             jsonschema:"Order by field (created_at, updated_at, priority, due_date, relative_position, label_priority, milestone_due, popularity, weight)"`
	Sort                string   `json:"sort,omitempty"                 jsonschema:"Sort direction (asc, desc)"`
	CreatedAfter        string   `json:"created_after,omitempty"        jsonschema:"Return issues created after date (ISO 8601)"`
	CreatedBefore       string   `json:"created_before,omitempty"       jsonschema:"Return issues created before date (ISO 8601)"`
	UpdatedAfter        string   `json:"updated_after,omitempty"        jsonschema:"Return issues updated after date (ISO 8601)"`
	UpdatedBefore       string   `json:"updated_before,omitempty"       jsonschema:"Return issues updated before date (ISO 8601)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListAll retrieves a paginated list of issues visible to the authenticated user
// across all projects (global scope).
func ListAll(ctx context.Context, client *gitlabclient.Client, input ListAllInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}

	opts := &gl.ListIssuesOptions{
		State:               optStr(input.State),
		Labels:              labelOptions(input.Labels),
		NotLabels:           labelOptions(input.NotLabels),
		WithLabelDetails:    input.WithLabelsDetails,
		Milestone:           optStr(input.Milestone),
		NotMilestone:        optStr(input.NotMilestone),
		Scope:               optStr(input.Scope),
		Search:              optStr(input.Search),
		NotSearch:           optStr(input.NotSearch),
		In:                  optStr(input.In),
		NotIn:               optStr(input.NotIn),
		AssigneeID:          optAssignee(input.AssigneeID),
		NotAssigneeID:       optIIDs(input.NotAssigneeID),
		AssigneeUsername:    optStr(input.AssigneeUsername),
		NotAssigneeUsername: optStr(input.NotAssigneeUsername),
		AuthorID:            input.AuthorID,
		NotAuthorID:         optIIDs(input.NotAuthorID),
		AuthorUsername:      optStr(input.AuthorUsername),
		NotAuthorUsername:   optStr(input.NotAuthorUsername),
		MyReactionEmoji:     optStr(input.MyReactionEmoji),
		NotMyReactionEmoji:  optStrings(input.NotMyReactionEmoji),
		IIDs:                optIIDs(input.IIDs),
		IssueType:           optStr(input.IssueType),
		IterationID:         input.IterationID,
		Confidential:        input.Confidential,
		DueDate:             optStr(input.DueDate),
		OrderBy:             optStr(input.OrderBy),
		Sort:                optStr(input.Sort),
		CreatedAfter:        toolutil.ParseOptionalTime(input.CreatedAfter),
		CreatedBefore:       toolutil.ParseOptionalTime(input.CreatedBefore),
		UpdatedAfter:        toolutil.ParseOptionalTime(input.UpdatedAfter),
		UpdatedBefore:       toolutil.ParseOptionalTime(input.UpdatedBefore),
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)

	result, resp, err := client.GL().Issues.ListIssues(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("issueListAll", err, http.StatusUnauthorized,
			"global issue listing requires an authenticated token; results are scoped to issues visible to the calling user (use scope=created_by_me or scope=assigned_to_me to narrow)")
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
		return Output{}, toolutil.WrapErrWithStatusHint("issueGetByID", err, http.StatusNotFound,
			"issue_id is the global database ID, not the project-scoped iid; for iid lookups use gitlab_issue_get with project_id+issue_iid")
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
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("issueReorder", err,
				"exactly one of move_after_id or move_before_id must be set, and the referenced issue must be in the same project as issue_iid")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("issueReorder", err, http.StatusNotFound,
			hintVerifyIssue)
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
	return changeIssueSubscription(ctx, client, input.ProjectID, input.IssueIID, "issueSubscribe",
		func(projectID string, issueIID int64) (*gl.Issue, *gl.Response, error) {
			return client.GL().Issues.SubscribeToIssue(projectID, issueIID, gl.WithContext(ctx))
		})
}

// UnsubscribeInput defines parameters for unsubscribing from an issue.
type UnsubscribeInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID,required"`
}

// Unsubscribe removes the authenticated user's subscription from an issue.
func Unsubscribe(ctx context.Context, client *gitlabclient.Client, input UnsubscribeInput) (Output, error) {
	return changeIssueSubscription(ctx, client, input.ProjectID, input.IssueIID, "issueUnsubscribe",
		func(projectID string, issueIID int64) (*gl.Issue, *gl.Response, error) {
			return client.GL().Issues.UnsubscribeFromIssue(projectID, issueIID, gl.WithContext(ctx))
		})
}

// changeIssueSubscription is the shared implementation behind [Subscribe]
// and [Unsubscribe]. It calls the provided change function (which performs
// the subscribe or unsubscribe API call) and falls back to a fresh [Get]
// when GitLab returns io.EOF or 304 Not Modified (no change occurred).
func changeIssueSubscription(ctx context.Context, client *gitlabclient.Client, projectID toolutil.StringOrInt, issueIID int64, operation string, change func(string, int64) (*gl.Issue, *gl.Response, error)) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if projectID == "" {
		return Output{}, fmt.Errorf("%s: project_id is required", operation)
	}
	if issueIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64(operation, "issue_iid")
	}
	issue, _, err := change(string(projectID), issueIID)
	if err != nil {
		if errors.Is(err, io.EOF) || toolutil.IsHTTPStatus(err, http.StatusNotModified) {
			return Get(ctx, client, GetInput{ProjectID: projectID, IssueIID: issueIID})
		}
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint(operation, err, hintConfirmIssueExists)
		}
		return Output{}, toolutil.WrapErrWithMessage(operation, err)
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
				hintConfirmIssueExists)
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
		return TimeStatsOutput{}, toolutil.WrapErrWithStatusHint("issueResetTimeEstimate", err, http.StatusNotFound,
			hintVerifyIssue)
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
		return TimeStatsOutput{}, toolutil.WrapErrWithStatusHint("issueResetSpentTime", err, http.StatusNotFound,
			hintVerifyIssue)
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
		return TimeStatsOutput{}, toolutil.WrapErrWithStatusHint("issueGetTimeStats", err, http.StatusNotFound,
			hintVerifyIssue)
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
		return ParticipantsOutput{}, toolutil.WrapErrWithStatusHint("issueGetParticipants", err, http.StatusNotFound,
			hintVerifyIssue)
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

// RelatedMROutput mirrors gl.BasicMergeRequest, the merge-request object
// returned by the closing-MR and related-MR issue endpoints. Per the 1:1 audit
// policy it surfaces every SDK field with the correct type: user objects
// (author, assignee, assignees, reviewers, merge_user, merged_by, closed_by)
// are full *toolutil.BasicUserOutput/[]*toolutil.BasicUserOutput rather than flattened
// usernames, and milestone/references/time_stats/task_completion_status/
// label_details are full nested objects reusing the issue sub-object shapes.
//
// The canonical author key carries the full user object. The MR's own internal
// id is surfaced on iid (the BasicMergeRequest.iid field).
type RelatedMROutput struct {
	ID                          int64                                `json:"id"`
	IID                         int64                                `json:"iid"`
	ProjectID                   int64                                `json:"project_id"`
	Title                       string                               `json:"title"`
	State                       string                               `json:"state"`
	Description                 string                               `json:"description"`
	SourceBranch                string                               `json:"source_branch"`
	TargetBranch                string                               `json:"target_branch"`
	SourceProjectID             int64                                `json:"source_project_id"`
	TargetProjectID             int64                                `json:"target_project_id"`
	Author                      *toolutil.BasicUserOutput            `json:"author"`
	Assignee                    *toolutil.BasicUserOutput            `json:"assignee"`
	Assignees                   []*toolutil.BasicUserOutput          `json:"assignees"`
	Reviewers                   []*toolutil.BasicUserOutput          `json:"reviewers"`
	MergeUser                   *toolutil.BasicUserOutput            `json:"merge_user"`
	MergedBy                    *toolutil.BasicUserOutput            `json:"merged_by"`
	ClosedBy                    *toolutil.BasicUserOutput            `json:"closed_by"`
	Milestone                   *toolutil.MRMilestoneOutput          `json:"milestone"`
	Labels                      []string                             `json:"labels"`
	LabelDetails                []*toolutil.LabelDetailsOutput       `json:"label_details"`
	References                  *toolutil.ReferencesOutput           `json:"references"`
	TimeStats                   *TimeStatsOutput                     `json:"time_stats"`
	TaskCompletionStatus        *toolutil.TaskCompletionStatusOutput `json:"task_completion_status"`
	Draft                       bool                                 `json:"draft"`
	Imported                    bool                                 `json:"imported"`
	ImportedFrom                string                               `json:"imported_from"`
	DetailedMergeStatus         string                               `json:"detailed_merge_status"`
	MergeWhenPipelineSucceeds   bool                                 `json:"merge_when_pipeline_succeeds"`
	SHA                         string                               `json:"sha"`
	MergeCommitSHA              string                               `json:"merge_commit_sha"`
	SquashCommitSHA             string                               `json:"squash_commit_sha"`
	Squash                      bool                                 `json:"squash"`
	SquashOnMerge               bool                                 `json:"squash_on_merge"`
	ShouldRemoveSourceBranch    bool                                 `json:"should_remove_source_branch"`
	ForceRemoveSourceBranch     bool                                 `json:"force_remove_source_branch"`
	AllowCollaboration          bool                                 `json:"allow_collaboration"`
	AllowMaintainerToPush       bool                                 `json:"allow_maintainer_to_push"`
	DiscussionLocked            bool                                 `json:"discussion_locked"`
	HasConflicts                bool                                 `json:"has_conflicts"`
	BlockingDiscussionsResolved bool                                 `json:"blocking_discussions_resolved"`
	Upvotes                     int64                                `json:"upvotes"`
	Downvotes                   int64                                `json:"downvotes"`
	UserNotesCount              int64                                `json:"user_notes_count"`
	CreatedAt                   string                               `json:"created_at,omitempty"`
	UpdatedAt                   string                               `json:"updated_at,omitempty"`
	MergedAt                    string                               `json:"merged_at,omitempty"`
	MergeAfter                  string                               `json:"merge_after,omitempty"`
	PreparedAt                  string                               `json:"prepared_at,omitempty"`
	ClosedAt                    string                               `json:"closed_at,omitempty"`
	WebURL                      string                               `json:"web_url"`
}

// RelatedMRsOutput holds a paginated list of merge requests related to an issue.
type RelatedMRsOutput struct {
	toolutil.HintableOutput
	MergeRequests []RelatedMROutput         `json:"merge_requests"`
	Pagination    toolutil.PaginationOutput `json:"pagination"`
}

// basicMRToOutput converts a gl.BasicMergeRequest into the full-fidelity
// RelatedMROutput, surfacing every SDK field with nested user/milestone/
// references/time-stats objects.
func basicMRToOutput(mr *gl.BasicMergeRequest) RelatedMROutput {
	out := RelatedMROutput{
		ID:              mr.ID,
		IID:             mr.IID,
		ProjectID:       mr.ProjectID,
		Title:           mr.Title,
		State:           mr.State,
		Description:     mr.Description,
		SourceBranch:    mr.SourceBranch,
		TargetBranch:    mr.TargetBranch,
		SourceProjectID: mr.SourceProjectID,
		TargetProjectID: mr.TargetProjectID,
		Author:          toolutil.NewBasicUserOutput(mr.Author),
		Assignee:        toolutil.NewBasicUserOutput(mr.Assignee),
		Assignees:       toolutil.NewBasicUserOutputs(mr.Assignees),
		Reviewers:       toolutil.NewBasicUserOutputs(mr.Reviewers),
		MergeUser:       toolutil.NewBasicUserOutput(mr.MergeUser),
		// MergedBy is deprecated in the SDK in favor of MergeUser, but the 1:1
		// audit requires surfacing every BasicMergeRequest field the API still
		// returns, so it is mirrored additively under merged_by.
		MergedBy:                    toolutil.NewBasicUserOutput(mr.MergedBy), //nolint:staticcheck // SA1019: intentional 1:1 fidelity for deprecated merged_by
		ClosedBy:                    toolutil.NewBasicUserOutput(mr.ClosedBy),
		Milestone:                   mrMilestoneOutputPtr(mr.Milestone),
		Labels:                      []string(mr.Labels),
		LabelDetails:                toolutil.NewLabelDetailsOutputs(mr.LabelDetails),
		References:                  toolutil.NewReferencesOutput(mr.References),
		TimeStats:                   timeStatsPtr(mr.TimeStats),
		TaskCompletionStatus:        toolutil.NewTaskCompletionStatusOutput(mr.TaskCompletionStatus),
		Draft:                       mr.Draft,
		Imported:                    mr.Imported,
		ImportedFrom:                mr.ImportedFrom,
		DetailedMergeStatus:         mr.DetailedMergeStatus,
		MergeWhenPipelineSucceeds:   mr.MergeWhenPipelineSucceeds,
		SHA:                         mr.SHA,
		MergeCommitSHA:              mr.MergeCommitSHA,
		SquashCommitSHA:             mr.SquashCommitSHA,
		Squash:                      mr.Squash,
		SquashOnMerge:               mr.SquashOnMerge,
		ShouldRemoveSourceBranch:    mr.ShouldRemoveSourceBranch,
		ForceRemoveSourceBranch:     mr.ForceRemoveSourceBranch,
		AllowCollaboration:          mr.AllowCollaboration,
		AllowMaintainerToPush:       mr.AllowMaintainerToPush,
		DiscussionLocked:            mr.DiscussionLocked,
		HasConflicts:                mr.HasConflicts,
		BlockingDiscussionsResolved: mr.BlockingDiscussionsResolved,
		Upvotes:                     mr.Upvotes,
		Downvotes:                   mr.Downvotes,
		UserNotesCount:              mr.UserNotesCount,
		CreatedAt:                   toolutil.FormatTimePtr(mr.CreatedAt),
		UpdatedAt:                   toolutil.FormatTimePtr(mr.UpdatedAt),
		MergedAt:                    toolutil.FormatTimePtr(mr.MergedAt),
		MergeAfter:                  toolutil.FormatTimePtr(mr.MergeAfter),
		PreparedAt:                  toolutil.FormatTimePtr(mr.PreparedAt),
		ClosedAt:                    toolutil.FormatTimePtr(mr.ClosedAt),
		WebURL:                      mr.WebURL,
	}
	return out
}

// relatedMRsOutput converts a slice of basic merge requests into a
// [RelatedMRsOutput] with pagination metadata derived from resp.
func relatedMRsOutput(mrs []*gl.BasicMergeRequest, resp *gl.Response) RelatedMRsOutput {
	out := make([]RelatedMROutput, len(mrs))
	for i, mr := range mrs {
		out[i] = basicMRToOutput(mr)
	}
	return RelatedMRsOutput{MergeRequests: out, Pagination: toolutil.PaginationFromResponse(resp)}
}

// mrListOptions captures the shared list-query parameters (pagination, keyset,
// order_by, sort) for the issue <-> merge request link list helpers so the
// closing-MR and related-MR handlers can build the underlying gl.ListOptions
// once. OrderBy and Sort are applied after ApplyListOptions so an explicit
// value always wins.
type mrListOptions struct {
	pagination toolutil.PaginationInput
	keyset     toolutil.KeysetPaginationInput
	orderBy    string
	sort       string
}

// applyTo populates a [gl.ListOptions] from the captured input: offset/keyset
// pagination via ApplyListOptions, then order_by/sort when supplied.
func (o mrListOptions) applyTo(opts *gl.ListOptions) {
	toolutil.ApplyListOptions(opts, o.pagination, o.keyset)
	if o.orderBy != "" {
		opts.OrderBy = o.orderBy
	}
	if o.sort != "" {
		opts.Sort = o.sort
	}
}

// issueMergeRequestsListArgs parameterises [listIssueMergeRequests] so the
// closing-MR and related-MR list handlers can share validation and the
// request shape while differing only in the GitLab API call and hint.
type issueMergeRequestsListArgs struct {
	projectID toolutil.StringOrInt
	issueIID  int64
	listOpts  mrListOptions
	operation string
	hint      string
	list      func(string, int64, mrListOptions, ...gl.RequestOptionFunc) ([]*gl.BasicMergeRequest, *gl.Response, error)
}

// listIssueMergeRequests validates the inputs, invokes the supplied list
// function (closing or related MRs), and converts the result into a
// [RelatedMRsOutput] with pagination metadata.
func listIssueMergeRequests(ctx context.Context, args issueMergeRequestsListArgs) (RelatedMRsOutput, error) {
	if err := ctx.Err(); err != nil {
		return RelatedMRsOutput{}, err
	}
	if args.projectID == "" {
		return RelatedMRsOutput{}, fmt.Errorf("%s: project_id is required", args.operation)
	}
	if args.issueIID <= 0 {
		return RelatedMRsOutput{}, toolutil.ErrRequiredInt64(args.operation, "issue_iid")
	}
	mrs, resp, err := args.list(string(args.projectID), args.issueIID, args.listOpts, gl.WithContext(ctx))
	if err != nil {
		return RelatedMRsOutput{}, toolutil.WrapErrWithStatusHint(args.operation, err, http.StatusNotFound, args.hint)
	}
	return relatedMRsOutput(mrs, resp), nil
}

// ListMRsClosingInput defines parameters for listing MRs that close an issue on merge.
type ListMRsClosingInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"         jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"          jsonschema:"Issue internal ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. created_at, updated_at)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListMRsClosing retrieves merge requests that will close this issue on merge.
func ListMRsClosing(ctx context.Context, client *gitlabclient.Client, input ListMRsClosingInput) (RelatedMRsOutput, error) {
	return listIssueMergeRequests(ctx, issueMergeRequestsListArgs{
		projectID: input.ProjectID, issueIID: input.IssueIID,
		listOpts: mrListOptions{
			pagination: input.PaginationInput, keyset: input.KeysetPaginationInput,
			orderBy: input.OrderBy, sort: input.Sort,
		},
		operation: "issueListMRsClosing",
		hint:      "verify project_id and issue_iid with gitlab_issue_get - only MRs that include 'Closes #N' are returned",
		list: func(projectID string, issueIID int64, lo mrListOptions, opts ...gl.RequestOptionFunc) ([]*gl.BasicMergeRequest, *gl.Response, error) {
			listOptions := &gl.ListMergeRequestsClosingIssueOptions{}
			lo.applyTo(&listOptions.ListOptions)
			return client.GL().Issues.ListMergeRequestsClosingIssue(projectID, issueIID, listOptions, opts...)
		},
	})
}

// ListMRsRelatedInput defines parameters for listing MRs related to an issue.
type ListMRsRelatedInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"         jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"          jsonschema:"Issue internal ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. created_at, updated_at)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListMRsRelated retrieves merge requests related to this issue.
func ListMRsRelated(ctx context.Context, client *gitlabclient.Client, input ListMRsRelatedInput) (RelatedMRsOutput, error) {
	return listIssueMergeRequests(ctx, issueMergeRequestsListArgs{
		projectID: input.ProjectID, issueIID: input.IssueIID,
		listOpts: mrListOptions{
			pagination: input.PaginationInput, keyset: input.KeysetPaginationInput,
			orderBy: input.OrderBy, sort: input.Sort,
		},
		operation: "issueListMRsRelated",
		hint:      "verify project_id and issue_iid with gitlab_issue_get - returns MRs mentioning the issue in description/notes (broader than 'closing')",
		list: func(projectID string, issueIID int64, lo mrListOptions, opts ...gl.RequestOptionFunc) ([]*gl.BasicMergeRequest, *gl.Response, error) {
			listOptions := &gl.ListMergeRequestsRelatedToIssueOptions{}
			lo.applyTo(&listOptions.ListOptions)
			return client.GL().Issues.ListMergeRequestsRelatedToIssue(projectID, issueIID, listOptions, opts...)
		},
	})
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
