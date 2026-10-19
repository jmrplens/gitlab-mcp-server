package groupmilestones

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintVerifyGroupMilestoneID is the 404 hint shared by group milestone tools.
const hintVerifyGroupMilestoneID = "verify milestone_id with group.group_milestone_list"

// ---------- Input types ----------.

// ListInput defines parameters for listing milestones in a GitLab group.
type ListInput struct {
	GroupID                 toolutil.StringOrInt `json:"group_id"                       jsonschema:"Group ID or URL-encoded path,required"`
	State                   string               `json:"state,omitempty"                jsonschema:"Filter by state (active, closed)"`
	Title                   string               `json:"title,omitempty"                jsonschema:"Filter by exact milestone title"`
	Search                  string               `json:"search,omitempty"               jsonschema:"Search milestones by title or description"`
	SearchTitle             string               `json:"search_title,omitempty"         jsonschema:"Search milestones by title only"`
	IncludeAncestors        bool                 `json:"include_ancestors,omitempty"    jsonschema:"Include milestones from ancestor groups"`
	IncludeDescendants      bool                 `json:"include_descendants,omitempty"  jsonschema:"Include milestones from descendant groups and their projects. A project's milestone carries its project_id and no group_id"`
	IIDs                    []int64              `json:"iids,omitempty"                 jsonschema:"Filter by milestone IIDs"`
	UpdatedBefore           string               `json:"updated_before,omitempty"       jsonschema:"Return milestones updated before date (YYYY-MM-DD)"`
	UpdatedAfter            string               `json:"updated_after,omitempty"        jsonschema:"Return milestones updated after date (YYYY-MM-DD)"`
	ContainingDate          string               `json:"containing_date,omitempty"      jsonschema:"Return milestones containing this date (YYYY-MM-DD)"`
	StartDate               string               `json:"start_date,omitempty"           jsonschema:"Return milestones with a start date on or after this date (YYYY-MM-DD)"`
	EndDate                 string               `json:"end_date,omitempty"             jsonschema:"Return milestones with a due date on or before this date (YYYY-MM-DD)"`
	OrderBy                 string               `json:"order_by,omitempty"             jsonschema:"Order results by field (e.g. created_at, updated_at, due_date, title)"`
	Sort                    string               `json:"sort,omitempty"                 jsonschema:"Sort direction (asc, desc)"`
	IncludeParentMilestones bool                 `json:"include_parent_milestones,omitempty" jsonschema:"Deprecated (GitLab 16.7): use include_ancestors instead. Include milestones from parent groups"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetInput defines parameters for getting a single group milestone.
type GetInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"       jsonschema:"Group ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"  jsonschema:"Milestone IID (group-scoped). Use group.group_milestone_list to find IIDs,required"`
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
	MilestoneIID int64                `json:"milestone_iid"         jsonschema:"Milestone IID (group-scoped). Use group.group_milestone_list to find IIDs,required"`
	Title        string               `json:"title,omitempty"       jsonschema:"Milestone title"`
	Description  *string              `json:"description,omitempty" jsonschema:"Milestone description. Pass an empty string to remove it. Omit the field to leave it untouched"`
	StartDate    *string              `json:"start_date,omitempty"  jsonschema:"Start date (YYYY-MM-DD). Pass an empty string to remove it. Omit the field to leave it untouched"`
	DueDate      *string              `json:"due_date,omitempty"    jsonschema:"Due date (YYYY-MM-DD). Pass an empty string to remove it. Omit the field to leave it untouched"`
	StateEvent   string               `json:"state_event,omitempty" jsonschema:"State transition: activate or close"`
}

// DeleteInput defines parameters for deleting a group milestone.
type DeleteInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"       jsonschema:"Group ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"  jsonschema:"Milestone IID (group-scoped). Use group.group_milestone_list to find IIDs,required"`
}

// GetIssuesInput defines parameters for listing issues assigned to a group milestone.
type GetIssuesInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"       jsonschema:"Group ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"  jsonschema:"Milestone IID (group-scoped). Use group.group_milestone_list to find IIDs,required"`
	OrderBy      string               `json:"order_by,omitempty" jsonschema:"Order results by field (e.g. created_at, updated_at)"`
	Sort         string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetMergeRequestsInput defines parameters for listing merge requests assigned to a group milestone.
type GetMergeRequestsInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"       jsonschema:"Group ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"  jsonschema:"Milestone IID (group-scoped). Use group.group_milestone_list to find IIDs,required"`
	OrderBy      string               `json:"order_by,omitempty" jsonschema:"Order results by field (e.g. created_at, updated_at)"`
	Sort         string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetBurndownChartEventsInput defines parameters for listing the burndown
// chart events of a group milestone. Nothing pages them: the route declares
// no page, per_page, order_by or sort, and GitLab presents the whole burndown
// in one answer (ee/lib/ee/api/milestone_responses.rb), so the paging fields
// client-go's options struct carries would be sent and ignored.
type GetBurndownChartEventsInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"       jsonschema:"Group ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"  jsonschema:"Milestone IID (group-scoped). Use group.group_milestone_list to find IIDs,required"`
}

// ---------- Output types ----------.

// Output represents a single group milestone.
type Output struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	GroupID     int64  `json:"group_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	StartDate   string `json:"start_date"`
	DueDate     string `json:"due_date"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Expired     bool   `json:"expired"`
	// WebURL is the milestone's own page. ProjectID is set only on a
	// project-scoped milestone, which a list asked for include_descendants
	// returns beside the group's own, so a group one leaves it zero.
	WebURL    string `json:"web_url,omitempty"`
	ProjectID int64  `json:"project_id,omitempty"`
}

