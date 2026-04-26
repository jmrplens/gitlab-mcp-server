// Package milestones implements MCP tool handlers for GitLab milestone
// operations including list, get, create, update, and delete.
// It wraps the Milestones service from client-go v2.
package milestones

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

// ListInput defines parameters for listing milestones in a GitLab project.
type ListInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id"                  jsonschema:"Project ID or URL-encoded path,required"`
	State            string               `json:"state,omitempty"             jsonschema:"Filter by state (active, closed)"`
	Title            string               `json:"title,omitempty"             jsonschema:"Filter by exact milestone title"`
	Search           string               `json:"search,omitempty"            jsonschema:"Search milestones by title or description"`
	IncludeAncestors bool                 `json:"include_ancestors,omitempty" jsonschema:"Include milestones from parent groups"`
	IIDs             []int64              `json:"iids,omitempty"              jsonschema:"Filter by milestone IIDs"`
	toolutil.PaginationInput
}

// Output represents a single project milestone.
type Output struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	ProjectID   int64  `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	StartDate   string `json:"start_date"`
	DueDate     string `json:"due_date"`
	WebURL      string `json:"web_url"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Expired     bool   `json:"expired"`
	GroupID     int64  `json:"group_id,omitempty"`
}

// ListOutput holds a paginated list of milestones.
type ListOutput struct {
	toolutil.HintableOutput
	Milestones []Output                  `json:"milestones"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves a paginated list of milestones for a GitLab project.
// Supports filtering by state, title, search keyword, and ancestor inclusion.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("milestoneList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	opts := &gl.ListMilestonesOptions{}
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.Title != "" {
		opts.Title = new(input.Title)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.IncludeAncestors {
		opts.IncludeAncestors = new(true)
	}
	if len(input.IIDs) > 0 {
		opts.IIDs = &input.IIDs
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	milestones, resp, err := client.GL().Milestones.ListMilestones(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("milestoneList", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get")
	}

	out := make([]Output, len(milestones))
	for i, m := range milestones {
		out[i] = ToOutput(m)
	}
	return ListOutput{Milestones: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ToOutput converts a GitLab API [gl.Milestone] to MCP output format.
func ToOutput(m *gl.Milestone) Output {
	out := Output{
		ID:          m.ID,
		IID:         m.IID,
		ProjectID:   m.ProjectID,
		Title:       m.Title,
		Description: m.Description,
		State:       m.State,
		WebURL:      m.WebURL,
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
	out.GroupID = m.GroupID
	return out
}

// ---------- Input types ----------.

// GetInput defines parameters for getting a single milestone.
type GetInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"     jsonschema:"Project ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"  jsonschema:"Milestone IID (project-scoped). Use gitlab_milestone_list to find IIDs,required"`
}

// CreateInput defines parameters for creating a milestone.
type CreateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	Title       string               `json:"title"                   jsonschema:"Milestone title,required"`
	Description string               `json:"description,omitempty"   jsonschema:"Milestone description"`
	StartDate   string               `json:"start_date,omitempty"    jsonschema:"Start date (YYYY-MM-DD)"`
	DueDate     string               `json:"due_date,omitempty"      jsonschema:"Due date (YYYY-MM-DD)"`
}

// UpdateInput defines parameters for updating a milestone.
type UpdateInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"           jsonschema:"Milestone IID (project-scoped). Use gitlab_milestone_list to find IIDs,required"`
	Title        string               `json:"title,omitempty"         jsonschema:"Milestone title"`
	Description  string               `json:"description,omitempty"   jsonschema:"Milestone description"`
	StartDate    string               `json:"start_date,omitempty"    jsonschema:"Start date (YYYY-MM-DD)"`
	DueDate      string               `json:"due_date,omitempty"      jsonschema:"Due date (YYYY-MM-DD)"`
	StateEvent   string               `json:"state_event,omitempty"   jsonschema:"State transition: activate or close"`
}

// DeleteInput defines parameters for deleting a milestone.
type DeleteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"     jsonschema:"Project ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"  jsonschema:"Milestone IID (project-scoped). Use gitlab_milestone_list to find IIDs,required"`
}

// GetIssuesInput defines parameters for listing issues assigned to a milestone.
type GetIssuesInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"     jsonschema:"Project ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"  jsonschema:"Milestone IID (project-scoped). Use gitlab_milestone_list to find IIDs,required"`
	toolutil.PaginationInput
}

