package accessrequests

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// Output represents a single access request: what [gl.AccessRequest] decodes,
// plus what lib/api/entities/access_requester.rb sends that the SDK does not
// carry, read from the captured response (ADR-0021). The entity inherits
// Member and merges UserBasic into it, and the keys are in that order.
//
// public_email, locked, avatar_url, web_url, expires_at and membership_state
// are on every access request; created_by, the two identities, email,
// override and member_role wait on a condition and are absent otherwise.
//
// The entity can also send avatar_path, custom_attributes and is_using_seat,
// and none of them is published: each waits on a presenter option, and no
// access-request route declares only_path, with_custom_attributes or
// show_seat_info, so publishing them would advertise keys this endpoint
// cannot return.
type Output struct {
	toolutil.HintableOutput
	ID                int64                        `json:"id"`
	Username          string                       `json:"username"`
	Name              string                       `json:"name"`
	PublicEmail       string                       `json:"public_email,omitempty"`
	State             string                       `json:"state"`
	Locked            bool                         `json:"locked"`
	AvatarURL         string                       `json:"avatar_url,omitempty"`
	WebURL            string                       `json:"web_url,omitempty"`
	AccessLevel       int                          `json:"access_level"`
	CreatedAt         string                       `json:"created_at,omitempty"`
	CreatedBy         *toolutil.MemberUserOutput   `json:"created_by,omitempty"`
	ExpiresAt         string                       `json:"expires_at,omitempty"`
	GroupSAMLIdentity *toolutil.SAMLIdentityOutput `json:"group_saml_identity,omitempty" tier:"premium"`
	GroupSCIMIdentity *toolutil.SCIMIdentityOutput `json:"group_scim_identity,omitempty" tier:"premium"`
	Email             string                       `json:"email,omitempty"`
	Override          *bool                        `json:"override,omitempty" tier:"premium"`
	MembershipState   string                       `json:"membership_state,omitempty" tier:"premium"`
	MemberRole        *toolutil.MemberRoleOutput   `json:"member_role,omitempty" tier:"ultimate"`
	RequestedAt       string                       `json:"requested_at,omitempty"`
}

// ListOutput represents a paginated list of access requests.
type ListOutput struct {
	toolutil.HintableOutput
	AccessRequests []Output                  `json:"access_requests"`
	Pagination     toolutil.PaginationOutput `json:"pagination"`
}

// buildListOptions assembles ListAccessRequestsOptions from offset, keyset, and
// ordering inputs shared by the project and group access-request list handlers.
func buildListOptions(page toolutil.PaginationInput, keyset toolutil.KeysetPaginationInput, orderBy, sort string) *gl.ListAccessRequestsOptions {
	opts := &gl.ListAccessRequestsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, page, keyset)
	if orderBy != "" {
		opts.OrderBy = orderBy
	}
	if sort != "" {
		opts.Sort = sort
	}
	return opts
}

// convertAccessRequest maps a GitLab access request into the MCP output shape,
// filling from the decoded request and from what the capture read beside it.
func convertAccessRequest(ar *gl.AccessRequest, extra toolutil.AccessRequestExtra) Output {
	o := Output{
		ID:                ar.ID,
		Username:          ar.Username,
		Name:              ar.Name,
		PublicEmail:       extra.PublicEmail,
		State:             ar.State,
		Locked:            extra.Locked,
		AvatarURL:         extra.AvatarURL,
		WebURL:            extra.WebURL,
		AccessLevel:       int(ar.AccessLevel),
		CreatedBy:         extra.CreatedBy,
		ExpiresAt:         extra.ExpiresAt,
		GroupSAMLIdentity: extra.GroupSAMLIdentity,
		GroupSCIMIdentity: extra.GroupSCIMIdentity,
		Email:             extra.Email,
		Override:          extra.Override,
		MembershipState:   extra.MembershipState,
		MemberRole:        extra.MemberRole,
	}
	if ar.CreatedAt != nil {
		o.CreatedAt = ar.CreatedAt.Format(time.RFC3339)
	}
	if ar.RequestedAt != nil {
		o.RequestedAt = ar.RequestedAt.Format(time.RFC3339)
	}
	return o
}

