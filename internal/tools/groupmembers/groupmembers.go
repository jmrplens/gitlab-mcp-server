package groupmembers

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ──────────────────────────────────────────────
// Output types
// ──────────────────────────────────────────────.

// Output represents a single group member.
//
// Fields mirror gl.GroupMember (1:1 audit policy: full nested objects). The
// created_by, group_saml_identity, and member_role sub-objects are surfaced as
// full local mirrors on their canonical json keys (C-IMPORTS: replicated here
// rather than imported from sibling packages to preserve the zero-import-cycle
// constraint).
type Output struct {
	toolutil.HintableOutput
	ID                int64               `json:"id"`
	Username          string              `json:"username"`
	Name              string              `json:"name"`
	State             string              `json:"state"`
	AvatarURL         string              `json:"avatar_url,omitempty"`
	WebURL            string              `json:"web_url"`
	AccessLevel       int                 `json:"access_level"`
	CreatedAt         string              `json:"created_at,omitempty"`
	CreatedBy         *MemberUserOutput   `json:"created_by,omitempty"`
	ExpiresAt         string              `json:"expires_at,omitempty"`
	Email             string              `json:"email,omitempty"`
	PublicEmail       string              `json:"public_email,omitempty"`
	GroupSAMLIdentity *SAMLIdentityOutput `json:"group_saml_identity,omitempty"`
	MemberRole        *MemberRoleOutput   `json:"member_role,omitempty"`
	IsUsingSeat       bool                `json:"is_using_seat,omitempty"`
}

// MemberUserOutput mirrors gl.MemberCreatedBy (the created_by object).
type MemberUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// SAMLIdentityOutput mirrors gl.GroupMemberSAMLIdentity (the
// group_saml_identity object).
type SAMLIdentityOutput struct {
	ExternUID      string `json:"extern_uid"`
	Provider       string `json:"provider"`
	SAMLProviderID int64  `json:"saml_provider_id"`
}

// MemberRoleOutput mirrors gl.MemberRole (the member_role object). Custom
// member roles are an Enterprise (Premium/Ultimate) feature; the object is nil
// on instances or members without a custom role.
type MemberRoleOutput struct {
	ID                         int64  `json:"id"`
	Name                       string `json:"name"`
	Description                string `json:"description,omitempty"`
	GroupID                    int64  `json:"group_id"`
	BaseAccessLevel            int    `json:"base_access_level"`
	AdminCICDVariables         bool   `json:"admin_cicd_variables,omitempty"`
	AdminComplianceFramework   bool   `json:"admin_compliance_framework,omitempty"`
	AdminGroupMembers          bool   `json:"admin_group_member,omitempty"`
	AdminMergeRequests         bool   `json:"admin_merge_request,omitempty"`
	AdminPushRules             bool   `json:"admin_push_rules,omitempty"`
	AdminTerraformState        bool   `json:"admin_terraform_state,omitempty"`
	AdminVulnerability         bool   `json:"admin_vulnerability,omitempty"`
	AdminWebHook               bool   `json:"admin_web_hook,omitempty"`
	ArchiveProject             bool   `json:"archive_project,omitempty"`
	ManageDeployTokens         bool   `json:"manage_deploy_tokens,omitempty"`
	ManageGroupAccessTokens    bool   `json:"manage_group_access_tokens,omitempty"`
	ManageMergeRequestSettings bool   `json:"manage_merge_request_settings,omitempty"`
	ManageProjectAccessTokens  bool   `json:"manage_project_access_tokens,omitempty"`
	ManageSecurityPolicyLink   bool   `json:"manage_security_policy_link,omitempty"`
	ReadCode                   bool   `json:"read_code,omitempty"`
	ReadRunners                bool   `json:"read_runners,omitempty"`
	ReadDependency             bool   `json:"read_dependency,omitempty"`
	ReadVulnerability          bool   `json:"read_vulnerability,omitempty"`
	RemoveGroup                bool   `json:"remove_group,omitempty"`
	RemoveProject              bool   `json:"remove_project,omitempty"`
}

// ShareOutput represents the result of sharing with a group.
type ShareOutput struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
	WebURL      string `json:"web_url"`
}