// ListOutput holds a paginated list of group milestones.
type ListOutput struct {
	toolutil.HintableOutput
	Milestones []Output                  `json:"milestones"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// IssueItem is one issue of a group milestone as a compact row: what tells
// the issues apart and lets a caller pick one, including the project each
// belongs to, the people and labels on it, and its dates. GitLab answers the
// list with IssueBasic, whose other keys (the description, the milestone being
// listed, time tracking and the counters) are what issue.get returns;
// repeating them on every row of a milestone is a cost this row does not pay.
type IssueItem struct {
	ID           int64                       `json:"id"`
	IID          int64                       `json:"iid"`
	ProjectID    int64                       `json:"project_id"`
	Title        string                      `json:"title"`
	State        string                      `json:"state"`
	Labels       []string                    `json:"labels,omitempty"`
	Author       *toolutil.IssueUserOutput   `json:"author,omitempty"`
	Assignees    []*toolutil.IssueUserOutput `json:"assignees,omitempty"`
	Confidential bool                        `json:"confidential,omitempty"`
	Weight       int64                       `json:"weight,omitempty" tier:"premium"`
	DueDate      string                      `json:"due_date,omitempty"`
	WebURL       string                      `json:"web_url"`
	CreatedAt    string                      `json:"created_at"`
	UpdatedAt    string                      `json:"updated_at,omitempty"`
	ClosedAt     string                      `json:"closed_at,omitempty"`
}

// IssuesOutput holds a paginated list of issues for a group milestone.
type IssuesOutput struct {
	toolutil.HintableOutput
	Issues     []IssueItem               `json:"issues"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// MergeRequestItem is one merge request of a group milestone as a compact row:
// what tells the merge requests apart, the project each belongs to, whether
// each can merge, the branches, the people and labels on it, and its dates.
// GitLab answers the list with MergeRequestBasic, whose other keys (the
// description, the commit SHAs, the merge options, time tracking and the
// counters) are what merge_request.get returns; repeating them on every row of
// a milestone is a cost this row does not pay.
type MergeRequestItem struct {
	ID                  int64                       `json:"id"`
	IID                 int64                       `json:"iid"`
	ProjectID           int64                       `json:"project_id"`
	Title               string                      `json:"title"`
	State               string                      `json:"state"`
	Draft               bool                        `json:"draft,omitempty"`
	DetailedMergeStatus string                      `json:"detailed_merge_status,omitempty"`
	SourceBranch        string                      `json:"source_branch"`
	TargetBranch        string                      `json:"target_branch"`
	Labels              []string                    `json:"labels,omitempty"`
	Author              *toolutil.BasicUserOutput   `json:"author,omitempty"`
	Assignees           []*toolutil.BasicUserOutput `json:"assignees,omitempty"`
	Reviewers           []*toolutil.BasicUserOutput `json:"reviewers,omitempty"`
	WebURL              string                      `json:"web_url"`
	CreatedAt           string                      `json:"created_at"`
	UpdatedAt           string                      `json:"updated_at,omitempty"`
	MergedAt            string                      `json:"merged_at,omitempty"`
	ClosedAt            string                      `json:"closed_at,omitempty"`
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
	Weight    *int64 `json:"weight,omitempty"`
	Action    string `json:"action"`
}

// BurndownChartEventsOutput holds every burndown chart event of a group
// milestone.
type BurndownChartEventsOutput struct {
	toolutil.HintableOutput
	Events []BurndownChartEventItem `json:"events"`
}

// resolveGroupIID translates a group-scoped milestone IID to the global ID
// required by the GitLab API. It lists milestones filtered by IID and
// returns the first match's global ID.
func resolveGroupIID(ctx context.Context, client *gitlabclient.Client, groupID toolutil.StringOrInt, iid int64) (int64, error) {
	//gitlab:request mandatory: the action is addressed by IID and GitLab's milestone routes by ID, so this lookup runs before the action's own request on every call
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

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	milestones, resp, err := client.GL().GroupMilestones.ListGroupMilestones(string(input.GroupID), opts, gl.WithContext(ctx),
		includeDescendants(input.IncludeDescendants))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("groupMilestoneList", err, http.StatusNotFound,
			"verify group_id with group.get; group milestones differ from project milestones")
	}
	extras, err := toolutil.CapturedMilestones(captured, len(milestones))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("groupMilestoneList", err)
	}

	out := make([]Output, len(milestones))
	for i, m := range milestones {
		out[i] = toOutput(m, extras[i])
	}
	return ListOutput{Milestones: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// includeDescendants adds include_descendants to the query client-go encoded,
// the parameter lib/api/group_milestones.rb declares. client-go's
// ListGroupMilestonesOptions spells it include_descendents, a key GitLab
// ignores, so setting that field asked for nothing and the list answered with
// the group's own milestones alone (upstream register row 95). When the
// caller did not ask for them it returns no option at all, which client-go
// skips, so the query goes out exactly as client-go encoded it.
func includeDescendants(include bool) gl.RequestOptionFunc {
	if !include {
		return nil
	}
	return func(req *retryablehttp.Request) error {
		query := req.URL.Query()
		query.Set("include_descendants", "true")
		req.URL.RawQuery = query.Encode()
		return nil
	}
}

// buildListOpts handles build list opts and returns [*gl.ListGroupMilestonesOptions].
func buildListOpts(input ListInput) (*gl.ListGroupMilestonesOptions, error) {
	opts := &gl.ListGroupMilestonesOptions{}
	applyListFilters(opts, input)
	if err := applyListDates(opts, input); err != nil {
		return nil, err
	}
	applyListPagination(opts, input)
	return opts, nil
}

// applyListFilters applies list filters transformations.
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
	if input.IncludeParentMilestones {
		//nolint:staticcheck // SA1019: mirror deprecated SDK field for 1:1 API coverage
		opts.IncludeParentMilestones = new(true)
	}
	// No emptiness guard: the query encoder leaves an empty slice out of the
	// request whatever pointer holds it, so a guard would decide nothing.
	opts.IIDs = new(input.IIDs)
}

// applyListDates reads the five date filters of a group milestone list into
// their option fields, refusing the first that does not parse.
func applyListDates(opts *gl.ListGroupMilestonesOptions, input ListInput) error {
	if err := toolutil.ParseDateInputs(
		toolutil.DateInput{Field: "updated_before", Value: input.UpdatedBefore, Dest: &opts.UpdatedBefore},
		toolutil.DateInput{Field: "updated_after", Value: input.UpdatedAfter, Dest: &opts.UpdatedAfter},
		toolutil.DateInput{Field: "containing_date", Value: input.ContainingDate, Dest: &opts.ContainingDate},
		toolutil.DateInput{Field: "start_date", Value: input.StartDate, Dest: &opts.StartDate},
		toolutil.DateInput{Field: "end_date", Value: input.EndDate, Dest: &opts.EndDate},
	); err != nil {
		return fmt.Errorf("groupMilestoneList: %w", err)
	}
	return nil
}

