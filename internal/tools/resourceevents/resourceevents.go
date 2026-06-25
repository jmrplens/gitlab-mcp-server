package resourceevents

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort direction for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	MRIID     int64                `json:"merge_request_iid" jsonschema:"Merge request internal ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort direction for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetMRLabelEventInput defines parameters for getting a single MR label event.
type GetMRLabelEventInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID        int64                `json:"merge_request_iid" jsonschema:"Merge request internal ID,required"`
	LabelEventID int64                `json:"label_event_id" jsonschema:"Label event ID,required"`
}

// ListGroupEpicLabelEventsInput defines parameters for listing group epic label events.
type ListGroupEpicLabelEventsInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID int64                `json:"epic_iid" jsonschema:"Epic internal ID (IID) within the group,required"`
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort    string               `json:"sort,omitempty" jsonschema:"Sort direction for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetGroupEpicLabelEventInput defines parameters for getting a single group epic label event.
type GetGroupEpicLabelEventInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID      int64                `json:"epic_iid" jsonschema:"Epic internal ID (IID) within the group,required"`
	LabelEventID int64                `json:"label_event_id" jsonschema:"Label event ID,required"`
}

// ListIssueMilestoneEventsInput defines parameters for listing issue milestone events.
type ListIssueMilestoneEventsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort direction for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	MRIID     int64                `json:"merge_request_iid" jsonschema:"Merge request internal ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort direction for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetMRMilestoneEventInput defines parameters for getting a single MR milestone event.
type GetMRMilestoneEventInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID            int64                `json:"merge_request_iid" jsonschema:"Merge request internal ID,required"`
	MilestoneEventID int64                `json:"milestone_event_id" jsonschema:"Milestone event ID,required"`
}

// ListIssueStateEventsInput defines parameters for listing issue state events.
type ListIssueStateEventsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid" jsonschema:"Issue internal ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort direction for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	MRIID     int64                `json:"merge_request_iid" jsonschema:"Merge request internal ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort direction for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetMRStateEventInput defines parameters for getting a single MR state event.
type GetMRStateEventInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID        int64                `json:"merge_request_iid" jsonschema:"Merge request internal ID,required"`
	StateEventID int64                `json:"state_event_id" jsonschema:"State event ID,required"`
}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// LabelEventOutput represents a resource label event.
type LabelEventOutput struct {
	toolutil.HintableOutput
	ID           int64                  `json:"id"`
	Action       string                 `json:"action"`
	CreatedAt    string                 `json:"created_at"`
	ResourceType string                 `json:"resource_type"`
	ResourceID   int64                  `json:"resource_id"`
	User         *EventUserOutput       `json:"user,omitempty"`
	Label        *LabelEventLabelOutput `json:"label,omitempty"`
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
	ID           int64            `json:"id"`
	Action       string           `json:"action"`
	CreatedAt    string           `json:"created_at"`
	ResourceType string           `json:"resource_type"`
	ResourceID   int64            `json:"resource_id"`
	User         *EventUserOutput `json:"user,omitempty"`
	Milestone    *MilestoneOutput `json:"milestone,omitempty"`
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
	ID           int64            `json:"id"`
	State        string           `json:"state"`
	CreatedAt    string           `json:"created_at"`
	ResourceType string           `json:"resource_type"`
	ResourceID   int64            `json:"resource_id"`
	User         *EventUserOutput `json:"user,omitempty"`
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
	opts := &gl.ListLabelEventsOptions{}
	applyEventListOptions(&opts.ListOptions, input.OrderBy, input.Sort, input.PaginationInput, input.KeysetPaginationInput)
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
		return ListLabelEventsOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_label_event_list", "merge_request_iid")
	}
	opts := &gl.ListLabelEventsOptions{}
	applyEventListOptions(&opts.ListOptions, input.OrderBy, input.Sort, input.PaginationInput, input.KeysetPaginationInput)
	events, resp, err := client.GL().ResourceLabelEvents.ListMergeRequestsLabelEvents(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListLabelEventsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_mr_label_event_list", err, http.StatusNotFound,
			"verify project_id and merge_request_iid (per-project MR number) with gitlab_mr_get")
	}
	return toLabelEventsOutput(events, resp), nil
}

