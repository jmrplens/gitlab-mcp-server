// Package epics implements GitLab group epic operations including list, get,
// get links (child epics), create, update, and delete. Epics are high-level
// planning items attached to groups (not projects).
package epics

import (
	"context"
	"errors"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput defines parameters for listing group epics.
type ListInput struct {
	GroupID                 toolutil.StringOrInt `json:"group_id"                            jsonschema:"Group ID or URL-encoded path,required"`
	AuthorID                *int64               `json:"author_id,omitempty"                 jsonschema:"Filter by author user ID"`
	Labels                  string               `json:"labels,omitempty"                    jsonschema:"Comma-separated label names to filter by"`
	OrderBy                 string               `json:"order_by,omitempty"                  jsonschema:"Order by field (created_at, updated_at)"`
	Sort                    string               `json:"sort,omitempty"                      jsonschema:"Sort direction (asc, desc)"`
	Search                  string               `json:"search,omitempty"                    jsonschema:"Search epics by title and description"`
	State                   string               `json:"state,omitempty"                     jsonschema:"Filter by state (opened, closed)"`
	IncludeAncestorGroups   *bool                `json:"include_ancestor_groups,omitempty"    jsonschema:"Include epics from ancestor groups"`
	IncludeDescendantGroups *bool                `json:"include_descendant_groups,omitempty"  jsonschema:"Include epics from descendant groups"`
	toolutil.PaginationInput
}

// GetInput defines parameters for getting a single group epic.
type GetInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID int64                `json:"epic_iid" jsonschema:"Epic internal ID within the group,required"`
}

// GetLinksInput defines parameters for listing child epics.
type GetLinksInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID int64                `json:"epic_iid" jsonschema:"Epic internal ID within the group,required"`
}

// CreateInput defines parameters for creating a new epic.
type CreateInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"                    jsonschema:"Group ID or URL-encoded path,required"`
	Title        string               `json:"title"                       jsonschema:"Epic title,required"`
	Description  string               `json:"description,omitempty"       jsonschema:"Epic description (Markdown supported)"`
	Labels       string               `json:"labels,omitempty"            jsonschema:"Comma-separated label names"`
	Confidential *bool                `json:"confidential,omitempty"      jsonschema:"Whether the epic is confidential"`
	ParentID     *int64               `json:"parent_id,omitempty"         jsonschema:"ID of the parent epic"`
	Color        string               `json:"color,omitempty"             jsonschema:"Epic color (hex format, e.g. #FF0000)"`
}

// UpdateInput defines parameters for updating an existing epic.
type UpdateInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"                    jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID      int64                `json:"epic_iid"                    jsonschema:"Epic internal ID within the group,required"`
	Title        string               `json:"title,omitempty"             jsonschema:"Updated epic title"`
	Description  string               `json:"description,omitempty"       jsonschema:"Updated description (Markdown supported)"`
	Labels       string               `json:"labels,omitempty"            jsonschema:"Comma-separated label names (replaces existing)"`
	AddLabels    string               `json:"add_labels,omitempty"        jsonschema:"Comma-separated label names to add"`
	RemoveLabels string               `json:"remove_labels,omitempty"     jsonschema:"Comma-separated label names to remove"`
	StateEvent   string               `json:"state_event,omitempty"       jsonschema:"State event (close, reopen)"`
	Confidential *bool                `json:"confidential,omitempty"      jsonschema:"Whether the epic is confidential"`
	ParentID     *int64               `json:"parent_id,omitempty"         jsonschema:"ID of the parent epic"`
	Color        string               `json:"color,omitempty"             jsonschema:"Epic color (hex format)"`
}

// DeleteInput defines parameters for deleting an epic.
type DeleteInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EpicIID int64                `json:"epic_iid" jsonschema:"Epic internal ID within the group,required"`
}

