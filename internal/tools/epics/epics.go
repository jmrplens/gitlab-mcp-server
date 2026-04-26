// Package epics implements GitLab group epic operations using the Work Items
// GraphQL API. Epics are high-level planning items attached to groups.
//
// This package was migrated from the deprecated Epics REST API (deprecated
// GitLab 17.0, removal planned 19.0) to the Work Items GraphQL API per
// ADR-0009 (progressive GraphQL migration).
//
// The GetLinks handler remains on REST because client-go v2 does not yet
// expose a GraphQL query for work item children.
package epics

import (
	"context"
	"errors"
	"net/http"
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

// ListInput defines parameters for listing group epics via Work Items API.
type ListInput struct {
	FullPath           string   `json:"full_path" jsonschema:"Full path of the group (e.g. my-group or my-group/sub-group),required"`
	State              string   `json:"state,omitempty" jsonschema:"Filter by state (opened/closed/all)"`
	Search             string   `json:"search,omitempty" jsonschema:"Search in title and description"`
	AuthorUsername     string   `json:"author_username,omitempty" jsonschema:"Filter by author username"`
	LabelName          []string `json:"label_name,omitempty" jsonschema:"Filter by label names"`
	Confidential       *bool    `json:"confidential,omitempty" jsonschema:"Filter by confidentiality"`
	Sort               string   `json:"sort,omitempty" jsonschema:"Sort order"`
	First              *int64   `json:"first,omitempty" jsonschema:"Number of items to return (cursor-based pagination)"`
	After              string   `json:"after,omitempty" jsonschema:"Cursor for forward pagination"`
	IncludeAncestors   *bool    `json:"include_ancestors,omitempty" jsonschema:"Include epics from ancestor groups"`
	IncludeDescendants *bool    `json:"include_descendants,omitempty" jsonschema:"Include epics from descendant groups"`
}

// GetInput defines parameters for getting a single epic.
type GetInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID      int64  `json:"iid" jsonschema:"Epic IID within the group,required"`
}

// GetLinksInput defines parameters for listing child epics (REST).
type GetLinksInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID      int64  `json:"iid" jsonschema:"Epic IID within the group,required"`
}

// CreateInput defines parameters for creating a new epic.
type CreateInput struct {
	FullPath     string  `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	Title        string  `json:"title" jsonschema:"Epic title,required"`
	Description  string  `json:"description,omitempty" jsonschema:"Epic description (Markdown supported)"`
	Confidential *bool   `json:"confidential,omitempty" jsonschema:"Whether the epic is confidential"`
	Color        string  `json:"color,omitempty" jsonschema:"Epic color (hex format, e.g. #FF0000)"`
	StartDate    string  `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD)"`
	DueDate      string  `json:"due_date,omitempty" jsonschema:"Due date (YYYY-MM-DD)"`
	AssigneeIDs  []int64 `json:"assignee_ids,omitempty" jsonschema:"Global IDs of assignees"`
	LabelIDs     []int64 `json:"label_ids,omitempty" jsonschema:"Global IDs of labels"`
	Weight       *int64  `json:"weight,omitempty" jsonschema:"Weight of the epic"`
	HealthStatus string  `json:"health_status,omitempty" jsonschema:"Health status (onTrack/needsAttention/atRisk)"`
}

// UpdateInput defines parameters for updating an existing epic.
type UpdateInput struct {
	FullPath       string  `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID            int64   `json:"iid" jsonschema:"Epic IID within the group,required"`
	Title          string  `json:"title,omitempty" jsonschema:"Updated epic title"`
	Description    string  `json:"description,omitempty" jsonschema:"Updated description (Markdown supported)"`
	StateEvent     string  `json:"state_event,omitempty" jsonschema:"State event: CLOSE or REOPEN"`
	ParentID       *int64  `json:"parent_id,omitempty" jsonschema:"Global ID of the parent epic work item"`
	Color          string  `json:"color,omitempty" jsonschema:"Epic color (hex format)"`
	StartDate      string  `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD)"`
	DueDate        string  `json:"due_date,omitempty" jsonschema:"Due date (YYYY-MM-DD)"`
	AddLabelIDs    []int64 `json:"add_label_ids,omitempty" jsonschema:"Global IDs of labels to add"`
	RemoveLabelIDs []int64 `json:"remove_label_ids,omitempty" jsonschema:"Global IDs of labels to remove"`
	AssigneeIDs    []int64 `json:"assignee_ids,omitempty" jsonschema:"Global IDs of assignees (empty array to remove all)"`
	Weight         *int64  `json:"weight,omitempty" jsonschema:"Weight of the epic"`
	HealthStatus   string  `json:"health_status,omitempty" jsonschema:"Health status (onTrack/needsAttention/atRisk)"`
	Status         string  `json:"status,omitempty" jsonschema:"Work item status: TODO, IN_PROGRESS, DONE, WONT_DO, or DUPLICATE"`
}

