// Package groupmembers provides MCP tool handlers for GitLab group member operations.
package groupmembers

import (
	"context"
	"errors"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ──────────────────────────────────────────────
// Output types
// ──────────────────────────────────────────────.

// Output represents a single group member.
type Output struct {
	toolutil.HintableOutput
	ID                     int64  `json:"id"`
	Username               string `json:"username"`
	Name                   string `json:"name"`
	State                  string `json:"state"`
	AvatarURL              string `json:"avatar_url,omitempty"`
	WebURL                 string `json:"web_url"`
	AccessLevel            int    `json:"access_level"`
	AccessLevelDescription string `json:"access_level_description"`
	CreatedAt              string `json:"created_at,omitempty"`
	ExpiresAt              string `json:"expires_at,omitempty"`
	Email                  string `json:"email,omitempty"`
	MemberRoleName         string `json:"member_role_name,omitempty"`
	IsUsingSeat            bool   `json:"is_using_seat,omitempty"`
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
	GroupID     toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	UserID      int64                `json:"user_id,omitempty" jsonschema:"User ID to add,required"`
	Username    string               `json:"username,omitempty" jsonschema:"Username to add (alternative to user_id)"`
	AccessLevel int                  `json:"access_level" jsonschema:"Access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner)"`
	ExpiresAt   string               `json:"expires_at,omitempty" jsonschema:"Membership expiration date (YYYY-MM-DD)"`
}

// EditInput contains parameters for editing a group member.
type EditInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	UserID      int64                `json:"user_id" jsonschema:"User ID,required"`
	AccessLevel int                  `json:"access_level,omitempty" jsonschema:"New access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner)"`
	ExpiresAt   string               `json:"expires_at,omitempty" jsonschema:"New membership expiration date (YYYY-MM-DD)"`
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
	GroupAccess  int                  `json:"group_access" jsonschema:"Access level for the shared group (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer)"`
	ExpiresAt    string               `json:"expires_at,omitempty" jsonschema:"Share expiration date (YYYY-MM-DD)"`
}

// UnshareInput contains parameters for unsharing a group.
type UnshareInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	ShareGroupID int64                `json:"share_group_id" jsonschema:"Group ID to stop sharing with,required"`
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
		return Output{}, toolutil.WrapErrWithMessage("group_member_get", err)
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
		return Output{}, toolutil.WrapErrWithMessage("group_member_get_inherited", err)
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
	m, _, err := client.GL().GroupMembers.AddGroupMember(
		string(input.GroupID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("group_member_add", err)
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
	m, _, err := client.GL().GroupMembers.EditGroupMember(
		string(input.GroupID), input.UserID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("group_member_edit", err)
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
		return toolutil.WrapErrWithMessage("group_member_remove", err)
	}
	return nil
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
		return ShareOutput{}, toolutil.WrapErrWithMessage("group_share", err)
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
		return toolutil.WrapErrWithMessage("group_unshare", err)
	}
	return nil
}

// ──────────────────────────────────────────────
// Converters
// ──────────────────────────────────────────────.

// groupAccessLevelNames maps GitLab access level values to human-readable labels.
var groupAccessLevelNames = map[gl.AccessLevelValue]string{
	gl.NoPermissions:            "No access",
	gl.MinimalAccessPermissions: "Minimal access",
	gl.GuestPermissions:         "Guest",
	gl.ReporterPermissions:      "Reporter",
	gl.DeveloperPermissions:     "Developer",
	gl.MaintainerPermissions:    "Maintainer",
	gl.OwnerPermissions:         "Owner",
}

// accessLevelDescription is an internal helper for the groupmembers package.
func accessLevelDescription(level gl.AccessLevelValue) string {
	if name, ok := groupAccessLevelNames[level]; ok {
		return name
	}
	return "Unknown"
}

// convertMember is an internal helper for the groupmembers package.
func convertMember(m *gl.GroupMember) Output {
	out := Output{
		ID:                     m.ID,
		Username:               m.Username,
		Name:                   m.Name,
		State:                  m.State,
		AvatarURL:              m.AvatarURL,
		WebURL:                 m.WebURL,
		AccessLevel:            int(m.AccessLevel),
		AccessLevelDescription: accessLevelDescription(m.AccessLevel),
		Email:                  m.Email,
	}
	if m.CreatedAt != nil {
		out.CreatedAt = m.CreatedAt.Format(time.RFC3339)
	}
	if m.ExpiresAt != nil {
		out.ExpiresAt = m.ExpiresAt.String()
	}
	if m.MemberRole != nil {
		out.MemberRoleName = m.MemberRole.Name
	}
	out.IsUsingSeat = m.IsUsingSeat
	return out
}

// ──────────────────────────────────────────────
// Markdown formatters
// ──────────────────────────────────────────────.
