// Package groupmilestones implements GitLab group milestone operations including
// list, get, create, update, delete, and related resource retrieval (issues,
// merge requests, burndown chart events).
package groupmilestones

import (
	"context"
	"errors"
	"fmt"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------- Input types ----------.

// ListInput defines parameters for listing milestones in a GitLab group.
type ListInput struct {
	GroupID            toolutil.StringOrInt `json:"group_id"                       jsonschema:"Group ID or URL-encoded path,required"`
	State              string               `json:"state,omitempty"                jsonschema:"Filter by state (active, closed)"`
	Title              string               `json:"title,omitempty"                jsonschema:"Filter by exact milestone title"`
	Search             string               `json:"search,omitempty"               jsonschema:"Search milestones by title or description"`
	SearchTitle        string               `json:"search_title,omitempty"         jsonschema:"Search milestones by title only"`
	IncludeAncestors   bool                 `json:"include_ancestors,omitempty"    jsonschema:"Include milestones from ancestor groups"`
	IncludeDescendants bool                 `json:"include_descendants,omitempty"  jsonschema:"Include milestones from descendant groups"`
	IIDs               []int64              `json:"iids,omitempty"                 jsonschema:"Filter by milestone IIDs"`
	UpdatedBefore      string               `json:"updated_before,omitempty"       jsonschema:"Return milestones updated before date (YYYY-MM-DD)"`
	UpdatedAfter       string               `json:"updated_after,omitempty"        jsonschema:"Return milestones updated after date (YYYY-MM-DD)"`
	ContainingDate     string               `json:"containing_date,omitempty"      jsonschema:"Return milestones containing this date (YYYY-MM-DD)"`
	toolutil.PaginationInput
}

// GetInput defines parameters for getting a single group milestone.
type GetInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"       jsonschema:"Group ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"  jsonschema:"Milestone IID (group-scoped). Use gitlab_group_milestone_list to find IIDs,required"`
}

// CreateInput defines parameters for creating a group milestone.
type CreateInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id"              jsonschema:"Group ID or URL-encoded path,required"`
	Title       string               `json:"title"                 jsonschema:"Milestone title,required"`
	Description string               `json:"description,omitempty" jsonschema:"Milestone description"`
	StartDate   string               `json:"start_date,omitempty"  jsonschema:"Start date (YYYY-MM-DD)"`
	DueDate     string               `json:"due_date,omitempty"    jsonschema:"Due date (YYYY-MM-DD)"`
}

// UpdateInput defines parameters for updating a group milestone.
type UpdateInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"              jsonschema:"Group ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"         jsonschema:"Milestone IID (group-scoped). Use gitlab_group_milestone_list to find IIDs,required"`
	Title        string               `json:"title,omitempty"       jsonschema:"Milestone title"`
	Description  string               `json:"description,omitempty" jsonschema:"Milestone description"`
	StartDate    string               `json:"start_date,omitempty"  jsonschema:"Start date (YYYY-MM-DD)"`
	DueDate      string               `json:"due_date,omitempty"    jsonschema:"Due date (YYYY-MM-DD)"`
	StateEvent   string               `json:"state_event,omitempty" jsonschema:"State transition: activate or close"`
}

// DeleteInput defines parameters for deleting a group milestone.
type DeleteInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"       jsonschema:"Group ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"  jsonschema:"Milestone IID (group-scoped). Use gitlab_group_milestone_list to find IIDs,required"`
}

// GetIssuesInput defines parameters for listing issues assigned to a group milestone.
type GetIssuesInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"       jsonschema:"Group ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"  jsonschema:"Milestone IID (group-scoped). Use gitlab_group_milestone_list to find IIDs,required"`
	toolutil.PaginationInput
}

// GetMergeRequestsInput defines parameters for listing merge requests assigned to a group milestone.
type GetMergeRequestsInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"       jsonschema:"Group ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"  jsonschema:"Milestone IID (group-scoped). Use gitlab_group_milestone_list to find IIDs,required"`
	toolutil.PaginationInput
}

