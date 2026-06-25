package users

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Output mirrors the top-level fields of gl.User. The GitLab user endpoints
// return full User objects, so this shape reflects gl.User as completely as
// practical: every top-level scalar plus the nested identities, SCIM
// identities, custom attributes, and created_by sub-objects. It is kept in
// sync with the enterpriseusers package UserOutput for cross-domain
// consistency.
type Output struct {
	toolutil.HintableOutput
	ID                             int64                   `json:"id"`
	Username                       string                  `json:"username"`
	Email                          string                  `json:"email"`
	Name                           string                  `json:"name"`
	State                          string                  `json:"state"`
	WebURL                         string                  `json:"web_url"`
	AvatarURL                      string                  `json:"avatar_url"`
	IsAdmin                        bool                    `json:"is_admin"`
	IsAuditor                      bool                    `json:"is_auditor"`
	Bot                            bool                    `json:"bot"`
	Bio                            string                  `json:"bio,omitempty"`
	Location                       string                  `json:"location,omitempty"`
	JobTitle                       string                  `json:"job_title,omitempty"`
	Organization                   string                  `json:"organization,omitempty"`
	CreatedAt                      string                  `json:"created_at,omitempty"`
	ConfirmedAt                    string                  `json:"confirmed_at,omitempty"`
	PublicEmail                    string                  `json:"public_email,omitempty"`
	Skype                          string                  `json:"skype,omitempty"`
	Linkedin                       string                  `json:"linkedin,omitempty"`
	Twitter                        string                  `json:"twitter,omitempty"`
	WebsiteURL                     string                  `json:"website_url,omitempty"`
	ExternUID                      string                  `json:"extern_uid,omitempty"`
	Provider                       string                  `json:"provider,omitempty"`
	LastActivityOn                 string                  `json:"last_activity_on,omitempty"`
	TwoFactorEnabled               bool                    `json:"two_factor_enabled"`
	External                       bool                    `json:"external"`
	Locked                         bool                    `json:"locked"`
	PrivateProfile                 bool                    `json:"private_profile"`
	CurrentSignInAt                string                  `json:"current_sign_in_at,omitempty"`
	CurrentSignInIP                string                  `json:"current_sign_in_ip,omitempty"`
	LastSignInAt                   string                  `json:"last_sign_in_at,omitempty"`
	LastSignInIP                   string                  `json:"last_sign_in_ip,omitempty"`
	ProjectsLimit                  int64                   `json:"projects_limit"`
	CanCreateProject               bool                    `json:"can_create_project"`
	CanCreateGroup                 bool                    `json:"can_create_group"`
	CanCreateOrganization          bool                    `json:"can_create_organization"`
	Note                           string                  `json:"note,omitempty"`
	UsingLicenseSeat               bool                    `json:"using_license_seat"`
	ThemeID                        int64                   `json:"theme_id,omitempty"`
	ColorSchemeID                  int64                   `json:"color_scheme_id,omitempty"`
	SharedRunnersMinutesLimit      int64                   `json:"shared_runners_minutes_limit,omitempty"`
	ExtraSharedRunnersMinutesLimit int64                   `json:"extra_shared_runners_minutes_limit,omitempty"`
	NamespaceID                    int64                   `json:"namespace_id,omitempty"`
	Identities                     []IdentityOutput        `json:"identities,omitempty"`
	SCIMIdentities                 []SCIMIdentityOutput    `json:"scim_identities,omitempty"`
	CustomAttributes               []CustomAttributeOutput `json:"custom_attributes,omitempty"`
	CreatedBy                      *BasicUserOutput        `json:"created_by,omitempty"`
}

// IdentityOutput mirrors gl.UserIdentity, a provider/extern_uid pair linking a
// user to an external authentication source.
type IdentityOutput struct {
	Provider  string `json:"provider"`
	ExternUID string `json:"extern_uid"`
}

// SCIMIdentityOutput represents a SCIM identity associated with a user.
type SCIMIdentityOutput struct {
	ExternUID string `json:"extern_uid"`
	GroupID   int64  `json:"group_id"`
	Active    bool   `json:"active"`
}

