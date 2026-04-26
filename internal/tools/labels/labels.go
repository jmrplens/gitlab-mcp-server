// Package labels implements MCP tool handlers for GitLab label operations
// including get, create, update, delete, subscribe, unsubscribe, promote,
// and list. It wraps the Labels service from client-go v2.
package labels

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GetInput defines parameters for retrieving a single label.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	LabelID   toolutil.StringOrInt `json:"label_id"   jsonschema:"Label ID or name,required"`
}

// CreateInput defines parameters for creating a label.
type CreateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	Name        string               `json:"name"                  jsonschema:"Label name,required"`
	Color       string               `json:"color"                 jsonschema:"Label color in hex format (e.g. #FF0000),required"`
	Description string               `json:"description,omitempty" jsonschema:"Label description"`
	Priority    int64                `json:"priority,omitempty"    jsonschema:"Label priority (lower is higher priority, 0 means no priority)"`
}

// UpdateInput defines parameters for updating a label.
type UpdateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	LabelID     toolutil.StringOrInt `json:"label_id"              jsonschema:"Label ID or name,required"`
	NewName     string               `json:"new_name,omitempty"    jsonschema:"New label name"`
	Color       string               `json:"color,omitempty"       jsonschema:"New label color in hex format"`
	Description string               `json:"description,omitempty" jsonschema:"New label description"`
	Priority    int64                `json:"priority,omitempty"    jsonschema:"New label priority (0 to remove)"`
}

// DeleteInput defines parameters for deleting a label.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	LabelID   toolutil.StringOrInt `json:"label_id"   jsonschema:"Label ID or name,required"`
}

// SubscribeInput defines parameters for subscribing/unsubscribing to a label.
type SubscribeInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	LabelID   toolutil.StringOrInt `json:"label_id"   jsonschema:"Label ID or name,required"`
}

// PromoteInput defines parameters for promoting a project label to a group label.
type PromoteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	LabelID   toolutil.StringOrInt `json:"label_id"   jsonschema:"Label ID or name,required"`
}

// ListInput defines parameters for listing labels in a GitLab project.
type ListInput struct {
	ProjectID             toolutil.StringOrInt `json:"project_id"                       jsonschema:"Project ID or URL-encoded path,required"`
	Search                string               `json:"search,omitempty"                 jsonschema:"Filter labels by keyword search"`
	WithCounts            bool                 `json:"with_counts,omitempty"            jsonschema:"Include issue and merge request counts"`
	IncludeAncestorGroups bool                 `json:"include_ancestor_groups,omitempty" jsonschema:"Include labels from ancestor groups"`
	toolutil.PaginationInput
}

// Output represents a single project label.
type Output struct {
	toolutil.HintableOutput
	ID                     int64  `json:"id"`
	Name                   string `json:"name"`
	Color                  string `json:"color"`
	TextColor              string `json:"text_color"`
	Description            string `json:"description"`
	OpenIssuesCount        int64  `json:"open_issues_count"`
	ClosedIssuesCount      int64  `json:"closed_issues_count"`
	OpenMergeRequestsCount int64  `json:"open_merge_requests_count"`
	Priority               int64  `json:"priority"`
	IsProjectLabel         bool   `json:"is_project_label"`
	Subscribed             bool   `json:"subscribed"`
}

