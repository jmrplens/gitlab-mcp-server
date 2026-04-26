// Package ffuserlists provides MCP tool handlers for GitLab feature flag user list operations.
package ffuserlists

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ──────────────────────────────────────────────
// Output types
// ──────────────────────────────────────────────.

// Output represents a single feature flag user list.
type Output struct {
	toolutil.HintableOutput
	ID        int64  `json:"id"`
	IID       int64  `json:"iid"`
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	UserXIDs  string `json:"user_xids"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// ListOutput represents a paginated list of feature flag user lists.
type ListOutput struct {
	toolutil.HintableOutput
	UserLists  []Output                  `json:"user_lists"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ──────────────────────────────────────────────
// Input types
// ──────────────────────────────────────────────.

// ListInput contains parameters for listing feature flag user lists.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Search    string               `json:"search,omitempty" jsonschema:"Search by name"`
	toolutil.PaginationInput
}

// GetInput contains parameters for getting a feature flag user list.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	IID       int64                `json:"iid" jsonschema:"Feature flag user list internal ID,required"`
}

// CreateInput contains parameters for creating a feature flag user list.
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Name      string               `json:"name" jsonschema:"User list name,required"`
	UserXIDs  string               `json:"user_xids" jsonschema:"Comma-separated list of user external IDs,required"`
}

// UpdateInput contains parameters for updating a feature flag user list.
type UpdateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	IID       int64                `json:"iid" jsonschema:"Feature flag user list internal ID,required"`
	Name      string               `json:"name,omitempty" jsonschema:"New user list name"`
	UserXIDs  string               `json:"user_xids,omitempty" jsonschema:"Comma-separated list of user external IDs"`
}

// DeleteInput contains parameters for deleting a feature flag user list.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	IID       int64                `json:"iid" jsonschema:"Feature flag user list internal ID,required"`
}

// ──────────────────────────────────────────────
// Handlers
// ──────────────────────────────────────────────.

// ListUserLists lists feature flag user lists for a project.
func ListUserLists(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.WrapErrWithMessage("ff_user_list_list", toolutil.ErrFieldRequired("project_id"))
	}
	opts := &gl.ListFeatureFlagUserListsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	if input.Search != "" {
		opts.Search = input.Search
	}
	lists, resp, err := client.GL().FeatureFlagUserLists.ListFeatureFlagUserLists(
		string(input.ProjectID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return ListOutput{}, toolutil.WrapErrWithHint("ff_user_list_list", err,
				"feature flag user lists require GitLab Premium/Ultimate \u2014 verify the project tier and that you have Developer+ role")
		}
		return ListOutput{}, toolutil.WrapErrWithStatusHint("ff_user_list_list", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get")
	}
	out := ListOutput{
		UserLists:  make([]Output, 0, len(lists)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, l := range lists {
		out.UserLists = append(out.UserLists, convertUserList(l))
	}
	return out, nil
}

// GetUserList gets a single feature flag user list by IID.
func GetUserList(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("ff_user_list_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID == 0 {
		return Output{}, toolutil.WrapErrWithMessage("ff_user_list_get", toolutil.ErrFieldRequired("iid"))
	}
	l, _, err := client.GL().FeatureFlagUserLists.GetFeatureFlagUserList(
		string(input.ProjectID), input.IID, gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("ff_user_list_get", err, http.StatusNotFound,
			"verify iid with gitlab_ff_user_list_list \u2014 user lists are scoped per-project and require Premium/Ultimate")
	}
	return convertUserList(l), nil
}

// CreateUserList creates a new feature flag user list.
func CreateUserList(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("ff_user_list_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.Name == "" {
		return Output{}, toolutil.WrapErrWithMessage("ff_user_list_create", toolutil.ErrFieldRequired("name"))
	}
	opts := &gl.CreateFeatureFlagUserListOptions{
		Name:     input.Name,
		UserXIDs: input.UserXIDs,
	}
	l, _, err := client.GL().FeatureFlagUserLists.CreateFeatureFlagUserList(
		string(input.ProjectID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("ff_user_list_create", err,
				"creating feature flag user lists requires GitLab Premium/Ultimate and Developer+ role")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("ff_user_list_create", err,
				"name must be unique within the project; user_xids must be a comma-separated list of external user IDs")
		}
		return Output{}, toolutil.WrapErrWithMessage("ff_user_list_create", err)
	}
	return convertUserList(l), nil
}

// UpdateUserList updates an existing feature flag user list.
func UpdateUserList(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.WrapErrWithMessage("ff_user_list_update", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID == 0 {
		return Output{}, toolutil.WrapErrWithMessage("ff_user_list_update", toolutil.ErrFieldRequired("iid"))
	}
	opts := &gl.UpdateFeatureFlagUserListOptions{
		Name:     input.Name,
		UserXIDs: input.UserXIDs,
	}
	l, _, err := client.GL().FeatureFlagUserLists.UpdateFeatureFlagUserList(
		string(input.ProjectID), input.IID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("ff_user_list_update", err,
				"updating feature flag user lists requires Developer+ role on a Premium/Ultimate project")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("ff_user_list_update", err, http.StatusNotFound,
			"verify iid with gitlab_ff_user_list_list")
	}
	return convertUserList(l), nil
}

// DeleteUserList deletes a feature flag user list.
func DeleteUserList(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("ff_user_list_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.IID == 0 {
		return toolutil.WrapErrWithMessage("ff_user_list_delete", toolutil.ErrFieldRequired("iid"))
	}
	_, err := client.GL().FeatureFlagUserLists.DeleteFeatureFlagUserList(
		string(input.ProjectID), input.IID, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("ff_user_list_delete", err,
				"deleting feature flag user lists requires Developer+ role; the list cannot be in use by an enabled feature flag strategy")
		}
		return toolutil.WrapErrWithStatusHint("ff_user_list_delete", err, http.StatusNotFound,
			"verify iid with gitlab_ff_user_list_list")
	}
	return nil
}

// ──────────────────────────────────────────────
// Converter
// ──────────────────────────────────────────────.

// convertUserList is an internal helper for the ffuserlists package.
func convertUserList(l *gl.FeatureFlagUserList) Output {
	out := Output{
		ID:        l.ID,
		IID:       l.IID,
		ProjectID: l.ProjectID,
		Name:      l.Name,
		UserXIDs:  l.UserXIDs,
	}
	if l.CreatedAt != nil {
		out.CreatedAt = l.CreatedAt.Format(time.RFC3339)
	}
	if l.UpdatedAt != nil {
		out.UpdatedAt = l.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// ──────────────────────────────────────────────
// Markdown formatters
// ──────────────────────────────────────────────.