// CustomAttributeOutput mirrors gl.CustomAttribute, a key/value custom
// attribute attached to a user.
type CustomAttributeOutput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// BasicUserOutput mirrors gl.BasicUser, the compact user object referenced by
// gl.User.CreatedBy.
type BasicUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// CurrentInput is an empty struct for the current user tool (no parameters needed).
type CurrentInput struct{}

// Current retrieves the currently authenticated GitLab user.
func Current(ctx context.Context, client *gitlabclient.Client, _ CurrentInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	u, _, err := client.GL().Users.CurrentUser(gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("userCurrent", err, http.StatusUnauthorized,
			"verify your token is valid and has read_user or api scope; expired tokens must be refreshed")
	}
	return toOutput(u), nil
}

// List Users.

// ListInput holds parameters for listing GitLab users.
type ListInput struct {
	Search               string `json:"search,omitempty" jsonschema:"Search users by name or username or email"`
	Username             string `json:"username,omitempty" jsonschema:"Filter by exact username"`
	Active               *bool  `json:"active,omitempty" jsonschema:"Filter for active users only"`
	Blocked              *bool  `json:"blocked,omitempty" jsonschema:"Filter for blocked users only"`
	External             *bool  `json:"external,omitempty" jsonschema:"Filter for external users only"`
	Admins               *bool  `json:"admins,omitempty" jsonschema:"Filter for administrators only"`
	Humans               *bool  `json:"humans,omitempty" jsonschema:"Filter for human (non-bot, non-internal) users only"`
	ExcludeActive        *bool  `json:"exclude_active,omitempty" jsonschema:"Exclude active users from the result"`
	ExcludeExternal      *bool  `json:"exclude_external,omitempty" jsonschema:"Exclude external users from the result"`
	ExcludeHumans        *bool  `json:"exclude_humans,omitempty" jsonschema:"Exclude human users from the result"`
	ExcludeInternal      *bool  `json:"exclude_internal,omitempty" jsonschema:"Exclude internal (bot/system) users from the result"`
	WithoutProjects      *bool  `json:"without_projects,omitempty" jsonschema:"Filter for users without any projects"`
	WithoutProjectBots   *bool  `json:"without_project_bots,omitempty" jsonschema:"Exclude project bot users from the result"`
	WithCustomAttributes *bool  `json:"with_custom_attributes,omitempty" jsonschema:"Include custom attributes in the response (admin only)"`
	TwoFactor            string `json:"two_factor,omitempty" jsonschema:"Filter by 2FA status: enabled or disabled"`
	ExternUID            string `json:"extern_uid,omitempty" jsonschema:"Filter by external UID (use with provider)"`
	Provider             string `json:"provider,omitempty" jsonschema:"Filter by external provider name (use with extern_uid)"`
	PublicEmail          string `json:"public_email,omitempty" jsonschema:"Filter by exact public email address"`
	CreatedAfter         string `json:"created_after,omitempty" jsonschema:"Filter users created after this date (RFC3339)"`
	CreatedBefore        string `json:"created_before,omitempty" jsonschema:"Filter users created before this date (RFC3339)"`
	OrderBy              string `json:"order_by,omitempty" jsonschema:"Order by: id | name | username | created_at | updated_at"`
	Sort                 string `json:"sort,omitempty" jsonschema:"Sort order: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListOutput holds a paginated list of users.
type ListOutput struct {
	toolutil.HintableOutput
	Users      []Output                  `json:"users"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves a paginated list of GitLab users.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := buildListUsersOptions(input)

	users, resp, err := client.GL().Users.ListUsers(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_users", err, http.StatusForbidden,
			"listing all users may require admin token on private instances; use search by username/email/two_factor filters to narrow results")
	}

	out := make([]Output, 0, len(users))
	for _, u := range users {
		out = append(out, toOutput(u))
	}
	return ListOutput{
		Users:      out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// buildListUsersOptions maps a ListInput onto the client-go ListUsersOptions,
// including offset and keyset pagination. All optional filters use omitempty
// pointers, so nil values are simply omitted from the request.
func buildListUsersOptions(input ListInput) *gl.ListUsersOptions {
	opts := &gl.ListUsersOptions{
		Active:               input.Active,
		Blocked:              input.Blocked,
		External:             input.External,
		Admins:               input.Admins,
		Humans:               input.Humans,
		ExcludeActive:        input.ExcludeActive,
		ExcludeExternal:      input.ExcludeExternal,
		ExcludeHumans:        input.ExcludeHumans,
		ExcludeInternal:      input.ExcludeInternal,
		WithoutProjects:      input.WithoutProjects,
		WithoutProjectBots:   input.WithoutProjectBots,
		WithCustomAttributes: input.WithCustomAttributes,
		Search:               strPtr(input.Search),
		Username:             strPtr(input.Username),
		TwoFactor:            strPtr(input.TwoFactor),
		ExternalUID:          strPtr(input.ExternUID),
		Provider:             strPtr(input.Provider),
		PublicEmail:          strPtr(input.PublicEmail),
		CreatedAfter:         toolutil.ParseOptionalTime(input.CreatedAfter),
		CreatedBefore:        toolutil.ParseOptionalTime(input.CreatedBefore),
		OrderBy:              strPtr(input.OrderBy),
		Sort:                 strPtr(input.Sort),
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	return opts
}

// applyOrderSort copies the optional order_by and sort parameters onto a
// gl.ListOptions, which exposes these for both offset and keyset pagination on
// endpoints whose options embed ListOptions without dedicated fields.
func applyOrderSort(opts *gl.ListOptions, orderBy, sort string) {
	if orderBy != "" {
		opts.OrderBy = orderBy
	}
	if sort != "" {
		opts.Sort = sort
	}
}

// Get User.

// GetInput holds parameters for retrieving a single user.
type GetInput struct {
	UserID               int64 `json:"user_id" jsonschema:"The ID of the user to retrieve,required"`
	WithCustomAttributes *bool `json:"with_custom_attributes,omitempty" jsonschema:"Include custom attributes in the response (admin only)"`
}

// Get retrieves a single GitLab user by ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.UserID == 0 {
		return Output{}, errors.New("get_user: user_id is required")
	}

	opts := &gl.GetUserOptions{}
	if input.WithCustomAttributes != nil {
		opts.WithCustomAttributes = input.WithCustomAttributes
	}
	u, _, err := client.GL().Users.GetUser(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get_user", err, http.StatusNotFound,
			"verify user_id with gitlab_list_users (search by username); user_id must be a positive integer")
	}
	return toOutput(u), nil
}

