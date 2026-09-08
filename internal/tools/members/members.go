package members

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ListInput defines parameters for listing project members.
type ListInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Query        string               `json:"query,omitempty" jsonschema:"Filter members by name or username"`
	UserIDs      []int                `json:"user_ids,omitempty" jsonschema:"Filter the results to only the listed user IDs"`
	ShowSeatInfo bool                 `json:"show_seat_info,omitempty" jsonschema:"Include seat usage information (is_using_seat) for each member"`
	OrderBy      string               `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. id, name, username, access_level)"`
	Sort         string               `json:"sort,omitempty" jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// Output represents a project member: what [gl.ProjectMember] decodes, plus
// what lib/api/entities/member.rb sends that the SDK does not carry, read
// from the captured response (ADR-0021). locked and public_email are on
// every member, membership_state on every member of an Enterprise instance,
// and two_factor_enabled, group_saml_identity, group_scim_identity and
// override when the caller may see them.
type Output struct {
	toolutil.HintableOutput
	ID                int64               `json:"id"`
	Username          string              `json:"username"`
	Name              string              `json:"name"`
	State             string              `json:"state"`
	Locked            bool                `json:"locked"`
	AvatarURL         string              `json:"avatar_url,omitempty"`
	AccessLevel       int                 `json:"access_level"`
	WebURL            string              `json:"web_url"`
	CreatedAt         string              `json:"created_at,omitempty"`
	CreatedBy         *CreatedByOutput    `json:"created_by,omitempty"`
	ExpiresAt         string              `json:"expires_at,omitempty"`
	Email             string              `json:"email,omitempty"`
	PublicEmail       string              `json:"public_email,omitempty"`
	TwoFactorEnabled  *bool               `json:"two_factor_enabled,omitempty"`
	GroupSAMLIdentity *SAMLIdentityOutput `json:"group_saml_identity,omitempty" tier:"premium"`
	GroupSCIMIdentity *SCIMIdentityOutput `json:"group_scim_identity,omitempty" tier:"premium"`
	Override          *bool               `json:"override,omitempty" tier:"premium"`
	MembershipState   string              `json:"membership_state,omitempty" tier:"premium"`
	MemberRole        *MemberRoleOutput   `json:"member_role,omitempty" tier:"ultimate"`
	IsUsingSeat       bool                `json:"is_using_seat,omitempty"`
}

// CreatedByOutput mirrors [gl.MemberCreatedBy], the user who created the
// membership record (1:1 SDK fidelity); canonical shape shared via toolutil.
type CreatedByOutput = toolutil.MemberUserOutput

// SAMLIdentityOutput mirrors the group_saml_identity object, which
// [gl.ProjectMember] does not carry; canonical shape shared via toolutil.
type SAMLIdentityOutput = toolutil.SAMLIdentityOutput

// SCIMIdentityOutput mirrors the group_scim_identity object, which client-go
// does not model; canonical shape shared via toolutil.
type SCIMIdentityOutput = toolutil.SCIMIdentityOutput

// MemberRoleOutput mirrors [gl.MemberRole], the custom member role attached
// to a membership (Premium/Ultimate). All permission flags are surfaced for
// 1:1 SDK fidelity. Canonical shape shared via toolutil.
type MemberRoleOutput = toolutil.MemberRoleOutput

// createdByOutput converts [gl.MemberCreatedBy] into the shared mirror, or nil.
func createdByOutput(c *gl.MemberCreatedBy) *CreatedByOutput {
	return toolutil.NewMemberUserOutput(c)
}

// memberRoleOutput converts [gl.MemberRole] into the shared mirror, or nil.
func memberRoleOutput(r *gl.MemberRole) *MemberRoleOutput {
	return toolutil.NewMemberRoleOutput(r)
}

// ListOutput holds a paginated list of members.
type ListOutput struct {
	toolutil.HintableOutput
	Members    []Output                  `json:"members"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// AccessLevelDescription delegates to [toolutil.AccessLevelDescription].
//
// Deprecated: Import toolutil.AccessLevelDescription directly instead.
func AccessLevelDescription(level gl.AccessLevelValue) string {
	return toolutil.AccessLevelDescription(level)
}

