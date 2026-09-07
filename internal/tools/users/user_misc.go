package users

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// CurrentUserStatus retrieves the status of the current authenticated user.
func CurrentUserStatus(ctx context.Context, client *gitlabclient.Client, _ CurrentInput) (StatusOutput, error) {
	if err := ctx.Err(); err != nil {
		return StatusOutput{}, err
	}

	s, _, err := client.GL().Users.CurrentUserStatus(gl.WithContext(ctx))
	if err != nil {
		return StatusOutput{}, toolutil.WrapErrWithStatusHint("current_user_status", err, http.StatusUnauthorized,
			"verify your token is valid with read_user or api scope")
	}
	return toStatusOutput(s), nil
}

// UploadCurrentUserAvatarInput defines parameters for uploading the current
// authenticated user's avatar. Exactly one of FilePath or ContentBase64 must be
// provided. There is no user identifier: the GitLab endpoint always targets the
// token's own user (PUT /user/avatar).
type UploadCurrentUserAvatarInput struct {
	Filename      string `json:"filename" jsonschema:"Avatar filename (e.g. avatar.png),required"`
	FilePath      string `json:"file_path,omitempty" jsonschema:"Absolute path to a local image file on the MCP server filesystem. Alternative to content_base64 for files too large to base64-encode. Only one of file_path or content_base64 should be provided."`
	ContentBase64 string `json:"content_base64,omitempty" jsonschema:"Base64-encoded image content. Only one of file_path or content_base64 should be provided."`
}

// UploadCurrentUserAvatar uploads or replaces the avatar for the current
// authenticated user. Accepts either file_path (local file) or content_base64
// (base64-encoded string), mirroring the project avatar upload tool.
func UploadCurrentUserAvatar(ctx context.Context, client *gitlabclient.Client, input UploadCurrentUserAvatarInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.Filename == "" {
		return Output{}, errors.New("upload_user_avatar: filename is required")
	}

	reader, _, cleanup, err := toolutil.OpenFileOrBase64Source("upload_user_avatar", input.FilePath, input.ContentBase64)
	if err != nil {
		return Output{}, err
	}
	defer cleanup()

	u, _, err := client.GL().Users.UploadAvatar(reader, input.Filename, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("upload_user_avatar", err,
				"avatar must be JPG/PNG/GIF and under 200 KB; verify filename has a valid image extension")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("upload_user_avatar", err, http.StatusUnauthorized,
			"verify your token is valid with the api scope; the avatar is set for the token's own user")
	}
	out := toOutput(u)
	// GitLab 19 responds with only {avatar_url}, so the decoded user ID is
	// legitimately zero even though the upload succeeded.
	if out.ID == 0 && out.AvatarURL != "" {
		out.NextSteps = []string{
			"The avatar was updated; GitLab 19 returns only avatar_url for this endpoint. Use gitlab_user_current to fetch the caller's full profile",
		}
	}
	return out, nil
}

// UserActivityOutput represents a user activity entry.
type UserActivityOutput struct {
	Username       string `json:"username"`
	LastActivityOn string `json:"last_activity_on,omitempty"`
}

