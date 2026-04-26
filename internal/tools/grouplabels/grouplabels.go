// Package grouplabels implements GitLab group label operations including list,
// get, create, update, delete, subscribe, and unsubscribe. It exposes typed
// input/output structs and handler functions registered as MCP tools.
package grouplabels

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput defines parameters for listing labels in a GitLab group.
type ListInput struct {
	GroupID                 toolutil.StringOrInt `json:"group_id"                          jsonschema:"Group ID or URL-encoded path,required"`
	Search                  string               `json:"search,omitempty"                  jsonschema:"Filter labels by keyword search"`
	WithCounts              bool                 `json:"with_counts,omitempty"             jsonschema:"Include issue and merge request counts"`
	IncludeAncestorGroups   bool                 `json:"include_ancestor_groups,omitempty"  jsonschema:"Include labels from ancestor groups"`
	IncludeDescendantGroups bool                 `json:"include_descendant_groups,omitempty" jsonschema:"Include labels from descendant groups"`
	OnlyGroupLabels         bool                 `json:"only_group_labels,omitempty"       jsonschema:"Only return group-level labels (exclude project labels)"`
	toolutil.PaginationInput
}

// GetInput defines parameters for retrieving a single group label.
type GetInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	LabelID toolutil.StringOrInt `json:"label_id" jsonschema:"Label ID or name,required"`
}

// CreateInput defines parameters for creating a group label.
type CreateInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id"              jsonschema:"Group ID or URL-encoded path,required"`
	Name        string               `json:"name"                  jsonschema:"Label name,required"`
	Color       string               `json:"color"                 jsonschema:"Label color in hex format (e.g. #FF0000),required"`
	Description string               `json:"description,omitempty" jsonschema:"Label description"`
	Priority    int64                `json:"priority,omitempty"    jsonschema:"Label priority (lower is higher priority, 0 means no priority)"`
}

// UpdateInput defines parameters for updating a group label.
type UpdateInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id"              jsonschema:"Group ID or URL-encoded path,required"`
	LabelID     toolutil.StringOrInt `json:"label_id"              jsonschema:"Label ID or name,required"`
	NewName     string               `json:"new_name,omitempty"    jsonschema:"New label name"`
	Color       string               `json:"color,omitempty"       jsonschema:"New label color in hex format"`
	Description string               `json:"description,omitempty" jsonschema:"New label description"`
	Priority    int64                `json:"priority,omitempty"    jsonschema:"New label priority (0 to remove)"`
}

// DeleteInput defines parameters for deleting a group label.
type DeleteInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	LabelID toolutil.StringOrInt `json:"label_id" jsonschema:"Label ID or name,required"`
}

// SubscribeInput defines parameters for subscribing/unsubscribing to a group label.
type SubscribeInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	LabelID toolutil.StringOrInt `json:"label_id" jsonschema:"Label ID or name,required"`
}

// Output represents a single group label.
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

// ListOutput holds a paginated list of group labels.
type ListOutput struct {
	toolutil.HintableOutput
	Labels     []Output                  `json:"labels"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves a paginated list of labels for a GitLab group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, errors.New("groupLabelList: group_id is required")
	}

	opts := &gl.ListGroupLabelsOptions{}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.WithCounts {
		opts.WithCounts = new(true)
	}
	if input.IncludeAncestorGroups {
		opts.IncludeAncestorGroups = new(true)
	}
	if input.IncludeDescendantGroups {
		opts.IncludeDescendantGroups = new(true)
	}
	if input.OnlyGroupLabels {
		opts.OnlyGroupLabels = new(true)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	labels, resp, err := client.GL().GroupLabels.ListGroupLabels(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("groupLabelList", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get")
	}

	out := make([]Output, len(labels))
	for i, l := range labels {
		out[i] = toOutput(l)
	}
	return ListOutput{Labels: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// Get retrieves a single group label by ID or name.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("groupLabelGet: group_id is required")
	}
	l, _, err := client.GL().GroupLabels.GetGroupLabel(string(input.GroupID), string(input.LabelID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("groupLabelGet", err, http.StatusNotFound,
			"verify label_id (numeric ID or name) with gitlab_group_label_list; label names are case-sensitive")
	}
	return toOutput(l), nil
}

// Create creates a new label in a GitLab group.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("groupLabelCreate: group_id is required")
	}
	opts := &gl.CreateGroupLabelOptions{
		Name:  new(input.Name),
		Color: new(input.Color),
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Priority > 0 {
		opts.Priority = gl.NewNullableWithValue(input.Priority)
	}
	l, _, err := client.GL().GroupLabels.CreateGroupLabel(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("groupLabelCreate", err, http.StatusBadRequest,
			"name must be unique within the group; color must be a 6-digit hex string with leading # (e.g. #FF0000); creating group labels requires Reporter role or higher")
	}
	return toOutput(l), nil
}

// Update modifies an existing group label. Only non-empty fields are applied.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("groupLabelUpdate: group_id is required")
	}
	opts := &gl.UpdateGroupLabelOptions{}
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
	l, _, err := client.GL().GroupLabels.UpdateGroupLabel(string(input.GroupID), string(input.LabelID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("groupLabelUpdate", err, http.StatusBadRequest,
			"new_name must be unique within the group; color must be a 6-digit hex string with leading #; verify label_id with gitlab_group_label_list")
	}
	return toOutput(l), nil
}

// Delete removes a label from a GitLab group.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return errors.New("groupLabelDelete: group_id is required")
	}
	_, err := client.GL().GroupLabels.DeleteGroupLabel(string(input.GroupID), string(input.LabelID), &gl.DeleteGroupLabelOptions{}, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("groupLabelDelete", err, http.StatusForbidden,
			"deleting group labels requires Maintainer or Owner role")
	}
	return nil
}

// Subscribe subscribes the authenticated user to a group label.
func Subscribe(ctx context.Context, client *gitlabclient.Client, input SubscribeInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("groupLabelSubscribe: group_id is required")
	}
	l, _, err := client.GL().GroupLabels.SubscribeToGroupLabel(string(input.GroupID), string(input.LabelID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("groupLabelSubscribe", err, http.StatusNotModified,
			"the user is already subscribed to this label")
	}
	return toOutput(l), nil
}

// Unsubscribe removes the authenticated user's subscription from a group label.
func Unsubscribe(ctx context.Context, client *gitlabclient.Client, input SubscribeInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return errors.New("groupLabelUnsubscribe: group_id is required")
	}
	_, err := client.GL().GroupLabels.UnsubscribeFromGroupLabel(string(input.GroupID), string(input.LabelID), gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("groupLabelUnsubscribe", err, http.StatusNotModified,
			"the user is not subscribed to this label")
	}
	return nil
}

// toOutput converts a GitLab API [gl.GroupLabel] to MCP output format.
func toOutput(l *gl.GroupLabel) Output {
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