// BillableMemberOutput mirrors gl.BillableGroupMember 1:1 (Enterprise
// Premium/Ultimate). A billable member is a user who counts toward the group's
// seat usage, including members inherited from subgroups and shared projects.
type BillableMemberOutput struct {
	ID             int64  `json:"id"`
	Username       string `json:"username"`
	Name           string `json:"name"`
	State          string `json:"state"`
	AvatarURL      string `json:"avatar_url,omitempty"`
	WebURL         string `json:"web_url"`
	Email          string `json:"email,omitempty"`
	LastActivityOn string `json:"last_activity_on,omitempty"`
	MembershipType string `json:"membership_type,omitempty"`
	Removable      bool   `json:"removable"`
	CreatedAt      string `json:"created_at,omitempty"`
	IsLastOwner    bool   `json:"is_last_owner"`
	LastLoginAt    string `json:"last_login_at,omitempty"`
}

// BillableMembersOutput holds a paginated list of billable group members.
type BillableMembersOutput struct {
	toolutil.HintableOutput
	Members    []BillableMemberOutput    `json:"members"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// BillableMembershipOutput mirrors gl.BillableUserMembership 1:1: one of the
// group/project memberships through which a billable member counts toward the
// group's seat usage. The access_level sub-object surfaces both the numeric
// and string forms of the role.
type BillableMembershipOutput struct {
	ID               int64                     `json:"id"`
	SourceID         int64                     `json:"source_id"`
	SourceFullName   string                    `json:"source_full_name"`
	SourceMembersURL string                    `json:"source_members_url,omitempty"`
	CreatedAt        string                    `json:"created_at,omitempty"`
	ExpiresAt        string                    `json:"expires_at,omitempty"`
	AccessLevel      *AccessLevelDetailsOutput `json:"access_level,omitempty"`
}

// AccessLevelDetailsOutput mirrors gl.AccessLevelDetails (the access_level
// object on a billable membership).
type AccessLevelDetailsOutput struct {
	IntegerValue int    `json:"integer_value"`
	StringValue  string `json:"string_value"`
}

// BillableMembershipsOutput holds a paginated list of a billable member's
// memberships.
type BillableMembershipsOutput struct {
	toolutil.HintableOutput
	Memberships []BillableMembershipOutput `json:"memberships"`
	Pagination  toolutil.PaginationOutput  `json:"pagination"`
}

// ──────────────────────────────────────────────
// Input types
// ──────────────────────────────────────────────.

// GetInput contains parameters for getting a group member.
type GetInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	UserID  int64                `json:"user_id" jsonschema:"User ID,required"`
}

// AddInput contains parameters for adding a group member.
type AddInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	UserID       int64                `json:"user_id,omitempty" jsonschema:"User ID to add,required"`
	Username     string               `json:"username,omitempty" jsonschema:"Username to add (alternative to user_id)"`
	AccessLevel  int                  `json:"access_level" jsonschema:"Access level (5=Minimal access, 10=Guest, 15=Planner (Premium/Ultimate), 20=Reporter, 25=Security Manager (Premium/Ultimate), 30=Developer, 40=Maintainer, 50=Owner, 60=Admin where supported)"`
	ExpiresAt    string               `json:"expires_at,omitempty" jsonschema:"Membership expiration date (YYYY-MM-DD)"`
	MemberRoleID int64                `json:"member_role_id,omitempty" jsonschema:"Custom member role ID to assign (Premium/Ultimate); the role's base access level must match access_level"`
}

// EditInput contains parameters for editing a group member.
type EditInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	UserID       int64                `json:"user_id" jsonschema:"User ID,required"`
	AccessLevel  int                  `json:"access_level,omitempty" jsonschema:"New access level (5=Minimal access, 10=Guest, 15=Planner (Premium), 20=Reporter, 25=Security Manager (Premium), 30=Developer, 40=Maintainer, 50=Owner, 60=Admin where supported)"`
	ExpiresAt    string               `json:"expires_at,omitempty" jsonschema:"New membership expiration date (YYYY-MM-DD)"`
	MemberRoleID int64                `json:"member_role_id,omitempty" jsonschema:"Custom member role ID to assign (Premium/Ultimate); the role's base access level must match access_level"`
}

// RemoveInput contains parameters for removing a group member.
type RemoveInput struct {
	GroupID           toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	UserID            int64                `json:"user_id" jsonschema:"User ID to remove,required"`
	SkipSubresources  bool                 `json:"skip_subresources,omitempty" jsonschema:"Skip removal from subresources"`
	UnassignIssuables bool                 `json:"unassign_issuables,omitempty" jsonschema:"Unassign issues and merge requests"`
}

// ShareInput contains parameters for sharing a group with another group.
type ShareInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path to share,required"`
	ShareGroupID int64                `json:"share_group_id" jsonschema:"Group ID to share with,required"`
	GroupAccess  int                  `json:"group_access" jsonschema:"Access level for the shared group (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer); 5=Minimal access, 15=Planner, 25=Security Manager, 60=Admin are not valid for group shares"`
	ExpiresAt    string               `json:"expires_at,omitempty" jsonschema:"Share expiration date (YYYY-MM-DD)"`
}

