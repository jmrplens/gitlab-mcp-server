// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package accessrequests

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// Output represents a single pending access request: what [gl.AccessRequest]
// decodes of it, plus what lib/api/entities/access_requester.rb sends that the
// SDK does not carry, read from the captured response (ADR-0021).
//
// That entity is short, and the shortness is the point. It exposes `user`,
// merged from UserBasic, and `requested_at`, and nothing else: a pending
// request is a person asking, not a membership, so there is no access level to
// report, no expiry, no role, no membership state and no SAML or SCIM
// identity. GitLab answers with a membership only once the request is
// approved, which is [MemberOutput].
//
// This type carried all eleven Member keys until the entity was read rather
// than assumed. They reached a caller empty on every list and every request,
// and `access_level` reached one as a real-looking zero, which the list table
// rendered as "No access (0)" for every row: a state the request was not in,
// published from a key GitLab had not sent.
//
// public_email, locked, avatar_url and web_url are the UserBasic keys
// client-go leaves out, and all four are on every access request. The same
// UserBasic can send avatar_path and custom_attributes, and neither is
// published: both wait on a presenter option, and no access-request route
// declares only_path or with_custom_attributes, so publishing them would
// advertise keys this endpoint cannot return.
type Output struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	PublicEmail string `json:"public_email,omitempty"`
	State       string `json:"state"`
	Locked      bool   `json:"locked"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	WebURL      string `json:"web_url,omitempty"`
	RequestedAt string `json:"requested_at,omitempty"`
}

// MemberOutput represents the membership an approved access request became:
// what [gl.AccessRequest] decodes, plus what lib/api/entities/member.rb sends
// beside it, read from the captured response (ADR-0021).
//
// Approving is the one route of this family GitLab answers with Member instead
// of AccessRequester, which is why it is the one shape here with an access
// level in it. created_at, expires_at and membership_state are on every
// member; created_by, the two identities, email, override and member_role wait
// on a condition and are absent otherwise.
//
// The entity can also send avatar_path, custom_attributes and is_using_seat,
// and none of them is published: each waits on a presenter option, and the
// approve route declares neither only_path, with_custom_attributes nor
// show_seat_info.
type MemberOutput struct {
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
}

// ListOutput represents a paginated list of access requests.
type ListOutput struct {
	toolutil.HintableOutput
	AccessRequests []Output                  `json:"access_requests"`
	Pagination     toolutil.PaginationOutput `json:"pagination"`
}

// buildListOptions assembles ListAccessRequestsOptions from the offset and
// ordering inputs shared by the project and group access-request list
// handlers. Both routes page by offset alone.
func buildListOptions(page toolutil.PaginationInput, orderBy, sort string) *gl.ListAccessRequestsOptions {
	opts := &gl.ListAccessRequestsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, page, toolutil.KeysetPaginationInput{})
	if orderBy != "" {
		opts.OrderBy = orderBy
	}
	if sort != "" {
		opts.Sort = sort
	}
	return opts
}

// convertAccessRequester maps a pending access request into the MCP output
// shape, filling from the decoded request and from what the capture read
// beside it.
//
// ar.AccessLevel is deliberately not read. client-go's AccessRequest models it
// because approving answers with a member, and on a pending request GitLab
// sends no such key, so it decodes to zero and publishing it would report
// "no access" as though it were the state of the request.
func convertAccessRequester(ar *gl.AccessRequest, extra toolutil.AccessRequesterExtra) Output {
	o := Output{
		ID:          ar.ID,
		Username:    ar.Username,
		Name:        ar.Name,
		PublicEmail: extra.PublicEmail,
		State:       ar.State,
		Locked:      extra.Locked,
		AvatarURL:   extra.AvatarURL,
		WebURL:      extra.WebURL,
	}
	if ar.RequestedAt != nil {
		o.RequestedAt = ar.RequestedAt.Format(time.RFC3339)
	}
	return o
}

// convertApprovedMember maps the membership an approved request became, which
// is the one answer of this family that carries a membership at all.
func convertApprovedMember(ar *gl.AccessRequest, extra toolutil.ApprovedMemberExtra) MemberOutput {
	o := MemberOutput{
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
	return o
}

// capturedAccessRequester converts one pending access request with what its
// captured answer carries beside the SDK's decode, or reports under op the
// answer the type cannot hold. It is the whole tail of the two handlers that
// return one pending request.
func capturedAccessRequester(op string, ar *gl.AccessRequest, capture *gitlabclient.ResponseCapture) (Output, error) {
	extra, err := toolutil.CapturedAccessRequester(capture)
	if err != nil {
		return Output{}, toolutil.WrapErr(op, err)
	}
	return convertAccessRequester(ar, extra), nil
}

// capturedApprovedMember does the same for the two handlers that approve one.
func capturedApprovedMember(op string, ar *gl.AccessRequest, capture *gitlabclient.ResponseCapture) (MemberOutput, error) {
	extra, err := toolutil.CapturedApprovedMember(capture)
	if err != nil {
		return MemberOutput{}, toolutil.WrapErr(op, err)
	}
	return convertApprovedMember(ar, extra), nil
}

// capturedAccessRequesters does the same for a list, pairing each request with
// the extra read at the same position.
func capturedAccessRequesters(op string, requests []*gl.AccessRequest, capture *gitlabclient.ResponseCapture) ([]Output, error) {
	extras, err := toolutil.CapturedAccessRequesters(capture, len(requests))
	if err != nil {
		return nil, toolutil.WrapErr(op, err)
	}
	// Left nil for an empty page rather than an empty slice, which is the
	// answer the list handlers gave before the capture was threaded through
	// them and what the pagination fields already distinguish.
	var out []Output
	for i, ar := range requests {
		out = append(out, convertAccessRequester(ar, extras[i]))
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
}

// ListProject lists access requests for a project.
func ListProject(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := buildListOptions(input.PaginationInput, input.OrderBy, input.Sort)
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	requests, resp, err := client.GL().AccessRequests.ListProjectAccessRequests(
		string(input.ProjectID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("access_request_list_project", err, http.StatusNotFound,
			"verify project_id with project.get; listing access requests requires Maintainer role or higher")
	}
	items, err := capturedAccessRequesters("access_request_list_project", requests, captured)
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
}

// ListGroup lists access requests for a group.
func ListGroup(ctx context.Context, client *gitlabclient.Client, input ListGroupInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := buildListOptions(input.PaginationInput, input.OrderBy, input.Sort)
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	requests, resp, err := client.GL().AccessRequests.ListGroupAccessRequests(
		string(input.GroupID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("access_request_list_group", err, http.StatusNotFound,
			"verify group_id with group.get; listing access requests requires Owner role")
	}
	items, err := capturedAccessRequesters("access_request_list_group", requests, captured)
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
			"the authenticated user may already be a member or have a pending request; check project.member_get; project must allow access requests in its settings")
	}
	return capturedAccessRequester("access_request_request_project", ar, captured)
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
	return capturedAccessRequester("access_request_request_group", ar, captured)
}

// ---------------------------------------------------------------------------
// ApproveProjectAccessRequest
// ---------------------------------------------------------------------------.

// The approval handlers' operation labels, which open each error they return.
const (
	opApproveProject = "access_request_approve_project"
	opApproveGroup   = "access_request_approve_group"
)

// ApproveProjectInput identifies the project access request to approve and the optional granted role.
//
// AccessLevel is a pointer because leaving it out and sending 0 are two
// different calls. Left out, GitLab grants its default, Developer; 0 used to
// be dropped as though it had been left out, so a caller asking for No access
// got a Developer, and sent it would be answered as an approval that GitLab
// never saved. It is refused instead, with every other level a project does
// not grant (see [refuseUngrantedLevel]).
type ApproveProjectInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	UserID      int64                `json:"user_id" jsonschema:"User ID of the access requester,required"`
	AccessLevel *int                 `json:"access_level,omitempty" jsonschema:"Access level to grant (10=Guest, 15=Planner, 20=Reporter, 25=Security Manager, 30=Developer, 40=Maintainer, 50=Owner). Omit it to grant Developer, GitLab's default. A project grants no other level"`
}

// ApproveProject approves a project access request.
func ApproveProject(ctx context.Context, client *gitlabclient.Client, input ApproveProjectInput) (MemberOutput, error) {
	if input.ProjectID == "" {
		return MemberOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.UserID == 0 {
		return MemberOutput{}, toolutil.ErrFieldRequired("user_id")
	}
	if err := refuseUngrantedLevel(opApproveProject, input.AccessLevel, toolutil.ProjectAccessLevels); err != nil {
		return MemberOutput{}, err
	}
	opts := &gl.ApproveAccessRequestOptions{}
	if input.AccessLevel != nil {
		opts.AccessLevel = new(gl.AccessLevelValue(*input.AccessLevel))
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	ar, _, err := client.GL().AccessRequests.ApproveProjectAccessRequest(
		string(input.ProjectID), input.UserID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		return MemberOutput{}, toolutil.WrapErrWithStatusHint(opApproveProject, err, http.StatusNotFound,
			"verify user_id with access.request_list_project; approving requires the Maintainer role")
	}
	return capturedApprovedMember(opApproveProject, ar, captured)
}

// ---------------------------------------------------------------------------
// ApproveGroupAccessRequest
// ---------------------------------------------------------------------------.

// ApproveGroupInput identifies the group access request to approve and the optional granted role.
//
// AccessLevel is a pointer for the reason [ApproveProjectInput] gives.
type ApproveGroupInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	UserID      int64                `json:"user_id" jsonschema:"User ID of the access requester,required"`
	AccessLevel *int                 `json:"access_level,omitempty" jsonschema:"Access level to grant (5=Minimal access (only on a top-level group with a Premium or Ultimate license), 10=Guest, 15=Planner, 20=Reporter, 25=Security Manager, 30=Developer, 40=Maintainer, 50=Owner). Omit it to grant Developer, GitLab's default. Minimal access is refused on a subgroup and on the Free tier, and reported as an approval only once the requester is found among the group's members"`
}

// ApproveGroup approves a group access request.
//
// Minimal access is the one level a group grants in some places and not in
// others, and the approval route cannot say which happened: where GitLab does
// not grant it, the route still answers 201 with the requester, and the
// request stays pending. So it costs up to two reads beside the approval. The
// group is read before it, and a subgroup is refused, as the Free tier is
// without a request; after it, the requester is read among the group's
// members, and an approval that left them out is answered as the pending
// request it is (see [confirmMinimalAccess]).
func ApproveGroup(ctx context.Context, client *gitlabclient.Client, input ApproveGroupInput) (MemberOutput, error) {
	if input.GroupID == "" {
		return MemberOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.UserID == 0 {
		return MemberOutput{}, toolutil.ErrFieldRequired("user_id")
	}
	if err := refuseUngrantedLevel(opApproveGroup, input.AccessLevel, toolutil.GroupAccessLevels); err != nil {
		return MemberOutput{}, err
	}
	opts := &gl.ApproveAccessRequestOptions{}
	if input.AccessLevel != nil {
		opts.AccessLevel = new(gl.AccessLevelValue(*input.AccessLevel))
	}
	minimal := opts.AccessLevel != nil && *opts.AccessLevel == gl.MinimalAccessPermissions
	if minimal {
		if refusal := refuseMinimalAccessGroup(ctx, client, input.GroupID); refusal != nil {
			return MemberOutput{}, refusal
		}
	}
	// The capture holds the approval's own answer, so the member read below
	// goes out under the caller's context: a capture keeps the last answer
	// it sees, and the read's would replace the one the SDK decoded.
	approvalCtx, captured := gitlabclient.WithResponseCapture(ctx)
	ar, _, err := client.GL().AccessRequests.ApproveGroupAccessRequest(
		string(input.GroupID), input.UserID, opts, gl.WithContext(approvalCtx),
	)
	if err != nil {
		return MemberOutput{}, toolutil.WrapErrWithStatusHint(opApproveGroup, err, http.StatusNotFound,
			"verify user_id with access.request_list_group; approving requires the Owner role")
	}
	if minimal {
		if unconfirmed := confirmMinimalAccess(ctx, client, input.GroupID, input.UserID); unconfirmed != nil {
			return MemberOutput{}, unconfirmed
		}
	}
	return capturedApprovedMember(opApproveGroup, ar, captured)
}

// refuseUngrantedLevel refuses under op, before any request, an approval
// naming a level the scope does not grant, and lets one naming no level
// through.
//
// Such a level is not refused by GitLab. The approval service sets it on the
// request and saves, the save fails the member's validation, and the route
// presents the unsaved record with 201, which reads like a membership while
// the request stays pending. 0 is one such level on both scopes, 5 on a
// project and 60 everywhere.
//
// The handlers build the SDK options themselves, from their own input, so
// the 1:1 audit pairs each input with ApproveAccessRequestOptions and judges
// the level it offers against client-go's.
func refuseUngrantedLevel(op string, level *int, accepted toolutil.AccessLevelSet) error {
	if level == nil || accepted.Accepts(*level) {
		return nil
	}
	return fmt.Errorf("%s: access_level %d is not a level GitLab grants here, and it would answer the approval as done while the request stays pending. Use one of %s, or omit access_level to grant Developer (30), GitLab's default", op, *level, accepted.Choices())
}

// minimalAccessRetry is what a refusal or a pending answer about Minimal
// access tells the caller to do instead.
const minimalAccessRetry = "Approve with 10 (Guest) or higher, or omit access_level to grant Developer (30), GitLab's default"

// refuseMinimalAccessGroup refuses a Minimal access approval GitLab could not
// save: on the Free tier, which has no Minimal Access role, before any
// request, and on a subgroup, which the group's parent_id tells, before the
// approval.
func refuseMinimalAccessGroup(ctx context.Context, client *gitlabclient.Client, groupID toolutil.StringOrInt) error {
	if !client.IsEnterprise() {
		return fmt.Errorf("%s: access_level 5 (Minimal access) is granted only on a top-level group with a Premium or Ultimate license, and this session serves the Free tier, so GitLab would leave the request pending. %s", opApproveGroup, minimalAccessRetry)
	}
	//gitlab:request optional: the group is read only for a Minimal access approval, to refuse it on a subgroup before the approval is sent
	group, _, err := client.GL().Groups.GetGroup(string(groupID), &gl.GetGroupOptions{
		WithProjects: new(false), //nolint:staticcheck // SA1019: parent_id is all this read needs, and without it GitLab answers with up to 100 projects as well
	}, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint(opApproveGroup, err, http.StatusNotFound, "verify group_id with group.get")
	}
	if group.ParentID != 0 {
		return fmt.Errorf("%s: group %s is a subgroup, and GitLab grants access_level 5 (Minimal access) only on a top-level group, so it would leave the request pending. %s", opApproveGroup, groupID, minimalAccessRetry)
	}
	return nil
}

// confirmMinimalAccess reads the requester back among the group's members
// after a Minimal access approval, which is the only way to tell an approval
// GitLab saved from one it answered without saving: on a top-level group
// whose license does not include the role, GitLab.com's free groups among
// them while the session's tier is the best plan the credential reaches.
// A requester who is not found is still pending, and is answered so.
func confirmMinimalAccess(ctx context.Context, client *gitlabclient.Client, groupID toolutil.StringOrInt, userID int64) error {
	//gitlab:request optional: the requester is read back only after a Minimal access approval, since the approval answers alike whether GitLab saved it or not
	_, _, err := client.GL().GroupMembers.GetGroupMember(string(groupID), userID, gl.WithContext(ctx))
	if err == nil {
		return nil
	}
	if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return fmt.Errorf("%s: GitLab answered the approval, and user %d is still not a member of group %s, so the request is still pending: GitLab grants access_level 5 (Minimal access) only where the top-level group's license includes it. %s", opApproveGroup, userID, groupID, minimalAccessRetry)
	}
	return toolutil.WrapErrWithHint(opApproveGroup, err,
		"GitLab answered the approval, and reading the requester back among the group's members failed, so whether Minimal access was granted is not known; check with group.members")
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
			"verify user_id with access.request_list_project; denying requires Maintainer role; the request must still be pending")
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
			"verify user_id with access.request_list_group; denying requires Owner role; the request must still be pending")
	}
	return nil
}