// DeleteInput defines parameters for deleting an epic.
type DeleteInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID      int64  `json:"iid" jsonschema:"Epic IID within the group,required"`
}

// Output represents a single epic (backed by a Work Item of type Epic).
type Output struct {
	toolutil.HintableOutput
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
	Color        string       `json:"color,omitempty"`
	StartDate    string       `json:"start_date,omitempty"`
	DueDate      string       `json:"due_date,omitempty"`
	HealthStatus string       `json:"health_status,omitempty"`
	Weight       *int64       `json:"weight,omitempty"`
	ParentIID    int64        `json:"parent_iid,omitempty"`
	ParentPath   string       `json:"parent_path,omitempty"`
	CreatedAt    string       `json:"created_at,omitempty"`
	UpdatedAt    string       `json:"updated_at,omitempty"`
	ClosedAt     string       `json:"closed_at,omitempty"`
}

// ListOutput holds a list of epics with cursor-based pagination info.
type ListOutput struct {
	toolutil.HintableOutput
	Epics []Output `json:"epics"`
}

// LinksOutput holds child epics of a parent epic (REST-backed).
type LinksOutput struct {
	toolutil.HintableOutput
	ChildEpics []LinksItem `json:"child_epics"`
}

// LinksItem is a simplified epic output for the GetLinks REST endpoint.
type LinksItem struct {
	ID           int64    `json:"id"`
	IID          int64    `json:"iid"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	WebURL       string   `json:"web_url,omitempty"`
	Author       string   `json:"author,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	Confidential bool     `json:"confidential,omitempty"`
	CreatedAt    string   `json:"created_at,omitempty"`
}

// toOutput converts a GitLab Work Item to the epic Output format.
func toOutput(wi *gl.WorkItem) Output {
	out := Output{
		ID:           wi.ID,
		IID:          wi.IID,
		Type:         wi.Type,
		State:        wi.State,
		Title:        wi.Title,
		Description:  wi.Description,
		WebURL:       wi.WebURL,
		Confidential: wi.Confidential,
		Weight:       wi.Weight,
	}
	if wi.Status != nil {
		out.Status = *wi.Status
	}
	if wi.Author != nil {
		out.Author = wi.Author.Username
	}
	for _, a := range wi.Assignees {
		out.Assignees = append(out.Assignees, a.Username)
	}
	for _, l := range wi.Labels {
		out.Labels = append(out.Labels, l.Name)
	}
	for _, li := range wi.LinkedItems {
		out.LinkedItems = append(out.LinkedItems, LinkedItem{
			IID:      li.IID,
			LinkType: li.LinkType,
			Path:     li.NamespacePath,
		})
	}
	if wi.Color != nil {
		out.Color = *wi.Color
	}
	if wi.HealthStatus != nil {
		out.HealthStatus = *wi.HealthStatus
	}
	if wi.Parent != nil {
		out.ParentIID = wi.Parent.IID
		out.ParentPath = wi.Parent.NamespacePath
	}
	if wi.StartDate != nil {
		out.StartDate = time.Time(*wi.StartDate).Format(time.DateOnly)
	}
	if wi.DueDate != nil {
		out.DueDate = time.Time(*wi.DueDate).Format(time.DateOnly)
	}
	if wi.CreatedAt != nil {
		out.CreatedAt = wi.CreatedAt.Format(time.RFC3339)
	}
	if wi.UpdatedAt != nil {
		out.UpdatedAt = wi.UpdatedAt.Format(time.RFC3339)
	}
	if wi.ClosedAt != nil {
		out.ClosedAt = wi.ClosedAt.Format(time.RFC3339)
	}
	return out
}

