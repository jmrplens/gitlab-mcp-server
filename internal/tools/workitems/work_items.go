package workitems

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// LinkedItem represents a linked work item summary.
type LinkedItem struct {
	IID      int64  `json:"iid"`
	LinkType string `json:"link_type"`
	Path     string `json:"path,omitempty"`
}

// ChildItem is a summary reference to a child work item in the hierarchy.
type ChildItem struct {
	IID  int64  `json:"iid" jsonschema:"Internal ID (IID) of the child work item"`
	Path string `json:"path,omitempty" jsonschema:"Namespace full path of the child work item"`
}

// WorkItemItem is a summary of a work item.
//
// The widget-backed fields below (color, dates, health status, iteration,
// milestone, parent, weight) arrive on every get, create and update, because
// the static fragment client-go uses for those three selects them
// unconditionally. On the list path they arrive only when the query asked for
// them: see [listReturnedFields].
type WorkItemItem struct {
	ID           int64        `json:"id"`
	IID          int64        `json:"iid"`
	Type         string       `json:"type"`
	State        string       `json:"state"`
	Status       string       `json:"status,omitempty"`
	Title        string       `json:"title"`
	Description  string       `json:"description,omitempty"`
	WebURL       string       `json:"web_url,omitempty"`
	Author       string       `json:"author,omitempty"`
	Assignees    []string     `json:"assignees,omitempty"`
	Labels       []string     `json:"labels,omitempty"`
	LinkedItems  []LinkedItem `json:"linked_items,omitempty"`
	Parent       *ChildItem   `json:"parent,omitempty" jsonschema:"Parent work item in the hierarchy (namespace path and IID)"`
	Children     []ChildItem  `json:"children,omitempty" jsonschema:"Child work items in the hierarchy (each with namespace path and IID)"`
	Color        string       `json:"color,omitempty" jsonschema:"Color of the work item as a hex code (e.g. #fefefe)"`
	MilestoneID  int64        `json:"milestone_id,omitempty" jsonschema:"Numeric ID of the milestone the work item belongs to"`
	IterationID  int64        `json:"iteration_id,omitempty" tier:"premium" jsonschema:"Numeric ID of the iteration the work item belongs to"`
	Weight       *int64       `json:"weight,omitempty" tier:"premium" jsonschema:"Weight of the work item"`
	HealthStatus string       `json:"health_status,omitempty" tier:"ultimate" jsonschema:"Health status (onTrack/needsAttention/atRisk)"`
	StartDate    string       `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD)"`
	DueDate      string       `json:"due_date,omitempty" jsonschema:"Due date (YYYY-MM-DD)"`
	Confidential bool         `json:"confidential,omitempty"`
	CreatedAt    string       `json:"created_at,omitempty"`
	UpdatedAt    string       `json:"updated_at,omitempty"`
	ClosedAt     string       `json:"closed_at,omitempty"`
}

// mapStatusToID maps a human-readable status string to the GitLab WorkItemStatusID GID.
func mapStatusToID(s string) gl.WorkItemStatusID {
	switch s {
	case "TODO":
		return gl.WorkItemStatusToDo
	case "IN_PROGRESS":
		return gl.WorkItemStatusInProgress
	case "DONE":
		return gl.WorkItemStatusDone
	case "WONT_DO":
		return gl.WorkItemStatusWontDo
	case "DUPLICATE":
		return gl.WorkItemStatusDuplicate
	default:
		return gl.WorkItemStatusID(s)
	}
}

// workItemToItem maps work item to item between API and evaluator models.
func workItemToItem(wi *gl.WorkItem) WorkItemItem {
	item := WorkItemItem{
		ID:           wi.ID,
		IID:          wi.IID,
		Type:         wi.Type,
		State:        wi.State,
		Title:        wi.Title,
		Description:  wi.Description,
		WebURL:       wi.WebURL,
		Confidential: wi.Confidential,
	}
	if wi.Status != nil {
		item.Status = *wi.Status
	}
	if wi.Author != nil {
		item.Author = wi.Author.Username
	}
	for _, a := range wi.Assignees {
		item.Assignees = append(item.Assignees, a.Username)
	}
	for _, l := range wi.Labels {
		item.Labels = append(item.Labels, l.Name)
	}
	for _, li := range wi.LinkedItems {
		item.LinkedItems = append(item.LinkedItems, LinkedItem{
			IID:      li.IID,
			LinkType: li.LinkType,
			Path:     li.NamespacePath,
		})
	}
	for _, c := range wi.Children {
		item.Children = append(item.Children, ChildItem{
			IID:  c.IID,
			Path: c.NamespacePath,
		})
	}
	applyWidgetFields(&item, wi)
	// RFC 3339 rather than time.Time's own String(), which is what every other
	// timestamp in this server emits (toolutil.FormatTimePtr) and what the raw
	// GraphQL list handler passed through verbatim before it moved onto the SDK.
	item.CreatedAt = toolutil.FormatTimePtr(wi.CreatedAt)
	item.UpdatedAt = toolutil.FormatTimePtr(wi.UpdatedAt)
	item.ClosedAt = toolutil.FormatTimePtr(wi.ClosedAt)
	return item
}