// applyListPagination applies offset/keyset pagination and ordering. OrderBy
// and Sort are applied after ApplyListOptions so explicit values win.
func applyListPagination(opts *gl.ListGroupMilestonesOptions, input ListInput) {
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
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

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	m, _, err := client.GL().GroupMilestones.GetGroupMilestone(string(input.GroupID), globalID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("groupMilestoneGet", err, http.StatusNotFound,
			hintVerifyGroupMilestoneID)
	}
	extra, err := toolutil.CapturedMilestone(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("groupMilestoneGet", err)
	}
	return toOutput(m, extra), nil
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
	if err := parseMilestoneDates(input.StartDate, input.DueDate, &opts.StartDate, &opts.DueDate); err != nil {
		return Output{}, fmt.Errorf("groupMilestoneCreate: %w", err)
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	m, _, err := client.GL().GroupMilestones.CreateGroupMilestone(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("groupMilestoneCreate", err, http.StatusBadRequest,
			"title is required and must be unique within the group; start_date and due_date must be YYYY-MM-DD with start_date <= due_date; creating group milestones requires Reporter role or higher")
	}
	extra, err := toolutil.CapturedMilestone(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("groupMilestoneCreate", err)
	}
	return toOutput(m, extra), nil
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

	// The dates are read before the IID is resolved, so one that does not
	// parse is refused without a request rather than after one.
	opts := &gl.UpdateGroupMilestoneOptions{}
	if err := toolutil.ClearableDateInputs(
		toolutil.ClearableDateInput{Field: "start_date", Value: input.StartDate, Dest: &opts.StartDate},
		toolutil.ClearableDateInput{Field: "due_date", Value: input.DueDate, Dest: &opts.DueDate},
	); err != nil {
		return Output{}, fmt.Errorf("groupMilestoneUpdate: %w", err)
	}

	globalID, err := resolveGroupIID(ctx, client, input.GroupID, input.MilestoneIID)
	if err != nil {
		return Output{}, err
	}

	if input.Title != "" {
		opts.Title = new(input.Title)
	}
	// Presence decides, never emptiness: an empty description is the caller
	// removing it (issue 1306).
	opts.Description = input.Description
	if input.StateEvent != "" {
		opts.StateEvent = new(input.StateEvent)
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	m, _, err := client.GL().GroupMilestones.UpdateGroupMilestone(string(input.GroupID), globalID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("groupMilestoneUpdate", err, http.StatusBadRequest,
			"state_event must be 'close' or 'activate'; start_date/due_date must be YYYY-MM-DD; verify milestone_id with group.group_milestone_list")
	}
	extra, err := toolutil.CapturedMilestone(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("groupMilestoneUpdate", err)
	}
	return toOutput(m, extra), nil
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
		return toolutil.WrapErrWithStatusHint("groupMilestoneDelete", err, http.StatusForbidden,
			"deleting group milestones requires Owner role")
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
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}

	issues, resp, err := client.GL().GroupMilestones.GetGroupMilestoneIssues(string(input.GroupID), globalID, opts, gl.WithContext(ctx))
	if err != nil {
		return IssuesOutput{}, toolutil.WrapErrWithStatusHint("groupMilestoneGetIssues", err, http.StatusNotFound,
			hintVerifyGroupMilestoneID)
	}

	items := make([]IssueItem, len(issues))
	for i, issue := range issues {
		items[i] = IssueItem{
			ID:           issue.ID,
			IID:          issue.IID,
			ProjectID:    issue.ProjectID,
			Title:        issue.Title,
			State:        issue.State,
			Labels:       []string(issue.Labels),
			Author:       toolutil.NewIssueUserOutputFromIssueAuthor(issue.Author),
			Assignees:    toolutil.NewIssueUserOutputsFromIssueAssignees(issue.Assignees),
			Confidential: issue.Confidential,
			Weight:       issue.Weight,
			DueDate:      toolutil.FormatISOTimePtr(issue.DueDate),
			WebURL:       issue.WebURL,
			CreatedAt:    toolutil.RFC3339Ptr(issue.CreatedAt),
			UpdatedAt:    toolutil.RFC3339Ptr(issue.UpdatedAt),
			ClosedAt:     toolutil.RFC3339Ptr(issue.ClosedAt),
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
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}

	mrs, resp, err := client.GL().GroupMilestones.GetGroupMilestoneMergeRequests(string(input.GroupID), globalID, opts, gl.WithContext(ctx))
	if err != nil {
		return MergeRequestsOutput{}, toolutil.WrapErrWithStatusHint("groupMilestoneGetMergeRequests", err, http.StatusNotFound,
			hintVerifyGroupMilestoneID)
	}

	items := make([]MergeRequestItem, len(mrs))
	for i, mr := range mrs {
		items[i] = MergeRequestItem{
			ID:                  mr.ID,
			IID:                 mr.IID,
			ProjectID:           mr.ProjectID,
			Title:               mr.Title,
			State:               mr.State,
			Draft:               mr.Draft,
			DetailedMergeStatus: mr.DetailedMergeStatus,
			SourceBranch:        mr.SourceBranch,
			TargetBranch:        mr.TargetBranch,
			Labels:              []string(mr.Labels),
			Author:              toolutil.NewBasicUserOutput(mr.Author),
			Assignees:           toolutil.NewBasicUserOutputs(mr.Assignees),
			Reviewers:           toolutil.NewBasicUserOutputs(mr.Reviewers),
			WebURL:              mr.WebURL,
			CreatedAt:           toolutil.RFC3339Ptr(mr.CreatedAt),
			UpdatedAt:           toolutil.RFC3339Ptr(mr.UpdatedAt),
			MergedAt:            toolutil.RFC3339Ptr(mr.MergedAt),
			ClosedAt:            toolutil.RFC3339Ptr(mr.ClosedAt),
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
	events, _, err := client.GL().GroupMilestones.GetGroupMilestoneBurndownChartEvents(string(input.GroupID), globalID, opts, gl.WithContext(ctx))
	if err != nil {
		return BurndownChartEventsOutput{}, toolutil.WrapErrWithStatusHint("groupMilestoneGetBurndownChartEvents", err, http.StatusNotFound,
			"verify milestone_id with group.group_milestone_list; burndown charts require GitLab Premium or higher")
	}

	items := make([]BurndownChartEventItem, len(events))
	for i, e := range events {
		items[i] = BurndownChartEventItem{}
		if e.CreatedAt != nil {
			items[i].CreatedAt = e.CreatedAt.Format(time.RFC3339)
		}
		// client-go models this weight as a pointer, so the null of an
		// event whose issue carries no weight arrives as nil and stays so.
		items[i].Weight = e.Weight
		if e.Action != nil {
			items[i].Action = *e.Action
		}
	}
	return BurndownChartEventsOutput{Events: items}, nil
}

// ---------- Converters ----------.

// toOutput converts a GitLab API [gl.GroupMilestone] to MCP output format,
// filling from the decoded milestone and from the project the capture read
// beside it.
func toOutput(m *gl.GroupMilestone, extra toolutil.MilestoneExtra) Output {
	out := Output{
		ID:          m.ID,
		IID:         m.IID,
		GroupID:     m.GroupID,
		Title:       m.Title,
		Description: m.Description,
		State:       m.State,
		WebURL:      m.WebURL,
		ProjectID:   extra.ProjectID,
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

// parseMilestoneDates reads a group milestone's start and due dates into
// their option fields through the parser every date input shares, refusing
// the first that does not parse.
func parseMilestoneDates(startDate, dueDate string, start, due **gl.ISOTime) error {
	return toolutil.ParseDateInputs(
		toolutil.DateInput{Field: "start_date", Value: startDate, Dest: start},
		toolutil.DateInput{Field: "due_date", Value: dueDate, Dest: due},
	)
}

// ---------- Formatters ----------.