// GetMergeRequestsInput defines parameters for listing merge requests assigned to a milestone.
type GetMergeRequestsInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"     jsonschema:"Project ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid"  jsonschema:"Milestone IID (project-scoped). Use gitlab_milestone_list to find IIDs,required"`
	toolutil.PaginationInput
}

// ---------- Output types for related resources ----------.

// IssueItem is a simplified issue representation for milestone context.
type IssueItem struct {
	ID        int64  `json:"id"`
	IID       int64  `json:"iid"`
	Title     string `json:"title"`
	State     string `json:"state"`
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at"`
}

// MilestoneIssuesOutput holds a paginated list of issues for a milestone.
type MilestoneIssuesOutput struct {
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

// MilestoneMergeRequestsOutput holds a paginated list of merge requests for a milestone.
type MilestoneMergeRequestsOutput struct {
	toolutil.HintableOutput
	MergeRequests []MergeRequestItem        `json:"merge_requests"`
	Pagination    toolutil.PaginationOutput `json:"pagination"`
}

// ---------- Helpers ----------.

// resolveIID looks up a milestone by its project-scoped IID and returns the global ID
// needed by the GitLab API. The GitLab milestone endpoints use the global ID in the
// URL path, but users naturally work with IIDs (like issues and merge requests).
func resolveIID(ctx context.Context, client *gitlabclient.Client, projectID toolutil.StringOrInt, iid int64) (int64, error) {
	iids := []int64{iid}
	opts := &gl.ListMilestonesOptions{
		IIDs: &iids,
	}
	milestones, _, err := client.GL().Milestones.ListMilestones(string(projectID), opts, gl.WithContext(ctx))
	if err != nil {
		return 0, fmt.Errorf("resolving milestone IID %d: %w", iid, err)
	}
	if len(milestones) == 0 {
		return 0, fmt.Errorf("milestone with IID %d not found in project %s", iid, projectID)
	}
	return milestones[0].ID, nil
}

// ---------- Handlers ----------.

// Get retrieves a single milestone by IID (resolves to global ID internally).
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, errors.New("milestoneGet: project_id is required")
	}
	if input.MilestoneIID == 0 {
		return Output{}, toolutil.ErrRequiredInt64("milestoneGet", "milestone_iid")
	}

	globalID, err := resolveIID(ctx, client, input.ProjectID, input.MilestoneIID)
	if err != nil {
		return Output{}, fmt.Errorf("milestoneGet: %w", err)
	}

	m, _, err := client.GL().Milestones.GetMilestone(string(input.ProjectID), globalID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("milestoneGet", err, http.StatusNotFound,
			"verify milestone_iid with gitlab_milestone_list; project_id must match the project that owns the milestone")
	}
	return ToOutput(m), nil
}

// Create creates a new milestone in a GitLab project.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, errors.New("milestoneCreate: project_id is required")
	}
	if input.Title == "" {
		return Output{}, errors.New("milestoneCreate: title is required")
	}

	opts := &gl.CreateMilestoneOptions{
		Title: new(input.Title),
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.StartDate != "" {
		d, err := parseISODate(input.StartDate)
		if err != nil {
			return Output{}, fmt.Errorf("milestoneCreate: start_date: %w", err)
		}
		opts.StartDate = d
	}
	if input.DueDate != "" {
		d, err := parseISODate(input.DueDate)
		if err != nil {
			return Output{}, fmt.Errorf("milestoneCreate: due_date: %w", err)
		}
		opts.DueDate = d
	}

	m, _, err := client.GL().Milestones.CreateMilestone(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("milestoneCreate", err, "check that the title is unique and dates are in YYYY-MM-DD format")
		}
		return Output{}, toolutil.WrapErrWithMessage("milestoneCreate", err)
	}
	return ToOutput(m), nil
}

// Update modifies an existing milestone (resolved by IID).
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, errors.New("milestoneUpdate: project_id is required")
	}
	if input.MilestoneIID == 0 {
		return Output{}, toolutil.ErrRequiredInt64("milestoneUpdate", "milestone_iid")
	}

	globalID, err := resolveIID(ctx, client, input.ProjectID, input.MilestoneIID)
	if err != nil {
		return Output{}, fmt.Errorf("milestoneUpdate: %w", err)
	}

	opts := &gl.UpdateMilestoneOptions{}
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
			return Output{}, fmt.Errorf("milestoneUpdate: start_date: %w", err)
		}
		opts.StartDate = d
	}
	if input.DueDate != "" {
		var d *gl.ISOTime
		d, err = parseISODate(input.DueDate)
		if err != nil {
			return Output{}, fmt.Errorf("milestoneUpdate: due_date: %w", err)
		}
		opts.DueDate = d
	}
	if input.StateEvent != "" {
		opts.StateEvent = new(input.StateEvent)
	}

	m, _, err := client.GL().Milestones.UpdateMilestone(string(input.ProjectID), globalID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("milestoneUpdate", err, http.StatusBadRequest,
			"state_event must be 'close' or 'activate'; dates must be YYYY-MM-DD with start_date <= due_date")
	}
	return ToOutput(m), nil
}