// applyWidgetFields copies the widget-backed values of wi onto item.
//
// Each is a pointer on the SDK type because the widget it comes from may be
// absent from the answer: a work item type that carries no weight widget, or a
// list query that did not ask for it, is not a work item of weight zero.
func applyWidgetFields(item *WorkItemItem, wi *gl.WorkItem) {
	if wi.Parent != nil {
		item.Parent = &ChildItem{IID: wi.Parent.IID, Path: wi.Parent.NamespacePath}
	}
	if wi.Color != nil {
		item.Color = *wi.Color
	}
	if wi.MilestoneID != nil {
		item.MilestoneID = *wi.MilestoneID
	}
	if wi.IterationID != nil {
		item.IterationID = *wi.IterationID
	}
	if wi.HealthStatus != nil {
		item.HealthStatus = *wi.HealthStatus
	}
	item.Weight = wi.Weight
	item.StartDate = toolutil.FormatISOTimePtr(wi.StartDate)
	item.DueDate = toolutil.FormatISOTimePtr(wi.DueDate)
}

// Get.

// GetInput is the input for getting a single work item.
type GetInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the project or group (e.g. my-group/my-project),required"`
	IID      int64  `json:"work_item_iid" jsonschema:"Work item IID,required"`
}

// GetOutput is the output for getting a single work item.
type GetOutput struct {
	toolutil.HintableOutput
	WorkItem WorkItemItem `json:"work_item"`
}

// Get retrieves a single work item by IID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if input.IID <= 0 {
		return GetOutput{}, toolutil.ErrRequiredInt64("get_work_item", "work_item_iid")
	}
	wi, _, err := client.GL().WorkItems.GetWorkItem(input.FullPath, input.IID, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("get_work_item", err, http.StatusNotFound,
			"verify full_path (group or project path) and iid (work item IID) with gitlab_list_work_items; Work Items API is experimental. Verify GitLab version supports the work item type")
	}
	return GetOutput{WorkItem: workItemToItem(wi)}, nil
}

// List.

