// Package resourceevents implements MCP tool handlers for GitLab resource
// label events, milestone events, and state events. These track changes to
// labels, milestones, and states on issues and merge requests.
package resourceevents

import (
	"context"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	fmtPropertyValueTableHeader = "| Property | Value |\n|---|---|\n"
	fmtUserRow                  = "| User | %s |\n"
	fmtResourceRow              = "| Resource | %s #%d |\n"
	fmtCreatedRow               = "| Created | %s |\n"
	fmtEventTableRow            = "| %d | %s | %s | %s | %s |\n"
	fmtActionRow                = "| Action | %s |\n"
)

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------.

// ListIssueLabelEventsInput defines parameters for listing issue label events.
type ListIssueLabelEventsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	toolutil.PaginationInput
}

// GetIssueLabelEventInput defines parameters for getting a single issue label event.
type GetIssueLabelEventInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID     int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	LabelEventID int64                `json:"label_event_id" jsonschema:"Label event ID,required"`
}

// ListMRLabelEventsInput defines parameters for listing merge request label events.
type ListMRLabelEventsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid" jsonschema:"Merge request internal ID,required"`
	toolutil.PaginationInput
}

// GetMRLabelEventInput defines parameters for getting a single MR label event.
type GetMRLabelEventInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID        int64                `json:"mr_iid" jsonschema:"Merge request internal ID,required"`
	LabelEventID int64                `json:"label_event_id" jsonschema:"Label event ID,required"`
}

// ListIssueMilestoneEventsInput defines parameters for listing issue milestone events.
type ListIssueMilestoneEventsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	toolutil.PaginationInput
}

// GetIssueMilestoneEventInput defines parameters for getting a single issue milestone event.
type GetIssueMilestoneEventInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID         int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	MilestoneEventID int64                `json:"milestone_event_id" jsonschema:"Milestone event ID,required"`
}

// ListMRMilestoneEventsInput defines parameters for listing MR milestone events.
type ListMRMilestoneEventsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid" jsonschema:"Merge request internal ID,required"`
	toolutil.PaginationInput
}

// GetMRMilestoneEventInput defines parameters for getting a single MR milestone event.
type GetMRMilestoneEventInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID            int64                `json:"mr_iid" jsonschema:"Merge request internal ID,required"`
	MilestoneEventID int64                `json:"milestone_event_id" jsonschema:"Milestone event ID,required"`
}

// ListIssueStateEventsInput defines parameters for listing issue state events.
type ListIssueStateEventsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	toolutil.PaginationInput
}

// GetIssueStateEventInput defines parameters for getting a single issue state event.
type GetIssueStateEventInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID     int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	StateEventID int64                `json:"state_event_id" jsonschema:"State event ID,required"`
}

// ListMRStateEventsInput defines parameters for listing MR state events.
type ListMRStateEventsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid" jsonschema:"Merge request internal ID,required"`
	toolutil.PaginationInput
}

// GetMRStateEventInput defines parameters for getting a single MR state event.
type GetMRStateEventInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID        int64                `json:"mr_iid" jsonschema:"Merge request internal ID,required"`
	StateEventID int64                `json:"state_event_id" jsonschema:"State event ID,required"`
}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// LabelEventLabelOutput represents the label in a label event.
type LabelEventLabelOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	TextColor   string `json:"text_color"`
	Description string `json:"description"`
}

// LabelEventOutput represents a resource label event.
type LabelEventOutput struct {
	toolutil.HintableOutput
	ID           int64                 `json:"id"`
	Action       string                `json:"action"`
	CreatedAt    string                `json:"created_at"`
	ResourceType string                `json:"resource_type"`
	ResourceID   int64                 `json:"resource_id"`
	UserID       int64                 `json:"user_id"`
	Username     string                `json:"username"`
	Label        LabelEventLabelOutput `json:"label"`
}

