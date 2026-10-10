package labels

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/labeldata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	LabelID     toolutil.StringOrInt `json:"label_id,omitempty"    jsonschema:"Label ID or name. Give this or name"`
	Name        string               `json:"name,omitempty"        jsonschema:"Name of the label to update, used to select it when label_id is not given"`
	NewName     string               `json:"new_name,omitempty"    jsonschema:"New label name"`
	Color       string               `json:"color,omitempty"       jsonschema:"New label color in hex format"`
	Description string               `json:"description,omitempty" jsonschema:"New label description"`
	// A pointer so that "not given" and "given as zero" are different
	// requests. The schema has always promised that zero removes the priority,
	// and with a plain int64 that promise could not be kept: encoding/json
	// omits a zero under omitempty, so the handler could not tell a caller
	// asking for removal from one saying nothing at all.
	Priority *int64 `json:"priority,omitempty"    jsonschema:"New label priority, where 0 removes the priority GitLab has"`
	Archived *bool  `json:"archived,omitempty"    jsonschema:"Set true to archive, false to unarchive"`
}

// DeleteInput defines parameters for deleting a label.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"     jsonschema:"Project ID or URL-encoded path,required"`
	LabelID   toolutil.StringOrInt `json:"label_id,omitempty" jsonschema:"Label ID or name. Give this or name"`
	Name      string               `json:"name,omitempty"     jsonschema:"Name of the label to delete, used to select it when label_id is not given"`
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
	OrderBy               string               `json:"order_by,omitempty"               jsonschema:"Column to order results by (e.g. name, created_at, updated_at)"`
	Sort                  string               `json:"sort,omitempty"                   jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
		return ListOutput{}, errors.New("labelList: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}

	opts := labeldata.NewProjectListOptions(input.Page, input.PerPage, input.Search, input.WithCounts, input.IncludeAncestorGroups)
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	labels, resp, err := client.GL().Labels.ListLabels(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("labelList", err, http.StatusNotFound,
			"verify project_id with project.get")
	}
	extras, err := toolutil.CapturedLabels(captured, len(labels))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("labelList", err)
	}

	out := make([]Output, len(labels))
	for i, l := range labels {
		out[i] = labeldata.ProjectListOutput(l, extras[i], input.WithCounts)
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
		return Output{}, errors.New("labelGet: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	l, _, err := client.GL().Labels.GetLabel(string(input.ProjectID), string(input.LabelID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("labelGet", err, http.StatusNotFound,
			"verify label_id (numeric ID or name) with project.label_list; label names are case-sensitive")
	}
	return capturedOutput("labelGet", l, captured)
}

// Create creates a new label in a GitLab project via the GitLab Labels
// API (POST /projects/:id/labels). The Color must be a 6-digit hex
// value; existing label names return 409 Conflict.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("labelCreate: project_id is required. Use project.list to find the ID first, then pass it as project_id")
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	l, _, err := client.GL().Labels.CreateLabel(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		switch {
		case toolutil.IsHTTPStatus(err, http.StatusConflict):
			return Output{}, toolutil.WrapErrWithHint("labelCreate", err, "a label with this name already exists. Use project.label_update to modify it, or project.label_list to see existing labels")
		case toolutil.IsHTTPStatus(err, http.StatusBadRequest):
			return Output{}, toolutil.WrapErrWithHint("labelCreate", err, "check the color format (#RRGGBB) and that the name is not empty")
		default:
			return Output{}, toolutil.WrapErrWithMessage("labelCreate", err)
		}
	}
	return capturedOutput("labelCreate", l, captured)
}

// Update modifies an existing project label via the GitLab Labels API
// (PUT /projects/:id/labels/:label_id). Only non-empty fields in the
// input are applied; new_name must be unique within the project.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("labelUpdate: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}
	if input.LabelID == "" && input.Name == "" {
		return Output{}, errors.New("labelUpdate: label_id or name is required to select the label")
	}
	opts := &gl.UpdateLabelOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.NewName != "" {
		opts.NewName = new(input.NewName)
	}
	if input.Color != "" {
		opts.Color = new(input.Color)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Priority != nil {
		priority, err := updatedPriority(*input.Priority)
		if err != nil {
			return Output{}, err
		}
		opts.Priority = priority
	}
	if input.Archived != nil {
		opts.Archived = input.Archived
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	l, _, err := client.GL().Labels.UpdateLabel(string(input.ProjectID), labelSelector(input.LabelID, input.Name), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("labelUpdate", err, http.StatusBadRequest,
			"verify label_id (numeric ID or name) with project.label_list; new_name must be unique; color must be 6-digit hex (e.g. #FF0000)")
	}
	return capturedOutput("labelUpdate", l, captured)
}

// Delete removes a label from a GitLab project via the GitLab Labels
// API (DELETE /projects/:id/labels/:label_id). Requires Maintainer or
// Owner role; group-inherited labels must be deleted at the group level.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("labelDelete: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}
	if input.LabelID == "" && input.Name == "" {
		return errors.New("labelDelete: label_id or name is required to select the label")
	}
	delOpts := &gl.DeleteLabelOptions{}
	if input.Name != "" {
		delOpts.Name = new(input.Name)
	}
	_, err := client.GL().Labels.DeleteLabel(string(input.ProjectID), labelSelector(input.LabelID, input.Name), delOpts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("labelDelete", err, http.StatusForbidden,
			"deleting project labels requires Maintainer or Owner role; group-inherited labels must be deleted at the group level")
	}
	return nil
}

// updatedPriority is the priority an update sends for the one a caller asked
// for. Early returns rather than a tagless switch, so the mutation tool, which
// cannot see a case expression, measures each condition.
func updatedPriority(priority int64) (gl.Nullable[int64], error) {
	if priority > 0 {
		return gl.NewNullableWithValue(priority), nil
	}
	if priority == 0 {
		// Zero is the removal the schema promises, and it goes out as an
		// explicit null: sending the number 0 would set a priority of zero,
		// which is a priority rather than the absence of one.
		return gl.NewNullNullable[int64](), nil
	}
	// A negative is neither a priority GitLab accepts nor the removal zero
	// asks for, and treating it as removal would perform a change the caller
	// did not ask for on a value they got wrong.
	return nil, errors.New("labelUpdate: priority must be zero or greater. Pass 0 to remove the label's priority, or a positive number to set one")
}

// labelSelector is the label argument client-go takes for an update or a
// delete: label_id when the caller gave one, and the name otherwise. client-go
// puts any non-nil value in the path, so either reaches PUT or DELETE
// /projects/:id/labels/:name, which GitLab documents as taking the name or
// the id of the label. Handed nil, client-go would select the label by the
// name in the body or the query instead, through the collection route that
// GitLab 12.4 retired and doc/api/labels.md no longer lists; handed an empty
// string, it named a label called nothing at /labels/. The handlers refuse a
// call carrying neither before this is reached.
func labelSelector(labelID toolutil.StringOrInt, name string) string {
	if labelID == "" {
		return name
	}
	return string(labelID)
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
		return Output{}, errors.New("labelSubscribe: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	l, _, err := client.GL().Labels.SubscribeToLabel(string(input.ProjectID), string(input.LabelID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("labelSubscribe", err, http.StatusNotModified,
			"the user is already subscribed to this label")
	}
	return capturedOutput("labelSubscribe", l, captured)
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
		return errors.New("labelUnsubscribe: project_id is required. Use project.list to find the ID first, then pass it as project_id")
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
		return errors.New("labelPromote: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}
	_, err := client.GL().Labels.PromoteLabel(string(input.ProjectID), string(input.LabelID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("labelPromote", err, "label promotion requires group-level Maintainer or higher access")
		}
		return toolutil.WrapErrWithStatusHint("labelPromote", err, http.StatusNotFound,
			"verify label_id with project.label_list; project must belong to a group (cannot promote labels in personal projects)")
	}
	return nil
}

// toOutput delegates to [labeldata.ProjectOutput] so the package shares
// the same [Output] shape with the [labeldata] sub-package, and passes on
// the field the capture read beside the SDK.
func toOutput(label *gl.Label, extra toolutil.LabelExtra) Output {
	return labeldata.ProjectOutput(label, extra)
}

// capturedOutput converts one label with what its captured answer carries
// beside the SDK's decode, or reports the answer the type cannot hold.
func capturedOutput(op string, label *gl.Label, captured *gitlabclient.ResponseCapture) (Output, error) {
	extra, err := toolutil.CapturedLabel(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr(op, err)
	}
	return toOutput(label, extra), nil
}