// toLinkItem converts a GitLab REST Epic to the LinksItem format.
func toLinkItem(e *gl.Epic) LinksItem {
	out := LinksItem{
		ID:           e.ID,
		IID:          e.IID,
		Title:        e.Title,
		State:        e.State,
		WebURL:       e.WebURL,
		Labels:       e.Labels,
		Confidential: e.Confidential,
	}
	if e.Author != nil {
		out.Author = e.Author.Username
	}
	if e.CreatedAt != nil {
		out.CreatedAt = e.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// mapStatusToID maps a human-readable status string to the GitLab WorkItemStatusID.
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

// List retrieves epics for a group using the Work Items API with type filter.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.FullPath == "" {
		return ListOutput{}, errors.New("epicList: full_path is required. Use gitlab_group_list to find the group path first")
	}
	opts := &gl.ListWorkItemsOptions{
		Types: []string{"Epic"},
	}
	if input.State != "" {
		opts.State = &input.State
	}
	if input.Search != "" {
		opts.Search = &input.Search
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
	if input.First != nil {
		opts.First = input.First
	}
	if input.After != "" {
		opts.After = &input.After
	}
	if input.IncludeAncestors != nil {
		opts.IncludeAncestors = input.IncludeAncestors
	}
	if input.IncludeDescendants != nil {
		opts.IncludeDescendants = input.IncludeDescendants
	}

	items, _, err := client.GL().WorkItems.ListWorkItems(input.FullPath, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("epicList", err, http.StatusNotFound,
			"verify full_path with gitlab_group_list; epics require GitLab Premium or Ultimate")
	}
	out := make([]Output, 0, len(items))
	for _, wi := range items {
		out = append(out, toOutput(wi))
	}
	return ListOutput{Epics: out}, nil
}

// Get retrieves a single epic by its IID using the Work Items API.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.FullPath == "" {
		return Output{}, errors.New("epicGet: full_path is required. Use gitlab_group_list to find the group path first")
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicGet", "iid")
	}
	wi, _, err := client.GL().WorkItems.GetWorkItem(input.FullPath, input.IID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("epicGet", err, http.StatusNotFound,
			"verify iid with gitlab_epic_list; full_path must be the group path (e.g. group/subgroup) where the epic lives")
	}
	return toOutput(wi), nil
}

// GetLinks retrieves all child epics of a parent epic.
// This handler uses the REST API because client-go v2 does not yet expose
// a GraphQL query for work item children.
func GetLinks(ctx context.Context, client *gitlabclient.Client, input GetLinksInput) (LinksOutput, error) {
	if err := ctx.Err(); err != nil {
		return LinksOutput{}, err
	}
	if input.FullPath == "" {
		return LinksOutput{}, errors.New("epicGetLinks: full_path is required. Use gitlab_group_list to find the group path first")
	}
	if input.IID <= 0 {
		return LinksOutput{}, toolutil.ErrRequiredInt64("epicGetLinks", "iid")
	}
	epics, _, err := client.GL().Epics.GetEpicLinks(input.FullPath, input.IID, gl.WithContext(ctx))
	if err != nil {
		return LinksOutput{}, toolutil.WrapErrWithStatusHint("epicGetLinks", err, http.StatusNotFound,
			"verify iid with gitlab_epic_list; child epics are returned only when the parent epic exists in the given group")
	}
	out := make([]LinksItem, len(epics))
	for i, e := range epics {
		out[i] = toLinkItem(e)
	}
	return LinksOutput{ChildEpics: out}, nil
}