// ToOutput converts a GitLab API [gl.ProjectMember] to the MCP tool output
// format, mirroring the SDK fields 1:1 including the full created_by and
// member_role sub-objects, and takes the fields the capture read beside the
// SDK. The numeric access level is surfaced directly; callers can map it via
// [toolutil.AccessLevelDescription] when a label is needed.
func ToOutput(m *gl.ProjectMember, extra toolutil.MemberExtra) Output {
	out := Output{
		ID:                m.ID,
		Username:          m.Username,
		Name:              m.Name,
		State:             m.State,
		Locked:            extra.Locked,
		AvatarURL:         m.AvatarURL,
		AccessLevel:       int(m.AccessLevel),
		WebURL:            m.WebURL,
		Email:             m.Email,
		PublicEmail:       extra.PublicEmail,
		TwoFactorEnabled:  extra.TwoFactorEnabled,
		CreatedBy:         createdByOutput(m.CreatedBy),
		GroupSAMLIdentity: extra.GroupSAMLIdentity,
		GroupSCIMIdentity: extra.GroupSCIMIdentity,
		Override:          extra.Override,
		MembershipState:   extra.MembershipState,
		MemberRole:        memberRoleOutput(m.MemberRole),
	}
	if m.CreatedAt != nil {
		out.CreatedAt = m.CreatedAt.Format(time.RFC3339)
	}
	if m.ExpiresAt != nil {
		out.ExpiresAt = m.ExpiresAt.String()
	}
	out.IsUsingSeat = m.IsUsingSeat
	return out
}

// List retrieves all project members (including inherited members
// from parent groups) via the GitLab Project members API
// (GET /projects/:id/members/all). Supports filtering by name or
// username via the Query field and pagination.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("projectMembersList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	opts := &gl.ListProjectMembersOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.Query != "" {
		opts.Query = new(input.Query)
	}
	if input.ShowSeatInfo {
		opts.ShowSeatInfo = new(true)
	}
	if len(input.UserIDs) > 0 {
		ids := make([]int64, len(input.UserIDs))
		for i, id := range input.UserIDs {
			ids[i] = int64(id)
		}
		opts.UserIDs = &ids
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	members, resp, err := client.GL().ProjectMembers.ListAllProjectMembers(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("projectMembersList", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; lists direct + inherited members from parent groups")
	}
	extras, err := toolutil.CapturedMembers(captured, len(members))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("projectMembersList", err)
	}

	out := ListOutput{
		Members:    make([]Output, len(members)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, m := range members {
		out.Members[i] = ToOutput(m, extras[i])
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Input types for member CRUD
// ---------------------------------------------------------------------------.

// GetInput defines parameters for retrieving a single project member.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	UserID    int64                `json:"user_id"    jsonschema:"User ID of the member,required"`
}

// AddInput defines parameters for adding a project member.
type AddInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	UserID       int64                `json:"user_id,omitempty"       jsonschema:"User ID to add (provide user_id or username),required"`
	Username     string               `json:"username,omitempty"      jsonschema:"Username to add (provide user_id or username)"`
	AccessLevel  int                  `json:"access_level"            jsonschema:"Access level (5=Minimal access, 10=Guest, 15=Planner (Premium/Ultimate), 20=Reporter, 25=Security Manager (Premium/Ultimate), 30=Developer, 40=Maintainer, 50=Owner)"`
	ExpiresAt    string               `json:"expires_at,omitempty"    jsonschema:"Membership expiration date (YYYY-MM-DD)"`
	MemberRoleID int64                `json:"member_role_id,omitempty" jsonschema:"Custom member role ID"`
}

// EditInput defines parameters for editing a project member.
type EditInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	UserID       int64                `json:"user_id"                 jsonschema:"User ID of the member to edit,required"`
	AccessLevel  int                  `json:"access_level"            jsonschema:"New access level (5=Minimal access, 10=Guest, 15=Planner (Premium/Ultimate), 20=Reporter, 25=Security Manager (Premium/Ultimate), 30=Developer, 40=Maintainer, 50=Owner)"`
	ExpiresAt    string               `json:"expires_at,omitempty"    jsonschema:"Membership expiration date (YYYY-MM-DD)"`
	MemberRoleID int64                `json:"member_role_id,omitempty" jsonschema:"Custom member role ID"`
}

// DeleteInput defines parameters for removing a project member.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	UserID    int64                `json:"user_id"    jsonschema:"User ID of the member to remove,required"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// Get retrieves a direct project member by user ID via the GitLab
// Project members API (GET /projects/:id/members/:user_id). Does not
// include members inherited from parent groups; use [GetInherited] for
// that.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, errors.New("memberGet: project_id is required")
	}
	if input.UserID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("memberGet", "user_id")
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	m, _, err := client.GL().ProjectMembers.GetProjectMember(string(input.ProjectID), input.UserID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("memberGet", err, http.StatusNotFound,
			"user is not a direct member of this project; use gitlab_project_member_get_inherited to include parent-group inheritance, or gitlab_project_members_list to enumerate members")
	}
	extra, err := toolutil.CapturedMember(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("memberGet", err)
	}
	return ToOutput(m, extra), nil
}

