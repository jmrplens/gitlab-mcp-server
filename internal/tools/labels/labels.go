package labels

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/labeldata"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	Archived    *bool                `json:"archived,omitempty"    jsonschema:"Whether to create the label in archived state"`
}

// UpdateInput defines parameters for updating a label.
type UpdateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	LabelID     toolutil.StringOrInt `json:"label_id"              jsonschema:"Label ID or name,required"`
	NewName     string               `json:"new_name,omitempty"    jsonschema:"New label name"`
	Color       string               `json:"color,omitempty"       jsonschema:"New label color in hex format"`
	Description string               `json:"description,omitempty" jsonschema:"New label description"`
	Priority    int64                `json:"priority,omitempty"    jsonschema:"New label priority (0 to remove)"`
	Archived    *bool                `json:"archived,omitempty"    jsonschema:"Set true to archive, false to unarchive"`
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
type Output = labeldata.Output

// ListOutput holds a paginated list of labels.
type ListOutput struct {
	toolutil.HintableOutput
	Labels     []Output                  `json:"labels"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves a paginated list of project labels via the GitLab
// Labels API (GET /projects/:id/labels). Supports filtering by search
// keyword, including ancestor group labels, and including issue/MR
// counts.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("labelList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	opts := labeldata.NewProjectListOptions(input.Page, input.PerPage, input.Search, input.WithCounts, input.IncludeAncestorGroups)

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

// Get retrieves a single project label by its ID or name via the
// GitLab Labels API (GET /projects/:id/labels/:label_id).
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

// Create creates a new label in a GitLab project via the GitLab Labels
// API (POST /projects/:id/labels). The Color must be a 6-digit hex
// value; existing label names return 409 Conflict.
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
	if input.Archived != nil {
		opts.Archived = input.Archived
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

// Update modifies an existing project label via the GitLab Labels API
// (PUT /projects/:id/labels/:label_id). Only non-empty fields in the
// input are applied; new_name must be unique within the project.
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
	if input.Archived != nil {
		opts.Archived = input.Archived
	}
	l, _, err := client.GL().Labels.UpdateLabel(string(input.ProjectID), string(input.LabelID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("labelUpdate", err, http.StatusBadRequest,
			"verify label_id (numeric ID or name) with gitlab_label_list; new_name must be unique; color must be 6-digit hex (e.g. #FF0000)")
	}
	return toOutput(l), nil
}

// Delete removes a label from a GitLab project via the GitLab Labels
// API (DELETE /projects/:id/labels/:label_id). Requires Maintainer or
// Owner role; group-inherited labels must be deleted at the group level.
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

// Subscribe subscribes the authenticated user to a label to receive
// notifications via the GitLab Labels subscribe API
// (POST /projects/:id/labels/:label_id/subscribe). Returns 304 Not
// Modified when the user is already subscribed.
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

// Unsubscribe removes the authenticated user's subscription from a
// project label via the GitLab Labels unsubscribe API
// (POST /projects/:id/labels/:label_id/unsubscribe). Returns 304 Not
// Modified when the user is not currently subscribed.
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

// Promote promotes a project label to a group label via the GitLab
// Labels promote API (POST /projects/:id/labels/:label_id/promote).
// Requires the project to belong to a group; cannot promote labels in
// personal (user-namespace) projects.
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

// toOutput delegates to [labeldata.ProjectOutput] so the package shares
// the same [Output] shape with the [labeldata] sub-package.
func toOutput(label *gl.Label) Output {
	return labeldata.ProjectOutput(label)
}
