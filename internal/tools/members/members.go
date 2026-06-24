package members

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// Output represents a project or group member.
type Output struct {
	toolutil.HintableOutput
	ID          int64             `json:"id"`
	Username    string            `json:"username"`
	Name        string            `json:"name"`
	State       string            `json:"state"`
	AvatarURL   string            `json:"avatar_url,omitempty"`
	AccessLevel int               `json:"access_level"`
	WebURL      string            `json:"web_url"`
	CreatedAt   string            `json:"created_at,omitempty"`
	CreatedBy   *CreatedByOutput  `json:"created_by,omitempty"`
	ExpiresAt   string            `json:"expires_at,omitempty"`
	Email       string            `json:"email,omitempty"`
	MemberRole  *MemberRoleOutput `json:"member_role,omitempty"`
	IsUsingSeat bool              `json:"is_using_seat,omitempty"`
}

// CreatedByOutput mirrors [gl.MemberCreatedBy], the user who created the
// membership record (1:1 SDK fidelity).
type CreatedByOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// MemberRoleOutput mirrors [gl.MemberRole], the custom member role attached
// to a membership (Premium/Ultimate). All permission flags are surfaced for
// 1:1 SDK fidelity.
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

// createdByOutput converts [gl.MemberCreatedBy] into the local mirror, or nil.
func createdByOutput(c *gl.MemberCreatedBy) *CreatedByOutput {
	if c == nil {
		return nil
	}
	return &CreatedByOutput{
		ID: c.ID, Username: c.Username, Name: c.Name,
		State: c.State, AvatarURL: c.AvatarURL, WebURL: c.WebURL,
	}
}

// memberRoleOutput converts [gl.MemberRole] into the local mirror, or nil.
func memberRoleOutput(r *gl.MemberRole) *MemberRoleOutput {
	if r == nil {
		return nil
	}
	return &MemberRoleOutput{
		ID: r.ID, Name: r.Name, Description: r.Description, GroupID: r.GroupID,
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
// member_role sub-objects. The numeric access level is surfaced directly;
// callers can map it via [toolutil.AccessLevelDescription] when a label is
// needed.
func ToOutput(m *gl.ProjectMember) Output {
	out := Output{
		ID:          m.ID,
		Username:    m.Username,
		Name:        m.Name,
		State:       m.State,
		AvatarURL:   m.AvatarURL,
		AccessLevel: int(m.AccessLevel),
		WebURL:      m.WebURL,
		Email:       m.Email,
		CreatedBy:   createdByOutput(m.CreatedBy),
		MemberRole:  memberRoleOutput(m.MemberRole),
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
	AccessLevel  int                  `json:"access_level"            jsonschema:"Access level (5=Minimal access, 10=Guest, 15=Planner (Premium/Ultimate), 20=Reporter, 25=Security Manager (Premium/Ultimate), 30=Developer, 40=Maintainer, 50=Owner, 60=Admin where supported)"`
	ExpiresAt    string               `json:"expires_at,omitempty"    jsonschema:"Membership expiration date (YYYY-MM-DD)"`
	MemberRoleID int64                `json:"member_role_id,omitempty" jsonschema:"Custom member role ID"`
}

// EditInput defines parameters for editing a project member.
type EditInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	UserID       int64                `json:"user_id"                 jsonschema:"User ID of the member to edit,required"`
	AccessLevel  int                  `json:"access_level"            jsonschema:"New access level (5=Minimal access, 10=Guest, 15=Planner (Premium/Ultimate), 20=Reporter, 25=Security Manager (Premium/Ultimate), 30=Developer, 40=Maintainer, 50=Owner, 60=Admin where supported)"`
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

	m, _, err := client.GL().ProjectMembers.GetProjectMember(string(input.ProjectID), input.UserID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("memberGet", err, http.StatusNotFound,
			"user is not a direct member of this project; use gitlab_project_member_get_inherited to include parent-group inheritance, or gitlab_project_members_list to enumerate members")
	}
	return ToOutput(m), nil
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

	m, _, err := client.GL().ProjectMembers.GetInheritedProjectMember(string(input.ProjectID), input.UserID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("memberGetInherited", err, http.StatusNotFound,
			"user is not a member of this project nor any parent group; verify user_id with gitlab_list_users")
	}
	return ToOutput(m), nil
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

	m, _, err := client.GL().ProjectMembers.EditProjectMember(string(input.ProjectID), input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("memberEdit", err, "you need at least the same or higher access level as the target member")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("memberEdit", err, http.StatusBadRequest,
			"access_level must be one of 5/10/15/20/25/30/40/50/60 (Minimal/Guest/Planner/Reporter/Security Manager/Developer/Maintainer/Owner/Admin where supported); expires_at must be YYYY-MM-DD format; member_role_id (if provided) must exist for the namespace (Premium/Ultimate)")
	}
	return ToOutput(m), nil
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