// capturedAccessRequest converts one access request with what its captured
// answer carries beside the SDK's decode, or reports under op the answer the
// type cannot hold. It is the whole tail of a handler that returns one
// request, which is four of the six here.
func capturedAccessRequest(op string, ar *gl.AccessRequest, capture *gitlabclient.ResponseCapture) (Output, error) {
	extra, err := toolutil.CapturedAccessRequest(capture)
	if err != nil {
		return Output{}, toolutil.WrapErr(op, err)
	}
	return convertAccessRequest(ar, extra), nil
}

// capturedAccessRequests does the same for a list, pairing each request with
// the extra read at the same position.
func capturedAccessRequests(op string, requests []*gl.AccessRequest, capture *gitlabclient.ResponseCapture) ([]Output, error) {
	extras, err := toolutil.CapturedAccessRequests(capture, len(requests))
	if err != nil {
		return nil, toolutil.WrapErr(op, err)
	}
	// Left nil for an empty page rather than an empty slice, which is the
	// answer the list handlers gave before the capture was threaded through
	// them and what the pagination fields already distinguish.
	var out []Output
	for i, ar := range requests {
		out = append(out, convertAccessRequest(ar, extras[i]))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// ListProjectAccessRequests
// ---------------------------------------------------------------------------.

// ListProjectInput selects a project and pagination window for pending access requests.
type ListProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order results by for keyset-paginated result sets (e.g. id)"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort order: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListProject lists access requests for a project.
func ListProject(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := buildListOptions(input.PaginationInput, input.KeysetPaginationInput, input.OrderBy, input.Sort)
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	requests, resp, err := client.GL().AccessRequests.ListProjectAccessRequests(
		string(input.ProjectID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("access_request_list_project", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; listing access requests requires Maintainer role or higher")
	}
	items, err := capturedAccessRequests("access_request_list_project", requests, captured)
	if err != nil {
		return ListOutput{}, err
	}
	return ListOutput{AccessRequests: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// ListGroupAccessRequests
// ---------------------------------------------------------------------------.

// ListGroupInput selects a group and pagination window for pending access requests.
type ListGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Column to order results by for keyset-paginated result sets (e.g. id)"`
	Sort    string               `json:"sort,omitempty" jsonschema:"Sort order: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListGroup lists access requests for a group.
func ListGroup(ctx context.Context, client *gitlabclient.Client, input ListGroupInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := buildListOptions(input.PaginationInput, input.KeysetPaginationInput, input.OrderBy, input.Sort)
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	requests, resp, err := client.GL().AccessRequests.ListGroupAccessRequests(
		string(input.GroupID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("access_request_list_group", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get; listing access requests requires Owner role")
	}
	items, err := capturedAccessRequests("access_request_list_group", requests, captured)
	if err != nil {
		return ListOutput{}, err
	}
	return ListOutput{AccessRequests: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// RequestProjectAccess
// ---------------------------------------------------------------------------.

// RequestProjectInput identifies the project the authenticated user wants to join.
type RequestProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
}

// RequestProject requests access to a project for the authenticated user.
func RequestProject(ctx context.Context, client *gitlabclient.Client, input RequestProjectInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	ar, _, err := client.GL().AccessRequests.RequestProjectAccess(
		string(input.ProjectID), gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("access_request_request_project", err, http.StatusConflict,
			"the authenticated user may already be a member or have a pending request; check gitlab_project_member_get; project must allow access requests in its settings")
	}
	return capturedAccessRequest("access_request_request_project", ar, captured)
}

// ---------------------------------------------------------------------------
// RequestGroupAccess
// ---------------------------------------------------------------------------.

// RequestGroupInput identifies the group the authenticated user wants to join.
type RequestGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
}

// RequestGroup requests access to a group for the authenticated user.
func RequestGroup(ctx context.Context, client *gitlabclient.Client, input RequestGroupInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	ar, _, err := client.GL().AccessRequests.RequestGroupAccess(
		string(input.GroupID), gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("access_request_request_group", err, http.StatusConflict,
			"the authenticated user may already be a member or have a pending request; group must allow access requests (Owner-controlled setting)")
	}
	return capturedAccessRequest("access_request_request_group", ar, captured)
}

// ---------------------------------------------------------------------------
// ApproveProjectAccessRequest
// ---------------------------------------------------------------------------.

// ApproveProjectInput identifies the project access request to approve and the optional granted role.
type ApproveProjectInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	UserID      int64                `json:"user_id" jsonschema:"User ID of the access requester,required"`
	AccessLevel int                  `json:"access_level,omitempty" jsonschema:"Access level to grant (0=No access, 5=Minimal access, 10=Guest, 15=Planner (Premium), 20=Reporter, 25=Security Manager (Premium), 30=Developer, 40=Maintainer, 50=Owner). Default 30"`
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	ar, _, err := client.GL().AccessRequests.ApproveProjectAccessRequest(
		string(input.ProjectID), input.UserID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("access_request_approve_project", err, http.StatusNotFound,
			"verify user_id with gitlab_access_request_list_project; access_level must be valid (5=Minimal access, 10=Guest, 15=Planner (Premium), 20=Reporter, 25=Security Manager (Premium), 30=Developer, 40=Maintainer); approving requires Maintainer role")
	}
	return capturedAccessRequest("access_request_approve_project", ar, captured)
}

// ---------------------------------------------------------------------------
// ApproveGroupAccessRequest
// ---------------------------------------------------------------------------.

// ApproveGroupInput identifies the group access request to approve and the optional granted role.
type ApproveGroupInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	UserID      int64                `json:"user_id" jsonschema:"User ID of the access requester,required"`
	AccessLevel int                  `json:"access_level,omitempty" jsonschema:"Access level to grant (0=No access, 5=Minimal access, 10=Guest, 15=Planner (Premium), 20=Reporter, 25=Security Manager (Premium), 30=Developer, 40=Maintainer, 50=Owner). Default 30"`
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	ar, _, err := client.GL().AccessRequests.ApproveGroupAccessRequest(
		string(input.GroupID), input.UserID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("access_request_approve_group", err, http.StatusNotFound,
			"verify user_id with gitlab_access_request_list_group; access_level must be valid (5/10/15/20/25/30/40/50; 60=Admin not valid for access requests); approving requires Owner role")
	}
	return capturedAccessRequest("access_request_approve_group", ar, captured)
}

// ---------------------------------------------------------------------------
// DenyProjectAccessRequest
// ---------------------------------------------------------------------------.

// DenyProjectInput identifies the pending project access request to reject.
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
		string(input.ProjectID), input.UserID, gl.WithContext(ctx),
	)
	if err != nil {
		return toolutil.WrapErrWithStatusHint("access_request_deny_project", err, http.StatusNotFound,
			"verify user_id with gitlab_access_request_list_project; denying requires Maintainer role; the request must still be pending")
	}
	return nil
}

// ---------------------------------------------------------------------------
// DenyGroupAccessRequest
// ---------------------------------------------------------------------------.

// DenyGroupInput identifies the pending group access request to reject.
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
		string(input.GroupID), input.UserID, gl.WithContext(ctx),
	)
	if err != nil {
		return toolutil.WrapErrWithStatusHint("access_request_deny_group", err, http.StatusNotFound,
			"verify user_id with gitlab_access_request_list_group; denying requires Owner role; the request must still be pending")
	}
	return nil
}