// UserActivitiesOutput holds a paginated list of user activities.
type UserActivitiesOutput struct {
	Activities []UserActivityOutput      `json:"activities"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// GetUserActivitiesInput holds parameters for listing user activities (admin only).
type GetUserActivitiesInput struct {
	From    string `json:"from,omitempty" jsonschema:"Only activities after this date (YYYY-MM-DD)"`
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort    string `json:"sort,omitempty" jsonschema:"Sort order for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetUserActivities retrieves user activity entries (admin only).
func GetUserActivities(ctx context.Context, client *gitlabclient.Client, input GetUserActivitiesInput) (UserActivitiesOutput, error) {
	if err := ctx.Err(); err != nil {
		return UserActivitiesOutput{}, err
	}

	opts := &gl.GetUserActivitiesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyOrderSort(&opts.ListOptions, input.OrderBy, input.Sort)
	if input.From != "" {
		d := gl.ISOTime(parseDate(input.From))
		opts.From = &d
	}

	activities, resp, err := client.GL().Users.GetUserActivities(opts, gl.WithContext(ctx))
	if err != nil {
		return UserActivitiesOutput{}, toolutil.WrapErrWithStatusHint("get_user_activities", err, http.StatusForbidden,
			"user activities require admin token; from format YYYY-MM-DD; activity is reported as last_activity_on date")
	}

	out := make([]UserActivityOutput, 0, len(activities))
	for _, a := range activities {
		o := UserActivityOutput{Username: a.Username}
		if a.LastActivityOn != nil {
			o.LastActivityOn = time.Time(*a.LastActivityOn).Format(toolutil.DateFormatISO)
		}
		out = append(out, o)
	}
	return UserActivitiesOutput{
		Activities: out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// UserMembershipOutput represents a user's membership in a project or group.
type UserMembershipOutput struct {
	SourceID    int64  `json:"source_id"`
	SourceName  string `json:"source_name"`
	SourceType  string `json:"source_type"`
	AccessLevel int64  `json:"access_level"`
}

// UserMembershipsOutput holds a paginated list of user memberships.
type UserMembershipsOutput struct {
	Memberships []UserMembershipOutput    `json:"memberships"`
	Pagination  toolutil.PaginationOutput `json:"pagination"`
}

// GetUserMembershipsInput holds parameters for listing a user's memberships.
type GetUserMembershipsInput struct {
	UserID  int64  `json:"user_id" jsonschema:"The ID of the user,required"`
	Type    string `json:"type,omitempty" jsonschema:"Filter by membership type: Project or Namespace"`
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort    string `json:"sort,omitempty" jsonschema:"Sort order for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetUserMemberships retrieves a user's project and group memberships.
func GetUserMemberships(ctx context.Context, client *gitlabclient.Client, input GetUserMembershipsInput) (UserMembershipsOutput, error) {
	if input.UserID == 0 {
		return UserMembershipsOutput{}, errors.New("get_user_memberships: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return UserMembershipsOutput{}, err
	}

	opts := &gl.GetUserMembershipOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyOrderSort(&opts.ListOptions, input.OrderBy, input.Sort)
	if input.Type != "" {
		opts.Type = new(input.Type)
	}

	memberships, resp, err := client.GL().Users.GetUserMemberships(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return UserMembershipsOutput{}, toolutil.WrapErrWithStatusHint("get_user_memberships", err, http.StatusForbidden,
			"user memberships require admin token; type filter must be one of {Project, Namespace}; verify user_id with gitlab_get_user")
	}

	out := make([]UserMembershipOutput, 0, len(memberships))
	for _, m := range memberships {
		out = append(out, UserMembershipOutput{
			SourceID:    m.SourceID,
			SourceName:  m.SourceName,
			SourceType:  m.SourceType,
			AccessLevel: int64(m.AccessLevel),
		})
	}
	return UserMembershipsOutput{
		Memberships: out,
		Pagination:  toolutil.PaginationFromResponse(resp),
	}, nil
}

// UserRunnerOutput represents a GitLab runner linked to the current user.
type UserRunnerOutput struct {
	ID             int64  `json:"id"`
	Token          string `json:"token"`
	TokenExpiresAt string `json:"token_expires_at,omitempty"`
}

// CreateUserRunnerInput holds parameters for creating a runner linked to a user.
type CreateUserRunnerInput struct {
	RunnerType      string   `json:"runner_type" jsonschema:"Runner type: instance_type or group_type or project_type,required"`
	GroupID         *int64   `json:"group_id,omitempty" jsonschema:"Group ID (required for group_type runners)"`
	ProjectID       *int64   `json:"project_id,omitempty" jsonschema:"Project ID (required for project_type runners)"`
	Description     string   `json:"description,omitempty" jsonschema:"Runner description"`
	Paused          *bool    `json:"paused,omitempty" jsonschema:"Whether the runner should be paused"`
	Locked          *bool    `json:"locked,omitempty" jsonschema:"Whether the runner should be locked"`
	RunUntagged     *bool    `json:"run_untagged,omitempty" jsonschema:"Whether the runner can run untagged jobs"`
	TagList         []string `json:"tag_list,omitempty" jsonschema:"List of runner tags"`
	AccessLevel     string   `json:"access_level,omitempty" jsonschema:"Access level: not_protected or ref_protected"`
	MaximumTimeout  *int64   `json:"maximum_timeout,omitempty" jsonschema:"Maximum timeout for jobs in seconds"`
	MaintenanceNote string   `json:"maintenance_note,omitempty" jsonschema:"Maintenance note for the runner"`
}

// CreateUserRunner creates a runner linked to the current user.
func CreateUserRunner(ctx context.Context, client *gitlabclient.Client, input CreateUserRunnerInput) (UserRunnerOutput, error) {
	if input.RunnerType == "" {
		return UserRunnerOutput{}, errors.New("create_user_runner: runner_type is required")
	}
	if err := ctx.Err(); err != nil {
		return UserRunnerOutput{}, err
	}

	opts := &gl.CreateUserRunnerOptions{
		RunnerType: new(input.RunnerType),
	}
	if input.GroupID != nil {
		opts.GroupID = input.GroupID
	}
	if input.ProjectID != nil {
		opts.ProjectID = input.ProjectID
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Paused != nil {
		opts.Paused = input.Paused
	}
	if input.Locked != nil {
		opts.Locked = input.Locked
	}
	if input.RunUntagged != nil {
		opts.RunUntagged = input.RunUntagged
	}
	if len(input.TagList) > 0 {
		opts.TagList = &input.TagList
	}
	if input.AccessLevel != "" {
		opts.AccessLevel = new(input.AccessLevel)
	}
	if input.MaximumTimeout != nil {
		opts.MaximumTimeout = input.MaximumTimeout
	}
	if input.MaintenanceNote != "" {
		opts.MaintenanceNote = new(input.MaintenanceNote)
	}

	r, _, err := client.GL().Users.CreateUserRunner(opts, gl.WithContext(ctx))
	if err != nil {
		return UserRunnerOutput{}, toolutil.WrapErrWithStatusHint("create_user_runner", err, http.StatusBadRequest,
			"runner_type must be one of {instance_type, group_type, project_type}; group_id required for group_type, project_id for project_type; tag_list optional; access_level one of {not_protected, ref_protected}")
	}

	out := UserRunnerOutput{ID: r.ID, Token: r.Token}
	if r.TokenExpiresAt != nil {
		out.TokenExpiresAt = r.TokenExpiresAt.Format(time.RFC3339)
	}
	return out, nil
}

// DeleteUserIdentityInput holds parameters for deleting a user's identity provider.
type DeleteUserIdentityInput struct {
	UserID   int64  `json:"user_id" jsonschema:"The ID of the user,required"`
	Provider string `json:"provider" jsonschema:"The external provider name (e.g. ldap or saml),required"`
}

// DeleteUserIdentityOutput represents the result of deleting a user identity.
type DeleteUserIdentityOutput struct {
	UserID   int64  `json:"user_id"`
	Provider string `json:"provider"`
	Deleted  bool   `json:"deleted"`
}

// DeleteUserIdentity deletes a user's identity provider (admin only).
func DeleteUserIdentity(ctx context.Context, client *gitlabclient.Client, input DeleteUserIdentityInput) (DeleteUserIdentityOutput, error) {
	if input.UserID == 0 {
		return DeleteUserIdentityOutput{}, errors.New("delete_user_identity: user_id is required")
	}
	if input.Provider == "" {
		return DeleteUserIdentityOutput{}, errors.New("delete_user_identity: provider is required")
	}
	if err := ctx.Err(); err != nil {
		return DeleteUserIdentityOutput{}, err
	}

	_, err := client.GL().Users.DeleteUserIdentity(input.UserID, input.Provider, gl.WithContext(ctx))
	if err != nil {
		return DeleteUserIdentityOutput{}, toolutil.WrapErrWithStatusHint("delete_user_identity", err, http.StatusForbidden,
			"deleting user identity providers requires admin token; verify user_id and provider name (e.g. 'github', 'google_oauth2', 'ldapmain'); identity must currently be associated")
	}
	return DeleteUserIdentityOutput{
		UserID:   input.UserID,
		Provider: input.Provider,
		Deleted:  true,
	}, nil
}

// Markdown helpers for misc tools.

// FormatUserActivitiesMarkdownString renders user activities as a Markdown string.
func FormatUserActivitiesMarkdownString(o UserActivitiesOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## User Activities (%d)\n\n", len(o.Activities))
	toolutil.WriteListSummary(&b, len(o.Activities), o.Pagination)
	if len(o.Activities) == 0 {
		b.WriteString("No user activities found.\n")
	} else {
		b.WriteString("| Username | Last Activity |\n")
		b.WriteString("|---|---|\n")
		for _, a := range o.Activities {
			fmt.Fprintf(&b, "| %s | %s |\n",
				//gitlab:allow-unescaped a.LastActivityOn: a date this package formatted itself, with time.Time.Format on the ISO date layout.
				toolutil.EscapeMdTableCell(a.Username), a.LastActivityOn)
		}
	}
	toolutil.WritePagination(&b, o.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_get_user` to view full details for a user",
	)
	return b.String()
}

// FormatUserMembershipsMarkdownString renders user memberships as a Markdown string.
func FormatUserMembershipsMarkdownString(o UserMembershipsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## User Memberships (%d)\n\n", len(o.Memberships))
	toolutil.WriteListSummary(&b, len(o.Memberships), o.Pagination)
	if len(o.Memberships) == 0 {
		b.WriteString("No memberships found.\n")
	} else {
		b.WriteString("| Source ID | Source Name | Source Type | Access Level |\n")
		b.WriteString("|---|---|---|---|\n")
		for _, m := range o.Memberships {
			fmt.Fprintf(&b, "| %d | %s | %s | %d |\n",
				//gitlab:allow-unescaped m.SourceType: GitLab names the membership source Project or Namespace, and nothing else.
				m.SourceID, toolutil.EscapeMdTableCell(m.SourceName), m.SourceType, m.AccessLevel)
		}
	}
	toolutil.WritePagination(&b, o.Pagination)
	toolutil.WriteHints(
		&b,
		"Use `gitlab_get_user` to view the user's profile",
	)
	return b.String()
}

// FormatUserRunnerMarkdownString renders a user runner as a Markdown string.
func FormatUserRunnerMarkdownString(o UserRunnerOutput) string {
	var b strings.Builder
	b.WriteString("## User Runner Created\n\n")
	fmt.Fprintf(&b, toolutil.FmtMdID, o.ID)
	//gitlab:allow-unescaped o.Token: the runner token GitLab minted, which the reader has to copy back verbatim.
	fmt.Fprintf(&b, "- **Token**: %s\n", o.Token)
	if o.TokenExpiresAt != "" {
		fmt.Fprintf(&b, "- **Token Expires At**: %s\n", toolutil.FormatTime(o.TokenExpiresAt))
	}
	toolutil.WriteHints(
		&b,
		"Save the runner token. It cannot be retrieved again",
	)
	return b.String()
}

// FormatDeleteUserIdentityMarkdownString renders a delete identity result as Markdown.
func FormatDeleteUserIdentityMarkdownString(o DeleteUserIdentityOutput) string {
	return fmt.Sprintf("## User Identity Deleted\n\n"+
		toolutil.FmtMdID+
		"- **Provider**: %s\n"+
		"- **Deleted**: %s %v\n",
		// The provider is the caller's own argument echoed back, not a response
		// field, so nothing in this repository constrains it.
		o.UserID, toolutil.EscapeMdTableCell(o.Provider), toolutil.EmojiSuccess, o.Deleted)
}

// parseDate parses a YYYY-MM-DD string to time.Time, returning zero on failure.
func parseDate(s string) time.Time {
	t, _ := time.Parse(toolutil.DateFormatISO, s)
	return t
}