// GetBurndownChartEventsInput defines parameters for listing burndown chart events for a group milestone.
type GetBurndownChartEventsInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"       jsonschema:"Group ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"  jsonschema:"Milestone IID (group-scoped). Use gitlab_group_milestone_list to find IIDs,required"`
	toolutil.PaginationInput
}

// ---------- Output types ----------.

// Output represents a single group milestone.
type Output struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	GroupID     int64  `json:"group_id"`
	GroupPath   string `json:"group_path,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	StartDate   string `json:"start_date"`
	DueDate     string `json:"due_date"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Expired     bool   `json:"expired"`
}

// ListOutput holds a paginated list of group milestones.
type ListOutput struct {
	toolutil.HintableOutput
	Milestones []Output                  `json:"milestones"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// IssueItem is a simplified issue representation for milestone context.
type IssueItem struct {
	ID        int64  `json:"id"`
	IID       int64  `json:"iid"`
	Title     string `json:"title"`
	State     string `json:"state"`
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at"`
}

// IssuesOutput holds a paginated list of issues for a group milestone.
type IssuesOutput struct {
	toolutil.HintableOutput
	Issues     []IssueItem               `json:"issues"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// MergeRequestItem is a simplified merge request representation for milestone context.
type MergeRequestItem struct {
	ID           int64  `json:"id"`
	IID          int64  `json:"iid"`
	Title        string `json:"title"`
	State        string `json:"state"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	WebURL       string `json:"web_url"`
	CreatedAt    string `json:"created_at"`
}

// MergeRequestsOutput holds a paginated list of merge requests for a group milestone.
type MergeRequestsOutput struct {
	toolutil.HintableOutput
	MergeRequests []MergeRequestItem        `json:"merge_requests"`
	Pagination    toolutil.PaginationOutput `json:"pagination"`
}

// BurndownChartEventItem represents a single burndown chart event.
type BurndownChartEventItem struct {
	CreatedAt string `json:"created_at"`
	Weight    int64  `json:"weight"`
	Action    string `json:"action"`
}

