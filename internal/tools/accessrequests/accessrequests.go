// Package accessrequests implements MCP tools for GitLab project and group
// access request operations using the AccessRequestsService API.
package accessrequests

import (
	"context"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// Output represents a single access request.
type Output struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	State       string `json:"state"`
	CreatedAt   string `json:"created_at,omitempty"`
	RequestedAt string `json:"requested_at,omitempty"`
	AccessLevel int    `json:"access_level"`
}

// ListOutput represents a paginated list of access requests.
type ListOutput struct {
	toolutil.HintableOutput
	AccessRequests []Output                  `json:"access_requests"`
	Pagination     toolutil.PaginationOutput `json:"pagination"`
}

// convertAccessRequest is an internal helper for the accessrequests package.
func convertAccessRequest(ar *gl.AccessRequest) Output {
	o := Output{
		ID:          ar.ID,
		Username:    ar.Username,
		Name:        ar.Name,
		State:       ar.State,
		AccessLevel: int(ar.AccessLevel),
	}
	if ar.CreatedAt != nil {
		o.CreatedAt = ar.CreatedAt.Format(time.RFC3339)
	}
	if ar.RequestedAt != nil {
		o.RequestedAt = ar.RequestedAt.Format(time.RFC3339)
	}
	return o
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// ListProjectAccessRequests
// ---------------------------------------------------------------------------.

// ListProjectInput represents the input for listing project access requests.
type ListProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Page      int                  `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage   int                  `json:"per_page,omitempty" jsonschema:"Number of items per page"`
}

// ListProject lists access requests for a project.
func ListProject(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := &gl.ListAccessRequestsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	requests, resp, err := client.GL().AccessRequests.ListProjectAccessRequests(
		string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("access_request_list_project", err)
	}
	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, ar := range requests {
		out.AccessRequests = append(out.AccessRequests, convertAccessRequest(ar))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// ListGroupAccessRequests
// ---------------------------------------------------------------------------.

// ListGroupInput represents the input for listing group access requests.
type ListGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	Page    int                  `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int                  `json:"per_page,omitempty" jsonschema:"Number of items per page"`
}

// ListGroup lists access requests for a group.
func ListGroup(ctx context.Context, client *gitlabclient.Client, input ListGroupInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListAccessRequestsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	requests, resp, err := client.GL().AccessRequests.ListGroupAccessRequests(
		string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("access_request_list_group", err)
	}
	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, ar := range requests {
		out.AccessRequests = append(out.AccessRequests, convertAccessRequest(ar))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// RequestProjectAccess
// ---------------------------------------------------------------------------.

// RequestProjectInput represents the input for requesting access to a project.
type RequestProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
}

// RequestProject requests access to a project for the authenticated user.
func RequestProject(ctx context.Context, client *gitlabclient.Client, input RequestProjectInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	ar, _, err := client.GL().AccessRequests.RequestProjectAccess(
		string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("access_request_request_project", err)
	}
	return convertAccessRequest(ar), nil
}

// ---------------------------------------------------------------------------
// RequestGroupAccess
// ---------------------------------------------------------------------------.

// RequestGroupInput represents the input for requesting access to a group.
type RequestGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
}

// RequestGroup requests access to a group for the authenticated user.
func RequestGroup(ctx context.Context, client *gitlabclient.Client, input RequestGroupInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	ar, _, err := client.GL().AccessRequests.RequestGroupAccess(
		string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("access_request_request_group", err)
	}
	return convertAccessRequest(ar), nil
}

// ---------------------------------------------------------------------------
// ApproveProjectAccessRequest
// ---------------------------------------------------------------------------.

// ApproveProjectInput represents the input for approving a project access request.
type ApproveProjectInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	UserID      int64                `json:"user_id" jsonschema:"User ID of the access requester,required"`
	AccessLevel int                  `json:"access_level,omitempty" jsonschema:"Access level to grant (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer)"`
}

// ApproveProject approves a project access request.
func ApproveProject(ctx context.Context, client *gitlabclient.Client, input ApproveProjectInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.UserID == 0 {
		return Output{}, toolutil.ErrFieldRequired("user_id")
	}
	opts := &gl.ApproveAccessRequestOptions{}
	if input.AccessLevel != 0 {
		lvl := gl.AccessLevelValue(input.AccessLevel)
		opts.AccessLevel = &lvl
	}
	ar, _, err := client.GL().AccessRequests.ApproveProjectAccessRequest(
		string(input.ProjectID), input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("access_request_approve_project", err)
	}
	return convertAccessRequest(ar), nil
}

// ---------------------------------------------------------------------------
// ApproveGroupAccessRequest
// ---------------------------------------------------------------------------.

// ApproveGroupInput represents the input for approving a group access request.
type ApproveGroupInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	UserID      int64                `json:"user_id" jsonschema:"User ID of the access requester,required"`
	AccessLevel int                  `json:"access_level,omitempty" jsonschema:"Access level to grant (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer)"`
}

// ApproveGroup approves a group access request.
func ApproveGroup(ctx context.Context, client *gitlabclient.Client, input ApproveGroupInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.UserID == 0 {
		return Output{}, toolutil.ErrFieldRequired("user_id")
	}
	opts := &gl.ApproveAccessRequestOptions{}
	if input.AccessLevel != 0 {
		lvl := gl.AccessLevelValue(input.AccessLevel)
		opts.AccessLevel = &lvl
	}
	ar, _, err := client.GL().AccessRequests.ApproveGroupAccessRequest(
		string(input.GroupID), input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("access_request_approve_group", err)
	}
	return convertAccessRequest(ar), nil
}

// ---------------------------------------------------------------------------
// DenyProjectAccessRequest
// ---------------------------------------------------------------------------.

// DenyProjectInput represents the input for denying a project access request.
type DenyProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	UserID    int64                `json:"user_id" jsonschema:"User ID of the access requester,required"`
}

// DenyProject denies a project access request.
func DenyProject(ctx context.Context, client *gitlabclient.Client, input DenyProjectInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.UserID == 0 {
		return toolutil.ErrFieldRequired("user_id")
	}
	_, err := client.GL().AccessRequests.DenyProjectAccessRequest(
		string(input.ProjectID), input.UserID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("access_request_deny_project", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// DenyGroupAccessRequest
// ---------------------------------------------------------------------------.

// DenyGroupInput represents the input for denying a group access request.
type DenyGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	UserID  int64                `json:"user_id" jsonschema:"User ID of the access requester,required"`
}

// DenyGroup denies a group access request.
func DenyGroup(ctx context.Context, client *gitlabclient.Client, input DenyGroupInput) error {
	if input.GroupID == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if input.UserID == 0 {
		return toolutil.ErrFieldRequired("user_id")
	}
	_, err := client.GL().AccessRequests.DenyGroupAccessRequest(
		string(input.GroupID), input.UserID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("access_request_deny_group", err)
	}
	return nil
}