// Delete removes a milestone from a project (resolved by IID).
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return errors.New("milestoneDelete: project_id is required")
	}
	if input.MilestoneIID == 0 {
		return toolutil.ErrRequiredInt64("milestoneDelete", "milestone_iid")
	}

	globalID, err := resolveIID(ctx, client, input.ProjectID, input.MilestoneIID)
	if err != nil {
		return fmt.Errorf("milestoneDelete: %w", err)
	}

	_, err = client.GL().Milestones.DeleteMilestone(string(input.ProjectID), globalID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("milestoneDelete", err, http.StatusForbidden,
			"deleting milestones requires Maintainer or Owner role")
	}
	return nil
}

// GetIssues retrieves issues assigned to a milestone (resolved by IID).
func GetIssues(ctx context.Context, client *gitlabclient.Client, input GetIssuesInput) (MilestoneIssuesOutput, error) {
	if input.ProjectID == "" {
		return MilestoneIssuesOutput{}, errors.New("milestoneGetIssues: project_id is required")
	}
	if input.MilestoneIID == 0 {
		return MilestoneIssuesOutput{}, toolutil.ErrRequiredInt64("milestoneGetIssues", "milestone_iid")
	}

	globalID, err := resolveIID(ctx, client, input.ProjectID, input.MilestoneIID)
	if err != nil {
		return MilestoneIssuesOutput{}, fmt.Errorf("milestoneGetIssues: %w", err)
	}

	opts := &gl.GetMilestoneIssuesOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	issues, resp, err := client.GL().Milestones.GetMilestoneIssues(string(input.ProjectID), globalID, opts, gl.WithContext(ctx))
	if err != nil {
		return MilestoneIssuesOutput{}, toolutil.WrapErrWithStatusHint("milestoneGetIssues", err, http.StatusNotFound,
			"verify milestone_iid with gitlab_milestone_list")
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
	return MilestoneIssuesOutput{Issues: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetMergeRequests retrieves merge requests assigned to a milestone (resolved by IID).
func GetMergeRequests(ctx context.Context, client *gitlabclient.Client, input GetMergeRequestsInput) (MilestoneMergeRequestsOutput, error) {
	if input.ProjectID == "" {
		return MilestoneMergeRequestsOutput{}, errors.New("milestoneGetMergeRequests: project_id is required")
	}
	if input.MilestoneIID == 0 {
		return MilestoneMergeRequestsOutput{}, toolutil.ErrRequiredInt64("milestoneGetMergeRequests", "milestone_iid")
	}

	globalID, err := resolveIID(ctx, client, input.ProjectID, input.MilestoneIID)
	if err != nil {
		return MilestoneMergeRequestsOutput{}, fmt.Errorf("milestoneGetMergeRequests: %w", err)
	}

	opts := &gl.GetMilestoneMergeRequestsOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	mrs, resp, err := client.GL().Milestones.GetMilestoneMergeRequests(string(input.ProjectID), globalID, opts, gl.WithContext(ctx))
	if err != nil {
		return MilestoneMergeRequestsOutput{}, toolutil.WrapErrWithStatusHint("milestoneGetMergeRequests", err, http.StatusNotFound,
			"verify milestone_iid with gitlab_milestone_list")
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
			WebURL:       mr.WebURL,
		}
		if mr.CreatedAt != nil {
			items[i].CreatedAt = mr.CreatedAt.Format(time.RFC3339)
		}
	}
	return MilestoneMergeRequestsOutput{MergeRequests: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------- Formatters ----------.

// ---------- Helpers ----------.

// parseISODate converts a YYYY-MM-DD string to *gl.ISOTime.
func parseISODate(s string) (*gl.ISOTime, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, fmt.Errorf("invalid date format (expected YYYY-MM-DD): %w", err)
	}
	d := gl.ISOTime(t)
	return &d, nil
}