// ListInput is the input for listing work items.
//
// The cursor parameters come from the shared type because this connection is
// one GitLab really does page in both directions: the SDK's own document
// declares first, after, last and before, and the output reports a previous
// page and a start cursor. Publishing only the forward half named a cursor no
// parameter here could spend.
type ListInput struct {
	FullPath       string   `json:"full_path" jsonschema:"Full path of the project or group,required"`
	State          string   `json:"state,omitempty" jsonschema:"Filter by state (opened/closed/all)"`
	Search         string   `json:"search,omitempty" jsonschema:"Search in title and description"`
	In             []string `json:"in,omitempty" jsonschema:"Fields the search term is matched against: TITLE or DESCRIPTION. Only meaningful together with search"`
	Types          []string `json:"types,omitempty" jsonschema:"Filter by work item types, IssueType enum values such as ISSUE or TASK"`
	AuthorUsername string   `json:"author_username,omitempty" jsonschema:"Filter by author username"`

	AssigneeUsernames  []string `json:"assignee_usernames,omitempty" jsonschema:"Filter by assignee usernames"`
	AssigneeWildcardID string   `json:"assignee_wildcard_id,omitempty" jsonschema:"Assignee wildcard filter: ANY, ME or NONE"`
	MyReactionEmoji    string   `json:"my_reaction_emoji,omitempty" jsonschema:"Filter by the emoji the authenticated user reacted with"`
	Subscribed         string   `json:"subscribed,omitempty" jsonschema:"Filter by the authenticated user's subscription: EXPLICITLY_SUBSCRIBED or EXPLICITLY_UNSUBSCRIBED"`
	// GitLab types both arguments String and compares the value against a
	// numeric column without parsing it, so a gid:// form casts to 0 and
	// silently matches nothing.
	CRMContactID      string `json:"crm_contact_id,omitempty" jsonschema:"Filter by CRM contact numeric ID as a string, e.g. 1. Not a global ID"`
	CRMOrganizationID string `json:"crm_organization_id,omitempty" jsonschema:"Filter by CRM organization numeric ID as a string, e.g. 1. Not a global ID"`

	// The two identifier filters are deliberately different shapes because
	// GitLab types them differently: ids takes full global IDs and iids takes
	// the numbers shown in the UI, as strings.
	IDs       []string `json:"ids,omitempty" jsonschema:"Filter by work item global IDs, each the full gid://gitlab/WorkItem/<id> form"`
	IIDs      []string `json:"iids,omitempty" jsonschema:"Filter by work item internal IDs (IIDs) as strings, e.g. 12"`
	ParentIDs []string `json:"parent_ids,omitempty" jsonschema:"Filter by parent work item global IDs, each the full gid://gitlab/WorkItem/<id> form. Pairs with include_descendants"`

	LabelName            []string `json:"label_name,omitempty" jsonschema:"Filter by label names"`
	MilestoneTitle       []string `json:"milestone_title,omitempty" jsonschema:"Filter by milestone titles. Titles, not the milestone_id that create and update take"`
	MilestoneWildcardID  string   `json:"milestone_wildcard_id,omitempty" jsonschema:"Milestone wildcard filter: ANY, NONE, STARTED or UPCOMING"`
	ReleaseTag           []string `json:"release_tag,omitempty" jsonschema:"Filter by release tags"`
	ReleaseTagWildcardID string   `json:"release_tag_wildcard_id,omitempty" jsonschema:"Release tag wildcard filter: ANY or NONE"`

	IterationID         []string `json:"iteration_id,omitempty" tier:"premium" jsonschema:"Filter by iteration global IDs, each the full gid://gitlab/Iteration/<id> form. Unlike the iteration_id create and update take, this one is a list of global IDs rather than a single numeric ID"`
	IterationCadenceID  []string `json:"iteration_cadence_id,omitempty" tier:"premium" jsonschema:"Filter by iteration cadence global IDs, each the full gid://gitlab/Iterations::Cadence/<id> form"`
	IterationWildcardID string   `json:"iteration_wildcard_id,omitempty" tier:"premium" jsonschema:"Iteration wildcard filter: ANY, CURRENT or NONE"`
	Weight              string   `json:"weight,omitempty" tier:"premium" jsonschema:"Filter by weight. GitLab types this filter as a string, so send 3 quoted rather than as a number"`
	WeightWildcardID    string   `json:"weight_wildcard_id,omitempty" tier:"premium" jsonschema:"Weight wildcard filter: ANY or NONE"`
	HealthStatusFilter  string   `json:"health_status_filter,omitempty" tier:"ultimate" jsonschema:"Filter by health status: onTrack, needsAttention, atRisk, ANY or NONE. Case sensitive"`

	ClosedAfter   string `json:"closed_after,omitempty" jsonschema:"Match work items closed after this timestamp. ISO 8601 date-time such as 2025-01-01T00:00:00Z. A bare date is read as midnight UTC"`
	ClosedBefore  string `json:"closed_before,omitempty" jsonschema:"Match work items closed before this timestamp. ISO 8601 date-time such as 2025-01-01T00:00:00Z. A bare date is read as midnight UTC"`
	CreatedAfter  string `json:"created_after,omitempty" jsonschema:"Match work items created after this timestamp. ISO 8601 date-time such as 2025-01-01T00:00:00Z. A bare date is read as midnight UTC"`
	CreatedBefore string `json:"created_before,omitempty" jsonschema:"Match work items created before this timestamp. ISO 8601 date-time such as 2025-01-01T00:00:00Z. A bare date is read as midnight UTC"`
	DueAfter      string `json:"due_after,omitempty" jsonschema:"Match work items due after this timestamp. ISO 8601 date-time such as 2025-01-01T00:00:00Z. A bare date is read as midnight UTC"`
	DueBefore     string `json:"due_before,omitempty" jsonschema:"Match work items due before this timestamp. ISO 8601 date-time such as 2025-01-01T00:00:00Z. A bare date is read as midnight UTC"`
	UpdatedAfter  string `json:"updated_after,omitempty" jsonschema:"Match work items updated after this timestamp. ISO 8601 date-time such as 2025-01-01T00:00:00Z. A bare date is read as midnight UTC"`
	UpdatedBefore string `json:"updated_before,omitempty" jsonschema:"Match work items updated before this timestamp. ISO 8601 date-time such as 2025-01-01T00:00:00Z. A bare date is read as midnight UTC"`

	Confidential       *bool  `json:"confidential,omitempty" jsonschema:"Filter by confidentiality"`
	Sort               string `json:"sort,omitempty" jsonschema:"Sort order, a WorkItemSort enum value such as CREATED_DESC or TITLE_ASC"`
	IncludeAncestors   *bool  `json:"include_ancestors,omitempty" jsonschema:"Include ancestor work items"`
	IncludeDescendants *bool  `json:"include_descendants,omitempty" jsonschema:"Include descendant work items"`
	toolutil.GraphQLCursorPaginationInput
}

// ListOutput is the output for listing work items.
type ListOutput struct {
	toolutil.HintableOutput
	WorkItems  []WorkItemItem                   `json:"work_items"`
	Pagination toolutil.GraphQLPaginationOutput `json:"pagination"`
}

const errHintWorkItemsFullPath = "verify full_path with gitlab_project_list or gitlab_group_list; Work Items API requires Premium/Ultimate for some types (Epic, Objective, Key Result)"

// buildListOptions translates the tool input into SDK list options.
//
// Every filter [gl.ListWorkItemsOptions] accepts is exposed, and client-go
// declares a GraphQL variable and passes an argument for each one it finds
// set, so nothing here needs a document of its own.
//
// The cursor arrives already resolved, so exactly one of first and last
// reaches GitLab: the cursor picks the direction and the count only sizes the
// page.
func buildListOptions(input ListInput, cursor toolutil.GraphQLCursor) (*gl.ListWorkItemsOptions, error) {
	opts := &gl.ListWorkItemsOptions{}
	applyCursorOptions(opts, cursor)
	applyScopeFilters(opts, input)
	applyPeopleFilters(opts, input)
	applyIdentifierFilters(opts, input)
	applyPlanningFilters(opts, input)
	if err := applyTimeFilters(opts, input); err != nil {
		return nil, err
	}
	return opts, nil
}

func applyCursorOptions(opts *gl.ListWorkItemsOptions, cursor toolutil.GraphQLCursor) {
	if cursor.First != nil {
		opts.First = new(int64(*cursor.First))
	}
	if cursor.Last != nil {
		opts.Last = new(int64(*cursor.Last))
	}
	if cursor.After != "" {
		opts.After = new(cursor.After)
	}
	if cursor.Before != "" {
		opts.Before = new(cursor.Before)
	}
}