// Output represents a single group epic.
type Output struct {
	toolutil.HintableOutput
	ID             int64    `json:"id"`
	IID            int64    `json:"iid"`
	GroupID        int64    `json:"group_id"`
	ParentID       int64    `json:"parent_id,omitempty"`
	Title          string   `json:"title"`
	Description    string   `json:"description,omitempty"`
	State          string   `json:"state"`
	Confidential   bool     `json:"confidential"`
	WebURL         string   `json:"web_url"`
	Author         string   `json:"author"`
	Labels         []string `json:"labels,omitempty"`
	StartDate      string   `json:"start_date,omitempty"`
	DueDate        string   `json:"due_date,omitempty"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
	ClosedAt       string   `json:"closed_at,omitempty"`
	Upvotes        int64    `json:"upvotes"`
	Downvotes      int64    `json:"downvotes"`
	UserNotesCount int64    `json:"user_notes_count"`
}

// ListOutput holds a paginated list of group epics.
type ListOutput struct {
	toolutil.HintableOutput
	Epics      []Output                  `json:"epics"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// LinksOutput holds child epics of a parent epic.
type LinksOutput struct {
	toolutil.HintableOutput
	ChildEpics []Output `json:"child_epics"`
}

// toOutput converts a GitLab API Epic to the MCP tool output format.
func toOutput(e *gl.Epic) Output {
	out := Output{
		ID:             e.ID,
		IID:            e.IID,
		GroupID:        e.GroupID,
		ParentID:       e.ParentID,
		Title:          e.Title,
		Description:    e.Description,
		State:          e.State,
		Confidential:   e.Confidential,
		WebURL:         e.WebURL,
		Labels:         e.Labels,
		Upvotes:        e.Upvotes,
		Downvotes:      e.Downvotes,
		UserNotesCount: e.UserNotesCount,
	}
	if e.Author != nil {
		out.Author = e.Author.Username
	}
	if e.StartDate != nil {
		out.StartDate = time.Time(*e.StartDate).Format(time.DateOnly)
	}
	if e.DueDate != nil {
		out.DueDate = time.Time(*e.DueDate).Format(time.DateOnly)
	}
	if e.CreatedAt != nil {
		out.CreatedAt = e.CreatedAt.Format(time.RFC3339)
	}
	if e.UpdatedAt != nil {
		out.UpdatedAt = e.UpdatedAt.Format(time.RFC3339)
	}
	if e.ClosedAt != nil {
		out.ClosedAt = e.ClosedAt.Format(time.RFC3339)
	}
	return out
}

// splitLabels converts a comma-separated label string to a LabelOptions pointer.
func splitLabels(s string) *gl.LabelOptions {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	lo := gl.LabelOptions(parts)
	return &lo
}

// List retrieves a paginated list of epics for a group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, errors.New("epicList: group_id is required. Use gitlab_group_list to find the group ID first")
	}
	opts := &gl.ListGroupEpicsOptions{}
	if input.AuthorID != nil {
		opts.AuthorID = input.AuthorID
	}
	if input.Labels != "" {
		opts.Labels = splitLabels(input.Labels)
	}
	if input.OrderBy != "" {
		opts.OrderBy = &input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = &input.Sort
	}
	if input.Search != "" {
		opts.Search = &input.Search
	}
	if input.State != "" {
		opts.State = &input.State
	}
	if input.IncludeAncestorGroups != nil {
		opts.IncludeAncestorGroups = input.IncludeAncestorGroups
	}
	if input.IncludeDescendantGroups != nil {
		opts.IncludeDescendantGroups = input.IncludeDescendantGroups
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	epics, resp, err := client.GL().Epics.ListGroupEpics(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("epicList", err)
	}
	out := make([]Output, len(epics))
	for i, e := range epics {
		out[i] = toOutput(e)
	}
	return ListOutput{Epics: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// Get retrieves a single group epic by its IID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("epicGet: group_id is required. Use gitlab_group_list to find the group ID first")
	}
	if input.EpicIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicGet", "epic_iid")
	}
	e, _, err := client.GL().Epics.GetEpic(string(input.GroupID), input.EpicIID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("epicGet", err)
	}
	return toOutput(e), nil
}

// GetLinks retrieves all child epics of a parent epic.
func GetLinks(ctx context.Context, client *gitlabclient.Client, input GetLinksInput) (LinksOutput, error) {
	if err := ctx.Err(); err != nil {
		return LinksOutput{}, err
	}
	if input.GroupID == "" {
		return LinksOutput{}, errors.New("epicGetLinks: group_id is required. Use gitlab_group_list to find the group ID first")
	}
	if input.EpicIID <= 0 {
		return LinksOutput{}, toolutil.ErrRequiredInt64("epicGetLinks", "epic_iid")
	}
	epics, _, err := client.GL().Epics.GetEpicLinks(string(input.GroupID), input.EpicIID, gl.WithContext(ctx))
	if err != nil {
		return LinksOutput{}, toolutil.WrapErrWithMessage("epicGetLinks", err)
	}
	out := make([]Output, len(epics))
	for i, e := range epics {
		out[i] = toOutput(e)
	}
	return LinksOutput{ChildEpics: out}, nil
}

// Create creates a new epic in a group.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("epicCreate: group_id is required. Use gitlab_group_list to find the group ID first")
	}
	if input.Title == "" {
		return Output{}, errors.New("epicCreate: title is required")
	}
	opts := &gl.CreateEpicOptions{
		Title: &input.Title,
	}
	if input.Description != "" {
		desc := toolutil.NormalizeText(input.Description)
		opts.Description = &desc
	}
	if input.Labels != "" {
		opts.Labels = splitLabels(input.Labels)
	}
	if input.Confidential != nil {
		opts.Confidential = input.Confidential
	}
	if input.ParentID != nil {
		opts.ParentID = input.ParentID
	}
	if input.Color != "" {
		opts.Color = &input.Color
	}
	e, _, err := client.GL().Epics.CreateEpic(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("epicCreate", err)
	}
	return toOutput(e), nil
}

// Update modifies an existing group epic.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("epicUpdate: group_id is required. Use gitlab_group_list to find the group ID first")
	}
	if input.EpicIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicUpdate", "epic_iid")
	}
	opts := &gl.UpdateEpicOptions{}
	if input.Title != "" {
		opts.Title = &input.Title
	}
	if input.Description != "" {
		desc := toolutil.NormalizeText(input.Description)
		opts.Description = &desc
	}
	if input.Labels != "" {
		opts.Labels = splitLabels(input.Labels)
	}
	if input.AddLabels != "" {
		opts.AddLabels = splitLabels(input.AddLabels)
	}
	if input.RemoveLabels != "" {
		opts.RemoveLabels = splitLabels(input.RemoveLabels)
	}
	if input.StateEvent != "" {
		opts.StateEvent = &input.StateEvent
	}
	if input.Confidential != nil {
		opts.Confidential = input.Confidential
	}
	if input.ParentID != nil {
		opts.ParentID = input.ParentID
	}
	if input.Color != "" {
		opts.Color = &input.Color
	}
	e, _, err := client.GL().Epics.UpdateEpic(string(input.GroupID), input.EpicIID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("epicUpdate", err)
	}
	return toOutput(e), nil
}

// Delete removes an epic from a group.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return errors.New("epicDelete: group_id is required. Use gitlab_group_list to find the group ID first")
	}
	if input.EpicIID <= 0 {
		return toolutil.ErrRequiredInt64("epicDelete", "epic_iid")
	}
	_, err := client.GL().Epics.DeleteEpic(string(input.GroupID), input.EpicIID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("epicDelete", err)
	}
	return nil
}