// User Status.

// StatusOutput represents a user's status.
type StatusOutput struct {
	toolutil.HintableOutput
	Emoji         string `json:"emoji,omitempty"`
	Availability  string `json:"availability,omitempty"`
	Message       string `json:"message,omitempty"`
	MessageHTML   string `json:"message_html,omitempty"`
	ClearStatusAt string `json:"clear_status_at,omitempty"`
}

// GetStatusInput holds parameters for retrieving a user's status.
type GetStatusInput struct {
	UserID int64 `json:"user_id" jsonschema:"The ID of the user whose status to retrieve,required"`
}

// GetStatus retrieves the status of a specific user.
func GetStatus(ctx context.Context, client *gitlabclient.Client, input GetStatusInput) (StatusOutput, error) {
	if input.UserID == 0 {
		return StatusOutput{}, errors.New("get_user_status: user_id is required")
	}

	s, _, err := client.GL().Users.GetUserStatus(input.UserID, gl.WithContext(ctx))
	if err != nil {
		return StatusOutput{}, toolutil.WrapErrWithStatusHint("get_user_status", err, http.StatusNotFound,
			"verify user_id or username; status may be empty if the user has not set one")
	}
	if s == nil {
		return StatusOutput{}, nil
	}
	return toStatusOutput(s), nil
}

// Set Status.

// SetStatusInput holds parameters for setting the current user's status.
type SetStatusInput struct {
	Emoji            string `json:"emoji,omitempty" jsonschema:"The emoji to set for the status (e.g. coffee or speech_balloon)"`
	Message          string `json:"message,omitempty" jsonschema:"The status message text"`
	Availability     string `json:"availability,omitempty" jsonschema:"The availability: not_set or busy"`
	ClearStatusAfter string `json:"clear_status_after,omitempty" jsonschema:"Duration after which to clear status: 30_minutes | 3_hours | 8_hours | 1_day | 3_days | 7_days | 30_days"`
}