// applyScopeFilters sets the filters that decide which work items of the
// namespace are in scope at all: state, text search, type and hierarchy.
func applyScopeFilters(opts *gl.ListWorkItemsOptions, input ListInput) {
	if input.State != "" {
		opts.State = &input.State
	}
	if input.Search != "" {
		opts.Search = &input.Search
	}
	if len(input.In) > 0 {
		opts.In = input.In
	}
	if len(input.Types) > 0 {
		opts.Types = upperEach(input.Types)
	}
	if input.Confidential != nil {
		opts.Confidential = input.Confidential
	}
	if input.Sort != "" {
		opts.Sort = &input.Sort
	}
	if input.IncludeAncestors != nil {
		opts.IncludeAncestors = input.IncludeAncestors
	}
	if input.IncludeDescendants != nil {
		opts.IncludeDescendants = input.IncludeDescendants
	}
}

// applyPeopleFilters sets the filters keyed on a person: the author, the
// assignees, the caller's own reaction and subscription, and the CRM records a
// work item is attached to.
func applyPeopleFilters(opts *gl.ListWorkItemsOptions, input ListInput) {
	if input.AuthorUsername != "" {
		opts.AuthorUsername = &input.AuthorUsername
	}
	if len(input.AssigneeUsernames) > 0 {
		opts.AssigneeUsernames = input.AssigneeUsernames
	}
	if input.AssigneeWildcardID != "" {
		opts.AssigneeWildcardID = &input.AssigneeWildcardID
	}
	if input.MyReactionEmoji != "" {
		opts.MyReactionEmoji = &input.MyReactionEmoji
	}
	if input.Subscribed != "" {
		opts.Subscribed = &input.Subscribed
	}
	if input.CRMContactID != "" {
		opts.CRMContactID = &input.CRMContactID
	}
	if input.CRMOrganizationID != "" {
		opts.CRMOrganizationID = &input.CRMOrganizationID
	}
}

// applyIdentifierFilters sets the filters that name work items outright.
//
// client-go passes all three through verbatim, without the newGIDStrings
// wrapping it applies on create and update, so ids and parent_ids have to
// arrive as full global IDs and iids as the plain numbers.
func applyIdentifierFilters(opts *gl.ListWorkItemsOptions, input ListInput) {
	if len(input.IDs) > 0 {
		opts.IDs = input.IDs
	}
	if len(input.IIDs) > 0 {
		opts.IIDs = input.IIDs
	}
	if len(input.ParentIDs) > 0 {
		opts.ParentIDs = input.ParentIDs
	}
}

// applyPlanningFilters sets the filters over the planning widgets: labels,
// milestone, release, iteration, weight and health status.
func applyPlanningFilters(opts *gl.ListWorkItemsOptions, input ListInput) {
	if len(input.LabelName) > 0 {
		opts.LabelName = input.LabelName
	}
	if len(input.MilestoneTitle) > 0 {
		opts.MilestoneTitle = input.MilestoneTitle
	}
	if input.MilestoneWildcardID != "" {
		opts.MilestoneWildcardID = &input.MilestoneWildcardID
	}
	if len(input.ReleaseTag) > 0 {
		opts.ReleaseTag = input.ReleaseTag
	}
	if input.ReleaseTagWildcardID != "" {
		opts.ReleaseTagWildcardID = &input.ReleaseTagWildcardID
	}
	if len(input.IterationID) > 0 {
		opts.IterationID = input.IterationID
	}
	if len(input.IterationCadenceID) > 0 {
		opts.IterationCadenceID = input.IterationCadenceID
	}
	if input.IterationWildcardID != "" {
		opts.IterationWildcardID = &input.IterationWildcardID
	}
	if input.Weight != "" {
		opts.Weight = &input.Weight
	}
	if input.WeightWildcardID != "" {
		opts.WeightWildcardID = &input.WeightWildcardID
	}
	if input.HealthStatusFilter != "" {
		opts.HealthStatusFilter = &input.HealthStatusFilter
	}
}

// applyTimeFilters parses the eight date-range filters and refuses the whole
// call on the first one it cannot read.
//
// Dropping an unparseable filter silently would be the dangerous half of the
// choice: a list narrowed by a date the server ignored answers with more work
// items than were asked for, and nothing in the answer says the range was not
// applied.
func applyTimeFilters(opts *gl.ListWorkItemsOptions, input ListInput) error {
	filters := []struct {
		name  string
		value string
		dest  **time.Time
	}{
		{"closed_after", input.ClosedAfter, &opts.ClosedAfter},
		{"closed_before", input.ClosedBefore, &opts.ClosedBefore},
		{"created_after", input.CreatedAfter, &opts.CreatedAfter},
		{"created_before", input.CreatedBefore, &opts.CreatedBefore},
		{"due_after", input.DueAfter, &opts.DueAfter},
		{"due_before", input.DueBefore, &opts.DueBefore},
		{"updated_after", input.UpdatedAfter, &opts.UpdatedAfter},
		{"updated_before", input.UpdatedBefore, &opts.UpdatedBefore},
	}
	for _, filter := range filters {
		if filter.value == "" {
			continue
		}
		parsed, err := parseWorkItemTime(filter.name, filter.value)
		if err != nil {
			return err
		}
		*filter.dest = parsed
	}
	return nil
}