// ListLabelEventsOutput wraps a list of label events.
type ListLabelEventsOutput struct {
	toolutil.HintableOutput
	Events     []LabelEventOutput        `json:"events"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// MilestoneEventOutput represents a resource milestone event.
type MilestoneEventOutput struct {
	toolutil.HintableOutput
	ID             int64  `json:"id"`
	Action         string `json:"action"`
	CreatedAt      string `json:"created_at"`
	ResourceType   string `json:"resource_type"`
	ResourceID     int64  `json:"resource_id"`
	UserID         int64  `json:"user_id"`
	Username       string `json:"username"`
	MilestoneID    int64  `json:"milestone_id"`
	MilestoneTitle string `json:"milestone_title"`
}

// ListMilestoneEventsOutput wraps a list of milestone events.
type ListMilestoneEventsOutput struct {
	toolutil.HintableOutput
	Events     []MilestoneEventOutput    `json:"events"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// StateEventOutput represents a resource state event.
type StateEventOutput struct {
	toolutil.HintableOutput
	ID           int64  `json:"id"`
	State        string `json:"state"`
	CreatedAt    string `json:"created_at"`
	ResourceType string `json:"resource_type"`
	ResourceID   int64  `json:"resource_id"`
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
}

// ListStateEventsOutput wraps a list of state events.
type ListStateEventsOutput struct {
	toolutil.HintableOutput
	Events     []StateEventOutput        `json:"events"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Handlers — Label Events
// ---------------------------------------------------------------------------.

// ListIssueLabelEvents lists label events for an issue.
func ListIssueLabelEvents(ctx context.Context, client *gitlabclient.Client, input ListIssueLabelEventsInput) (ListLabelEventsOutput, error) {
	if input.ProjectID == "" {
		return ListLabelEventsOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.IssueIID <= 0 {
		return ListLabelEventsOutput{}, toolutil.ErrRequiredInt64("gitlab_issue_label_event_list", "issue_iid")
	}
	opts := &gl.ListLabelEventsOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	events, resp, err := client.GL().ResourceLabelEvents.ListIssueLabelEvents(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListLabelEventsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_issue_label_event_list", err, http.StatusNotFound,
			"verify project_id and issue_iid (the per-project issue number) with gitlab_issue_get")
	}
	return toLabelEventsOutput(events, resp), nil
}

// GetIssueLabelEvent gets a single label event for an issue.
func GetIssueLabelEvent(ctx context.Context, client *gitlabclient.Client, input GetIssueLabelEventInput) (LabelEventOutput, error) {
	if input.ProjectID == "" {
		return LabelEventOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.IssueIID <= 0 {
		return LabelEventOutput{}, toolutil.ErrRequiredInt64("gitlab_issue_label_event_get", "issue_iid")
	}
	if input.LabelEventID <= 0 {
		return LabelEventOutput{}, toolutil.ErrRequiredInt64("gitlab_issue_label_event_get", "label_event_id")
	}
	event, _, err := client.GL().ResourceLabelEvents.GetIssueLabelEvent(string(input.ProjectID), input.IssueIID, input.LabelEventID, gl.WithContext(ctx))
	if err != nil {
		return LabelEventOutput{}, toolutil.WrapErrWithStatusHint("gitlab_issue_label_event_get", err, http.StatusNotFound,
			"verify label_event_id with gitlab_issue_label_event_list")
	}
	return toLabelEventOutput(event), nil
}

// ListMRLabelEvents lists label events for a merge request.
func ListMRLabelEvents(ctx context.Context, client *gitlabclient.Client, input ListMRLabelEventsInput) (ListLabelEventsOutput, error) {
	if input.ProjectID == "" {
		return ListLabelEventsOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return ListLabelEventsOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_label_event_list", "mr_iid")
	}
	opts := &gl.ListLabelEventsOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	events, resp, err := client.GL().ResourceLabelEvents.ListMergeRequestsLabelEvents(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListLabelEventsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_mr_label_event_list", err, http.StatusNotFound,
			"verify project_id and mr_iid (per-project MR number) with gitlab_mr_get")
	}
	return toLabelEventsOutput(events, resp), nil
}

// GetMRLabelEvent gets a single label event for a merge request.
func GetMRLabelEvent(ctx context.Context, client *gitlabclient.Client, input GetMRLabelEventInput) (LabelEventOutput, error) {
	if input.ProjectID == "" {
		return LabelEventOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return LabelEventOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_label_event_get", "mr_iid")
	}
	if input.LabelEventID <= 0 {
		return LabelEventOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_label_event_get", "label_event_id")
	}
	event, _, err := client.GL().ResourceLabelEvents.GetMergeRequestLabelEvent(string(input.ProjectID), input.MRIID, input.LabelEventID, gl.WithContext(ctx))
	if err != nil {
		return LabelEventOutput{}, toolutil.WrapErrWithStatusHint("gitlab_mr_label_event_get", err, http.StatusNotFound,
			"verify label_event_id with gitlab_mr_label_event_list")
	}
	return toLabelEventOutput(event), nil
}

// ---------------------------------------------------------------------------
// Handlers — Milestone Events
// ---------------------------------------------------------------------------.

// ListIssueMilestoneEvents lists milestone events for an issue.
func ListIssueMilestoneEvents(ctx context.Context, client *gitlabclient.Client, input ListIssueMilestoneEventsInput) (ListMilestoneEventsOutput, error) {
	if input.ProjectID == "" {
		return ListMilestoneEventsOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.IssueIID <= 0 {
		return ListMilestoneEventsOutput{}, toolutil.ErrRequiredInt64("gitlab_issue_milestone_event_list", "issue_iid")
	}
	opts := &gl.ListMilestoneEventsOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	events, resp, err := client.GL().ResourceMilestoneEvents.ListIssueMilestoneEvents(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListMilestoneEventsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_issue_milestone_event_list", err, http.StatusNotFound,
			"verify project_id and issue_iid with gitlab_issue_get")
	}
	return toMilestoneEventsOutput(events, resp), nil
}

// GetIssueMilestoneEvent gets a single milestone event for an issue.
func GetIssueMilestoneEvent(ctx context.Context, client *gitlabclient.Client, input GetIssueMilestoneEventInput) (MilestoneEventOutput, error) {
	if input.ProjectID == "" {
		return MilestoneEventOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.IssueIID <= 0 {
		return MilestoneEventOutput{}, toolutil.ErrRequiredInt64("gitlab_issue_milestone_event_get", "issue_iid")
	}
	if input.MilestoneEventID <= 0 {
		return MilestoneEventOutput{}, toolutil.ErrRequiredInt64("gitlab_issue_milestone_event_get", "milestone_event_id")
	}
	event, _, err := client.GL().ResourceMilestoneEvents.GetIssueMilestoneEvent(string(input.ProjectID), input.IssueIID, input.MilestoneEventID, gl.WithContext(ctx))
	if err != nil {
		return MilestoneEventOutput{}, toolutil.WrapErrWithStatusHint("gitlab_issue_milestone_event_get", err, http.StatusNotFound,
			"verify milestone_event_id with gitlab_issue_milestone_event_list")
	}
	return toMilestoneEventOutput(event), nil
}

// ListMRMilestoneEvents lists milestone events for a merge request.
func ListMRMilestoneEvents(ctx context.Context, client *gitlabclient.Client, input ListMRMilestoneEventsInput) (ListMilestoneEventsOutput, error) {
	if input.ProjectID == "" {
		return ListMilestoneEventsOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return ListMilestoneEventsOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_milestone_event_list", "mr_iid")
	}
	opts := &gl.ListMilestoneEventsOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	events, resp, err := client.GL().ResourceMilestoneEvents.ListMergeMilestoneEvents(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListMilestoneEventsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_mr_milestone_event_list", err, http.StatusNotFound,
			"verify project_id and mr_iid with gitlab_mr_get")
	}
	return toMilestoneEventsOutput(events, resp), nil
}

// GetMRMilestoneEvent gets a single milestone event for a merge request.
func GetMRMilestoneEvent(ctx context.Context, client *gitlabclient.Client, input GetMRMilestoneEventInput) (MilestoneEventOutput, error) {
	if input.ProjectID == "" {
		return MilestoneEventOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return MilestoneEventOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_milestone_event_get", "mr_iid")
	}
	if input.MilestoneEventID <= 0 {
		return MilestoneEventOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_milestone_event_get", "milestone_event_id")
	}
	event, _, err := client.GL().ResourceMilestoneEvents.GetMergeRequestMilestoneEvent(string(input.ProjectID), input.MRIID, input.MilestoneEventID, gl.WithContext(ctx))
	if err != nil {
		return MilestoneEventOutput{}, toolutil.WrapErrWithStatusHint("gitlab_mr_milestone_event_get", err, http.StatusNotFound,
			"verify milestone_event_id with gitlab_mr_milestone_event_list")
	}
	return toMilestoneEventOutput(event), nil
}

// ---------------------------------------------------------------------------
// Handlers — State Events
// ---------------------------------------------------------------------------.

// ListIssueStateEvents lists state events for an issue.
func ListIssueStateEvents(ctx context.Context, client *gitlabclient.Client, input ListIssueStateEventsInput) (ListStateEventsOutput, error) {
	if input.ProjectID == "" {
		return ListStateEventsOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.IssueIID <= 0 {
		return ListStateEventsOutput{}, toolutil.ErrRequiredInt64("gitlab_issue_state_event_list", "issue_iid")
	}
	opts := &gl.ListStateEventsOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	events, resp, err := client.GL().ResourceStateEvents.ListIssueStateEvents(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListStateEventsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_issue_state_event_list", err, http.StatusNotFound,
			"verify project_id and issue_iid with gitlab_issue_get")
	}
	return toStateEventsOutput(events, resp), nil
}

// GetIssueStateEvent gets a single state event for an issue.
func GetIssueStateEvent(ctx context.Context, client *gitlabclient.Client, input GetIssueStateEventInput) (StateEventOutput, error) {
	if input.ProjectID == "" {
		return StateEventOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.IssueIID <= 0 {
		return StateEventOutput{}, toolutil.ErrRequiredInt64("gitlab_issue_state_event_get", "issue_iid")
	}
	if input.StateEventID <= 0 {
		return StateEventOutput{}, toolutil.ErrRequiredInt64("gitlab_issue_state_event_get", "state_event_id")
	}
	event, _, err := client.GL().ResourceStateEvents.GetIssueStateEvent(string(input.ProjectID), input.IssueIID, input.StateEventID, gl.WithContext(ctx))
	if err != nil {
		return StateEventOutput{}, toolutil.WrapErrWithStatusHint("gitlab_issue_state_event_get", err, http.StatusNotFound,
			"verify state_event_id with gitlab_issue_state_event_list")
	}
	return toStateEventOutput(event), nil
}

// ListMRStateEvents lists state events for a merge request.
func ListMRStateEvents(ctx context.Context, client *gitlabclient.Client, input ListMRStateEventsInput) (ListStateEventsOutput, error) {
	if input.ProjectID == "" {
		return ListStateEventsOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return ListStateEventsOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_state_event_list", "mr_iid")
	}
	opts := &gl.ListStateEventsOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	events, resp, err := client.GL().ResourceStateEvents.ListMergeStateEvents(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListStateEventsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_mr_state_event_list", err, http.StatusNotFound,
			"verify project_id and mr_iid with gitlab_mr_get")
	}
	return toStateEventsOutput(events, resp), nil
}

// GetMRStateEvent gets a single state event for a merge request.
func GetMRStateEvent(ctx context.Context, client *gitlabclient.Client, input GetMRStateEventInput) (StateEventOutput, error) {
	if input.ProjectID == "" {
		return StateEventOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return StateEventOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_state_event_get", "mr_iid")
	}
	if input.StateEventID <= 0 {
		return StateEventOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_state_event_get", "state_event_id")
	}
	event, _, err := client.GL().ResourceStateEvents.GetMergeRequestStateEvent(string(input.ProjectID), input.MRIID, input.StateEventID, gl.WithContext(ctx))
	if err != nil {
		return StateEventOutput{}, toolutil.WrapErrWithStatusHint("gitlab_mr_state_event_get", err, http.StatusNotFound,
			"verify state_event_id with gitlab_mr_state_event_list")
	}
	return toStateEventOutput(event), nil
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// toLabelEventOutput converts the GitLab API response to the tool output format.
func toLabelEventOutput(e *gl.LabelEvent) LabelEventOutput {
	if e == nil {
		return LabelEventOutput{}
	}
	out := LabelEventOutput{
		ID:           e.ID,
		Action:       e.Action,
		ResourceType: e.ResourceType,
		ResourceID:   e.ResourceID,
		UserID:       e.User.ID,
		Username:     e.User.Username,
		Label: LabelEventLabelOutput{
			ID:          e.Label.ID,
			Name:        e.Label.Name,
			Color:       e.Label.Color,
			TextColor:   e.Label.TextColor,
			Description: e.Label.Description,
		},
	}
	if e.CreatedAt != nil {
		out.CreatedAt = e.CreatedAt.Format(toolutil.DateTimeFormat)
	}
	return out
}

// toLabelEventsOutput converts the GitLab API response to the tool output format.
func toLabelEventsOutput(events []*gl.LabelEvent, resp *gl.Response) ListLabelEventsOutput {
	out := ListLabelEventsOutput{
		Events:     make([]LabelEventOutput, 0, len(events)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, e := range events {
		out.Events = append(out.Events, toLabelEventOutput(e))
	}
	return out
}

// toMilestoneEventOutput converts the GitLab API response to the tool output format.
func toMilestoneEventOutput(e *gl.MilestoneEvent) MilestoneEventOutput {
	if e == nil {
		return MilestoneEventOutput{}
	}
	out := MilestoneEventOutput{
		ID:           e.ID,
		Action:       e.Action,
		ResourceType: e.ResourceType,
		ResourceID:   e.ResourceID,
	}
	if e.User != nil {
		out.UserID = e.User.ID
		out.Username = e.User.Username
	}
	if e.Milestone != nil {
		out.MilestoneID = e.Milestone.ID
		out.MilestoneTitle = e.Milestone.Title
	}
	if e.CreatedAt != nil {
		out.CreatedAt = e.CreatedAt.Format(toolutil.DateTimeFormat)
	}
	return out
}

// toMilestoneEventsOutput converts the GitLab API response to the tool output format.
func toMilestoneEventsOutput(events []*gl.MilestoneEvent, resp *gl.Response) ListMilestoneEventsOutput {
	out := ListMilestoneEventsOutput{
		Events:     make([]MilestoneEventOutput, 0, len(events)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, e := range events {
		out.Events = append(out.Events, toMilestoneEventOutput(e))
	}
	return out
}

// toStateEventOutput converts the GitLab API response to the tool output format.
func toStateEventOutput(e *gl.StateEvent) StateEventOutput {
	if e == nil {
		return StateEventOutput{}
	}
	out := StateEventOutput{
		ID:           e.ID,
		State:        string(e.State),
		ResourceType: e.ResourceType,
		ResourceID:   e.ResourceID,
	}
	if e.User != nil {
		out.UserID = e.User.ID
		out.Username = e.User.Username
	}
	if e.CreatedAt != nil {
		out.CreatedAt = e.CreatedAt.Format(toolutil.DateTimeFormat)
	}
	return out
}

// toStateEventsOutput converts the GitLab API response to the tool output format.
func toStateEventsOutput(events []*gl.StateEvent, resp *gl.Response) ListStateEventsOutput {
	out := ListStateEventsOutput{
		Events:     make([]StateEventOutput, 0, len(events)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, e := range events {
		out.Events = append(out.Events, toStateEventOutput(e))
	}
	return out
}

// ---------------------------------------------------------------------------
// Input types — Iteration Events
// ---------------------------------------------------------------------------.

// ListIssueIterationEventsInput defines parameters for listing issue iteration events.
type ListIssueIterationEventsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	toolutil.PaginationInput
}

// GetIssueIterationEventInput defines parameters for getting a single issue iteration event.
type GetIssueIterationEventInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID         int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	IterationEventID int64                `json:"iteration_event_id" jsonschema:"Iteration event ID,required"`
}

// ---------------------------------------------------------------------------
// Input types — Weight Events
// ---------------------------------------------------------------------------.

// ListIssueWeightEventsInput defines parameters for listing issue weight events.
type ListIssueWeightEventsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	toolutil.PaginationInput
}

// ---------------------------------------------------------------------------
// Output types — Iteration Events
// ---------------------------------------------------------------------------.

// IterationEventIterationOutput represents the iteration in an iteration event.
type IterationEventIterationOutput struct {
	ID        int64  `json:"id"`
	IID       int64  `json:"iid"`
	Sequence  int64  `json:"sequence"`
	GroupID   int64  `json:"group_id"`
	Title     string `json:"title"`
	State     int64  `json:"state"`
	WebURL    string `json:"web_url,omitempty"`
	StartDate string `json:"start_date,omitempty"`
	DueDate   string `json:"due_date,omitempty"`
}

// IterationEventOutput represents a resource iteration event.
type IterationEventOutput struct {
	toolutil.HintableOutput
	ID           int64                         `json:"id"`
	Action       string                        `json:"action"`
	CreatedAt    string                        `json:"created_at"`
	ResourceType string                        `json:"resource_type"`
	ResourceID   int64                         `json:"resource_id"`
	UserID       int64                         `json:"user_id"`
	Username     string                        `json:"username"`
	Iteration    IterationEventIterationOutput `json:"iteration"`
}

// ListIterationEventsOutput wraps a list of iteration events.
type ListIterationEventsOutput struct {
	toolutil.HintableOutput
	Events     []IterationEventOutput    `json:"events"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Output types — Weight Events
// ---------------------------------------------------------------------------.

// WeightEventOutput represents a resource weight event.
type WeightEventOutput struct {
	ID           int64  `json:"id"`
	CreatedAt    string `json:"created_at"`
	ResourceType string `json:"resource_type"`
	ResourceID   int64  `json:"resource_id"`
	State        string `json:"state"`
	IssueID      int64  `json:"issue_id"`
	Weight       int64  `json:"weight"`
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
}

// ListWeightEventsOutput wraps a list of weight events.
type ListWeightEventsOutput struct {
	toolutil.HintableOutput
	Events     []WeightEventOutput       `json:"events"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Handlers — Iteration Events
// ---------------------------------------------------------------------------.

// ListIssueIterationEvents lists iteration events for an issue.
func ListIssueIterationEvents(ctx context.Context, client *gitlabclient.Client, input ListIssueIterationEventsInput) (ListIterationEventsOutput, error) {
	if input.ProjectID == "" {
		return ListIterationEventsOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.IssueIID <= 0 {
		return ListIterationEventsOutput{}, toolutil.ErrRequiredInt64("gitlab_issue_iteration_event_list", "issue_iid")
	}
	opts := &gl.ListIterationEventsOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	events, resp, err := client.GL().ResourceIterationEvents.ListIssueIterationEvents(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListIterationEventsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_issue_iteration_event_list", err, http.StatusNotFound,
			"iteration events require GitLab Premium/Ultimate \u2014 verify the project tier and that issue_iid exists")
	}
	return toIterationEventsOutput(events, resp), nil
}

// GetIssueIterationEvent gets a single iteration event for an issue.
func GetIssueIterationEvent(ctx context.Context, client *gitlabclient.Client, input GetIssueIterationEventInput) (IterationEventOutput, error) {
	if input.ProjectID == "" {
		return IterationEventOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.IssueIID <= 0 {
		return IterationEventOutput{}, toolutil.ErrRequiredInt64("gitlab_issue_iteration_event_get", "issue_iid")
	}
	if input.IterationEventID <= 0 {
		return IterationEventOutput{}, toolutil.ErrRequiredInt64("gitlab_issue_iteration_event_get", "iteration_event_id")
	}
	event, _, err := client.GL().ResourceIterationEvents.GetIssueIterationEvent(string(input.ProjectID), input.IssueIID, input.IterationEventID, gl.WithContext(ctx))
	if err != nil {
		return IterationEventOutput{}, toolutil.WrapErrWithStatusHint("gitlab_issue_iteration_event_get", err, http.StatusNotFound,
			"iteration events require Premium/Ultimate \u2014 verify iteration_event_id with gitlab_issue_iteration_event_list")
	}
	return toIterationEventOutput(event), nil
}

// ---------------------------------------------------------------------------
// Handlers — Weight Events
// ---------------------------------------------------------------------------.

// ListIssueWeightEvents lists weight events for an issue.
func ListIssueWeightEvents(ctx context.Context, client *gitlabclient.Client, input ListIssueWeightEventsInput) (ListWeightEventsOutput, error) {
	if input.ProjectID == "" {
		return ListWeightEventsOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.IssueIID <= 0 {
		return ListWeightEventsOutput{}, toolutil.ErrRequiredInt64("gitlab_issue_weight_event_list", "issue_iid")
	}
	opts := &gl.ListWeightEventsOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	events, resp, err := client.GL().ResourceWeightEvents.ListIssueWeightEvents(string(input.ProjectID), input.IssueIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListWeightEventsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_issue_weight_event_list", err, http.StatusNotFound,
			"weight events require GitLab Premium/Ultimate \u2014 verify the project tier and that issue_iid exists")
	}
	return toWeightEventsOutput(events, resp), nil
}

// ---------------------------------------------------------------------------
// Converters — Iteration Events
// ---------------------------------------------------------------------------.

func toIterationEventOutput(e *gl.IterationEvent) IterationEventOutput {
	if e == nil {
		return IterationEventOutput{}
	}
	out := IterationEventOutput{
		ID:           e.ID,
		Action:       e.Action,
		ResourceType: e.ResourceType,
		ResourceID:   e.ResourceID,
	}
	if e.User != nil {
		out.UserID = e.User.ID
		out.Username = e.User.Username
	}
	if e.Iteration != nil {
		out.Iteration = IterationEventIterationOutput{
			ID:       e.Iteration.ID,
			IID:      e.Iteration.IID,
			Sequence: e.Iteration.Sequence,
			GroupID:  e.Iteration.GroupID,
			Title:    e.Iteration.Title,
			State:    e.Iteration.State,
			WebURL:   e.Iteration.WebURL,
		}
		if e.Iteration.StartDate != nil {
			out.Iteration.StartDate = e.Iteration.StartDate.String()
		}
		if e.Iteration.DueDate != nil {
			out.Iteration.DueDate = e.Iteration.DueDate.String()
		}
	}
	if e.CreatedAt != nil {
		out.CreatedAt = e.CreatedAt.Format(toolutil.DateTimeFormat)
	}
	return out
}

func toIterationEventsOutput(events []*gl.IterationEvent, resp *gl.Response) ListIterationEventsOutput {
	out := ListIterationEventsOutput{
		Events:     make([]IterationEventOutput, 0, len(events)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, e := range events {
		out.Events = append(out.Events, toIterationEventOutput(e))
	}
	return out
}

// ---------------------------------------------------------------------------
// Converters — Weight Events
// ---------------------------------------------------------------------------.

func toWeightEventOutput(e *gl.WeightEvent) WeightEventOutput {
	if e == nil {
		return WeightEventOutput{}
	}
	out := WeightEventOutput{
		ID:           e.ID,
		ResourceType: e.ResourceType,
		ResourceID:   e.ResourceID,
		State:        string(e.State),
		IssueID:      e.IssueID,
		Weight:       e.Weight,
	}
	if e.User != nil {
		out.UserID = e.User.ID
		out.Username = e.User.Username
	}
	if e.CreatedAt != nil {
		out.CreatedAt = e.CreatedAt.Format(toolutil.DateTimeFormat)
	}
	return out
}

func toWeightEventsOutput(events []*gl.WeightEvent, resp *gl.Response) ListWeightEventsOutput {
	out := ListWeightEventsOutput{
		Events:     make([]WeightEventOutput, 0, len(events)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, e := range events {
		out.Events = append(out.Events, toWeightEventOutput(e))
	}
	return out
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
