package enterpriseusers

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const hintVerifyEnterpriseUser = "verify user_id with gitlab_enterprise_user action 'list' or gitlab_list_enterprise_users; enterprise user actions only apply to users managed by the group's enterprise namespace"

// ListInput holds parameters for listing enterprise users.
type ListInput struct {
	GroupID       toolutil.StringOrInt `json:"group_id"        jsonschema:"Group ID or URL-encoded path,required"`
	Username      string               `json:"username,omitempty" jsonschema:"Filter by exact username"`
	Search        string               `json:"search,omitempty"   jsonschema:"Search by name or username or email"`
	Active        *bool                `json:"active,omitempty"   jsonschema:"Filter for active users only"`
	Blocked       *bool                `json:"blocked,omitempty"  jsonschema:"Filter for blocked users only"`
	CreatedAfter  string               `json:"created_after,omitempty"  jsonschema:"Filter users created after this date (ISO 8601)"`
	CreatedBefore string               `json:"created_before,omitempty" jsonschema:"Filter users created before this date (ISO 8601)"`
	TwoFactor     string               `json:"two_factor,omitempty"     jsonschema:"Filter by 2FA status: enabled or disabled"`
	OrderBy       string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort          string               `json:"sort,omitempty"     jsonschema:"Sort order for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetInput holds parameters for getting a single enterprise user.
type GetInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	UserID  int64                `json:"user_id"  jsonschema:"User ID,required"`
}

// Disable2FAInput holds parameters for disabling 2FA for an enterprise user.
type Disable2FAInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	UserID  int64                `json:"user_id"  jsonschema:"User ID,required"`
}

// DeleteInput holds parameters for deleting an enterprise user.
type DeleteInput struct {
	GroupID    toolutil.StringOrInt `json:"group_id"    jsonschema:"Group ID or URL-encoded path,required"`
	UserID     int64                `json:"user_id"     jsonschema:"User ID,required"`
	HardDelete *bool                `json:"hard_delete,omitempty" jsonschema:"Permanently delete user instead of soft delete"`
}

// Output mirrors the top-level fields of gl.User returned by the GitLab
// group_enterprise_users endpoints. Enterprise-user endpoints return full
// User objects, so this shape reflects gl.User as completely as practical:
// every top-level scalar plus the nested identities, SCIM identities, custom
// attributes, and created_by sub-objects.
type Output struct {
	toolutil.HintableOutput
	ID                             int64                   `json:"id"`
	Username                       string                  `json:"username"`
	Email                          string                  `json:"email"`
	Name                           string                  `json:"name"`
	State                          string                  `json:"state"`
	WebURL                         string                  `json:"web_url"`
	AvatarURL                      string                  `json:"avatar_url,omitempty"`
	Bio                            string                  `json:"bio,omitempty"`
	Bot                            bool                    `json:"bot"`
	Location                       string                  `json:"location,omitempty"`
	PublicEmail                    string                  `json:"public_email,omitempty"`
	Skype                          string                  `json:"skype,omitempty"`
	Linkedin                       string                  `json:"linkedin,omitempty"`
	Twitter                        string                  `json:"twitter,omitempty"`
	WebsiteURL                     string                  `json:"website_url,omitempty"`
	Organization                   string                  `json:"organization,omitempty"`
	JobTitle                       string                  `json:"job_title,omitempty"`
	ExternUID                      string                  `json:"extern_uid,omitempty"`
	Provider                       string                  `json:"provider,omitempty"`
	ThemeID                        int64                   `json:"theme_id,omitempty"`
	LastActivityOn                 string                  `json:"last_activity_on,omitempty"`
	ColorSchemeID                  int64                   `json:"color_scheme_id,omitempty"`
	IsAdmin                        bool                    `json:"is_admin"`
	IsAuditor                      bool                    `json:"is_auditor"`
	CanCreateGroup                 bool                    `json:"can_create_group"`
	CanCreateProject               bool                    `json:"can_create_project"`
	CanCreateOrganization          bool                    `json:"can_create_organization"`
	ProjectsLimit                  int64                   `json:"projects_limit"`
	CurrentSignInAt                string                  `json:"current_sign_in_at,omitempty"`
	CurrentSignInIP                string                  `json:"current_sign_in_ip,omitempty"`
	LastSignInAt                   string                  `json:"last_sign_in_at,omitempty"`
	LastSignInIP                   string                  `json:"last_sign_in_ip,omitempty"`
	ConfirmedAt                    string                  `json:"confirmed_at,omitempty"`
	TwoFactorEnabled               bool                    `json:"two_factor_enabled"`
	Note                           string                  `json:"note,omitempty"`
	Identities                     []IdentityOutput        `json:"identities,omitempty"`
	SCIMIdentities                 []SCIMIdentityOutput    `json:"scim_identities,omitempty"`
	External                       bool                    `json:"external"`
	PrivateProfile                 bool                    `json:"private_profile"`
	SharedRunnersMinutesLimit      int64                   `json:"shared_runners_minutes_limit,omitempty"`
	ExtraSharedRunnersMinutesLimit int64                   `json:"extra_shared_runners_minutes_limit,omitempty"`
	UsingLicenseSeat               bool                    `json:"using_license_seat"`
	CustomAttributes               []CustomAttributeOutput `json:"custom_attributes,omitempty"`
	NamespaceID                    int64                   `json:"namespace_id,omitempty"`
	Locked                         bool                    `json:"locked"`
	CreatedBy                      *BasicUserOutput        `json:"created_by,omitempty"`
	CreatedAt                      string                  `json:"created_at,omitempty"`
}

// IdentityOutput mirrors gl.UserIdentity, a provider/extern_uid pair linking a
// user to an external authentication source.
type IdentityOutput struct {
	Provider  string `json:"provider"`
	ExternUID string `json:"extern_uid"`
}

// SCIMIdentityOutput mirrors gl.SCIMIdentity, a SCIM provisioning identity for
// an enterprise user.
type SCIMIdentityOutput struct {
	ExternUID string `json:"extern_uid"`
	GroupID   int64  `json:"group_id"`
	Active    bool   `json:"active"`
}

// CustomAttributeOutput mirrors gl.CustomAttribute, a key/value custom
// attribute attached to a user. Canonical shape shared via toolutil.
type CustomAttributeOutput = toolutil.CustomAttributeOutput

// BasicUserOutput mirrors gl.BasicUser, the compact user object referenced by
// gl.User.CreatedBy. Canonical shape shared via toolutil.
type BasicUserOutput = toolutil.UserRefOutput

// ListOutput holds the list response.
type ListOutput struct {
	toolutil.HintableOutput
	Users      []Output                  `json:"users"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

func toOutput(u *gl.User) Output {
	if u == nil {
		return Output{}
	}
	o := Output{
		ID:                             u.ID,
		Username:                       u.Username,
		Email:                          u.Email,
		Name:                           u.Name,
		State:                          u.State,
		WebURL:                         u.WebURL,
		AvatarURL:                      u.AvatarURL,
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
		CanCreateGroup:                 u.CanCreateGroup,
		CanCreateProject:               u.CanCreateProject,
		CanCreateOrganization:          u.CanCreateOrganization,
		ProjectsLimit:                  u.ProjectsLimit,
		TwoFactorEnabled:               u.TwoFactorEnabled,
		Note:                           u.Note,
		External:                       u.External,
		PrivateProfile:                 u.PrivateProfile,
		SharedRunnersMinutesLimit:      u.SharedRunnersMinutesLimit,
		ExtraSharedRunnersMinutesLimit: u.ExtraSharedRunnersMinutesLimit,
		UsingLicenseSeat:               u.UsingLicenseSeat,
		NamespaceID:                    u.NamespaceID,
		Locked:                         u.Locked,
		Identities:                     toIdentityOutputs(u.Identities),
		SCIMIdentities:                 toSCIMIdentityOutputs(u.SCIMIdentities),
		CustomAttributes:               toCustomAttributeOutputs(u.CustomAttributes),
		CreatedBy:                      toBasicUserOutput(u.CreatedBy),
	}
	if u.CreatedAt != nil {
		o.CreatedAt = u.CreatedAt.Format(time.RFC3339)
	}
	if u.LastActivityOn != nil {
		o.LastActivityOn = time.Time(*u.LastActivityOn).Format(toolutil.DateFormatISO)
	}
	if u.CurrentSignInAt != nil {
		o.CurrentSignInAt = u.CurrentSignInAt.Format(time.RFC3339)
	}
	if u.LastSignInAt != nil {
		o.LastSignInAt = u.LastSignInAt.Format(time.RFC3339)
	}
	if u.ConfirmedAt != nil {
		o.ConfirmedAt = u.ConfirmedAt.Format(time.RFC3339)
	}
	if u.CurrentSignInIP != nil {
		o.CurrentSignInIP = u.CurrentSignInIP.String()
	}
	if u.LastSignInIP != nil {
		o.LastSignInIP = u.LastSignInIP.String()
	}
	return o
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

func toSCIMIdentityOutputs(identities []*gl.SCIMIdentity) []SCIMIdentityOutput {
	if len(identities) == 0 {
		return nil
	}
	out := make([]SCIMIdentityOutput, 0, len(identities))
	for _, id := range identities {
		if id == nil {
			continue
		}
		out = append(out, SCIMIdentityOutput{ExternUID: id.ExternUID, GroupID: id.GroupID, Active: id.Active})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// toCustomAttributeOutputs converts a []*gl.CustomAttribute slice into the
// shared output shape.
func toCustomAttributeOutputs(attrs []*gl.CustomAttribute) []CustomAttributeOutput {
	return toolutil.NewCustomAttributeOutputs(attrs)
}

// toBasicUserOutput converts a *gl.BasicUser into the shared user-reference
// shape, or nil.
func toBasicUserOutput(u *gl.BasicUser) *BasicUserOutput {
	return toolutil.NewUserRefOutput(u)
}

// List returns all enterprise users for a group.
func List(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if in.GroupID.String() == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListEnterpriseUsersOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, in.PaginationInput, in.KeysetPaginationInput)
	if in.OrderBy != "" {
		opts.OrderBy = in.OrderBy
	}
	if in.Sort != "" {
		opts.Sort = in.Sort
	}
	if in.Username != "" {
		opts.Username = in.Username
	}
	if in.Search != "" {
		opts.Search = in.Search
	}
	if in.Active != nil && *in.Active {
		opts.Active = true
	}
	if in.Blocked != nil && *in.Blocked {
		opts.Blocked = true
	}
	if in.TwoFactor != "" {
		opts.TwoFactor = in.TwoFactor
	}
	if in.CreatedAfter != "" {
		t, err := time.Parse(time.RFC3339, in.CreatedAfter)
		if err != nil {
			return ListOutput{}, errors.New("created_after must be a valid ISO 8601 date")
		}
		opts.CreatedAfter = &t
	}
	if in.CreatedBefore != "" {
		t, err := time.Parse(time.RFC3339, in.CreatedBefore)
		if err != nil {
			return ListOutput{}, errors.New("created_before must be a valid ISO 8601 date")
		}
		opts.CreatedBefore = &t
	}
	users, resp, err := client.GL().EnterpriseUsers.ListEnterpriseUsers(in.GroupID.String(), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list enterprise users", err, http.StatusNotFound, "verify group_id; enterprise users require Ultimate license")
	}
	out := ListOutput{
		Users:      make([]Output, 0, len(users)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, u := range users {
		out.Users = append(out.Users, toOutput(u))
	}
	return out, nil
}

// Get returns details for a single enterprise user.
func Get(ctx context.Context, client *gitlabclient.Client, in GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.GroupID.String() == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if in.UserID == 0 {
		return Output{}, toolutil.ErrFieldRequired("user_id")
	}
	u, _, err := client.GL().EnterpriseUsers.GetEnterpriseUser(in.GroupID.String(), in.UserID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint("get enterprise user", err, hintVerifyEnterpriseUser)
	}
	return toOutput(u), nil
}

// Disable2FA disables two-factor authentication for an enterprise user.
func Disable2FA(ctx context.Context, client *gitlabclient.Client, in Disable2FAInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if in.GroupID.String() == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if in.UserID == 0 {
		return toolutil.ErrFieldRequired("user_id")
	}
	_, err := client.GL().EnterpriseUsers.Disable2FAForEnterpriseUser(in.GroupID.String(), in.UserID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithHint("disable 2FA for enterprise user", err, hintVerifyEnterpriseUser)
	}
	return nil
}

// Delete removes an enterprise user.
func Delete(ctx context.Context, client *gitlabclient.Client, in DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if in.GroupID.String() == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if in.UserID == 0 {
		return toolutil.ErrFieldRequired("user_id")
	}
	opts := &gl.DeleteEnterpriseUserOptions{}
	if in.HardDelete != nil {
		opts.HardDelete = in.HardDelete
	}
	_, err := client.GL().EnterpriseUsers.DeleteEnterpriseUser(in.GroupID.String(), in.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithHint("delete enterprise user", err, hintVerifyEnterpriseUser+"; this action is irreversible")
	}
	return nil
}