// Create creates a new epic using the Work Items API with the Epic type.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.FullPath == "" {
		return Output{}, errors.New("epicCreate: full_path is required. Use gitlab_group_list to find the group path first")
	}
	if input.Title == "" {
		return Output{}, errors.New("epicCreate: title is required")
	}
	opts := &gl.CreateWorkItemOptions{
		Title: input.Title,
	}
	if input.Description != "" {
		desc := toolutil.NormalizeText(input.Description)
		opts.Description = &desc
	}
	if input.Confidential != nil {
		opts.Confidential = input.Confidential
	}
	if len(input.AssigneeIDs) > 0 {
		opts.AssigneeIDs = input.AssigneeIDs
	}
	if len(input.LabelIDs) > 0 {
		opts.LabelIDs = input.LabelIDs
	}
	if input.Weight != nil {
		opts.Weight = input.Weight
	}
	if input.HealthStatus != "" {
		opts.HealthStatus = &input.HealthStatus
	}
	if input.Color != "" {
		opts.Color = &input.Color
	}
	if input.StartDate != "" {
		d, err := time.Parse(time.DateOnly, input.StartDate)
		if err == nil {
			isoDate := gl.ISOTime(d)
			opts.StartDate = &isoDate
		}
	}
	if input.DueDate != "" {
		d, err := time.Parse(time.DateOnly, input.DueDate)
		if err == nil {
			isoDate := gl.ISOTime(d)
			opts.DueDate = &isoDate
		}
	}

	wi, _, err := client.GL().WorkItems.CreateWorkItem(input.FullPath, gl.WorkItemTypeEpic, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("epicCreate", err, http.StatusForbidden,
			"creating epics requires Reporter role or higher; epics require GitLab Premium or Ultimate")
	}
	return toOutput(wi), nil
}

// Update modifies an existing epic using the Work Items API.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.FullPath == "" {
		return Output{}, errors.New("epicUpdate: full_path is required. Use gitlab_group_list to find the group path first")
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicUpdate", "iid")
	}
	opts := &gl.UpdateWorkItemOptions{}
	if input.Title != "" {
		opts.Title = &input.Title
	}
	if input.Description != "" {
		desc := toolutil.NormalizeText(input.Description)
		opts.Description = &desc
	}
	if input.StateEvent != "" {
		ev := gl.WorkItemStateEvent(input.StateEvent)
		opts.StateEvent = &ev
	}
	if input.ParentID != nil {
		opts.ParentID = input.ParentID
	}
	if input.Color != "" {
		opts.Color = &input.Color
	}
	if len(input.AddLabelIDs) > 0 {
		opts.AddLabelIDs = input.AddLabelIDs
	}
	if len(input.RemoveLabelIDs) > 0 {
		opts.RemoveLabelIDs = input.RemoveLabelIDs
	}
	if input.AssigneeIDs != nil {
		opts.AssigneeIDs = input.AssigneeIDs
	}
	if input.Weight != nil {
		opts.Weight = input.Weight
	}
	if input.HealthStatus != "" {
		opts.HealthStatus = &input.HealthStatus
	}
	if input.StartDate != "" {
		d, err := time.Parse(time.DateOnly, input.StartDate)
		if err == nil {
			isoDate := gl.ISOTime(d)
			opts.StartDate = &isoDate
		}
	}
	if input.DueDate != "" {
		d, err := time.Parse(time.DateOnly, input.DueDate)
		if err == nil {
			isoDate := gl.ISOTime(d)
			opts.DueDate = &isoDate
		}
	}
	if input.Status != "" {
		status := mapStatusToID(input.Status)
		opts.Status = &status
	}

	wi, _, err := client.GL().WorkItems.UpdateWorkItem(input.FullPath, input.IID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("epicUpdate", err, http.StatusBadRequest,
			"state_event must be 'close' or 'reopen'; dates must be YYYY-MM-DD; verify iid with gitlab_epic_list")
	}
	return toOutput(wi), nil
}

// Delete permanently removes an epic using the Work Items API.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.FullPath == "" {
		return errors.New("epicDelete: full_path is required. Use gitlab_group_list to find the group path first")
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("epicDelete", "iid")
	}
	_, err := client.GL().WorkItems.DeleteWorkItem(input.FullPath, input.IID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("epicDelete", err, http.StatusForbidden,
			"deleting epics requires Owner role at the group level")
	}
	return nil
}
