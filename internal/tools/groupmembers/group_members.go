package groupmembers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ──────────────────────────────────────────────
// Output types
// ──────────────────────────────────────────────.

// Output represents a single group member.
//
// Fields mirror gl.GroupMember (1:1 audit policy: full nested objects) plus
// what lib/api/entities/member.rb sends that the SDK does not carry, read
// from the captured response (ADR-0021): locked on every member,
// membership_state on every member of an Enterprise instance, and
// two_factor_enabled, group_scim_identity and override when the caller may
// see them. avatar_path and custom_attributes are deliberately not published:
// each waits on a presenter option, and no group-member route declares
// only_path or with_custom_attributes, so GitLab never sends them here.
// The created_by, group_saml_identity, group_scim_identity and
// member_role sub-objects are surfaced as full local mirrors on their
// canonical json keys (C-IMPORTS: replicated here rather than imported from
// sibling packages to preserve the zero-import-cycle constraint).
type Output struct {
	toolutil.HintableOutput
	ID                int64               `json:"id"`
	Username          string              `json:"username"`
	Name              string              `json:"name"`
	State             string              `json:"state"`
	Locked            bool                `json:"locked"`
	AvatarURL         string              `json:"avatar_url,omitempty"`
	WebURL            string              `json:"web_url"`
	AccessLevel       int                 `json:"access_level"`
	CreatedAt         string              `json:"created_at,omitempty"`
	CreatedBy         *MemberUserOutput   `json:"created_by,omitempty"`
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

// MemberUserOutput mirrors gl.MemberCreatedBy (the created_by object);
// canonical shape shared via toolutil.
type MemberUserOutput = toolutil.MemberUserOutput

// SAMLIdentityOutput mirrors gl.GroupMemberSAMLIdentity (the
// group_saml_identity object); canonical shape shared via toolutil.
type SAMLIdentityOutput = toolutil.SAMLIdentityOutput

// SCIMIdentityOutput mirrors the group_scim_identity object, which client-go
// does not model; canonical shape shared via toolutil.
type SCIMIdentityOutput = toolutil.SCIMIdentityOutput

// MemberRoleOutput mirrors gl.MemberRole (the member_role object). Custom
// member roles are an Enterprise (Premium/Ultimate) feature; the object is nil
// on instances or members without a custom role. Canonical shape shared via
// toolutil.
type MemberRoleOutput = toolutil.MemberRoleOutput

// ShareOutput is the group that was shared, as POST /groups/:id/share answers
// with it (lib/api/groups.rb presents the whole Entities::GroupDetail): the
// keys that name the group, and the groups it is now shared with, which is
// what the write changed. The new share is one of those entries, with the
// access level and expiry GitLab recorded for it. The rest of the group, its
// settings, counts and links, is group.get's, which returns the whole group.
type ShareOutput struct {
	toolutil.HintableOutput
	ID               int64                   `json:"id"`
	Name             string                  `json:"name"`
	Path             string                  `json:"path"`
	FullName         string                  `json:"full_name,omitempty"`
	FullPath         string                  `json:"full_path,omitempty"`
	Description      string                  `json:"description,omitempty"`
	Visibility       string                  `json:"visibility,omitempty"`
	WebURL           string                  `json:"web_url"`
	SharedWithGroups []SharedWithGroupOutput `json:"shared_with_groups"`
}

// SharedWithGroupOutput is one entry of shared_with_groups, as
// lib/api/entities/shared_group_with_group.rb renders it: the group the
// shared group is shared with, the access its members gain, when the share
// expires, and the custom role it grants on an instance with custom roles
// enabled. Mirrored here rather than imported from internal/tools/groups, the
// way this package keeps its other shapes (C-IMPORTS).
type SharedWithGroupOutput struct {
	GroupID          int64  `json:"group_id"`
	GroupName        string `json:"group_name"`
	GroupFullPath    string `json:"group_full_path"`
	GroupAccessLevel int64  `json:"group_access_level"`
	ExpiresAt        string `json:"expires_at,omitempty"`
	MemberRoleID     int64  `json:"member_role_id,omitempty" tier:"ultimate"`
}

// BillableMemberOutput mirrors gl.BillableGroupMember 1:1 (Enterprise
// Premium/Ultimate). A billable member is a user who counts toward the group's
// seat usage, including members inherited from subgroups and shared projects.
//
// Beside what the SDK decodes it carries the two keys
// ee/lib/api/entities/billable_member.rb inherits from UserBasic and the SDK
// does not model, read from the captured response (ADR-0021): public_email and
// locked, both on every member. The same UserBasic exposes avatar_path and
// custom_attributes, which are not published because this route declares
// neither only_path nor with_custom_attributes and so never sends them.
//
// It carries no access level, expiry or role: this endpoint answers with a
// user who costs a seat and not with a membership record.
type BillableMemberOutput struct {
	ID             int64  `json:"id"`
	Username       string `json:"username"`
	PublicEmail    string `json:"public_email,omitempty"`
	Name           string `json:"name"`
	State          string `json:"state"`
	Locked         bool   `json:"locked"`
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
	AccessLevel  int                  `json:"access_level" jsonschema:"Access level (5=Minimal access (Premium/Ultimate), 10=Guest, 15=Planner, 20=Reporter, 25=Security Manager, 30=Developer, 40=Maintainer, 50=Owner)"`
	ExpiresAt    string               `json:"expires_at,omitempty" jsonschema:"Membership expiration date (YYYY-MM-DD)"`
	MemberRoleID int64                `json:"member_role_id,omitempty" jsonschema:"Custom member role ID to assign. Ultimate only. The role's base access level must match access_level"`
}

// EditInput contains parameters for editing a group member.
type EditInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	UserID       int64                `json:"user_id" jsonschema:"User ID,required"`
	AccessLevel  int                  `json:"access_level,omitempty" jsonschema:"New access level (5=Minimal access (Premium/Ultimate), 10=Guest, 15=Planner, 20=Reporter, 25=Security Manager, 30=Developer, 40=Maintainer, 50=Owner, 60=Admin)"`
	ExpiresAt    string               `json:"expires_at,omitempty" jsonschema:"New membership expiration date (YYYY-MM-DD)"`
	MemberRoleID int64                `json:"member_role_id,omitempty" jsonschema:"Custom member role ID to assign. Ultimate only. The role's base access level must match access_level"`
}

