package groupsaml

import (
	"context"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const groupSAMLLinkHint = "verify group_id with gitlab_group_get; group SAML links require Premium/Ultimate, Owner access, and group SAML SSO configured for the group; self-managed instances without SAML SSO can return 401 or 404"

// Output represents a single group SAML link.
type Output struct {
	toolutil.HintableOutput
	Name         string `json:"name"`
	AccessLevel  int    `json:"access_level"`
	MemberRoleID int64  `json:"member_role_id,omitempty"`
	Provider     string `json:"provider,omitempty"`
}

// ListOutput holds a list of group SAML links.
type ListOutput struct {
	toolutil.HintableOutput
	Links []Output `json:"links"`
}

func toOutput(l *gl.SAMLGroupLink) Output {
	return Output{
		Name:         l.Name,
		AccessLevel:  int(l.AccessLevel),
		MemberRoleID: l.MemberRoleID,
		Provider:     l.Provider,
	}
}

// ListInput holds parameters for listing group SAML links.
type ListInput struct {
	GroupID string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// List retrieves all SAML links for a group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	links, _, err := client.GL().Groups.ListGroupSAMLLinks(input.GroupID, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithHint("list group SAML links", err, groupSAMLLinkHint)
	}
	out := make([]Output, len(links))
	for i, l := range links {
		out[i] = toOutput(l)
	}
	return ListOutput{Links: out}, nil
}

// SAMLUserOutput represents a SAML-provisioned user of a top-level group. The
// GitLab saml_users endpoint returns the standard user object, so this mirrors
// the canonical user representation (gl.User) field-for-field. The groups
// sub-packages cannot import each other (zero import cycles), so the shape is
// replicated here rather than shared.
type SAMLUserOutput struct {
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
	Skype                          string                  `json:"skype,omitempty"`
	Linkedin                       string                  `json:"linkedin,omitempty"`
	Twitter                        string                  `json:"twitter,omitempty"`
	Provider                       string                  `json:"provider,omitempty"`
	ExternUID                      string                  `json:"extern_uid,omitempty"`
	CreatedAt                      string                  `json:"created_at,omitempty"`
	ConfirmedAt                    string                  `json:"confirmed_at,omitempty"`
	PublicEmail                    string                  `json:"public_email,omitempty"`
	WebsiteURL                     string                  `json:"website_url,omitempty"`
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
	NamespaceID                    int64                   `json:"namespace_id,omitempty"`
	SharedRunnersMinutesLimit      int64                   `json:"shared_runners_minutes_limit,omitempty"`
	ExtraSharedRunnersMinutesLimit int64                   `json:"extra_shared_runners_minutes_limit,omitempty"`
	Identities                     []UserIdentityOutput    `json:"identities,omitempty"`
	SCIMIdentities                 []SCIMIdentityOutput    `json:"scim_identities,omitempty"`
	CustomAttributes               []CustomAttributeOutput `json:"custom_attributes,omitempty"`
	CreatedBy                      *BasicUserOutput        `json:"created_by,omitempty"`
}

// SCIMIdentityOutput represents a SCIM identity associated with a SAML user.
type SCIMIdentityOutput struct {
	ExternUID string `json:"extern_uid"`
	GroupID   int64  `json:"group_id"`
	Active    bool   `json:"active"`
}

// UserIdentityOutput represents a linked external identity (provider, extern_uid).
type UserIdentityOutput struct {
	Provider  string `json:"provider"`
	ExternUID string `json:"extern_uid"`
}