// workItemTimeLayouts are the timestamp spellings the time filters accept.
//
// RFC 3339 is what the published schema advertises through its date-time
// format and what GitLab's Time scalar wants. The two shorter forms are here
// because a model told a value is a date sends exactly "2025-01-01", and
// refusing that would cost a round trip to learn nothing.
// internal/tools/workitemsavedviews accepts the same three spellings for the
// same eight filter names.
var workItemTimeLayouts = []string{time.RFC3339, "2006-01-02T15:04:05", toolutil.DateFormatISO}

// parseWorkItemTime reads one timestamp, naming the field it came from so the
// caller learns which of eight filters was malformed.
func parseWorkItemTime(field, value string) (*time.Time, error) {
	for _, layout := range workItemTimeLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("%s must be an ISO 8601 timestamp (e.g. 2025-01-01T00:00:00Z), got %q", field, value)
}

// workItemEEListFields are the five field names client-go keeps out of its
// CE-safe default set, because a Community Edition schema does not define the
// widgets behind them and asking for one fails the whole query.
var workItemEEListFields = []string{"color", "healthStatus", "iteration", "status", "weight"}

// listReturnedFields picks the field set client-go renders into the WorkItem
// fragment of a list query.
//
// Until this existed, list asked for the CE default alone, and the five names
// above were requested by nothing: every listed work item came back with no
// status, whatever the instance held, and the Markdown list printed a Status
// column that was permanently blank. get, create and update never had that
// problem, because the static fragment client-go uses for those three selects
// all five unconditionally.
//
// The choice is made per call from the resolved tier rather than once, because
// in HTTP mode one process serves many instances and the tier is a property of
// the credential's pool entry, not of the build.
func listReturnedFields(client *gitlabclient.Client) []string {
	fields := gl.WorkItemDefaultListFields()
	if client.IsEnterprise() {
		fields = append(fields, workItemEEListFields...)
	}
	return fields
}

// upperEach uppercases every entry of an enum-valued filter.
//
// The values reach GitLab as a GraphQL IssueType, which is case sensitive:
// "Issue" is answered with "Expected \"Issue\" to be one of: ISSUE, INCIDENT,
// ..." and nothing is executed. That is what this server sent until the pinned
// schema started judging the variables, and no test could see it because the
// mock answered whatever it was asked. Normalising rather than only publishing
// the enum keeps a caller who learned the old spelling working.
func upperEach(values []string) []string {
	upper := make([]string, len(values))
	for i, value := range values {
		upper[i] = strings.ToUpper(value)
	}
	return upper
}

// List retrieves work items for a project or group.
//
// The SDK's CE-safe default field set is a superset of the query this handler
// used to send: assignees, labels and linked items come back on every listed
// item instead of only on [Get]. The five Enterprise-only widgets are asked
// for on top of it when the instance can answer them: see
// [listReturnedFields].
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.FullPath == "" {
		return ListOutput{}, toolutil.ErrRequiredString("list_work_items", "full_path")
	}
	// The direction is resolved by the shared helper rather than here, so that
	// this domain and the ones querying GraphQL directly answer a backward
	// request the same way.
	cursor, err := input.Resolve()
	if err != nil {
		return ListOutput{}, fmt.Errorf("list_work_items: %w", err)
	}
	opts, err := buildListOptions(input, cursor)
	if err != nil {
		return ListOutput{}, fmt.Errorf("list_work_items: %w", err)
	}
	opts.ReturnedFields = listReturnedFields(client)

	items, resp, err := client.GL().WorkItems.ListWorkItems(input.FullPath, opts, gl.WithContext(ctx))
	if err != nil {
		// A query-level failure arrives as GraphQLResponseError with HTTP 200,
		// so the status-keyed hint would never fire for it.
		if _, isGraphQLErr := errors.AsType[*gl.GraphQLResponseError](err); isGraphQLErr {
			return ListOutput{}, toolutil.WrapErrWithHint("list_work_items", err, errHintWorkItemsFullPath)
		}
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_work_items", err, http.StatusNotFound,
			errHintWorkItemsFullPath)
	}

	result := make([]WorkItemItem, 0, len(items))
	for _, item := range items {
		result = append(result, workItemToItem(item))
	}
	out := ListOutput{WorkItems: result}
	if resp != nil && resp.PageInfo != nil {
		out.Pagination = toolutil.GraphQLPaginationOutput{
			HasNextPage:     resp.PageInfo.HasNextPage,
			HasPreviousPage: resp.PageInfo.HasPreviousPage,
			EndCursor:       resp.PageInfo.EndCursor,
			StartCursor:     resp.PageInfo.StartCursor,
		}
	}
	return out, nil
}

// Create.