// RemoveInput contains parameters for removing a group member.
type RemoveInput struct {
	GroupID           toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	UserID            int64                `json:"user_id" jsonschema:"User ID to remove,required"`
	SkipSubresources  bool                 `json:"skip_subresources,omitempty" jsonschema:"Skip removal from subresources"`
	UnassignIssuables bool                 `json:"unassign_issuables,omitempty" jsonschema:"Unassign issues and merge requests"`
}

// ShareInput contains parameters for sharing a group with another group.
//
// group.share_with_group reaches the same route with an input of its own
// (groups.ShareGroupInput). The two are kept apart because each action's
// parameter names are what a caller already sends, and renaming
// share_group_id or shared_group_id to match the other would break one of
// them; what they may send is the same.
type ShareInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path to share,required"`
	ShareGroupID int64                `json:"share_group_id" jsonschema:"Group ID to share with,required"`
	GroupAccess  int                  `json:"group_access" jsonschema:"Access level the members of the group shared with gain (5=Minimal access (Premium/Ultimate), 10=Guest, 15=Planner, 20=Reporter, 25=Security Manager, 30=Developer, 40=Maintainer, 50=Owner). 60=Admin is not valid for group shares"`
	ExpiresAt    string               `json:"expires_at,omitempty" jsonschema:"Share expiration date (YYYY-MM-DD)"`
	MemberRoleID int64                `json:"member_role_id,omitempty" jsonschema:"Custom member role the share grants (Ultimate only). Its base access level must equal group_access" tier:"ultimate"`
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	m, _, err := client.GL().GroupMembers.GetGroupMember(
		string(input.GroupID), input.UserID, gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("group_member_get", err, http.StatusNotFound,
			"verify group_id with group.get and user_id with user.list. Inherited members are not returned, use group.group_member_get_inherited for those")
	}
	extra, err := toolutil.CapturedMember(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("group_member_get", err)
	}
	return convertMember(m, extra), nil
}