// SetStatus sets the current user's status.
func SetStatus(ctx context.Context, client *gitlabclient.Client, input SetStatusInput) (StatusOutput, error) {
	opts := &gl.UserStatusOptions{}
	if input.Emoji != "" {
		opts.Emoji = new(input.Emoji)
	}
	if input.Message != "" {
		opts.Message = new(input.Message)
	}
	if input.Availability != "" {
		av := gl.AvailabilityValue(input.Availability)
		opts.Availability = &av
	}
	if input.ClearStatusAfter != "" {
		cs := gl.ClearStatusAfterValue(input.ClearStatusAfter)
		opts.ClearStatusAfter = &cs
	}

	s, _, err := client.GL().Users.SetUserStatus(opts, gl.WithContext(ctx))
	if err != nil {
		return StatusOutput{}, toolutil.WrapErrWithStatusHint("set_user_status", err, http.StatusBadRequest,
			"emoji must be a valid name (e.g. 'coffee' or 'palm_tree') without colons; availability must be one of {busy, not_set}; clear_status_after format like '30_minutes', '3_hours', '8_hours', '1_day', '3_days', '7_days', '30_days'")
	}
	if s == nil {
		return StatusOutput{}, nil
	}
	return toStatusOutput(s), nil
}

// SSH Keys.

// SSHKeyOutput represents an SSH key.
type SSHKeyOutput struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Key       string `json:"key"`
	CreatedAt string `json:"created_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	UsageType string `json:"usage_type,omitempty"`
}

// SSHKeyListOutput holds a paginated list of SSH keys.
type SSHKeyListOutput struct {
	toolutil.HintableOutput
	Keys       []SSHKeyOutput            `json:"keys"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListSSHKeysInput holds parameters for listing SSH keys.
