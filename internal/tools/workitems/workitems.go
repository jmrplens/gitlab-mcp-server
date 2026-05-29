package workitems

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
	if wi.CreatedAt != nil {
		item.CreatedAt = wi.CreatedAt.String()
	}
	if wi.UpdatedAt != nil {
		item.UpdatedAt = wi.UpdatedAt.String()
	}
	if wi.ClosedAt != nil {
		item.ClosedAt = wi.ClosedAt.String()
	}
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
			"verify full_path (group or project path) and iid (work item IID) with gitlab_work_item_list; Work Items API is experimental \u2014 verify GitLab version supports the work item type")
	}
	return GetOutput{WorkItem: workItemToItem(wi)}, nil
}

// List.

// ListInput is the input for listing work items.
type ListInput struct {
	FullPath           string   `json:"full_path" jsonschema:"Full path of the project or group,required"`
	State              string   `json:"state,omitempty" jsonschema:"Filter by state (opened/closed/all)"`
	Search             string   `json:"search,omitempty" jsonschema:"Search in title and description"`
	Types              []string `json:"types,omitempty" jsonschema:"Filter by work item types"`
	AuthorUsername     string   `json:"author_username,omitempty" jsonschema:"Filter by author username"`
	LabelName          []string `json:"label_name,omitempty" jsonschema:"Filter by label names"`
	Confidential       *bool    `json:"confidential,omitempty" jsonschema:"Filter by confidentiality"`
	Sort               string   `json:"sort,omitempty" jsonschema:"Sort order"`
	First              *int64   `json:"first,omitempty" jsonschema:"Number of items to return (cursor-based pagination)"`
	After              string   `json:"after,omitempty" jsonschema:"Cursor for forward pagination"`
	IncludeAncestors   *bool    `json:"include_ancestors,omitempty" jsonschema:"Include ancestor work items"`
	IncludeDescendants *bool    `json:"include_descendants,omitempty" jsonschema:"Include descendant work items"`
}

// ListOutput is the output for listing work items.
type ListOutput struct {
	toolutil.HintableOutput
	WorkItems []WorkItemItem `json:"work_items"`
}

const errHintWorkItemsFullPath = "verify full_path with gitlab_project_list or gitlab_group_list; Work Items API requires Premium/Ultimate for some types (Epic, Objective, Key Result)"

const queryListWorkItems = `
query ListWorkItems(
  $fullPath: ID!
  $state: IssuableState
  $search: String
  $types: [IssueType!]
  $authorUsername: String
  $labelName: [String!]
  $confidential: Boolean
  $sort: WorkItemSort
  $first: Int
  $after: String
  $includeAncestors: Boolean
  $includeDescendants: Boolean
) {
  namespace(fullPath: $fullPath) {
    workItems(
      state: $state
      search: $search
      types: $types
      authorUsername: $authorUsername
      labelName: $labelName
      confidential: $confidential
      sort: $sort
      first: $first
      after: $after
      includeAncestors: $includeAncestors
      includeDescendants: $includeDescendants
    ) {
      nodes {
        id
        iid
        workItemType {
          name
        }
        state
        title
        description
        confidential
        author {
          username
        }
        createdAt
        updatedAt
        closedAt
        webUrl
      }
    }
  }
}
`

type listWorkItemsNamespace struct {
	WorkItems struct {
		Nodes []gqlListWorkItem `json:"nodes"`
	} `json:"workItems"`
}

type listWorkItemsData struct {
	Namespace *listWorkItemsNamespace `json:"namespace"`
}

type listWorkItemsResponse struct {
	Data listWorkItemsData `json:"data"`
	gl.GenericGraphQLErrors
}

type gqlListWorkItem struct {
	ID           string `json:"id"`
	IID          string `json:"iid"`
	WorkItemType struct {
		Name string `json:"name"`
	} `json:"workItemType"`
	State        string `json:"state"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Confidential bool   `json:"confidential"`
	Author       *struct {
		Username string `json:"username"`
	} `json:"author"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	ClosedAt  string `json:"closedAt"`
	WebURL    string `json:"webUrl"`
}

// List retrieves work items for a project or group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.FullPath == "" {
		return ListOutput{}, toolutil.ErrRequiredString("list_work_items", "full_path")
	}

	defaultFirst := int64(20)
	first := &defaultFirst
	if input.First != nil {
		first = input.First
	}

	variables := map[string]any{
		"fullPath": input.FullPath,
		"first":    first,
	}
	if input.State != "" {
		variables["state"] = input.State
	}
	if input.Search != "" {
		variables["search"] = input.Search
	}
	if len(input.Types) > 0 {
		variables["types"] = input.Types
	}
	if input.AuthorUsername != "" {
		variables["authorUsername"] = input.AuthorUsername
	}
	if len(input.LabelName) > 0 {
		variables["labelName"] = input.LabelName
	}
	if input.Confidential != nil {
		variables["confidential"] = input.Confidential
	}
	if input.Sort != "" {
		variables["sort"] = input.Sort
	}
	if input.After != "" {
		variables["after"] = input.After
	}
	if input.IncludeAncestors != nil {
		variables["includeAncestors"] = input.IncludeAncestors
	}
	if input.IncludeDescendants != nil {
		variables["includeDescendants"] = input.IncludeDescendants
	}

	var resp listWorkItemsResponse
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryListWorkItems,
		Variables: variables,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_work_items", err, http.StatusNotFound,
			errHintWorkItemsFullPath)
	}
	if len(resp.Errors) > 0 {
		return ListOutput{}, toolutil.WrapErrWithHint("list_work_items", &gl.GraphQLResponseError{
			Err:    errors.New("GraphQL query failed"),
			Errors: resp.GenericGraphQLErrors,
		}, errHintWorkItemsFullPath)
	}
	if resp.Data.Namespace == nil {
		return ListOutput{}, toolutil.WrapErrWithHint("list_work_items", gl.ErrNotFound,
			errHintWorkItemsFullPath)
	}

	items := resp.Data.Namespace.WorkItems.Nodes
	result := make([]WorkItemItem, 0, len(items))
	for _, item := range items {
		mapped, mapErr := gqlListWorkItemToItem(item)
		if mapErr != nil {
			return ListOutput{}, fmt.Errorf("list_work_items: parsing work item: %w", mapErr)
		}
		result = append(result, mapped)
	}
	return ListOutput{WorkItems: result}, nil
}