// GetInheritedMember gets a single inherited group member.
func GetInheritedMember(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.WrapErrWithMessage("group_member_get_inherited", toolutil.ErrFieldRequired("group_id"))
	}
	if input.UserID == 0 {
		return Output{}, toolutil.WrapErrWithMessage("group_member_get_inherited", toolutil.ErrFieldRequired("user_id"))
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	m, _, err := client.GL().GroupMembers.GetInheritedGroupMember(
		string(input.GroupID), input.UserID, gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("group_member_get_inherited", err, http.StatusNotFound,
			"the user is not a member of this group or any ancestor group; verify with group.members (include_inherited=true)")
	}
	extra, err := toolutil.CapturedMember(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("group_member_get_inherited", err)
	}
	return convertMember(m, extra), nil
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	m, _, err := client.GL().GroupMembers.AddGroupMember(
		string(input.GroupID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusConflict) {
			return Output{}, toolutil.WrapErrWithHint("group_member_add", err,
				"the user is already a direct member of this group. Use group.group_member_edit to change their access level instead")
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
			"verify group_id with group.get and user_id/username with user.list")
	}
	extra, err := toolutil.CapturedMember(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("group_member_add", err)
	}
	return convertMember(m, extra), nil
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	m, _, err := client.GL().GroupMembers.EditGroupMember(
		string(input.GroupID), input.UserID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("group_member_edit", err,
				"editing members requires Owner role; cannot edit inherited members (only direct members) and cannot grant access higher than your own role")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("group_member_edit", err, http.StatusNotFound,
			"the user is not a direct member of this group. Use group.members to confirm direct membership before editing")
	}
	extra, err := toolutil.CapturedMember(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("group_member_edit", err)
	}
	return convertMember(m, extra), nil
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
				"inherited members cannot be removed from this group. They must be removed from the ancestor group where they were added directly. Removing direct members requires Owner role")
		}
		return toolutil.WrapErrWithStatusHint("group_member_remove", err, http.StatusNotFound,
			"the user is not a direct member of this group; use group.members to confirm direct membership")
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

// errShareGroupAccess refuses a group_access no share can carry. It names the
// levels POST /groups/:id/share accepts: lib/api/groups.rb validates the
// parameter against Gitlab::Access.values_with_minimal_access, which is Guest
// through Owner (Planner and Security Manager among them) on every build, and
// Minimal access as well on an Enterprise build. 60 (Admin) is not a
// membership level, and Grape would refuse it and any other number the same
// way, so asking GitLab would only move the refusal later.
var errShareGroupAccess = errors.New("group_access must be one of 5 (Minimal access), 10 (Guest), 15 (Planner), 20 (Reporter), 25 (Security Manager), 30 (Developer), 40 (Maintainer) or 50 (Owner); 60 (Admin) is not valid for group shares")

// ShareGroup shares a group with another group.
//
// It sends through Groups.ShareGroupWithGroup rather than
// GroupMembers.ShareWithGroup, which reaches the same route: the options of
// the second carry no member_role_id, and their expires_at has no omitempty,
// so a share without an expiry sent "expires_at": null on every call, while
// ShareGroupWithGroupOptions leaves out every field the caller did not set.
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
	case 5, 10, 15, 20, 25, 30, 40, 50:
	default:
		return ShareOutput{}, toolutil.WrapErrWithMessage("group_share", errShareGroupAccess)
	}
	opts := &gl.ShareGroupWithGroupOptions{
		GroupID:     new(input.ShareGroupID),
		GroupAccess: new(gl.AccessLevelValue(input.GroupAccess)),
	}
	if input.ExpiresAt != "" {
		expiresAt, err := gl.ParseISOTime(input.ExpiresAt)
		if err != nil {
			return ShareOutput{}, toolutil.WrapErrWithMessage("group_share", fmt.Errorf("expires_at must be a date in YYYY-MM-DD form: %w", err))
		}
		opts.ExpiresAt = &expiresAt
	}
	if input.MemberRoleID != 0 {
		opts.MemberRoleID = new(input.MemberRoleID)
	}
	g, _, err := client.GL().Groups.ShareGroupWithGroup(
		string(input.GroupID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusConflict) {
			return ShareOutput{}, toolutil.WrapErrWithHint("group_share", err,
				"GitLab could not record the share. If this group is already shared with the target group, use group.group_member_unshare first to change it. With member_role_id, the custom role must belong to this group's top-level group and its base access level must equal group_access. 5 (Minimal access) needs a Premium or Ultimate license and a top-level group. Where the top-level group restricts membership by email domain, the target group's allowed domains must be a subset of it")
		}
		if toolutil.IsPermissionRefusal(err) {
			return ShareOutput{}, toolutil.WrapErrWithHint("group_share", err,
				"the credential may not share groups: a fine-grained personal access token needs the share_group permission. A caller without the Owner role on this group, or without read access to the target group, is answered 404 rather than 403")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return ShareOutput{}, toolutil.WrapErrWithHint("group_share", err,
				"group_access must be one of 10/15/20/25/30/40/50 (Guest/Planner/Reporter/Security Manager/Developer/Maintainer/Owner), or 5 (Minimal access) on a Premium or Ultimate instance; expires_at must be YYYY-MM-DD")
		}
		return ShareOutput{}, toolutil.WrapErrWithStatusHint("group_share", err, http.StatusNotFound,
			"verify group_id and share_group_id with group.get. share_group_id must be a numeric group ID, not a path. GitLab also answers 404 when the caller may not link this group or read the target group, and when the top-level group prevents sharing outside its hierarchy")
	}
	return ShareOutput{
		ID:               g.ID,
		Name:             g.Name,
		Path:             g.Path,
		FullName:         g.FullName,
		FullPath:         g.FullPath,
		Description:      g.Description,
		Visibility:       string(g.Visibility),
		WebURL:           g.WebURL,
		SharedWithGroups: sharedWithGroupsOutput(g.SharedWithGroups),
	}, nil
}

