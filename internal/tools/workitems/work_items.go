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
	Children     []ChildItem  `json:"children,omitempty" jsonschema:"Child work items in the hierarchy (each with namespace path and IID)"`
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
	// RFC 3339 rather than time.Time's own String(), which is what every other
	// timestamp in this server emits (toolutil.FormatTimePtr) and what the raw
	// GraphQL list handler passed through verbatim before it moved onto the SDK.
	item.CreatedAt = toolutil.FormatTimePtr(wi.CreatedAt)
	item.UpdatedAt = toolutil.FormatTimePtr(wi.UpdatedAt)
	item.ClosedAt = toolutil.FormatTimePtr(wi.ClosedAt)
	return item
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
	FullPath           string   `json:"full_path" jsonschema:"Full path of the project or group,required"`
	State              string   `json:"state,omitempty" jsonschema:"Filter by state (opened/closed/all)"`
	Search             string   `json:"search,omitempty" jsonschema:"Search in title and description"`
	Types              []string `json:"types,omitempty" jsonschema:"Filter by work item types, IssueType enum values such as ISSUE or TASK"`
	AuthorUsername     string   `json:"author_username,omitempty" jsonschema:"Filter by author username"`
	LabelName          []string `json:"label_name,omitempty" jsonschema:"Filter by label names"`
	Confidential       *bool    `json:"confidential,omitempty" jsonschema:"Filter by confidentiality"`
	Sort               string   `json:"sort,omitempty" jsonschema:"Sort order, a WorkItemSort enum value such as CREATED_DESC or TITLE_ASC"`
	IncludeAncestors   *bool    `json:"include_ancestors,omitempty" jsonschema:"Include ancestor work items"`
	IncludeDescendants *bool    `json:"include_descendants,omitempty" jsonschema:"Include descendant work items"`
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
// Every filter the tool exposes has a direct counterpart on
// [gl.ListWorkItemsOptions], which accepts a superset: assignee, milestone,
// iteration, release, weight, CRM, reaction and date-range filters the tool
// does not surface today.
//
// The cursor arrives already resolved, so exactly one of first and last
// reaches GitLab: the cursor picks the direction and the count only sizes the
// page.
func buildListOptions(input ListInput, cursor toolutil.GraphQLCursor) *gl.ListWorkItemsOptions {
	opts := &gl.ListWorkItemsOptions{}
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
	if input.State != "" {
		opts.State = &input.State
	}
	if input.Search != "" {
		opts.Search = &input.Search
	}
	if len(input.Types) > 0 {
		opts.Types = upperEach(input.Types)
	}
	if input.AuthorUsername != "" {
		opts.AuthorUsername = &input.AuthorUsername
	}
	if len(input.LabelName) > 0 {
		opts.LabelName = input.LabelName
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
	return opts
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
// The SDK requests its CE-safe default field set, which is a superset of the
// query this handler used to send: assignees, labels and linked items now come
// back on every listed item instead of only on [Get]. Enterprise-only widgets
// (status, weight, health status, color, iteration) stay unrequested, because
// asking for them errors against a Community Edition instance.
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

	items, resp, err := client.GL().WorkItems.ListWorkItems(input.FullPath, buildListOptions(input, cursor), gl.WithContext(ctx))
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
	Weight         *int64             `json:"weight,omitempty" tier:"premium" jsonschema:"Weight of the work item"`
	HealthStatus   string             `json:"health_status,omitempty" tier:"ultimate" jsonschema:"Health status (onTrack/needsAttention/atRisk)"`
	Color          string             `json:"color,omitempty" jsonschema:"Color hex code (e.g. #fefefe)"`
	Status         string             `json:"status,omitempty" jsonschema:"Work item status: TODO, IN_PROGRESS, DONE, WONT_DO, or DUPLICATE"`
	DueDate        string             `json:"due_date,omitempty" jsonschema:"Due date (YYYY-MM-DD)"`
	StartDate      string             `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD)"`
	LinkedItems    *CreateLinkedItems `json:"linked_items,omitempty" jsonschema:"Linked work items to add on creation"`
}

// CreateLinkedItems specifies work items to link during creation.
type CreateLinkedItems struct {
	WorkItemIDs []int64 `json:"work_item_ids" jsonschema:"Global IDs of work items to link,required"`
	LinkType    string  `json:"link_type" jsonschema:"Link type: BLOCKS, BLOCKED_BY, or RELATED,required"`
}

// Create creates a new work item.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (GetOutput, error) {
	opts := &gl.CreateWorkItemOptions{
		Title: input.Title,
	}
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
	if input.MilestoneID != nil {
		opts.MilestoneID = input.MilestoneID
	}
	if len(input.LabelIDs) > 0 {
		opts.LabelIDs = input.LabelIDs
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
	if input.LinkedItems != nil && len(input.LinkedItems.WorkItemIDs) > 0 {
		opts.LinkedItems = &gl.CreateWorkItemOptionsLinkedItems{
			LinkType:    &input.LinkedItems.LinkType,
			WorkItemIDs: input.LinkedItems.WorkItemIDs,
		}
	}

	wi, _, err := client.GL().WorkItems.CreateWorkItem(input.FullPath, gl.WorkItemTypeID(input.WorkItemTypeID), opts, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("create_work_item", err, http.StatusBadRequest,
			"work_item_type_id must be a valid type GID; verify type compatibility with full_path (e.g. Epic only at group level + Premium); title is required; Work Items API is experimental")
	}
	return GetOutput{WorkItem: workItemToItem(wi)}, nil
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