// GetMRLabelEvent gets a single label event for a merge request.
func GetMRLabelEvent(ctx context.Context, client *gitlabclient.Client, input GetMRLabelEventInput) (LabelEventOutput, error) {
	if input.ProjectID == "" {
		return LabelEventOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return LabelEventOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_label_event_get", "merge_request_iid")
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

// ListGroupEpicLabelEvents lists label events for a group epic (Premium/Ultimate).
func ListGroupEpicLabelEvents(ctx context.Context, client *gitlabclient.Client, input ListGroupEpicLabelEventsInput) (ListLabelEventsOutput, error) {
	if input.GroupID == "" {
		return ListLabelEventsOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.EpicIID <= 0 {
		return ListLabelEventsOutput{}, toolutil.ErrRequiredInt64("gitlab_list_group_epic_label_events", "epic_iid")
	}
	opts := &gl.ListLabelEventsOptions{}
	applyEventListOptions(&opts.ListOptions, input.OrderBy, input.Sort, input.PaginationInput, input.KeysetPaginationInput)
	events, resp, err := client.GL().ResourceLabelEvents.ListGroupEpicLabelEvents(string(input.GroupID), input.EpicIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListLabelEventsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_list_group_epic_label_events", err, http.StatusNotFound,
			"epic label events require GitLab Premium/Ultimate — verify group_id and epic_iid (the per-group epic number) with gitlab_epic_get")
	}
	return toLabelEventsOutput(events, resp), nil
}

// GetGroupEpicLabelEvent gets a single label event for a group epic (Premium/Ultimate).
func GetGroupEpicLabelEvent(ctx context.Context, client *gitlabclient.Client, input GetGroupEpicLabelEventInput) (LabelEventOutput, error) {
	if input.GroupID == "" {
		return LabelEventOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.EpicIID <= 0 {
		return LabelEventOutput{}, toolutil.ErrRequiredInt64("gitlab_get_group_epic_label_event", "epic_iid")
	}
	if input.LabelEventID <= 0 {
		return LabelEventOutput{}, toolutil.ErrRequiredInt64("gitlab_get_group_epic_label_event", "label_event_id")
	}
	event, _, err := client.GL().ResourceLabelEvents.GetGroupEpicLabelEvent(string(input.GroupID), input.EpicIID, input.LabelEventID, gl.WithContext(ctx))
	if err != nil {
		return LabelEventOutput{}, toolutil.WrapErrWithStatusHint("gitlab_get_group_epic_label_event", err, http.StatusNotFound,
			"epic label events require GitLab Premium/Ultimate — verify label_event_id with gitlab_list_group_epic_label_events")
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
	opts := &gl.ListMilestoneEventsOptions{}
	applyEventListOptions(&opts.ListOptions, input.OrderBy, input.Sort, input.PaginationInput, input.KeysetPaginationInput)
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
		return ListMilestoneEventsOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_milestone_event_list", "merge_request_iid")
	}
	opts := &gl.ListMilestoneEventsOptions{}
	applyEventListOptions(&opts.ListOptions, input.OrderBy, input.Sort, input.PaginationInput, input.KeysetPaginationInput)
	events, resp, err := client.GL().ResourceMilestoneEvents.ListMergeMilestoneEvents(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListMilestoneEventsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_mr_milestone_event_list", err, http.StatusNotFound,
			"verify project_id and merge_request_iid with gitlab_mr_get")
	}
	return toMilestoneEventsOutput(events, resp), nil
}

// GetMRMilestoneEvent gets a single milestone event for a merge request.
func GetMRMilestoneEvent(ctx context.Context, client *gitlabclient.Client, input GetMRMilestoneEventInput) (MilestoneEventOutput, error) {
	if input.ProjectID == "" {
		return MilestoneEventOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return MilestoneEventOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_milestone_event_get", "merge_request_iid")
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
	opts := &gl.ListStateEventsOptions{}
	applyEventListOptions(&opts.ListOptions, input.OrderBy, input.Sort, input.PaginationInput, input.KeysetPaginationInput)
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
		return ListStateEventsOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_state_event_list", "merge_request_iid")
	}
	opts := &gl.ListStateEventsOptions{}
	applyEventListOptions(&opts.ListOptions, input.OrderBy, input.Sort, input.PaginationInput, input.KeysetPaginationInput)
	events, resp, err := client.GL().ResourceStateEvents.ListMergeStateEvents(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListStateEventsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_mr_state_event_list", err, http.StatusNotFound,
			"verify project_id and merge_request_iid with gitlab_mr_get")
	}
	return toStateEventsOutput(events, resp), nil
}

// GetMRStateEvent gets a single state event for a merge request.
func GetMRStateEvent(ctx context.Context, client *gitlabclient.Client, input GetMRStateEventInput) (StateEventOutput, error) {
	if input.ProjectID == "" {
		return StateEventOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return StateEventOutput{}, toolutil.ErrRequiredInt64("gitlab_mr_state_event_get", "merge_request_iid")
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

// applyEventListOptions copies the order_by/sort plus offset and keyset
// pagination parameters from a list input onto a gl.ListOptions. The resource
// event endpoints expose only the shared ListOptions, so order_by and sort are
// set directly on the embedded struct (gl.ListOptions.OrderBy / .Sort).
func applyEventListOptions(opts *gl.ListOptions, orderBy, sort string, page toolutil.PaginationInput, keyset toolutil.KeysetPaginationInput) {
	if opts == nil {
		return
	}
	if orderBy != "" {
		opts.OrderBy = orderBy
	}
	if sort != "" {
		opts.Sort = sort
	}
	toolutil.ApplyListOptions(opts, page, keyset)
}

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
		User:         eventUserOutput(&e.User),
		Label:        labelEventLabelOutput(e.Label),
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
		User:         eventUserOutput(e.User),
		Milestone:    milestoneOutput(e.Milestone),
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
		User:         eventUserOutput(e.User),
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
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort direction for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort direction for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ---------------------------------------------------------------------------
// Output types — Iteration Events
// ---------------------------------------------------------------------------.

// IterationEventOutput represents a resource iteration event.
type IterationEventOutput struct {
	toolutil.HintableOutput
	ID           int64            `json:"id"`
	Action       string           `json:"action"`
	CreatedAt    string           `json:"created_at"`
	ResourceType string           `json:"resource_type"`
	ResourceID   int64            `json:"resource_id"`
	User         *EventUserOutput `json:"user,omitempty"`
	Iteration    *IterationOutput `json:"iteration,omitempty"`
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
	ID           int64            `json:"id"`
	CreatedAt    string           `json:"created_at"`
	ResourceType string           `json:"resource_type"`
	ResourceID   int64            `json:"resource_id"`
	State        string           `json:"state"`
	IssueID      int64            `json:"issue_id"`
	Weight       int64            `json:"weight"`
	User         *EventUserOutput `json:"user,omitempty"`
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
	opts := &gl.ListIterationEventsOptions{}
	applyEventListOptions(&opts.ListOptions, input.OrderBy, input.Sort, input.PaginationInput, input.KeysetPaginationInput)
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
	opts := &gl.ListWeightEventsOptions{}
	applyEventListOptions(&opts.ListOptions, input.OrderBy, input.Sort, input.PaginationInput, input.KeysetPaginationInput)
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
		User:         eventUserOutput(e.User),
		Iteration:    iterationOutput(e.Iteration),
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
		User:         eventUserOutput(e.User),
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