// UnshareInput contains parameters for unsharing a group.
type UnshareInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	ShareGroupID int64                `json:"share_group_id" jsonschema:"Group ID to stop sharing with,required"`
}

// ListBillableMembersInput contains parameters for listing billable group
// members (Enterprise Premium/Ultimate).
type ListBillableMembersInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Search  string               `json:"search,omitempty" jsonschema:"Filter billable members by name or username"`
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Column to order billable members by (e.g. id, name, username, last_activity_on)"`
	Sort    string               `json:"sort,omitempty" jsonschema:"Sort order (e.g. name_asc, name_desc, last_activity_on_asc, last_activity_on_desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListBillableMemberMembershipsInput contains parameters for listing the
// memberships of a single billable group member (Enterprise Premium/Ultimate).
type ListBillableMemberMembershipsInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	UserID  int64                `json:"user_id" jsonschema:"User ID of the billable member,required"`
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Column to order memberships by (e.g. id, name)"`
	Sort    string               `json:"sort,omitempty" jsonschema:"Sort direction: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// RemoveBillableMemberInput contains parameters for removing a billable member
// from a group (Enterprise Premium/Ultimate).
type RemoveBillableMemberInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	UserID  int64                `json:"user_id" jsonschema:"User ID of the billable member to remove,required"`
}

// ──────────────────────────────────────────────
// Handlers
// ──────────────────────────────────────────────.

// GetMember gets a single group member.
func GetMember(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.WrapErrWithMessage("group_member_get", toolutil.ErrFieldRequired("group_id"))
	}
	if input.UserID == 0 {
		return Output{}, toolutil.WrapErrWithMessage("group_member_get", toolutil.ErrFieldRequired("user_id"))
	}
	m, _, err := client.GL().GroupMembers.GetGroupMember(
		string(input.GroupID), input.UserID, gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("group_member_get", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get and user_id with gitlab_list_users \u2014 inherited members are not returned, use gitlab_group_member_get_inherited for those")
	}
	return convertMember(m), nil
}

// GetInheritedMember gets a single inherited group member.
func GetInheritedMember(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.WrapErrWithMessage("group_member_get_inherited", toolutil.ErrFieldRequired("group_id"))
	}
	if input.UserID == 0 {
		return Output{}, toolutil.WrapErrWithMessage("group_member_get_inherited", toolutil.ErrFieldRequired("user_id"))
	}
	m, _, err := client.GL().GroupMembers.GetInheritedGroupMember(
		string(input.GroupID), input.UserID, gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("group_member_get_inherited", err, http.StatusNotFound,
			"the user is not a member of this group or any ancestor group; verify with gitlab_group_members_list (include_inherited=true)")
	}
	return convertMember(m), nil
}

// AddMember adds a member to a group.
func AddMember(ctx context.Context, client *gitlabclient.Client, input AddInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.WrapErrWithMessage("group_member_add", toolutil.ErrFieldRequired("group_id"))
	}
	if input.UserID == 0 && input.Username == "" {
		return Output{}, toolutil.WrapErrWithMessage("group_member_add", errors.New("user_id or username is required"))
	}
	if input.AccessLevel == 0 {
		return Output{}, toolutil.WrapErrWithMessage("group_member_add", toolutil.ErrFieldRequired("access_level"))
	}
	opts := &gl.AddGroupMemberOptions{
		AccessLevel: new(gl.AccessLevelValue(input.AccessLevel)),
	}
	if input.UserID != 0 {
		opts.UserID = new(input.UserID)
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
	m, _, err := client.GL().GroupMembers.AddGroupMember(
		string(input.GroupID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusConflict) {
			return Output{}, toolutil.WrapErrWithHint("group_member_add", err,
				"the user is already a direct member of this group \u2014 use gitlab_group_member_edit to change their access level instead")
		}
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("group_member_add", err,
				"adding members requires Owner role on the group; cannot grant access higher than your own role")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("group_member_add", err,
				"access_level must be one of 5/10/15/20/25/30/40/50/60 (Minimal/Guest/Planner/Reporter/Security Manager/Developer/Maintainer/Owner/Admin where supported); expires_at must be YYYY-MM-DD")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("group_member_add", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get and user_id/username with gitlab_list_users")
	}
	return convertMember(m), nil
}

// EditMember edits a group member.
func EditMember(ctx context.Context, client *gitlabclient.Client, input EditInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.WrapErrWithMessage("group_member_edit", toolutil.ErrFieldRequired("group_id"))
	}
	if input.UserID == 0 {
		return Output{}, toolutil.WrapErrWithMessage("group_member_edit", toolutil.ErrFieldRequired("user_id"))
	}
	opts := &gl.EditGroupMemberOptions{}
	if input.AccessLevel != 0 {
		opts.AccessLevel = new(gl.AccessLevelValue(input.AccessLevel))
	}
	if input.ExpiresAt != "" {
		opts.ExpiresAt = new(input.ExpiresAt)
	}
	if input.MemberRoleID != 0 {
		opts.MemberRoleID = new(input.MemberRoleID)
	}
	m, _, err := client.GL().GroupMembers.EditGroupMember(
		string(input.GroupID), input.UserID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("group_member_edit", err,
				"editing members requires Owner role; cannot edit inherited members (only direct members) and cannot grant access higher than your own role")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("group_member_edit", err, http.StatusNotFound,
			"the user is not a direct member of this group \u2014 use gitlab_group_members_list to confirm direct membership before editing")
	}
	return convertMember(m), nil
}

// RemoveMember removes a member from a group.
func RemoveMember(ctx context.Context, client *gitlabclient.Client, input RemoveInput) error {
	if input.GroupID == "" {
		return toolutil.WrapErrWithMessage("group_member_remove", toolutil.ErrFieldRequired("group_id"))
	}
	if input.UserID == 0 {
		return toolutil.WrapErrWithMessage("group_member_remove", toolutil.ErrFieldRequired("user_id"))
	}
	opts := &gl.RemoveGroupMemberOptions{}
	if input.SkipSubresources {
		opts.SkipSubresources = new(true)
	}
	if input.UnassignIssuables {
		opts.UnassignIssuables = new(true)
	}
	_, err := client.GL().GroupMembers.RemoveGroupMember(
		string(input.GroupID), input.UserID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("group_member_remove", err,
				"inherited members cannot be removed from this group \u2014 they must be removed from the ancestor group where they were added directly. Removing direct members requires Owner role")
		}
		return toolutil.WrapErrWithStatusHint("group_member_remove", err, http.StatusNotFound,
			"the user is not a direct member of this group; use gitlab_group_members_list to confirm direct membership")
	}
	return nil
}

// removeMemberOutput removes member output and returns [toolutil.DeleteOutput].
func removeMemberOutput(ctx context.Context, client *gitlabclient.Client, input RemoveInput) (toolutil.DeleteOutput, error) {
	if err := RemoveMember(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted group member."}, nil
}

// ShareGroup shares a group with another group.
func ShareGroup(ctx context.Context, client *gitlabclient.Client, input ShareInput) (ShareOutput, error) {
	if input.GroupID == "" {
		return ShareOutput{}, toolutil.WrapErrWithMessage("group_share", toolutil.ErrFieldRequired("group_id"))
	}
	if input.ShareGroupID == 0 {
		return ShareOutput{}, toolutil.WrapErrWithMessage("group_share", toolutil.ErrFieldRequired("share_group_id"))
	}
	if input.GroupAccess == 0 {
		return ShareOutput{}, toolutil.WrapErrWithMessage("group_share", toolutil.ErrFieldRequired("group_access"))
	}
	switch input.GroupAccess {
	case 10, 20, 30, 40:
	default:
		return ShareOutput{}, toolutil.WrapErrWithMessage("group_share", errors.New("group_access must be one of 10/20/30/40 (Guest/Reporter/Developer/Maintainer); 5=Minimal access, 15=Planner, 25=Security Manager, 60=Admin are not valid for project group shares"))
	}
	opts := &gl.ShareWithGroupOptions{
		GroupID:     new(input.ShareGroupID),
		GroupAccess: new(gl.AccessLevelValue(input.GroupAccess)),
	}
	if input.ExpiresAt != "" {
		opts.ExpiresAt = new(input.ExpiresAt)
	}
	g, _, err := client.GL().GroupMembers.ShareWithGroup(
		string(input.GroupID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusConflict) {
			return ShareOutput{}, toolutil.WrapErrWithHint("group_share", err,
				"this group is already shared with the target group \u2014 use gitlab_group_unshare first to change the access level")
		}
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return ShareOutput{}, toolutil.WrapErrWithHint("group_share", err,
				"sharing requires Owner role on this group AND Maintainer+ on the target group; cross-hierarchy sharing may be disabled in group/instance settings")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return ShareOutput{}, toolutil.WrapErrWithHint("group_share", err,
				"group_access must be one of 10/20/30/40 (Guest/Reporter/Developer/Maintainer); 5=Minimal access, 15=Planner, 25=Security Manager, 60=Admin are not valid for project group shares")
		}
		return ShareOutput{}, toolutil.WrapErrWithStatusHint("group_share", err, http.StatusNotFound,
			"verify group_id and share_group_id with gitlab_group_get \u2014 share_group_id must be a numeric group ID, not a path")
	}
	return ShareOutput{
		ID:          g.ID,
		Name:        g.Name,
		Path:        g.Path,
		Description: g.Description,
		WebURL:      g.WebURL,
	}, nil
}

// UnshareGroup removes a group share.
func UnshareGroup(ctx context.Context, client *gitlabclient.Client, input UnshareInput) error {
	if input.GroupID == "" {
		return toolutil.WrapErrWithMessage("group_unshare", toolutil.ErrFieldRequired("group_id"))
	}
	if input.ShareGroupID == 0 {
		return toolutil.WrapErrWithMessage("group_unshare", toolutil.ErrFieldRequired("share_group_id"))
	}
	_, err := client.GL().GroupMembers.DeleteShareWithGroup(
		string(input.GroupID), input.ShareGroupID, gl.WithContext(ctx),
	)
	if err != nil {
		return toolutil.WrapErrWithStatusHint("group_unshare", err, http.StatusNotFound,
			"the share does not exist \u2014 use gitlab_group_get to inspect shared_with_groups for the current shares")
	}
	return nil
}

// unshareGroupOutput unshares group output and returns [toolutil.DeleteOutput].
func unshareGroupOutput(ctx context.Context, client *gitlabclient.Client, input UnshareInput) (toolutil.DeleteOutput, error) {
	if err := UnshareGroup(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted group share."}, nil
}

// ListBillableMembers lists the billable members of a group (Enterprise
// Premium/Ultimate). The list includes members inherited from subgroups and
// shared projects.
func ListBillableMembers(ctx context.Context, client *gitlabclient.Client, input ListBillableMembersInput) (BillableMembersOutput, error) {
	if input.GroupID == "" {
		return BillableMembersOutput{}, toolutil.WrapErrWithMessage("group_billable_members_list", toolutil.ErrFieldRequired("group_id"))
	}
	opts := &gl.ListBillableGroupMembersOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	members, resp, err := client.GL().Groups.ListBillableGroupMembers(
		string(input.GroupID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		return BillableMembersOutput{}, toolutil.WrapErrWithStatusHint("group_billable_members_list", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get — billable members are a Premium/Ultimate feature and require Owner access on the group")
	}
	out := BillableMembersOutput{
		Members:    make([]BillableMemberOutput, len(members)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, m := range members {
		out.Members[i] = convertBillableMember(m)
	}
	return out, nil
}

// ListBillableMemberMemberships lists the memberships of a single billable
// member of a group (Enterprise Premium/Ultimate).
func ListBillableMemberMemberships(ctx context.Context, client *gitlabclient.Client, input ListBillableMemberMembershipsInput) (BillableMembershipsOutput, error) {
	if input.GroupID == "" {
		return BillableMembershipsOutput{}, toolutil.WrapErrWithMessage("group_billable_member_memberships_list", toolutil.ErrFieldRequired("group_id"))
	}
	if input.UserID == 0 {
		return BillableMembershipsOutput{}, toolutil.WrapErrWithMessage("group_billable_member_memberships_list", toolutil.ErrFieldRequired("user_id"))
	}
	opts := &gl.ListMembershipsForBillableGroupMemberOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	memberships, resp, err := client.GL().Groups.ListMembershipsForBillableGroupMember(
		string(input.GroupID), input.UserID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		return BillableMembershipsOutput{}, toolutil.WrapErrWithStatusHint("group_billable_member_memberships_list", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get and user_id with gitlab_list_billable_group_members — the user must be a billable member of the group (Premium/Ultimate)")
	}
	out := BillableMembershipsOutput{
		Memberships: make([]BillableMembershipOutput, len(memberships)),
		Pagination:  toolutil.PaginationFromResponse(resp),
	}
	for i, m := range memberships {
		out.Memberships[i] = convertBillableMembership(m)
	}
	return out, nil
}

// RemoveBillableMember removes a billable member from a group (Enterprise
// Premium/Ultimate).
func RemoveBillableMember(ctx context.Context, client *gitlabclient.Client, input RemoveBillableMemberInput) error {
	if input.GroupID == "" {
		return toolutil.WrapErrWithMessage("group_billable_member_remove", toolutil.ErrFieldRequired("group_id"))
	}
	if input.UserID == 0 {
		return toolutil.WrapErrWithMessage("group_billable_member_remove", toolutil.ErrFieldRequired("user_id"))
	}
	_, err := client.GL().Groups.RemoveBillableGroupMember(
		string(input.GroupID), input.UserID, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("group_billable_member_remove", err,
				"removing billable members requires Owner role on the group; the last owner cannot be removed (is_last_owner)")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return toolutil.WrapErrWithHint("group_billable_member_remove", err,
				"only directly removable billable members can be removed here; check the 'removable' flag from gitlab_list_billable_group_members — inherited members must be removed from their source group")
		}
		return toolutil.WrapErrWithStatusHint("group_billable_member_remove", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get and user_id with gitlab_list_billable_group_members")
	}
	return nil
}

// removeBillableMemberOutput removes a billable member and returns a
// [toolutil.DeleteOutput].
func removeBillableMemberOutput(ctx context.Context, client *gitlabclient.Client, input RemoveBillableMemberInput) (toolutil.DeleteOutput, error) {
	if err := RemoveBillableMember(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully removed billable group member."}, nil
}

// ──────────────────────────────────────────────
// Converters
// ──────────────────────────────────────────────.

// convertMember maps a GitLab group member into the MCP output shape.
func convertMember(m *gl.GroupMember) Output {
	out := Output{
		ID:                m.ID,
		Username:          m.Username,
		Name:              m.Name,
		State:             m.State,
		AvatarURL:         m.AvatarURL,
		WebURL:            m.WebURL,
		AccessLevel:       int(m.AccessLevel),
		Email:             m.Email,
		PublicEmail:       m.PublicEmail,
		CreatedBy:         memberUserOutput(m.CreatedBy),
		GroupSAMLIdentity: samlIdentityOutput(m.GroupSAMLIdentity),
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

// memberUserOutput mirrors a gl.MemberCreatedBy into the local output shape.
func memberUserOutput(u *gl.MemberCreatedBy) *MemberUserOutput {
	if u == nil {
		return nil
	}
	return &MemberUserOutput{
		ID:        u.ID,
		Username:  u.Username,
		Name:      u.Name,
		State:     u.State,
		AvatarURL: u.AvatarURL,
		WebURL:    u.WebURL,
	}
}

// samlIdentityOutput mirrors a gl.GroupMemberSAMLIdentity into the local
// output shape.
func samlIdentityOutput(s *gl.GroupMemberSAMLIdentity) *SAMLIdentityOutput {
	if s == nil {
		return nil
	}
	return &SAMLIdentityOutput{
		ExternUID:      s.ExternUID,
		Provider:       s.Provider,
		SAMLProviderID: s.SAMLProviderID,
	}
}

// memberRoleOutput mirrors a gl.MemberRole into the local output shape.
func memberRoleOutput(r *gl.MemberRole) *MemberRoleOutput {
	if r == nil {
		return nil
	}
	return &MemberRoleOutput{
		ID:                         r.ID,
		Name:                       r.Name,
		Description:                r.Description,
		GroupID:                    r.GroupID,
		BaseAccessLevel:            int(r.BaseAccessLevel),
		AdminCICDVariables:         r.AdminCICDVariables,
		AdminComplianceFramework:   r.AdminComplianceFramework,
		AdminGroupMembers:          r.AdminGroupMembers,
		AdminMergeRequests:         r.AdminMergeRequests,
		AdminPushRules:             r.AdminPushRules,
		AdminTerraformState:        r.AdminTerraformState,
		AdminVulnerability:         r.AdminVulnerability,
		AdminWebHook:               r.AdminWebHook,
		ArchiveProject:             r.ArchiveProject,
		ManageDeployTokens:         r.ManageDeployTokens,
		ManageGroupAccessTokens:    r.ManageGroupAccessTokens,
		ManageMergeRequestSettings: r.ManageMergeRequestSettings,
		ManageProjectAccessTokens:  r.ManageProjectAccessTokens,
		ManageSecurityPolicyLink:   r.ManageSecurityPolicyLink,
		ReadCode:                   r.ReadCode,
		ReadRunners:                r.ReadRunners,
		ReadDependency:             r.ReadDependency,
		ReadVulnerability:          r.ReadVulnerability,
		RemoveGroup:                r.RemoveGroup,
		RemoveProject:              r.RemoveProject,
	}
}

// convertBillableMember maps a gl.BillableGroupMember into the MCP output
// shape (1:1 field fidelity).
func convertBillableMember(m *gl.BillableGroupMember) BillableMemberOutput {
	out := BillableMemberOutput{
		ID:             m.ID,
		Username:       m.Username,
		Name:           m.Name,
		State:          m.State,
		AvatarURL:      m.AvatarURL,
		WebURL:         m.WebURL,
		Email:          m.Email,
		MembershipType: m.MembershipType,
		Removable:      m.Removable,
		IsLastOwner:    m.IsLastOwner,
	}
	if m.LastActivityOn != nil {
		out.LastActivityOn = m.LastActivityOn.String()
	}
	if m.CreatedAt != nil {
		out.CreatedAt = m.CreatedAt.Format(time.RFC3339)
	}
	if m.LastLoginAt != nil {
		out.LastLoginAt = m.LastLoginAt.Format(time.RFC3339)
	}
	return out
}

// convertBillableMembership maps a gl.BillableUserMembership into the MCP
// output shape (1:1 field fidelity).
func convertBillableMembership(m *gl.BillableUserMembership) BillableMembershipOutput {
	out := BillableMembershipOutput{
		ID:               m.ID,
		SourceID:         m.SourceID,
		SourceFullName:   m.SourceFullName,
		SourceMembersURL: m.SourceMembersURL,
	}
	if m.CreatedAt != nil {
		out.CreatedAt = m.CreatedAt.Format(time.RFC3339)
	}
	if m.ExpiresAt != nil {
		out.ExpiresAt = m.ExpiresAt.String()
	}
	if m.AccessLevel != nil {
		out.AccessLevel = &AccessLevelDetailsOutput{
			IntegerValue: int(m.AccessLevel.IntegerValue),
			StringValue:  m.AccessLevel.StringValue,
		}
	}
	return out
}

// ──────────────────────────────────────────────
// Markdown formatters
// ──────────────────────────────────────────────.