// CustomAttributeOutput represents a single custom attribute key/value pair.
type CustomAttributeOutput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// BasicUserOutput mirrors gl.BasicUser for the created_by reference.
type BasicUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// toSAMLUserOutput maps a GitLab API user into the MCP output, mirroring the
// canonical gl.User representation so every standard user field is surfaced 1:1.
func toSAMLUserOutput(u *gl.User) SAMLUserOutput {
	out := SAMLUserOutput{
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
		Skype:                          u.Skype,
		Linkedin:                       u.Linkedin,
		Twitter:                        u.Twitter,
		Provider:                       u.Provider,
		ExternUID:                      u.ExternUID,
		PublicEmail:                    u.PublicEmail,
		WebsiteURL:                     u.WebsiteURL,
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
		NamespaceID:                    u.NamespaceID,
		SharedRunnersMinutesLimit:      u.SharedRunnersMinutesLimit,
		ExtraSharedRunnersMinutesLimit: u.ExtraSharedRunnersMinutesLimit,
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
	if u.CurrentSignInIP != nil {
		out.CurrentSignInIP = u.CurrentSignInIP.String()
	}
	if u.LastSignInAt != nil {
		out.LastSignInAt = u.LastSignInAt.Format(time.RFC3339)
	}
	if u.LastSignInIP != nil {
		out.LastSignInIP = u.LastSignInIP.String()
	}
	for _, identity := range u.Identities {
		if identity == nil {
			continue
		}
		out.Identities = append(out.Identities, UserIdentityOutput{
			Provider:  identity.Provider,
			ExternUID: identity.ExternUID,
		})
	}
	for _, identity := range u.SCIMIdentities {
		if identity == nil {
			continue
		}
		out.SCIMIdentities = append(out.SCIMIdentities, SCIMIdentityOutput{
			ExternUID: identity.ExternUID,
			GroupID:   identity.GroupID,
			Active:    identity.Active,
		})
	}
	for _, attr := range u.CustomAttributes {
		if attr == nil {
			continue
		}
		out.CustomAttributes = append(out.CustomAttributes, CustomAttributeOutput{
			Key:   attr.Key,
			Value: attr.Value,
		})
	}
	if u.CreatedBy != nil {
		created := &BasicUserOutput{
			ID:        u.CreatedBy.ID,
			Username:  u.CreatedBy.Username,
			Name:      u.CreatedBy.Name,
			State:     u.CreatedBy.State,
			AvatarURL: u.CreatedBy.AvatarURL,
			WebURL:    u.CreatedBy.WebURL,
		}
		if u.CreatedBy.CreatedAt != nil {
			created.CreatedAt = u.CreatedBy.CreatedAt.Format(time.RFC3339)
		}
		out.CreatedBy = created
	}
	return out
}

// SAMLUsersListOutput holds a paginated list of SAML-provisioned group users.
type SAMLUsersListOutput struct {
	toolutil.HintableOutput
	Users      []SAMLUserOutput          `json:"users"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// SAMLUsersListInput holds parameters for listing the SAML users of a top-level group.
type SAMLUsersListInput struct {
	GroupID       string `json:"group_id" jsonschema:"Top-level group ID or URL-encoded path,required"`
	Search        string `json:"search,omitempty" jsonschema:"Filter SAML users by name, username, or public email"`
	Username      string `json:"username,omitempty" jsonschema:"Filter by an exact username"`
	Active        *bool  `json:"active,omitempty" jsonschema:"Limit to active users only"`
	Blocked       *bool  `json:"blocked,omitempty" jsonschema:"Limit to blocked users only"`
	CreatedAfter  string `json:"created_after,omitempty" jsonschema:"Return users created after the specified time. Format: ISO 8601 (YYYY-MM-DDTHH:MM:SSZ)"`
	CreatedBefore string `json:"created_before,omitempty" jsonschema:"Return users created before the specified time. Format: ISO 8601 (YYYY-MM-DDTHH:MM:SSZ)"`
	OrderBy       string `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id, name, username, created_at)"`
	Sort          string `json:"sort,omitempty" jsonschema:"Sort order for keyset pagination: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// SAMLUsersList retrieves the users provisioned via SAML SSO for a top-level group.
func SAMLUsersList(ctx context.Context, client *gitlabclient.Client, input SAMLUsersListInput) (SAMLUsersListOutput, error) {
	if input.GroupID == "" {
		return SAMLUsersListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListSAMLUsersOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
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
	opts.CreatedAfter = toolutil.ParseOptionalTime(input.CreatedAfter)
	opts.CreatedBefore = toolutil.ParseOptionalTime(input.CreatedBefore)
	users, resp, err := client.GL().Groups.ListSAMLUsers(input.GroupID, opts, gl.WithContext(ctx))
	if err != nil {
		return SAMLUsersListOutput{}, toolutil.WrapErrWithHint("list group SAML users", err, groupSAMLLinkHint)
	}
	out := SAMLUsersListOutput{
		Users:      make([]SAMLUserOutput, len(users)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, u := range users {
		out.Users[i] = toSAMLUserOutput(u)
	}
	return out, nil
}

// GetInput holds parameters for getting a single group SAML link.
type GetInput struct {
	GroupID       string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	SAMLGroupName string `json:"saml_group_name" jsonschema:"Name of the SAML group,required"`
}

// Get retrieves a single SAML link for a group.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.SAMLGroupName == "" {
		return Output{}, toolutil.ErrFieldRequired("saml_group_name")
	}
	link, _, err := client.GL().Groups.GetGroupSAMLLink(input.GroupID, input.SAMLGroupName, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint("get group SAML link", err, groupSAMLLinkHint)
	}
	return toOutput(link), nil
}

// AddInput holds parameters for adding a group SAML link.
type AddInput struct {
	GroupID       string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	SAMLGroupName string `json:"saml_group_name" jsonschema:"Name of the SAML group,required"`
	AccessLevel   int    `json:"access_level" jsonschema:"Access level (0=No access, 5=Minimal access, 10=Guest, 15=Planner, 20=Reporter, 25=Security Manager, 30=Developer, 40=Maintainer, 50=Owner),required"`
	MemberRoleID  *int64 `json:"member_role_id,omitempty" jsonschema:"Custom member role ID"`
	Provider      string `json:"provider,omitempty" jsonschema:"SAML provider name"`
}

// Add creates a new group SAML link.
func Add(ctx context.Context, client *gitlabclient.Client, input AddInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.SAMLGroupName == "" {
		return Output{}, toolutil.ErrFieldRequired("saml_group_name")
	}
	access := gl.AccessLevelValue(input.AccessLevel)
	opts := &gl.AddGroupSAMLLinkOptions{
		SAMLGroupName: &input.SAMLGroupName,
		AccessLevel:   &access,
		MemberRoleID:  input.MemberRoleID,
	}
	if input.Provider != "" {
		opts.Provider = &input.Provider
	}
	link, _, err := client.GL().Groups.AddGroupSAMLLink(input.GroupID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint("add group SAML link", err, groupSAMLLinkHint)
	}
	return toOutput(link), nil
}

// DeleteInput holds parameters for deleting a group SAML link.
type DeleteInput struct {
	GroupID       string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	SAMLGroupName string `json:"saml_group_name" jsonschema:"Name of the SAML group to delete,required"`
}

// Delete removes a group SAML link.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.GroupID == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if input.SAMLGroupName == "" {
		return toolutil.ErrFieldRequired("saml_group_name")
	}
	_, err := client.GL().Groups.DeleteGroupSAMLLink(input.GroupID, input.SAMLGroupName, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithHint("delete group SAML link", err, groupSAMLLinkHint)
	}
	return nil
}
