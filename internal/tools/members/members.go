// Package members implements MCP tool handlers for GitLab project member
// operations including listing all members (with inherited) and providing
// human-readable access level descriptions. It wraps the ProjectMembers
// service from client-go v2.
package members

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput defines parameters for listing project members.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Query     string               `json:"query,omitempty" jsonschema:"Filter members by name or username"`
	toolutil.PaginationInput
}

// Output represents a project or group member.
type Output struct {
	toolutil.HintableOutput
	ID                     int64  `json:"id"`
	Username               string `json:"username"`
	Name                   string `json:"name"`
	State                  string `json:"state"`
	AvatarURL              string `json:"avatar_url,omitempty"`
	AccessLevel            int    `json:"access_level"`
	AccessLevelDescription string `json:"access_level_description"`
	WebURL                 string `json:"web_url"`
	CreatedAt              string `json:"created_at,omitempty"`
	ExpiresAt              string `json:"expires_at,omitempty"`
	Email                  string `json:"email,omitempty"`
	MemberRoleName         string `json:"member_role_name,omitempty"`
	IsUsingSeat            bool   `json:"is_using_seat,omitempty"`
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

// ToOutput converts a GitLab API [gl.ProjectMember] to the MCP
// tool output format, including a human-readable access level description
// derived from the numeric access level value.
func ToOutput(m *gl.ProjectMember) Output {
	out := Output{
		ID:                     m.ID,
		Username:               m.Username,
		Name:                   m.Name,
		State:                  m.State,
		AvatarURL:              m.AvatarURL,
		AccessLevel:            int(m.AccessLevel),
		AccessLevelDescription: toolutil.AccessLevelDescription(m.AccessLevel),
		WebURL:                 m.WebURL,
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

// List retrieves all members of a GitLab project, including
// inherited members from parent groups. Supports filtering by name or
// username via the Query field and pagination. Returns the member list
// with pagination metadata.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("projectMembersList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	opts := &gl.ListProjectMembersOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	if input.Query != "" {
		opts.Query = new(input.Query)
	}

	members, resp, err := client.GL().ProjectMembers.ListAllProjectMembers(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("projectMembersList", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; lists direct + inherited members from parent groups")
	}

	out := ListOutput{
		Members:    make([]Output, len(members)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, m := range members {
		out.Members[i] = ToOutput(m)
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
	AccessLevel  int                  `json:"access_level"            jsonschema:"Access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner)"`
	ExpiresAt    string               `json:"expires_at,omitempty"    jsonschema:"Membership expiration date (YYYY-MM-DD)"`
	MemberRoleID int64                `json:"member_role_id,omitempty" jsonschema:"Custom member role ID"`
}

// EditInput defines parameters for editing a project member.
type EditInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	UserID       int64                `json:"user_id"                 jsonschema:"User ID of the member to edit,required"`
	AccessLevel  int                  `json:"access_level"            jsonschema:"New access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner)"`
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

// Get retrieves a single project member by user ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, errors.New("memberGet: project_id is required")
	}
	if input.UserID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("memberGet", "user_id")
	}

	m, _, err := client.GL().ProjectMembers.GetProjectMember(string(input.ProjectID), input.UserID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("memberGet", err, http.StatusNotFound,
			"user is not a direct member of this project; use gitlab_project_member_get_inherited to include parent-group inheritance, or gitlab_project_members_list to enumerate members")
	}
	return ToOutput(m), nil
}

// GetInherited retrieves a project member including inherited membership.
func GetInherited(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, errors.New("memberGetInherited: project_id is required")
	}
	if input.UserID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("memberGetInherited", "user_id")
	}

	m, _, err := client.GL().ProjectMembers.GetInheritedProjectMember(string(input.ProjectID), input.UserID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("memberGetInherited", err, http.StatusNotFound,
			"user is not a member of this project nor any parent group; verify user_id with gitlab_list_users")
	}
	return ToOutput(m), nil
}

// Add adds a user as a member of a project.
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

	m, _, err := client.GL().ProjectMembers.AddProjectMember(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		switch {
		case toolutil.IsHTTPStatus(err, http.StatusConflict):
			return Output{}, toolutil.WrapErrWithHint("memberAdd", err, "user is already a member of this project — use gitlab_project_member_edit to change their access level")
		case toolutil.IsHTTPStatus(err, http.StatusNotFound):
			return Output{}, toolutil.WrapErrWithHint("memberAdd", err, "user not found — use gitlab_list_users to search for the user")
		default:
			return Output{}, toolutil.WrapErrWithMessage("memberAdd", err)
		}
	}
	return ToOutput(m), nil
}

// Edit modifies an existing project member's access level or expiration.
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

	m, _, err := client.GL().ProjectMembers.EditProjectMember(string(input.ProjectID), input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("memberEdit", err, "you need at least the same or higher access level as the target member")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("memberEdit", err, http.StatusBadRequest,
			"access_level must be one of 10/20/30/40/50; expires_at must be YYYY-MM-DD format; member_role_id (if provided) must exist for the namespace (Premium/Ultimate)")
	}
	return ToOutput(m), nil
}

// Delete removes a member from a project.
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
