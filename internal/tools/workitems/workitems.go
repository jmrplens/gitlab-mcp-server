// Package workitems implements MCP tool handlers for GitLab Work Items.
// It wraps the WorkItemsService from client-go v2.
//
// NOTE: The Work Items API is experimental and may introduce breaking changes
// even between minor GitLab versions.
package workitems

import (
	"context"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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

// workItemToItem is an internal helper for the workitems package.
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
	IID      int64  `json:"iid" jsonschema:"Work item IID,required"`
}

// GetOutput is the output for getting a single work item.
type GetOutput struct {
	toolutil.HintableOutput
	WorkItem WorkItemItem `json:"work_item"`
}

// Get retrieves a single work item by IID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if input.IID <= 0 {
		return GetOutput{}, toolutil.ErrRequiredInt64("get_work_item", "iid")
	}
	wi, _, err := client.GL().WorkItems.GetWorkItem(input.FullPath, input.IID, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithMessage("get_work_item", err)
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

// List retrieves work items for a project or group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListWorkItemsOptions{}
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if len(input.Types) > 0 {
		opts.Types = input.Types
	}
	if input.AuthorUsername != "" {
		opts.AuthorUsername = new(input.AuthorUsername)
	}
	if len(input.LabelName) > 0 {
		opts.LabelName = input.LabelName
	}
	if input.Confidential != nil {
		opts.Confidential = input.Confidential
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.First != nil {
		opts.First = input.First
	}
	if input.After != "" {
		opts.After = new(input.After)
	}
	if input.IncludeAncestors != nil {
		opts.IncludeAncestors = input.IncludeAncestors
	}
	if input.IncludeDescendants != nil {
		opts.IncludeDescendants = input.IncludeDescendants
	}

	items, _, err := client.GL().WorkItems.ListWorkItems(input.FullPath, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list_work_items", err)
	}
	result := make([]WorkItemItem, 0, len(items))
	for _, wi := range items {
		result = append(result, workItemToItem(wi))
	}
	return ListOutput{WorkItems: result}, nil
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
		d, err := time.Parse("2006-01-02", input.DueDate)
		if err == nil {
			isoDate := gl.ISOTime(d)
			opts.DueDate = &isoDate
		}
	}
	if input.StartDate != "" {
		d, err := time.Parse("2006-01-02", input.StartDate)
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
		return GetOutput{}, toolutil.WrapErrWithMessage("create_work_item", err)
	}
	return GetOutput{WorkItem: workItemToItem(wi)}, nil
}

// Update.

// UpdateInput is the input for updating a work item.
type UpdateInput struct {
	FullPath       string  `json:"full_path" jsonschema:"Full path of the project or group (e.g. my-group/my-project),required"`
	IID            int64   `json:"iid" jsonschema:"Work item IID,required"`
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
		return GetOutput{}, toolutil.ErrRequiredInt64("update_work_item", "iid")
	}

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
		d, err := time.Parse("2006-01-02", input.StartDate)
		if err == nil {
			isoDate := gl.ISOTime(d)
			opts.StartDate = &isoDate
		}
	}
	if input.DueDate != "" {
		d, err := time.Parse("2006-01-02", input.DueDate)
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

	wi, _, err := client.GL().WorkItems.UpdateWorkItem(input.FullPath, input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithMessage("update_work_item", err)
	}
	return GetOutput{WorkItem: workItemToItem(wi)}, nil
}

// Delete.

// DeleteInput is the input for deleting a work item.
type DeleteInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the project or group (e.g. my-group/my-project),required"`
	IID      int64  `json:"iid" jsonschema:"Work item IID,required"`
}

// Delete permanently removes a work item by IID.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("delete_work_item", "iid")
	}
	_, err := client.GL().WorkItems.DeleteWorkItem(input.FullPath, input.IID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("delete_work_item", err)
	}
	return nil
}

// Markdown Formatters.