// GetInherited retrieves a project member including membership
// inherited from any parent group via the GitLab Project members API
// (GET /projects/:id/members/all/:user_id).
func GetInherited(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, errors.New("memberGetInherited: project_id is required")
	}
	if input.UserID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("memberGetInherited", "user_id")
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	m, _, err := client.GL().ProjectMembers.GetInheritedProjectMember(string(input.ProjectID), input.UserID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("memberGetInherited", err, http.StatusNotFound,
			"user is not a member of this project nor any parent group; verify user_id with gitlab_list_users")
	}
	extra, err := toolutil.CapturedMember(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("memberGetInherited", err)
	}
	return ToOutput(m, extra), nil
}

// Add adds a user as a member of a project via the GitLab Project
// members API (POST /projects/:id/members). The user may be identified
// by either user_id or username. The AccessLevel must be one of
// 5/10/15/20/25/30/40/50/60; a MemberRoleID (Premium/Ultimate) may be
// provided instead to attach a custom role.
func Add(ctx context.Context, client *gitlabclient.Client, input AddInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, errors.New("memberAdd: project_id is required")
	}
	if input.UserID <= 0 && input.Username == "" {
		return Output{}, toolutil.ErrRequiredInt64("memberAdd", "user_id")
	}
	if input.AccessLevel == 0 {
		return Output{}, errors.New("memberAdd: access_level is required")
	}

	opts := &gl.AddProjectMemberOptions{
		AccessLevel: new(gl.AccessLevelValue(input.AccessLevel)),
	}
	if input.UserID != 0 {
		opts.UserID = input.UserID
	}
	if input.Username != "" {
		opts.Username = new(input.Username)
	}
	if input.ExpiresAt != "" {
		opts.ExpiresAt = new(input.ExpiresAt)
	}
	if input.MemberRoleID != 0 {
		opts.MemberRoleID = new(input.MemberRoleID)
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	m, _, err := client.GL().ProjectMembers.AddProjectMember(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		switch {
		case toolutil.IsHTTPStatus(err, http.StatusConflict):
			return Output{}, toolutil.WrapErrWithHint("memberAdd", err, "user is already a member of this project. Use gitlab_project_member_edit to change their access level")
		case toolutil.IsHTTPStatus(err, http.StatusNotFound):
			return Output{}, toolutil.WrapErrWithHint("memberAdd", err, "user not found. Use gitlab_list_users to search for the user")
		default:
			return Output{}, toolutil.WrapErrWithMessage("memberAdd", err)
		}
	}
	extra, err := toolutil.CapturedMember(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("memberAdd", err)
	}
	return ToOutput(m, extra), nil
}

// Edit modifies an existing project member's access level, expiration
// date, or custom member role via the GitLab Project members API
// (PUT /projects/:id/members/:user_id). Requires at least the same
// access level as the target member.
func Edit(ctx context.Context, client *gitlabclient.Client, input EditInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, errors.New("memberEdit: project_id is required")
	}
	if input.UserID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("memberEdit", "user_id")
	}
	if input.AccessLevel == 0 {
		return Output{}, errors.New("memberEdit: access_level is required")
	}

	opts := &gl.EditProjectMemberOptions{
		AccessLevel: new(gl.AccessLevelValue(input.AccessLevel)),
	}
	if input.ExpiresAt != "" {
		opts.ExpiresAt = new(input.ExpiresAt)
	}
	if input.MemberRoleID != 0 {
		opts.MemberRoleID = new(input.MemberRoleID)
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	m, _, err := client.GL().ProjectMembers.EditProjectMember(string(input.ProjectID), input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("memberEdit", err, "you need at least the same or higher access level as the target member")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("memberEdit", err, http.StatusBadRequest,
			"access_level must be one of 5/10/15/20/25/30/40/50/60 (Minimal/Guest/Planner/Reporter/Security Manager/Developer/Maintainer/Owner/Admin where supported); expires_at must be YYYY-MM-DD format; member_role_id (if provided) must exist for the namespace (Premium/Ultimate)")
	}
	extra, err := toolutil.CapturedMember(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("memberEdit", err)
	}
	return ToOutput(m, extra), nil
}

// Delete removes a member from a project via the GitLab Project
// members API (DELETE /projects/:id/members/:user_id). Requires
// Maintainer role; the last Owner of a project cannot be removed.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return errors.New("memberDelete: project_id is required")
	}
	if input.UserID <= 0 {
		return toolutil.ErrRequiredInt64("memberDelete", "user_id")
	}

	_, err := client.GL().ProjectMembers.DeleteProjectMember(string(input.ProjectID), input.UserID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("memberDelete", err, http.StatusForbidden,
			"requires Maintainer role; you cannot remove members whose access level equals or exceeds yours; the last Owner of a project cannot be removed")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Markdown (single member)
// ---------------------------------------------------------------------------.