// sharedWithGroupsOutput converts the groups a group is shared with. It is
// never nil: the key is sent on every answer to a share, and an empty list
// says the group is shared with nobody the caller can see, which a missing key
// would not.
func sharedWithGroupsOutput(links []gl.SharedWithGroup) []SharedWithGroupOutput {
	out := make([]SharedWithGroupOutput, len(links))
	for i, link := range links {
		out[i] = SharedWithGroupOutput{
			GroupID:          link.GroupID,
			GroupName:        link.GroupName,
			GroupFullPath:    link.GroupFullPath,
			GroupAccessLevel: link.GroupAccessLevel,
			MemberRoleID:     link.MemberRoleID,
		}
		if link.ExpiresAt != nil {
			out[i].ExpiresAt = link.ExpiresAt.String()
		}
	}
	return out
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
			"the share does not exist. Use group.get to inspect shared_with_groups for the current shares")
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	members, resp, err := client.GL().Groups.ListBillableGroupMembers(
		string(input.GroupID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		return BillableMembersOutput{}, toolutil.WrapErrWithStatusHint("group_billable_members_list", err, http.StatusNotFound,
			"verify group_id with group.get. Billable members are a Premium/Ultimate feature and require Owner access on the group")
	}
	extras, err := toolutil.CapturedBillableMembers(captured, len(members))
	if err != nil {
		return BillableMembersOutput{}, toolutil.WrapErr("group_billable_members_list", err)
	}
	out := BillableMembersOutput{
		Members:    make([]BillableMemberOutput, len(members)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, m := range members {
		out.Members[i] = convertBillableMember(m, extras[i])
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
			"verify group_id with group.get and user_id with group.group_billable_members_list. The user must be a billable member of the group (Premium/Ultimate)")
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
				"only directly removable billable members can be removed here; check the 'removable' flag from group.group_billable_members_list. Inherited members must be removed from their source group")
		}
		return toolutil.WrapErrWithStatusHint("group_billable_member_remove", err, http.StatusNotFound,
			"verify group_id with group.get and user_id with group.group_billable_members_list")
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

// convertMember maps a GitLab group member into the MCP output shape: what
// client-go decoded, and what the capture read beside it.
func convertMember(m *gl.GroupMember, extra toolutil.MemberExtra) Output {
	out := Output{
		ID:                m.ID,
		Username:          m.Username,
		Name:              m.Name,
		State:             m.State,
		Locked:            extra.Locked,
		AvatarURL:         m.AvatarURL,
		WebURL:            m.WebURL,
		AccessLevel:       int(m.AccessLevel),
		Email:             m.Email,
		PublicEmail:       m.PublicEmail,
		TwoFactorEnabled:  extra.TwoFactorEnabled,
		CreatedBy:         memberUserOutput(m.CreatedBy),
		GroupSAMLIdentity: samlIdentityOutput(m.GroupSAMLIdentity),
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

// memberUserOutput mirrors a gl.MemberCreatedBy into the shared output shape.
func memberUserOutput(u *gl.MemberCreatedBy) *MemberUserOutput {
	return toolutil.NewMemberUserOutput(u)
}

// samlIdentityOutput mirrors a gl.GroupMemberSAMLIdentity into the shared
// output shape.
func samlIdentityOutput(s *gl.GroupMemberSAMLIdentity) *SAMLIdentityOutput {
	return toolutil.NewSAMLIdentityOutput(s)
}

// memberRoleOutput mirrors a gl.MemberRole into the shared output shape.
func memberRoleOutput(r *gl.MemberRole) *MemberRoleOutput {
	return toolutil.NewMemberRoleOutput(r)
}

// convertBillableMember maps a gl.BillableGroupMember into the MCP output
// shape (1:1 field fidelity), filling from the decoded member and from what
// the capture read beside it.
func convertBillableMember(m *gl.BillableGroupMember, extra toolutil.BillableMemberExtra) BillableMemberOutput {
	out := BillableMemberOutput{
		ID:             m.ID,
		Username:       m.Username,
		PublicEmail:    extra.PublicEmail,
		Name:           m.Name,
		State:          m.State,
		Locked:         extra.Locked,
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