// CreateInput is the input for creating a work item.
type CreateInput struct {
	FullPath       string             `json:"full_path" jsonschema:"Full path of the project or group,required"`
	WorkItemTypeID string             `json:"work_item_type_id" jsonschema:"Global ID of work item type (e.g. gid://gitlab/WorkItems::Type/1 for Issue),required"`
	Title          string             `json:"title" jsonschema:"Title of the work item,required"`
	Description    string             `json:"description,omitempty" jsonschema:"Description of the work item"`
	Confidential   *bool              `json:"confidential,omitempty" jsonschema:"Whether the work item is confidential"`
	AssigneeIDs    []int64            `json:"assignee_ids,omitempty" jsonschema:"Global IDs of assignees"`
	MilestoneID    *int64             `json:"milestone_id,omitempty" jsonschema:"Global ID of the milestone"`
	LabelIDs       []int64            `json:"label_ids,omitempty" jsonschema:"Global IDs of labels"`
	CRMContactIDs  []int64            `json:"crm_contact_ids,omitempty" jsonschema:"CRM contact IDs to attach to the new work item"`
	ParentID       *int64             `json:"parent_id,omitempty" jsonschema:"Global ID of the parent work item. Creates the item already under its parent, which otherwise takes a create followed by an update"`
	IterationID    *int64             `json:"iteration_id,omitempty" tier:"premium" jsonschema:"Global ID of the iteration"`
	Weight         *int64             `json:"weight,omitempty" tier:"premium" jsonschema:"Weight of the work item"`
	HealthStatus   string             `json:"health_status,omitempty" tier:"ultimate" jsonschema:"Health status (onTrack/needsAttention/atRisk)"`
	Color          string             `json:"color,omitempty" jsonschema:"Color hex code (e.g. #fefefe)"`
	Status         string             `json:"status,omitempty" jsonschema:"Work item status: TODO, IN_PROGRESS, DONE, WONT_DO, or DUPLICATE"`
	DueDate        string             `json:"due_date,omitempty" jsonschema:"Due date (YYYY-MM-DD)"`
	StartDate      string             `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD)"`
	CreatedAt      string             `json:"created_at,omitempty" jsonschema:"Creation timestamp to record instead of now. ISO 8601 date-time such as 2025-01-01T00:00:00Z. GitLab accepts it from instance administrators and project owners only"`
	CreateSource   string             `json:"create_source,omitempty" jsonschema:"Name of whatever triggered the creation. Recorded for tracking and changes nothing about the work item"`
	LinkedItems    *CreateLinkedItems `json:"linked_items,omitempty" jsonschema:"Linked work items to add on creation"`
}

// CreateLinkedItems specifies work items to link during creation.
type CreateLinkedItems struct {
	WorkItemIDs []int64 `json:"work_item_ids" jsonschema:"Global IDs of work items to link,required"`
	LinkType    string  `json:"link_type" jsonschema:"Link type: BLOCKS, BLOCKED_BY, or RELATED,required"`
}

// Create creates a new work item.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (GetOutput, error) {
	opts, err := buildCreateOptions(input)
	if err != nil {
		return GetOutput{}, fmt.Errorf("create_work_item: %w", err)
	}

	wi, _, err := client.GL().WorkItems.CreateWorkItem(input.FullPath, gl.WorkItemTypeID(input.WorkItemTypeID), opts, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("create_work_item", err, http.StatusBadRequest,
			"work_item_type_id must be a valid type GID; verify type compatibility with full_path (e.g. Epic only at group level + Premium); title is required; Work Items API is experimental")
	}
	return GetOutput{WorkItem: workItemToItem(wi)}, nil
}

// buildCreateOptions translates the tool input into SDK create options.
//
// client-go folds each of these into the widget of the WorkItemCreateInput
// that owns it, global ID wrapping included, so every field here is a plain
// numeric ID or a plain string.
func buildCreateOptions(input CreateInput) (*gl.CreateWorkItemOptions, error) {
	opts := &gl.CreateWorkItemOptions{
		Title: input.Title,
	}
	applyCreateCore(opts, input)
	applyCreateWidgets(opts, input)
	if input.CreatedAt != "" {
		createdAt, err := parseWorkItemTime("created_at", input.CreatedAt)
		if err != nil {
			return nil, err
		}
		opts.CreatedAt = createdAt
	}
	if input.LinkedItems != nil && len(input.LinkedItems.WorkItemIDs) > 0 {
		opts.LinkedItems = &gl.CreateWorkItemOptionsLinkedItems{
			LinkType:    &input.LinkedItems.LinkType,
			WorkItemIDs: input.LinkedItems.WorkItemIDs,
		}
	}
	return opts, nil
}

// applyCreateCore sets the fields every work item type carries.
func applyCreateCore(opts *gl.CreateWorkItemOptions, input CreateInput) {
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Confidential != nil {
		opts.Confidential = input.Confidential
	}
	if input.Status != "" {
		status := mapStatusToID(input.Status)
		opts.Status = &status
	}
	if len(input.AssigneeIDs) > 0 {
		opts.AssigneeIDs = input.AssigneeIDs
	}
	if len(input.LabelIDs) > 0 {
		opts.LabelIDs = input.LabelIDs
	}
	if len(input.CRMContactIDs) > 0 {
		opts.CRMContactIDs = input.CRMContactIDs
	}
	if input.CreateSource != "" {
		opts.CreateSource = new(input.CreateSource)
	}
}

