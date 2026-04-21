// Package users implements GitLab user operations including retrieving the current
// authenticated user.
package users

import (
	"context"
	"errors"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Output represents the current authenticated GitLab user.
type Output struct {
	toolutil.HintableOutput
	ID               int64  `json:"id"`
	Username         string `json:"username"`
	Email            string `json:"email"`
	Name             string `json:"name"`
	State            string `json:"state"`
	WebURL           string `json:"web_url"`
	AvatarURL        string `json:"avatar_url"`
	IsAdmin          bool   `json:"is_admin"`
	Bot              bool   `json:"bot"`
	Bio              string `json:"bio,omitempty"`
	Location         string `json:"location,omitempty"`
	JobTitle         string `json:"job_title,omitempty"`
	Organization     string `json:"organization,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
	PublicEmail      string `json:"public_email,omitempty"`
	WebsiteURL       string `json:"website_url,omitempty"`
	LastActivityOn   string `json:"last_activity_on,omitempty"`
	TwoFactorEnabled bool   `json:"two_factor_enabled"`
	External         bool   `json:"external"`
	Locked           bool   `json:"locked"`
	PrivateProfile   bool   `json:"private_profile"`
	CurrentSignInAt  string `json:"current_sign_in_at,omitempty"`
	ProjectsLimit    int64  `json:"projects_limit"`
	CanCreateProject bool   `json:"can_create_project"`
	CanCreateGroup   bool   `json:"can_create_group"`
	Note             string `json:"note,omitempty"`
	UsingLicenseSeat bool   `json:"using_license_seat"`
	ThemeID          int64  `json:"theme_id,omitempty"`
	ColorSchemeID    int64  `json:"color_scheme_id,omitempty"`
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
		return Output{}, toolutil.WrapErrWithMessage("userCurrent", err)
	}
	return toOutput(u), nil
}

// List Users.

// ListInput holds parameters for listing GitLab users.
type ListInput struct {
	Search   string `json:"search,omitempty" jsonschema:"Search users by name or username or email"`
	Username string `json:"username,omitempty" jsonschema:"Filter by exact username"`
	Active   *bool  `json:"active,omitempty" jsonschema:"Filter for active users only"`
	Blocked  *bool  `json:"blocked,omitempty" jsonschema:"Filter for blocked users only"`
	External *bool  `json:"external,omitempty" jsonschema:"Filter for external users only"`
	OrderBy  string `json:"order_by,omitempty" jsonschema:"Order by: id | name | username | created_at | updated_at"`
	Sort     string `json:"sort,omitempty" jsonschema:"Sort order: asc or desc"`
	Page     int64  `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage  int64  `json:"per_page,omitempty" jsonschema:"Number of items per page (max 100)"`
}

// ListOutput holds a paginated list of users.
type ListOutput struct {
	toolutil.HintableOutput
	Users      []Output                  `json:"users"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves a paginated list of GitLab users.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListUsersOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.Username != "" {
		opts.Username = new(input.Username)
	}
	if input.Active != nil {
		opts.Active = input.Active
	}
	if input.Blocked != nil {
		opts.Blocked = input.Blocked
	}
	if input.External != nil {
		opts.External = input.External
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}

	users, resp, err := client.GL().Users.ListUsers(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list_users", err)
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

// Get User.

// GetInput holds parameters for retrieving a single user.
type GetInput struct {
	UserID int64 `json:"user_id" jsonschema:"The ID of the user to retrieve,required"`
}

// Get retrieves a single GitLab user by ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.UserID == 0 {
		return Output{}, errors.New("get_user: user_id is required")
	}

	u, _, err := client.GL().Users.GetUser(input.UserID, &gl.GetUserOptions{}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("get_user", err)
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
		return StatusOutput{}, toolutil.WrapErrWithMessage("get_user_status", err)
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
		return StatusOutput{}, toolutil.WrapErrWithMessage("set_user_status", err)
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
	Page    int64 `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int64 `json:"per_page,omitempty" jsonschema:"Number of items per page (max 100)"`
}

// ListSSHKeys retrieves SSH keys for the current authenticated user.
func ListSSHKeys(ctx context.Context, client *gitlabclient.Client, input ListSSHKeysInput) (SSHKeyListOutput, error) {
	opts := &gl.ListSSHKeysOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
	}

	keys, resp, err := client.GL().Users.ListSSHKeys(opts, gl.WithContext(ctx))
	if err != nil {
		return SSHKeyListOutput{}, toolutil.WrapErrWithMessage("list_ssh_keys", err)
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
		return EmailListOutput{}, toolutil.WrapErrWithMessage("list_emails", err)
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
	Sort       string `json:"sort,omitempty" jsonschema:"Sort order: asc or desc"`
	Page       int64  `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage    int64  `json:"per_page,omitempty" jsonschema:"Number of items per page (max 100)"`
}

// ListContributionEvents retrieves contribution events for a user.
func ListContributionEvents(ctx context.Context, client *gitlabclient.Client, input ListContributionEventsInput) (ContributionEventsOutput, error) {
	if input.UserID == 0 {
		return ContributionEventsOutput{}, errors.New("list_contribution_events: user_id is required")
	}

	opts := &gl.ListContributionEventsOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
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
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}

	events, resp, err := client.GL().Users.ListUserContributionEvents(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return ContributionEventsOutput{}, toolutil.WrapErrWithMessage("list_contribution_events", err)
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
		return AssociationsCountOutput{}, toolutil.WrapErrWithMessage("get_user_associations_count", err)
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
	out := Output{
		ID:               u.ID,
		Username:         u.Username,
		Email:            u.Email,
		Name:             u.Name,
		State:            u.State,
		WebURL:           u.WebURL,
		AvatarURL:        u.AvatarURL,
		IsAdmin:          u.IsAdmin,
		Bot:              u.Bot,
		Bio:              u.Bio,
		Location:         u.Location,
		JobTitle:         u.JobTitle,
		Organization:     u.Organization,
		PublicEmail:      u.PublicEmail,
		WebsiteURL:       u.WebsiteURL,
		TwoFactorEnabled: u.TwoFactorEnabled,
		External:         u.External,
		Locked:           u.Locked,
	}
	if u.CreatedAt != nil {
		out.CreatedAt = u.CreatedAt.Format(time.RFC3339)
	}
	if u.LastActivityOn != nil {
		out.LastActivityOn = time.Time(*u.LastActivityOn).Format(toolutil.DateFormatISO)
	}
	out.PrivateProfile = u.PrivateProfile
	if u.CurrentSignInAt != nil {
		out.CurrentSignInAt = u.CurrentSignInAt.Format(time.RFC3339)
	}
	out.ProjectsLimit = u.ProjectsLimit
	out.CanCreateProject = u.CanCreateProject
	out.CanCreateGroup = u.CanCreateGroup
	out.Note = u.Note
	out.UsingLicenseSeat = u.UsingLicenseSeat
	out.ThemeID = u.ThemeID
	out.ColorSchemeID = u.ColorSchemeID
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