// ListOutput holds a paginated list of labels.
type ListOutput struct {
	toolutil.HintableOutput
	Labels     []Output                  `json:"labels"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves a paginated list of labels for a GitLab project.
// Supports filtering by search keyword and including ancestor group labels.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("labelList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	opts := &gl.ListLabelsOptions{}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.WithCounts {
		opts.WithCounts = new(true)
	}
	if input.IncludeAncestorGroups {
		opts.IncludeAncestorGroups = new(true)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	labels, resp, err := client.GL().Labels.ListLabels(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("labelList", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get")
	}

	out := make([]Output, len(labels))
	for i, l := range labels {
		out[i] = toOutput(l)
	}
	return ListOutput{Labels: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// Get retrieves a single label by ID or name.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("labelGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	l, _, err := client.GL().Labels.GetLabel(string(input.ProjectID), string(input.LabelID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("labelGet", err, http.StatusNotFound,
			"verify label_id (numeric ID or name) with gitlab_label_list; label names are case-sensitive")
	}
	return toOutput(l), nil
}

// Create creates a new label in a GitLab project.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("labelCreate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := &gl.CreateLabelOptions{
		Name:  new(input.Name),
		Color: new(input.Color),
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Priority > 0 {
		opts.Priority = gl.NewNullableWithValue(input.Priority)
	}
	l, _, err := client.GL().Labels.CreateLabel(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		switch {
		case toolutil.IsHTTPStatus(err, http.StatusConflict):
			return Output{}, toolutil.WrapErrWithHint("labelCreate", err, "a label with this name already exists — use gitlab_label_update to modify it, or gitlab_label_list to see existing labels")
		case toolutil.IsHTTPStatus(err, http.StatusBadRequest):
			return Output{}, toolutil.WrapErrWithHint("labelCreate", err, "check the color format (#RRGGBB) and that the name is not empty")
		default:
			return Output{}, toolutil.WrapErrWithMessage("labelCreate", err)
		}
	}
	return toOutput(l), nil
}

// Update modifies an existing label. Only non-empty fields are applied.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("labelUpdate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := &gl.UpdateLabelOptions{}
	if input.NewName != "" {
		opts.NewName = new(input.NewName)
	}
	if input.Color != "" {
		opts.Color = new(input.Color)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Priority > 0 {
		opts.Priority = gl.NewNullableWithValue(input.Priority)
	}
	l, _, err := client.GL().Labels.UpdateLabel(string(input.ProjectID), string(input.LabelID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("labelUpdate", err, http.StatusBadRequest,
			"verify label_id (numeric ID or name) with gitlab_label_list; new_name must be unique; color must be 6-digit hex (e.g. #FF0000)")
	}
	return toOutput(l), nil
}

// Delete removes a label from a GitLab project.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("labelDelete: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	_, err := client.GL().Labels.DeleteLabel(string(input.ProjectID), string(input.LabelID), &gl.DeleteLabelOptions{}, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("labelDelete", err, http.StatusForbidden,
			"deleting project labels requires Maintainer or Owner role; group-inherited labels must be deleted at the group level")
	}
	return nil
}

// Subscribe subscribes the authenticated user to a label to receive notifications.
func Subscribe(ctx context.Context, client *gitlabclient.Client, input SubscribeInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("labelSubscribe: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	l, _, err := client.GL().Labels.SubscribeToLabel(string(input.ProjectID), string(input.LabelID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("labelSubscribe", err, http.StatusNotModified,
			"the user is already subscribed to this label")
	}
	return toOutput(l), nil
}

// Unsubscribe removes the authenticated user's subscription from a label.
func Unsubscribe(ctx context.Context, client *gitlabclient.Client, input SubscribeInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("labelUnsubscribe: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	_, err := client.GL().Labels.UnsubscribeFromLabel(string(input.ProjectID), string(input.LabelID), gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("labelUnsubscribe", err, http.StatusNotModified,
			"the user is not subscribed to this label")
	}
	return nil
}

// Promote promotes a project label to a group label.
func Promote(ctx context.Context, client *gitlabclient.Client, input PromoteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("labelPromote: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	_, err := client.GL().Labels.PromoteLabel(string(input.ProjectID), string(input.LabelID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("labelPromote", err, "label promotion requires group-level Maintainer or higher access")
		}
		return toolutil.WrapErrWithStatusHint("labelPromote", err, http.StatusNotFound,
			"verify label_id with gitlab_label_list; project must belong to a group (cannot promote labels in personal projects)")
	}
	return nil
}

// labelToOutput converts a GitLab API [gl.Label] to MCP output format.
func toOutput(l *gl.Label) Output {
	return Output{
		ID:                     l.ID,
		Name:                   l.Name,
		Color:                  l.Color,
		TextColor:              l.TextColor,
		Description:            l.Description,
		OpenIssuesCount:        l.OpenIssuesCount,
		ClosedIssuesCount:      l.ClosedIssuesCount,
		OpenMergeRequestsCount: l.OpenMergeRequestsCount,
		Priority:               priorityFromNullable(l.Priority),
		IsProjectLabel:         l.IsProjectLabel,
		Subscribed:             l.Subscribed,
	}
}

// priorityFromNullable extracts the int64 value from a Nullable[int64], returning 0 if unset.
func priorityFromNullable(n gl.Nullable[int64]) int64 {
	if !n.IsSpecified() || n.IsNull() {
		return 0
	}
	return n.MustGet()
}