// applyCreateWidgets sets the fields that reach GitLab through a widget of
// their own, each of which a work item type may or may not have.
func applyCreateWidgets(opts *gl.CreateWorkItemOptions, input CreateInput) {
	if input.MilestoneID != nil {
		opts.MilestoneID = input.MilestoneID
	}
	if input.ParentID != nil {
		opts.ParentID = input.ParentID
	}
	if input.IterationID != nil {
		opts.IterationID = input.IterationID
	}
	if input.Weight != nil {
		opts.Weight = input.Weight
	}
	if input.HealthStatus != "" {
		opts.HealthStatus = new(input.HealthStatus)
	}
	if input.Color != "" {
		opts.Color = new(input.Color)
	}
	if input.DueDate != "" {
		d, err := time.Parse(toolutil.DateFormatISO, input.DueDate)
		if err == nil {
			isoDate := gl.ISOTime(d)
			opts.DueDate = &isoDate
		}
	}
	if input.StartDate != "" {
		d, err := time.Parse(toolutil.DateFormatISO, input.StartDate)
		if err == nil {
			isoDate := gl.ISOTime(d)
			opts.StartDate = &isoDate
		}
	}
}

// Update.

// UpdateInput is the input for updating a work item.
type UpdateInput struct {
	FullPath       string  `json:"full_path" jsonschema:"Full path of the project or group (e.g. my-group/my-project),required"`
	IID            int64   `json:"work_item_iid" jsonschema:"Work item IID,required"`
	Title          string  `json:"title,omitempty" jsonschema:"New title"`
	StateEvent     string  `json:"state_event,omitempty" jsonschema:"State event: CLOSE or REOPEN"`
	Description    string  `json:"description,omitempty" jsonschema:"New description"`
	AssigneeIDs    []int64 `json:"assignee_ids,omitempty" jsonschema:"Global IDs of assignees. Replaces the current assignees. Pass an empty array [] to remove every assignee. Omit the field to leave assignees untouched"`
	MilestoneID    *int64  `json:"milestone_id,omitempty" jsonschema:"Global ID of the milestone"`
	CRMContactIDs  []int64 `json:"crm_contact_ids,omitempty" jsonschema:"CRM contact IDs. Replaces the current contacts. Pass an empty array [] to remove every contact. Omit the field to leave contacts untouched"`
	ParentID       *int64  `json:"parent_id,omitempty" jsonschema:"Global ID of the parent work item"`
	AddLabelIDs    []int64 `json:"add_label_ids,omitempty" jsonschema:"Global IDs of labels to add"`
	RemoveLabelIDs []int64 `json:"remove_label_ids,omitempty" jsonschema:"Global IDs of labels to remove"`
	StartDate      string  `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD)"`
	DueDate        string  `json:"due_date,omitempty" jsonschema:"Due date (YYYY-MM-DD)"`
	Weight         *int64  `json:"weight,omitempty" tier:"premium" jsonschema:"Weight of the work item"`
	HealthStatus   string  `json:"health_status,omitempty" tier:"ultimate" jsonschema:"Health status (onTrack/needsAttention/atRisk)"`
	IterationID    *int64  `json:"iteration_id,omitempty" tier:"premium" jsonschema:"Global ID of the iteration"`
	Color          string  `json:"color,omitempty" jsonschema:"Color hex code (e.g. #fefefe)"`
	Status         string  `json:"status,omitempty" jsonschema:"Work item status: TODO, IN_PROGRESS, DONE, WONT_DO, or DUPLICATE"`
	// Confirm is declared so the input schema advertises the reserved confirm
	// key and strict validation accepts it. Its value is never populated:
	// toolutil strips reserved keys before unmarshalling, so the handler reads
	// the caller's confirmation from the raw request instead.
	Confirm bool `json:"confirm,omitempty" jsonschema:"Confirms removing existing assignees or CRM contacts when assignee_ids or crm_contact_ids is an empty array. Only required when entries would actually be deleted"`
}

// Update modifies an existing work item.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (GetOutput, error) {
	if input.IID <= 0 {
		return GetOutput{}, toolutil.ErrRequiredInt64("update_work_item", "work_item_iid")
	}
	if err := confirmListClearing(ctx, client, input); err != nil {
		return GetOutput{}, err
	}
	opts := buildUpdateOptions(input)
	wi, _, err := client.GL().WorkItems.UpdateWorkItem(input.FullPath, input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("update_work_item", err, http.StatusBadRequest,
			"verify full_path + iid with gitlab_list_work_items; only widget-supported fields can be updated for the type; state_event values: close|reopen")
	}
	return GetOutput{WorkItem: workItemToItem(wi)}, nil
}

