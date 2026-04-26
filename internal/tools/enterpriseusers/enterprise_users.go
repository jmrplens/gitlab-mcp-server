// Package enterpriseusers implements GitLab enterprise user operations for groups
// including list, get, disable 2FA, and delete.
package enterpriseusers

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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
	toolutil.PaginationInput
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

// Output represents an enterprise user.
type Output struct {
	toolutil.HintableOutput
	ID               int64  `json:"id"`
	Username         string `json:"username"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	State            string `json:"state"`
	WebURL           string `json:"web_url"`
	IsAdmin          bool   `json:"is_admin"`
	Bot              bool   `json:"bot"`
	TwoFactorEnabled bool   `json:"two_factor_enabled"`
	External         bool   `json:"external"`
	Locked           bool   `json:"locked"`
	CreatedAt        string `json:"created_at,omitempty"`
}

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
		ID:               u.ID,
		Username:         u.Username,
		Name:             u.Name,
		Email:            u.Email,
		State:            u.State,
		WebURL:           u.WebURL,
		IsAdmin:          u.IsAdmin,
		Bot:              u.Bot,
		TwoFactorEnabled: u.TwoFactorEnabled,
		External:         u.External,
		Locked:           u.Locked,
	}
	if u.CreatedAt != nil {
		o.CreatedAt = u.CreatedAt.Format(time.RFC3339)
	}
	return o
}

// List returns all enterprise users for a group.
func List(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if in.GroupID.String() == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListEnterpriseUsersOptions{
		ListOptions: gl.ListOptions{Page: int64(in.Page), PerPage: int64(in.PerPage)},
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
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list enterprise users", err, http.StatusNotFound, "verify group_id \u2014 enterprise users require Ultimate license")
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
		return Output{}, toolutil.WrapErrWithStatusHint("get enterprise user", err, http.StatusNotFound, "verify user_id with gitlab_list_enterprise_users")
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
		return toolutil.WrapErrWithStatusHint("disable 2FA for enterprise user", err, http.StatusNotFound, "verify user_id with gitlab_list_enterprise_users")
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
		return toolutil.WrapErrWithStatusHint("delete enterprise user", err, http.StatusNotFound, "verify user_id with gitlab_list_enterprise_users \u2014 this action is irreversible")
	}
	return nil
}
