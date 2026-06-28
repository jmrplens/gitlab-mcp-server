package groups

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ListProvisionedUsersInput defines parameters for listing the users provisioned
// for a group through SAML/SCIM. Mirrors gl.ListProvisionedUsersOptions filters
// plus offset and keyset pagination.
type ListProvisionedUsersInput struct {
	GroupID       toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Username      string               `json:"username,omitempty" jsonschema:"Filter provisioned users by exact username"`
	Search        string               `json:"search,omitempty" jsonschema:"Search provisioned users by name, username, or email"`
	Active        *bool                `json:"active,omitempty" jsonschema:"Filter to active (true) or inactive (false) users"`
	Blocked       *bool                `json:"blocked,omitempty" jsonschema:"Filter to blocked (true) or unblocked (false) users"`
	CreatedAfter  string               `json:"created_after,omitempty" jsonschema:"Return users created after this RFC3339 timestamp (e.g. 2024-01-02T15:04:05Z)"`
	CreatedBefore string               `json:"created_before,omitempty" jsonschema:"Return users created before this RFC3339 timestamp (e.g. 2024-12-31T23:59:59Z)"`
	OrderBy       string               `json:"order_by,omitempty" jsonschema:"Column to order provisioned users by (e.g. id, name, username, created_at, updated_at)"`
	Sort          string               `json:"sort,omitempty" jsonschema:"Sort direction: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ProvisionedUserOutput mirrors gl.User 1:1 for users provisioned through
// SAML/SCIM. Nested identities (identities, scim_identities, custom_attributes,
// created_by) are surfaced as full local mirrors on their canonical json keys
// (C-IMPORTS: replicated here rather than imported from sibling packages to
// preserve the zero-import-cycle constraint).
type ProvisionedUserOutput struct {
	ID                             int64                            `json:"id"`
	Username                       string                           `json:"username"`
	Email                          string                           `json:"email,omitempty"`
	Name                           string                           `json:"name"`
	State                          string                           `json:"state"`
	WebURL                         string                           `json:"web_url,omitempty"`
	CreatedAt                      string                           `json:"created_at,omitempty"`
	Bio                            string                           `json:"bio,omitempty"`
	Bot                            bool                             `json:"bot,omitempty"`
	Location                       string                           `json:"location,omitempty"`
	PublicEmail                    string                           `json:"public_email,omitempty"`
	Skype                          string                           `json:"skype,omitempty"`
	Linkedin                       string                           `json:"linkedin,omitempty"`
	Twitter                        string                           `json:"twitter,omitempty"`
	WebsiteURL                     string                           `json:"website_url,omitempty"`
	Organization                   string                           `json:"organization,omitempty"`
	JobTitle                       string                           `json:"job_title,omitempty"`
	ExternUID                      string                           `json:"extern_uid,omitempty"`
	Provider                       string                           `json:"provider,omitempty"`
	ThemeID                        int64                            `json:"theme_id,omitempty"`
	LastActivityOn                 string                           `json:"last_activity_on,omitempty"`
	ColorSchemeID                  int64                            `json:"color_scheme_id,omitempty"`
	IsAdmin                        bool                             `json:"is_admin,omitempty"`
	IsAuditor                      bool                             `json:"is_auditor,omitempty"`
	AvatarURL                      string                           `json:"avatar_url,omitempty"`
	CanCreateGroup                 bool                             `json:"can_create_group,omitempty"`
	CanCreateProject               bool                             `json:"can_create_project,omitempty"`
	CanCreateOrganization          bool                             `json:"can_create_organization,omitempty"`
	ProjectsLimit                  int64                            `json:"projects_limit,omitempty"`
	CurrentSignInAt                string                           `json:"current_sign_in_at,omitempty"`
	CurrentSignInIP                string                           `json:"current_sign_in_ip,omitempty"`
	LastSignInAt                   string                           `json:"last_sign_in_at,omitempty"`
	LastSignInIP                   string                           `json:"last_sign_in_ip,omitempty"`
	ConfirmedAt                    string                           `json:"confirmed_at,omitempty"`
	TwoFactorEnabled               bool                             `json:"two_factor_enabled,omitempty"`
	Note                           string                           `json:"note,omitempty"`
	Identities                     []ProvisionedUserIdentity        `json:"identities,omitempty"`
	SCIMIdentities                 []ProvisionedUserSCIMIdentity    `json:"scim_identities,omitempty"`
	External                       bool                             `json:"external,omitempty"`
	PrivateProfile                 bool                             `json:"private_profile,omitempty"`
	SharedRunnersMinutesLimit      int64                            `json:"shared_runners_minutes_limit,omitempty"`
	ExtraSharedRunnersMinutesLimit int64                            `json:"extra_shared_runners_minutes_limit,omitempty"`
	UsingLicenseSeat               bool                             `json:"using_license_seat,omitempty"`
	CustomAttributes               []ProvisionedUserCustomAttribute `json:"custom_attributes,omitempty"`
	NamespaceID                    int64                            `json:"namespace_id,omitempty"`
	Locked                         bool                             `json:"locked,omitempty"`
	CreatedBy                      *ProvisionedUserBasicUser        `json:"created_by,omitempty"`
}

// ProvisionedUserIdentity mirrors gl.UserIdentity (the identities object).
type ProvisionedUserIdentity struct {
	Provider  string `json:"provider"`
	ExternUID string `json:"extern_uid"`
}

// ProvisionedUserSCIMIdentity mirrors gl.SCIMIdentity (the scim_identities object).
type ProvisionedUserSCIMIdentity struct {
	ExternUID string `json:"extern_uid"`
	GroupID   int64  `json:"group_id"`
	Active    bool   `json:"active"`
}

// ProvisionedUserCustomAttribute mirrors gl.CustomAttribute (custom_attributes).
type ProvisionedUserCustomAttribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ProvisionedUserBasicUser mirrors gl.BasicUser (the created_by object).
type ProvisionedUserBasicUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// ProvisionedUsersListOutput holds a paginated list of provisioned users.
type ProvisionedUsersListOutput struct {
	toolutil.HintableOutput
	Users      []ProvisionedUserOutput   `json:"users"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ProvisionedUserToOutput converts a gl.User to the full provisioned-user output
// shape (1:1 audit policy: full nested objects).
func ProvisionedUserToOutput(u *gl.User) ProvisionedUserOutput {
	out := ProvisionedUserOutput{
		ID:                             u.ID,
		Username:                       u.Username,
		Email:                          u.Email,
		Name:                           u.Name,
		State:                          u.State,
		WebURL:                         u.WebURL,
		Bio:                            u.Bio,
		Bot:                            u.Bot,
		Location:                       u.Location,
		PublicEmail:                    u.PublicEmail,
		Skype:                          u.Skype,
		Linkedin:                       u.Linkedin,
		Twitter:                        u.Twitter,
		WebsiteURL:                     u.WebsiteURL,
		Organization:                   u.Organization,
		JobTitle:                       u.JobTitle,
		ExternUID:                      u.ExternUID,
		Provider:                       u.Provider,
		ThemeID:                        u.ThemeID,
		ColorSchemeID:                  u.ColorSchemeID,
		IsAdmin:                        u.IsAdmin,
		IsAuditor:                      u.IsAuditor,
		AvatarURL:                      u.AvatarURL,
		CanCreateGroup:                 u.CanCreateGroup,
		CanCreateProject:               u.CanCreateProject,
		CanCreateOrganization:          u.CanCreateOrganization,
		ProjectsLimit:                  u.ProjectsLimit,
		TwoFactorEnabled:               u.TwoFactorEnabled,
		Note:                           u.Note,
		Identities:                     provisionedUserIdentities(u.Identities),
		SCIMIdentities:                 provisionedUserSCIMIdentities(u.SCIMIdentities),
		External:                       u.External,
		PrivateProfile:                 u.PrivateProfile,
		SharedRunnersMinutesLimit:      u.SharedRunnersMinutesLimit,
		ExtraSharedRunnersMinutesLimit: u.ExtraSharedRunnersMinutesLimit,
		UsingLicenseSeat:               u.UsingLicenseSeat,
		CustomAttributes:               provisionedUserCustomAttributes(u.CustomAttributes),
		NamespaceID:                    u.NamespaceID,
		Locked:                         u.Locked,
		CreatedBy:                      provisionedUserBasicUser(u.CreatedBy),
	}
	if u.CreatedAt != nil {
		out.CreatedAt = u.CreatedAt.Format(time.RFC3339)
	}
	if u.LastActivityOn != nil {
		out.LastActivityOn = u.LastActivityOn.String()
	}
	if u.CurrentSignInAt != nil {
		out.CurrentSignInAt = u.CurrentSignInAt.Format(time.RFC3339)
	}
	if u.CurrentSignInIP != nil {
		out.CurrentSignInIP = u.CurrentSignInIP.String()
	}
	if u.LastSignInAt != nil {
		out.LastSignInAt = u.LastSignInAt.Format(time.RFC3339)
	}
	if u.LastSignInIP != nil {
		out.LastSignInIP = u.LastSignInIP.String()
	}
	if u.ConfirmedAt != nil {
		out.ConfirmedAt = u.ConfirmedAt.Format(time.RFC3339)
	}
	return out
}

func provisionedUserIdentities(in []*gl.UserIdentity) []ProvisionedUserIdentity {
	if len(in) == 0 {
		return nil
	}
	out := make([]ProvisionedUserIdentity, 0, len(in))
	for _, id := range in {
		if id == nil {
			continue
		}
		out = append(out, ProvisionedUserIdentity{Provider: id.Provider, ExternUID: id.ExternUID})
	}
	return out
}

func provisionedUserSCIMIdentities(in []*gl.SCIMIdentity) []ProvisionedUserSCIMIdentity {
	if len(in) == 0 {
		return nil
	}
	out := make([]ProvisionedUserSCIMIdentity, 0, len(in))
	for _, id := range in {
		if id == nil {
			continue
		}
		out = append(out, ProvisionedUserSCIMIdentity{ExternUID: id.ExternUID, GroupID: id.GroupID, Active: id.Active})
	}
	return out
}

func provisionedUserCustomAttributes(in []*gl.CustomAttribute) []ProvisionedUserCustomAttribute {
	if len(in) == 0 {
		return nil
	}
	out := make([]ProvisionedUserCustomAttribute, 0, len(in))
	for _, a := range in {
		if a == nil {
			continue
		}
		out = append(out, ProvisionedUserCustomAttribute{Key: a.Key, Value: a.Value})
	}
	return out
}

func provisionedUserBasicUser(u *gl.BasicUser) *ProvisionedUserBasicUser {
	if u == nil {
		return nil
	}
	out := &ProvisionedUserBasicUser{
		ID:        u.ID,
		Username:  u.Username,
		Name:      u.Name,
		State:     u.State,
		AvatarURL: u.AvatarURL,
		WebURL:    u.WebURL,
	}
	if u.CreatedAt != nil {
		out.CreatedAt = u.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// ListProvisionedUsers lists the users provisioned for a group through SAML/SCIM
// (Premium/Ultimate). Supports username/search/active/blocked/created_after/
// created_before filters plus offset and keyset pagination.
func ListProvisionedUsers(ctx context.Context, client *gitlabclient.Client, input ListProvisionedUsersInput) (ProvisionedUsersListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProvisionedUsersListOutput{}, err
	}
	if input.GroupID == "" {
		return ProvisionedUsersListOutput{}, errors.New("ListProvisionedUsers: group_id is required. Use gitlab_group_list to find the ID first, then pass it as group_id")
	}

	opts, err := provisionedUsersOptions(input)
	if err != nil {
		return ProvisionedUsersListOutput{}, err
	}

	users, resp, err := client.GL().Groups.ListProvisionedUsers(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ProvisionedUsersListOutput{}, toolutil.WrapErrWithStatusHint("ListProvisionedUsers", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get; provisioned users require a SAML/SCIM-enabled group and Owner role (Premium/Ultimate)")
	}

	out := ProvisionedUsersListOutput{
		Users:      make([]ProvisionedUserOutput, len(users)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, u := range users {
		out.Users[i] = ProvisionedUserToOutput(u)
	}
	return out, nil
}

// provisionedUsersOptions builds the ListProvisionedUsers options from the input,
// applying offset/keyset pagination and every supported filter. Split out to keep
// the handler's cyclomatic complexity flat.
func provisionedUsersOptions(input ListProvisionedUsersInput) (*gl.ListProvisionedUsersOptions, error) {
	opts := &gl.ListProvisionedUsersOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.Username != "" {
		opts.Username = new(input.Username)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.Active != nil {
		opts.Active = input.Active
	}
	if input.Blocked != nil {
		opts.Blocked = input.Blocked
	}
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.CreatedAfter != "" {
		t, err := time.Parse(time.RFC3339, input.CreatedAfter)
		if err != nil {
			return nil, errors.New("ListProvisionedUsers: created_after must be an RFC3339 timestamp (e.g. 2024-01-02T15:04:05Z)")
		}
		opts.CreatedAfter = &t
	}
	if input.CreatedBefore != "" {
		t, err := time.Parse(time.RFC3339, input.CreatedBefore)
		if err != nil {
			return nil, errors.New("ListProvisionedUsers: created_before must be an RFC3339 timestamp (e.g. 2024-12-31T23:59:59Z)")
		}
		opts.CreatedBefore = &t
	}
	return opts, nil
}