func buildUpdateOptions(input UpdateInput) *gl.UpdateWorkItemOptions {
	opts := &gl.UpdateWorkItemOptions{}
	if input.Title != "" {
		opts.Title = &input.Title
	}
	if input.StateEvent != "" {
		ev := gl.WorkItemStateEvent(input.StateEvent)
		opts.StateEvent = &ev
	}
	if input.Description != "" {
		opts.Description = &input.Description
	}
	if input.AssigneeIDs != nil {
		opts.AssigneeIDs = input.AssigneeIDs
	}
	if input.MilestoneID != nil {
		opts.MilestoneID = input.MilestoneID
	}
	if input.CRMContactIDs != nil {
		opts.CRMContactIDs = input.CRMContactIDs
	}
	if input.ParentID != nil {
		opts.ParentID = input.ParentID
	}
	if len(input.AddLabelIDs) > 0 {
		opts.AddLabelIDs = input.AddLabelIDs
	}
	if len(input.RemoveLabelIDs) > 0 {
		opts.RemoveLabelIDs = input.RemoveLabelIDs
	}
	if input.StartDate != "" {
		d, err := time.Parse(toolutil.DateFormatISO, input.StartDate)
		if err == nil {
			isoDate := gl.ISOTime(d)
			opts.StartDate = &isoDate
		}
	}
	if input.DueDate != "" {
		d, err := time.Parse(toolutil.DateFormatISO, input.DueDate)
		if err == nil {
			isoDate := gl.ISOTime(d)
			opts.DueDate = &isoDate
		}
	}
	if input.Weight != nil {
		opts.Weight = input.Weight
	}
	if input.HealthStatus != "" {
		opts.HealthStatus = &input.HealthStatus
	}
	if input.IterationID != nil {
		opts.IterationID = input.IterationID
	}
	if input.Color != "" {
		opts.Color = &input.Color
	}
	if input.Status != "" {
		status := mapStatusToID(input.Status)
		opts.Status = &status
	}
	return opts
}

// Delete.

// DeleteInput is the input for deleting a work item.
type DeleteInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the project or group (e.g. my-group/my-project),required"`
	IID      int64  `json:"work_item_iid" jsonschema:"Work item IID,required"`
}

// Delete permanently removes a work item by IID.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("delete_work_item", "work_item_iid")
	}
	_, err := client.GL().WorkItems.DeleteWorkItem(input.FullPath, input.IID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("delete_work_item", err, http.StatusForbidden,
			"only the author or a Maintainer/Owner can delete; verify full_path + iid; deletion is irreversible. Some work item types are protected (e.g. system-managed)")
	}
	return nil
}

// List Work Item Types.

// WorkItemTypeOutput represents a work item type.
type WorkItemTypeOutput struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// WorkItemTypeListOutput holds a list of work item types.
type WorkItemTypeListOutput struct {
	toolutil.HintableOutput
	Types      []WorkItemTypeOutput             `json:"types"`
	Pagination toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// ListWorkItemTypesInput defines parameters for listing work item types.
//
// The cursor parameters come from the shared type so that this connection,
// which the SDK query pages in both directions, answers a backward request the
// way every other cursor-paginated list here does.
type ListWorkItemTypesInput struct {
	FullPath      string `json:"full_path"            jsonschema:"Project or group full path (namespace path),required"`
	Name          string `json:"name,omitempty"       jsonschema:"Filter by work item type name, an IssueType enum value such as ISSUE or TASK"`
	OnlyAvailable bool   `json:"only_available,omitempty" jsonschema:"Return only available work item types"`
	toolutil.GraphQLCursorPaginationInput
}

// ListWorkItemTypes lists work item types (system-defined and custom) for a namespace.
func ListWorkItemTypes(ctx context.Context, client *gitlabclient.Client, input ListWorkItemTypesInput) (WorkItemTypeListOutput, error) {
	if input.FullPath == "" {
		return WorkItemTypeListOutput{}, toolutil.ErrRequiredString("list_work_item_types", "full_path")
	}
	// The direction is resolved by the shared helper rather than here, so that
	// this domain and the ones querying GraphQL directly answer a backward
	// request the same way. A bare before used to reach GitLab with no count
	// at all, and graphql-ruby then fills first from its own default page
	// size, which answers the head of the list rather than the previous page.
	cursor, err := input.Resolve()
	if err != nil {
		return WorkItemTypeListOutput{}, fmt.Errorf("list_work_item_types: %w", err)
	}
	opts := &gl.ListWorkItemTypesOptions{}
	if input.Name != "" {
		name := strings.ToUpper(input.Name)
		opts.Name = &name
	}
	if input.OnlyAvailable {
		opts.OnlyAvailable = new(true)
	}
	if cursor.First != nil {
		opts.First = new(int64(*cursor.First))
	}
	if cursor.After != "" {
		opts.After = new(cursor.After)
	}
	if cursor.Last != nil {
		opts.Last = new(int64(*cursor.Last))
	}
	if cursor.Before != "" {
		opts.Before = new(cursor.Before)
	}
	types, resp, err := client.GL().WorkItems.ListWorkItemTypes(input.FullPath, opts, gl.WithContext(ctx))
	if err != nil {
		return WorkItemTypeListOutput{}, toolutil.WrapErrWithStatusHint("list_work_item_types", err, http.StatusNotFound,
			"verify full_path with gitlab_project_list or gitlab_group_list; Work Items API requires Premium/Ultimate for some types")
	}
	out := make([]WorkItemTypeOutput, 0, len(types))
	for _, t := range types {
		out = append(out, WorkItemTypeOutput{
			ID:      string(t.ID),
			Name:    t.Name,
			Enabled: t.Enabled,
		})
	}
	result := WorkItemTypeListOutput{Types: out}
	if resp != nil && resp.PageInfo != nil {
		result.Pagination = toolutil.GraphQLPaginationOutput{
			HasNextPage:     resp.PageInfo.HasNextPage,
			HasPreviousPage: resp.PageInfo.HasPreviousPage,
			EndCursor:       resp.PageInfo.EndCursor,
			StartCursor:     resp.PageInfo.StartCursor,
		}
	}
	return result, nil
}

// Markdown Formatters.