// BurndownChartEventsOutput holds a paginated list of burndown chart events.
type BurndownChartEventsOutput struct {
	toolutil.HintableOutput
	Events     []BurndownChartEventItem  `json:"events"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// resolveGroupIID translates a group-scoped milestone IID to the global ID
// required by the GitLab API. It lists milestones filtered by IID and
// returns the first match's global ID.
func resolveGroupIID(ctx context.Context, client *gitlabclient.Client, groupID toolutil.StringOrInt, iid int64) (int64, error) {
	milestones, _, err := client.GL().GroupMilestones.ListGroupMilestones(
		string(groupID),
		&gl.ListGroupMilestonesOptions{IIDs: new([]int64{iid})},
		gl.WithContext(ctx),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve group milestone IID %d: %w", iid, err)
	}
	if len(milestones) == 0 {
		return 0, fmt.Errorf("group milestone IID %d not found in group %q", iid, groupID)
	}
	return milestones[0].ID, nil
}

// ---------- Handlers ----------.

// List retrieves a paginated list of milestones for a GitLab group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, errors.New("groupMilestoneList: group_id is required")
	}

	opts, err := buildListOpts(input)
	if err != nil {
		return ListOutput{}, err
	}

	milestones, resp, err := client.GL().GroupMilestones.ListGroupMilestones(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("groupMilestoneList", err)
	}

	groupPath := string(input.GroupID)
	out := make([]Output, len(milestones))
	for i, m := range milestones {
		out[i] = toOutput(m, groupPath)
	}
	return ListOutput{Milestones: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// buildListOpts performs the build list opts operation using the GitLab API and returns [*gl.ListGroupMilestonesOptions].
func buildListOpts(input ListInput) (*gl.ListGroupMilestonesOptions, error) {
	opts := &gl.ListGroupMilestonesOptions{}
	applyListFilters(opts, input)
	if err := applyListDates(opts, input); err != nil {
		return nil, err
	}
	applyListPagination(opts, input)
	return opts, nil
}

// applyListFilters is an internal helper for the groupmilestones package.
func applyListFilters(opts *gl.ListGroupMilestonesOptions, input ListInput) {
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.Title != "" {
		opts.Title = new(input.Title)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.SearchTitle != "" {
		opts.SearchTitle = new(input.SearchTitle)
	}
	if input.IncludeAncestors {
		opts.IncludeAncestors = new(true)
	}
	if input.IncludeDescendants {
		opts.IncludeDescendents = new(true)
	}
	if len(input.IIDs) > 0 {
		opts.IIDs = new(input.IIDs)
	}
}

// applyListDates is an internal helper for the groupmilestones package.
func applyListDates(opts *gl.ListGroupMilestonesOptions, input ListInput) error {
	if input.UpdatedBefore != "" {
		d, err := parseISODate(input.UpdatedBefore)
		if err != nil {
			return fmt.Errorf("groupMilestoneList: updated_before: %w", err)
		}
		opts.UpdatedBefore = d
	}
	if input.UpdatedAfter != "" {
		d, err := parseISODate(input.UpdatedAfter)
		if err != nil {
			return fmt.Errorf("groupMilestoneList: updated_after: %w", err)
		}
		opts.UpdatedAfter = d
	}
	if input.ContainingDate != "" {
		d, err := parseISODate(input.ContainingDate)
		if err != nil {
			return fmt.Errorf("groupMilestoneList: containing_date: %w", err)
		}
		opts.ContainingDate = d
	}
	return nil
}

// applyListPagination is an internal helper for the groupmilestones package.
func applyListPagination(opts *gl.ListGroupMilestonesOptions, input ListInput) {
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
}

// Get retrieves a single group milestone by IID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("groupMilestoneGet: group_id is required")
	}
	if input.MilestoneIID == 0 {
		return Output{}, errors.New("groupMilestoneGet: milestone_iid is required")
	}

	globalID, err := resolveGroupIID(ctx, client, input.GroupID, input.MilestoneIID)
	if err != nil {
		return Output{}, err
	}

	m, _, err := client.GL().GroupMilestones.GetGroupMilestone(string(input.GroupID), globalID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("groupMilestoneGet", err)
	}
	return toOutput(m, string(input.GroupID)), nil
}

// Create creates a new milestone in a GitLab group.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("groupMilestoneCreate: group_id is required")
	}

	opts := &gl.CreateGroupMilestoneOptions{
		Title: new(input.Title),
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.StartDate != "" {
		d, err := parseISODate(input.StartDate)
		if err != nil {
			return Output{}, fmt.Errorf("groupMilestoneCreate: start_date: %w", err)
		}
		opts.StartDate = d
	}
	if input.DueDate != "" {
		d, err := parseISODate(input.DueDate)
		if err != nil {
			return Output{}, fmt.Errorf("groupMilestoneCreate: due_date: %w", err)
		}
		opts.DueDate = d
	}

	m, _, err := client.GL().GroupMilestones.CreateGroupMilestone(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("groupMilestoneCreate", err)
	}
	return toOutput(m, string(input.GroupID)), nil
}

// Update modifies an existing group milestone. Only non-empty fields are applied.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("groupMilestoneUpdate: group_id is required")
	}
	if input.MilestoneIID == 0 {
		return Output{}, errors.New("groupMilestoneUpdate: milestone_iid is required")
	}

	globalID, err := resolveGroupIID(ctx, client, input.GroupID, input.MilestoneIID)
	if err != nil {
		return Output{}, err
	}

	opts := &gl.UpdateGroupMilestoneOptions{}
	if input.Title != "" {
		opts.Title = new(input.Title)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.StartDate != "" {
		var d *gl.ISOTime
		d, err = parseISODate(input.StartDate)
		if err != nil {
			return Output{}, fmt.Errorf("groupMilestoneUpdate: start_date: %w", err)
		}
		opts.StartDate = d
	}
	if input.DueDate != "" {
		var d *gl.ISOTime
		d, err = parseISODate(input.DueDate)
		if err != nil {
			return Output{}, fmt.Errorf("groupMilestoneUpdate: due_date: %w", err)
		}
		opts.DueDate = d
	}
	if input.StateEvent != "" {
		opts.StateEvent = new(input.StateEvent)
	}

	m, _, err := client.GL().GroupMilestones.UpdateGroupMilestone(string(input.GroupID), globalID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("groupMilestoneUpdate", err)
	}
	return toOutput(m, string(input.GroupID)), nil
}

// Delete removes a group milestone.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return errors.New("groupMilestoneDelete: group_id is required")
	}
	if input.MilestoneIID == 0 {
		return errors.New("groupMilestoneDelete: milestone_iid is required")
	}

	globalID, err := resolveGroupIID(ctx, client, input.GroupID, input.MilestoneIID)
	if err != nil {
		return err
	}

	_, err = client.GL().GroupMilestones.DeleteGroupMilestone(string(input.GroupID), globalID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("groupMilestoneDelete", err)
	}
	return nil
}

// GetIssues retrieves issues assigned to a group milestone.
func GetIssues(ctx context.Context, client *gitlabclient.Client, input GetIssuesInput) (IssuesOutput, error) {
	if err := ctx.Err(); err != nil {
		return IssuesOutput{}, err
	}
	if input.GroupID == "" {
		return IssuesOutput{}, errors.New("groupMilestoneGetIssues: group_id is required")
	}
	if input.MilestoneIID == 0 {
		return IssuesOutput{}, errors.New("groupMilestoneGetIssues: milestone_iid is required")
	}

	globalID, err := resolveGroupIID(ctx, client, input.GroupID, input.MilestoneIID)
	if err != nil {
		return IssuesOutput{}, err
	}

	opts := &gl.GetGroupMilestoneIssuesOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	issues, resp, err := client.GL().GroupMilestones.GetGroupMilestoneIssues(string(input.GroupID), globalID, opts, gl.WithContext(ctx))
	if err != nil {
		return IssuesOutput{}, toolutil.WrapErrWithMessage("groupMilestoneGetIssues", err)
	}

	items := make([]IssueItem, len(issues))
	for i, issue := range issues {
		items[i] = IssueItem{
			ID:    issue.ID,
			IID:   issue.IID,
			Title: issue.Title,
			State: issue.State,
		}
		if issue.WebURL != "" {
			items[i].WebURL = issue.WebURL
		}
		if issue.CreatedAt != nil {
			items[i].CreatedAt = issue.CreatedAt.Format(time.RFC3339)
		}
	}
	return IssuesOutput{Issues: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetMergeRequests retrieves merge requests assigned to a group milestone.
func GetMergeRequests(ctx context.Context, client *gitlabclient.Client, input GetMergeRequestsInput) (MergeRequestsOutput, error) {
	if err := ctx.Err(); err != nil {
		return MergeRequestsOutput{}, err
	}
	if input.GroupID == "" {
		return MergeRequestsOutput{}, errors.New("groupMilestoneGetMergeRequests: group_id is required")
	}
	if input.MilestoneIID == 0 {
		return MergeRequestsOutput{}, errors.New("groupMilestoneGetMergeRequests: milestone_iid is required")
	}

	globalID, err := resolveGroupIID(ctx, client, input.GroupID, input.MilestoneIID)
	if err != nil {
		return MergeRequestsOutput{}, err
	}

	opts := &gl.GetGroupMilestoneMergeRequestsOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	mrs, resp, err := client.GL().GroupMilestones.GetGroupMilestoneMergeRequests(string(input.GroupID), globalID, opts, gl.WithContext(ctx))
	if err != nil {
		return MergeRequestsOutput{}, toolutil.WrapErrWithMessage("groupMilestoneGetMergeRequests", err)
	}

	items := make([]MergeRequestItem, len(mrs))
	for i, mr := range mrs {
		items[i] = MergeRequestItem{
			ID:           mr.ID,
			IID:          mr.IID,
			Title:        mr.Title,
			State:        mr.State,
			SourceBranch: mr.SourceBranch,
			TargetBranch: mr.TargetBranch,
		}
		if mr.WebURL != "" {
			items[i].WebURL = mr.WebURL
		}
		if mr.CreatedAt != nil {
			items[i].CreatedAt = mr.CreatedAt.Format(time.RFC3339)
		}
	}
	return MergeRequestsOutput{MergeRequests: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetBurndownChartEvents retrieves burndown chart events for a group milestone.
func GetBurndownChartEvents(ctx context.Context, client *gitlabclient.Client, input GetBurndownChartEventsInput) (BurndownChartEventsOutput, error) {
	if err := ctx.Err(); err != nil {
		return BurndownChartEventsOutput{}, err
	}
	if input.GroupID == "" {
		return BurndownChartEventsOutput{}, errors.New("groupMilestoneGetBurndownChartEvents: group_id is required")
	}
	if input.MilestoneIID == 0 {
		return BurndownChartEventsOutput{}, errors.New("groupMilestoneGetBurndownChartEvents: milestone_iid is required")
	}

	globalID, err := resolveGroupIID(ctx, client, input.GroupID, input.MilestoneIID)
	if err != nil {
		return BurndownChartEventsOutput{}, err
	}

	opts := &gl.GetGroupMilestoneBurndownChartEventsOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	events, resp, err := client.GL().GroupMilestones.GetGroupMilestoneBurndownChartEvents(string(input.GroupID), globalID, opts, gl.WithContext(ctx))
	if err != nil {
		return BurndownChartEventsOutput{}, toolutil.WrapErrWithMessage("groupMilestoneGetBurndownChartEvents", err)
	}

	items := make([]BurndownChartEventItem, len(events))
	for i, e := range events {
		items[i] = BurndownChartEventItem{}
		if e.CreatedAt != nil {
			items[i].CreatedAt = e.CreatedAt.Format(time.RFC3339)
		}
		if e.Weight != nil {
			items[i].Weight = *e.Weight
		}
		if e.Action != nil {
			items[i].Action = *e.Action
		}
	}
	return BurndownChartEventsOutput{Events: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------- Converters ----------.

// toOutput converts a GitLab API [gl.GroupMilestone] to MCP output format.
// groupPath carries the caller-supplied group identifier (path or numeric string).
func toOutput(m *gl.GroupMilestone, groupPath string) Output {
	out := Output{
		ID:          m.ID,
		IID:         m.IID,
		GroupID:     m.GroupID,
		GroupPath:   groupPath,
		Title:       m.Title,
		Description: m.Description,
		State:       m.State,
	}
	if m.StartDate != nil {
		out.StartDate = m.StartDate.String()
	}
	if m.DueDate != nil {
		out.DueDate = m.DueDate.String()
	}
	if m.CreatedAt != nil {
		out.CreatedAt = m.CreatedAt.Format(time.RFC3339)
	}
	if m.UpdatedAt != nil {
		out.UpdatedAt = m.UpdatedAt.Format(time.RFC3339)
	}
	if m.Expired != nil {
		out.Expired = *m.Expired
	}
	return out
}

// parseISODate converts a YYYY-MM-DD string to *gl.ISOTime.
func parseISODate(s string) (*gl.ISOTime, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, fmt.Errorf("invalid date format (expected YYYY-MM-DD): %w", err)
	}
	d := gl.ISOTime(t)
	return &d, nil
}

// ---------- Formatters ----------.
