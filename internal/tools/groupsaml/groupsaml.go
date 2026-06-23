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

// SAMLUserOutput represents a SAML-provisioned user of a top-level group.
type SAMLUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state,omitempty"`
	Email     string `json:"email,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
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
	CreatedAfter  string `json:"created_after,omitempty" jsonschema:"Return users created after this timestamp (RFC3339, e.g. 2026-01-02T15:04:05Z)"`
	CreatedBefore string `json:"created_before,omitempty" jsonschema:"Return users created before this timestamp (RFC3339, e.g. 2026-01-02T15:04:05Z)"`
	toolutil.PaginationInput
}

// SAMLUsersList retrieves the users provisioned via SAML SSO for a top-level group.
func SAMLUsersList(ctx context.Context, client *gitlabclient.Client, input SAMLUsersListInput) (SAMLUsersListOutput, error) {
	if input.GroupID == "" {
		return SAMLUsersListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListSAMLUsersOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
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
		item := SAMLUserOutput{
			ID:        u.ID,
			Username:  u.Username,
			Name:      u.Name,
			State:     u.State,
			Email:     u.Email,
			AvatarURL: u.AvatarURL,
			WebURL:    u.WebURL,
		}
		if u.CreatedAt != nil {
			item.CreatedAt = u.CreatedAt.Format(time.RFC3339)
		}
		out.Users[i] = item
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
	AccessLevel   int    `json:"access_level" jsonschema:"Access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner, 25=Security Manager where supported),required"`
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