func gqlListWorkItemToItem(item gqlListWorkItem) (WorkItemItem, error) {
	id, err := parseWorkItemGID(item.ID)
	if err != nil {
		return WorkItemItem{}, err
	}
	iid, err := parseWorkItemInt64("iid", item.IID)
	if err != nil {
		return WorkItemItem{}, err
	}

	mapped := WorkItemItem{
		ID:           id,
		IID:          iid,
		Type:         item.WorkItemType.Name,
		State:        item.State,
		Title:        item.Title,
		Description:  item.Description,
		WebURL:       item.WebURL,
		Confidential: item.Confidential,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
		ClosedAt:     item.ClosedAt,
	}
	if item.Author != nil {
		mapped.Author = item.Author.Username
	}
	return mapped, nil
}

func parseWorkItemGID(value string) (int64, error) {
	lastSlash := strings.LastIndex(value, "/")
	if lastSlash < 0 || lastSlash == len(value)-1 {
		return 0, fmt.Errorf("invalid work item id %q", value)
	}
	return parseWorkItemInt64("id", value[lastSlash+1:])
}

func parseWorkItemInt64(field, value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid work item %s %q: %w", field, value, err)
	}
	return parsed, nil
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
	Weight         *int64             `json:"weight,omitempty" jsonschema:"Weight of the work item"`
	HealthStatus   string             `json:"health_status,omitempty" jsonschema:"Health status (onTrack/needsAttention/atRisk)"`
	Color          string             `json:"color,omitempty" jsonschema:"Color hex code (e.g. #fefefe)"`
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
	AssigneeIDs    []int64 `json:"assignee_ids,omitempty" jsonschema:"Global IDs of assignees (empty array to remove all)"`
	MilestoneID    *int64  `json:"milestone_id,omitempty" jsonschema:"Global ID of the milestone"`
	CRMContactIDs  []int64 `json:"crm_contact_ids,omitempty" jsonschema:"CRM contact IDs (empty array to remove all)"`
	ParentID       *int64  `json:"parent_id,omitempty" jsonschema:"Global ID of the parent work item"`
	AddLabelIDs    []int64 `json:"add_label_ids,omitempty" jsonschema:"Global IDs of labels to add"`
	RemoveLabelIDs []int64 `json:"remove_label_ids,omitempty" jsonschema:"Global IDs of labels to remove"`
	StartDate      string  `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD)"`
	DueDate        string  `json:"due_date,omitempty" jsonschema:"Due date (YYYY-MM-DD)"`
	Weight         *int64  `json:"weight,omitempty" jsonschema:"Weight of the work item"`
	HealthStatus   string  `json:"health_status,omitempty" jsonschema:"Health status (onTrack/needsAttention/atRisk)"`
	IterationID    *int64  `json:"iteration_id,omitempty" jsonschema:"Global ID of the iteration"`
	Color          string  `json:"color,omitempty" jsonschema:"Color hex code (e.g. #fefefe)"`
	Status         string  `json:"status,omitempty" jsonschema:"Work item status: TODO, IN_PROGRESS, DONE, WONT_DO, or DUPLICATE"`
}

// Update modifies an existing work item.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (GetOutput, error) {
	if input.IID <= 0 {
		return GetOutput{}, toolutil.ErrRequiredInt64("update_work_item", "work_item_iid")
	}
	opts := buildUpdateOptions(input)
	wi, _, err := client.GL().WorkItems.UpdateWorkItem(input.FullPath, input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("update_work_item", err, http.StatusBadRequest,
			"verify full_path + iid with gitlab_work_item_list; only widget-supported fields can be updated for the type; state_event values: close|reopen")
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
			"only the author or a Maintainer/Owner can delete; verify full_path + iid; deletion is irreversible \u2014 some work item types are protected (e.g. system-managed)")
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
type ListWorkItemTypesInput struct {
	FullPath      string `json:"full_path"            jsonschema:"Project or group full path (namespace path),required"`
	Name          string `json:"name,omitempty"       jsonschema:"Filter by work item type name"`
	OnlyAvailable bool   `json:"only_available,omitempty" jsonschema:"Return only available work item types"`
	First         int64  `json:"first,omitempty"      jsonschema:"Number of types to return from the beginning (cursor pagination)"`
	After         string `json:"after,omitempty"      jsonschema:"Cursor for forward pagination"`
}

// ListWorkItemTypes lists work item types (system-defined and custom) for a namespace.
func ListWorkItemTypes(ctx context.Context, client *gitlabclient.Client, input ListWorkItemTypesInput) (WorkItemTypeListOutput, error) {
	if input.FullPath == "" {
		return WorkItemTypeListOutput{}, toolutil.ErrRequiredString("list_work_item_types", "full_path")
	}
	opts := &gl.ListWorkItemTypesOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.OnlyAvailable {
		opts.OnlyAvailable = new(true)
	}
	if input.First > 0 {
		first := input.First
		opts.First = &first
	}
	if input.After != "" {
		opts.After = new(input.After)
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