type ListSSHKeysInput struct {
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort    string `json:"sort,omitempty" jsonschema:"Sort order for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListSSHKeys retrieves SSH keys for the current authenticated user.
func ListSSHKeys(ctx context.Context, client *gitlabclient.Client, input ListSSHKeysInput) (SSHKeyListOutput, error) {
	opts := &gl.ListSSHKeysOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyOrderSort(&opts.ListOptions, input.OrderBy, input.Sort)

	keys, resp, err := client.GL().Users.ListSSHKeys(opts, gl.WithContext(ctx))
	if err != nil {
		return SSHKeyListOutput{}, toolutil.WrapErrWithStatusHint("list_ssh_keys", err, http.StatusUnauthorized,
			"listing your SSH keys requires a valid token with read_user or api scope")
	}

	out := make([]SSHKeyOutput, 0, len(keys))
	for _, k := range keys {
		out = append(out, toSSHKeyOutput(k))
	}
	return SSHKeyListOutput{
		Keys:       out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// Emails.

// EmailOutput represents an email address.
type EmailOutput struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	ConfirmedAt string `json:"confirmed_at,omitempty"`
}

// EmailListOutput holds a list of emails.
type EmailListOutput struct {
	toolutil.HintableOutput
	Emails []EmailOutput `json:"emails"`
}

// ListEmailsInput is an empty struct for listing current user's emails.
type ListEmailsInput struct{}

// ListEmails retrieves email addresses for the current authenticated user.
func ListEmails(ctx context.Context, client *gitlabclient.Client, _ ListEmailsInput) (EmailListOutput, error) {
	emails, _, err := client.GL().Users.ListEmails(gl.WithContext(ctx))
	if err != nil {
		return EmailListOutput{}, toolutil.WrapErrWithStatusHint("list_emails", err, http.StatusUnauthorized,
			"listing your emails requires a valid token with read_user or api scope")
	}

	out := make([]EmailOutput, 0, len(emails))
	for _, e := range emails {
		o := EmailOutput{ID: e.ID, Email: e.Email}
		if e.ConfirmedAt != nil {
			o.ConfirmedAt = e.ConfirmedAt.Format(time.RFC3339)
		}
		out = append(out, o)
	}
	return EmailListOutput{Emails: out}, nil
}

// Contribution Events.

// ContributionEventOutput represents a user contribution event.
type ContributionEventOutput struct {
	ID          int64  `json:"id"`
	Title       string `json:"title,omitempty"`
	ProjectID   int64  `json:"project_id"`
	ActionName  string `json:"action_name"`
	TargetID    int64  `json:"target_id,omitempty"`
	TargetIID   int64  `json:"target_iid,omitempty"`
	TargetType  string `json:"target_type,omitempty"`
	TargetURL   string `json:"target_url,omitempty"`
	TargetTitle string `json:"target_title,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// ContributionEventsOutput holds a paginated list of contribution events.
type ContributionEventsOutput struct {
	toolutil.HintableOutput
	Events     []ContributionEventOutput `json:"events"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListContributionEventsInput holds parameters for listing user contribution events.
type ListContributionEventsInput struct {
	UserID     int64  `json:"user_id" jsonschema:"The ID of the user whose events to retrieve,required"`
	Action     string `json:"action,omitempty" jsonschema:"Filter by action type: created | updated | closed | reopened | pushed | commented | merged | joined | left | destroyed | expired | approved"`
	TargetType string `json:"target_type,omitempty" jsonschema:"Filter by target type: Issue | Milestone | MergeRequest | Note | Project | Snippet | User"`
	Before     string `json:"before,omitempty" jsonschema:"Only events before this date (YYYY-MM-DD)"`
	After      string `json:"after,omitempty" jsonschema:"Only events after this date (YYYY-MM-DD)"`
	Scope      string `json:"scope,omitempty" jsonschema:"Include all events across a user's projects (e.g. 'all')"`
	OrderBy    string `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort       string `json:"sort,omitempty" jsonschema:"Sort order: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListContributionEvents retrieves contribution events for a user.
func ListContributionEvents(ctx context.Context, client *gitlabclient.Client, input ListContributionEventsInput) (ContributionEventsOutput, error) {
	if input.UserID == 0 {
		return ContributionEventsOutput{}, errors.New("list_contribution_events: user_id is required")
	}

	opts := &gl.ListContributionEventsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Action != "" {
		a := gl.EventTypeValue(input.Action)
		opts.Action = &a
	}
	if input.TargetType != "" {
		t := gl.EventTargetTypeValue(input.TargetType)
		opts.TargetType = &t
	}
	if input.Before != "" {
		if t, err := time.Parse(toolutil.DateFormatISO, input.Before); err == nil {
			d := gl.ISOTime(t)
			opts.Before = &d
		}
	}
	if input.After != "" {
		if t, err := time.Parse(toolutil.DateFormatISO, input.After); err == nil {
			d := gl.ISOTime(t)
			opts.After = &d
		}
	}
	if input.Scope != "" {
		opts.Scope = new(input.Scope)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}

	events, resp, err := client.GL().Users.ListUserContributionEvents(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return ContributionEventsOutput{}, toolutil.WrapErrWithStatusHint("list_contribution_events", err, http.StatusForbidden,
			"verify user_id and that the user's contribution events are visible to your token; private profiles are not visible")
	}

	out := make([]ContributionEventOutput, 0, len(events))
	for _, e := range events {
		o := ContributionEventOutput{
			ID:          e.ID,
			Title:       e.Title,
			ProjectID:   e.ProjectID,
			ActionName:  e.ActionName,
			TargetID:    e.TargetID,
			TargetIID:   e.TargetIID,
			TargetType:  e.TargetType,
			TargetTitle: e.TargetTitle,
		}
		if e.CreatedAt != nil {
			o.CreatedAt = e.CreatedAt.Format(time.RFC3339)
		}
		out = append(out, o)
	}

	enrichContributionEventURLs(ctx, client, out)

	return ContributionEventsOutput{
		Events:     out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// Associations Count.

// AssociationsCountOutput represents a user's association counts.
type AssociationsCountOutput struct {
	toolutil.HintableOutput
	GroupsCount        int64 `json:"groups_count"`
	ProjectsCount      int64 `json:"projects_count"`
	IssuesCount        int64 `json:"issues_count"`
	MergeRequestsCount int64 `json:"merge_requests_count"`
}

// GetAssociationsCountInput holds parameters for getting user association counts.
type GetAssociationsCountInput struct {
	UserID int64 `json:"user_id" jsonschema:"The ID of the user,required"`
}

// GetAssociationsCount retrieves the count of a user's associations.
func GetAssociationsCount(ctx context.Context, client *gitlabclient.Client, input GetAssociationsCountInput) (AssociationsCountOutput, error) {
	if input.UserID == 0 {
		return AssociationsCountOutput{}, errors.New("get_user_associations_count: user_id is required")
	}

	ac, _, err := client.GL().Users.GetUserAssociationsCount(input.UserID, gl.WithContext(ctx))
	if err != nil {
		return AssociationsCountOutput{}, toolutil.WrapErrWithStatusHint("get_user_associations_count", err, http.StatusForbidden,
			"associations count requires admin token; verify user_id with gitlab_get_user")
	}
	return AssociationsCountOutput{
		GroupsCount:        ac.GroupsCount,
		ProjectsCount:      ac.ProjectsCount,
		IssuesCount:        ac.IssuesCount,
		MergeRequestsCount: ac.MergeRequestsCount,
	}, nil
}

// resolveProjectWebURLs fetches the web URL for each unique project ID.
// Failures are silently ignored — missing URLs simply produce no links.
func resolveProjectWebURLs(ctx context.Context, client *gitlabclient.Client, projectIDs []int64) map[int64]string {
	seen := make(map[int64]string, len(projectIDs))
	for _, id := range projectIDs {
		if _, ok := seen[id]; ok || id == 0 {
			continue
		}
		proj, _, err := client.GL().Projects.GetProject(id, &gl.GetProjectOptions{}, gl.WithContext(ctx))
		if err != nil || proj == nil {
			seen[id] = ""
			continue
		}
		seen[id] = proj.WebURL
	}
	return seen
}

// enrichContributionEventURLs resolves project web URLs and sets TargetURL on each event.
func enrichContributionEventURLs(ctx context.Context, client *gitlabclient.Client, events []ContributionEventOutput) {
	ids := make([]int64, 0, len(events))
	for i := range events {
		ids = append(ids, events[i].ProjectID)
	}
	urls := resolveProjectWebURLs(ctx, client, ids)
	for i := range events {
		events[i].TargetURL = toolutil.BuildTargetURL(urls[events[i].ProjectID], events[i].TargetType, events[i].TargetIID)
	}
}

// Conversion helpers.

// toOutput converts a GitLab User to our Output type.
func toOutput(u *gl.User) Output {
	if u == nil {
		return Output{}
	}
	out := Output{
		ID:                             u.ID,
		Username:                       u.Username,
		Email:                          u.Email,
		Name:                           u.Name,
		State:                          u.State,
		WebURL:                         u.WebURL,
		AvatarURL:                      u.AvatarURL,
		IsAdmin:                        u.IsAdmin,
		IsAuditor:                      u.IsAuditor,
		Bot:                            u.Bot,
		Bio:                            u.Bio,
		Location:                       u.Location,
		JobTitle:                       u.JobTitle,
		Organization:                   u.Organization,
		PublicEmail:                    u.PublicEmail,
		Skype:                          u.Skype,
		Linkedin:                       u.Linkedin,
		Twitter:                        u.Twitter,
		WebsiteURL:                     u.WebsiteURL,
		ExternUID:                      u.ExternUID,
		Provider:                       u.Provider,
		TwoFactorEnabled:               u.TwoFactorEnabled,
		External:                       u.External,
		Locked:                         u.Locked,
		PrivateProfile:                 u.PrivateProfile,
		ProjectsLimit:                  u.ProjectsLimit,
		CanCreateProject:               u.CanCreateProject,
		CanCreateGroup:                 u.CanCreateGroup,
		CanCreateOrganization:          u.CanCreateOrganization,
		Note:                           u.Note,
		UsingLicenseSeat:               u.UsingLicenseSeat,
		ThemeID:                        u.ThemeID,
		ColorSchemeID:                  u.ColorSchemeID,
		SharedRunnersMinutesLimit:      u.SharedRunnersMinutesLimit,
		ExtraSharedRunnersMinutesLimit: u.ExtraSharedRunnersMinutesLimit,
		NamespaceID:                    u.NamespaceID,
		Identities:                     toIdentityOutputs(u.Identities),
		SCIMIdentities:                 toSCIMIdentityOutputs(u.SCIMIdentities),
		CustomAttributes:               toCustomAttributeOutputs(u.CustomAttributes),
		CreatedBy:                      toBasicUserOutput(u.CreatedBy),
	}
	if u.CreatedAt != nil {
		out.CreatedAt = u.CreatedAt.Format(time.RFC3339)
	}
	if u.ConfirmedAt != nil {
		out.ConfirmedAt = u.ConfirmedAt.Format(time.RFC3339)
	}
	if u.LastActivityOn != nil {
		out.LastActivityOn = time.Time(*u.LastActivityOn).Format(toolutil.DateFormatISO)
	}
	if u.CurrentSignInAt != nil {
		out.CurrentSignInAt = u.CurrentSignInAt.Format(time.RFC3339)
	}
	if u.LastSignInAt != nil {
		out.LastSignInAt = u.LastSignInAt.Format(time.RFC3339)
	}
	if u.CurrentSignInIP != nil {
		out.CurrentSignInIP = u.CurrentSignInIP.String()
	}
	if u.LastSignInIP != nil {
		out.LastSignInIP = u.LastSignInIP.String()
	}
	return out
}

func toIdentityOutputs(identities []*gl.UserIdentity) []IdentityOutput {
	if len(identities) == 0 {
		return nil
	}
	out := make([]IdentityOutput, 0, len(identities))
	for _, id := range identities {
		if id == nil {
			continue
		}
		out = append(out, IdentityOutput{Provider: id.Provider, ExternUID: id.ExternUID})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toCustomAttributeOutputs(attrs []*gl.CustomAttribute) []CustomAttributeOutput {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]CustomAttributeOutput, 0, len(attrs))
	for _, a := range attrs {
		if a == nil {
			continue
		}
		out = append(out, CustomAttributeOutput{Key: a.Key, Value: a.Value})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toBasicUserOutput(u *gl.BasicUser) *BasicUserOutput {
	if u == nil {
		return nil
	}
	o := &BasicUserOutput{
		ID:        u.ID,
		Username:  u.Username,
		Name:      u.Name,
		State:     u.State,
		AvatarURL: u.AvatarURL,
		WebURL:    u.WebURL,
	}
	if u.CreatedAt != nil {
		o.CreatedAt = u.CreatedAt.Format(time.RFC3339)
	}
	return o
}

func toSCIMIdentityOutputs(identities []*gl.SCIMIdentity) []SCIMIdentityOutput {
	if len(identities) == 0 {
		return nil
	}
	out := make([]SCIMIdentityOutput, 0, len(identities))
	for _, identity := range identities {
		if identity == nil {
			continue
		}
		out = append(out, SCIMIdentityOutput{
			ExternUID: identity.ExternUID,
			GroupID:   identity.GroupID,
			Active:    identity.Active,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// toStatusOutput converts the GitLab API response to the tool output format.
func toStatusOutput(s *gl.UserStatus) StatusOutput {
	o := StatusOutput{
		Emoji:        s.Emoji,
		Availability: string(s.Availability),
		Message:      s.Message,
		MessageHTML:  s.MessageHTML,
	}
	if s.ClearStatusAt != nil {
		o.ClearStatusAt = s.ClearStatusAt.Format(time.RFC3339)
	}
	return o
}

// toSSHKeyOutput converts the GitLab API response to the tool output format.
func toSSHKeyOutput(k *gl.SSHKey) SSHKeyOutput {
	o := SSHKeyOutput{
		ID:        k.ID,
		Title:     k.Title,
		Key:       k.Key,
		UsageType: k.UsageType,
	}
	if k.CreatedAt != nil {
		o.CreatedAt = k.CreatedAt.Format(time.RFC3339)
	}
	if k.ExpiresAt != nil {
		o.ExpiresAt = k.ExpiresAt.Format(time.RFC3339)
	}
	return o
}
