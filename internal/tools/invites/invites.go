// Package invites implements MCP tools for GitLab invitation operations
// including listing pending invitations and inviting users to projects/groups.
package invites

import (
	"context"
	"errors"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Input types.

// ListPendingProjectInvitationsInput contains parameters for listing pending project invitations.
type ListPendingProjectInvitationsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Query     string               `json:"query,omitempty" jsonschema:"Filter invitations by email or name"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination (default 1)"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page (default 20, max 100)"`
}

// ListPendingGroupInvitationsInput contains parameters for listing pending group invitations.
type ListPendingGroupInvitationsInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Query   string               `json:"query,omitempty" jsonschema:"Filter invitations by email or name"`
	Page    int64                `json:"page,omitempty" jsonschema:"Page number for pagination (default 1)"`
	PerPage int64                `json:"per_page,omitempty" jsonschema:"Number of items per page (default 20, max 100)"`
}

// ProjectInvitesInput contains parameters for inviting a user to a project.
type ProjectInvitesInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Email       string               `json:"email,omitempty" jsonschema:"Email address to invite (either email or user_id required)"`
	UserID      int64                `json:"user_id,omitempty" jsonschema:"User ID to invite (either email or user_id required)"`
	AccessLevel int                  `json:"access_level" jsonschema:"Access level (10=Guest 20=Reporter 30=Developer 40=Maintainer 50=Owner),required"`
	ExpiresAt   string               `json:"expires_at,omitempty" jsonschema:"Expiration date for the invitation (YYYY-MM-DD)"`
}

// GroupInvitesInput contains parameters for inviting a user to a group.
type GroupInvitesInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Email       string               `json:"email,omitempty" jsonschema:"Email address to invite (either email or user_id required)"`
	UserID      int64                `json:"user_id,omitempty" jsonschema:"User ID to invite (either email or user_id required)"`
	AccessLevel int                  `json:"access_level" jsonschema:"Access level (10=Guest 20=Reporter 30=Developer 40=Maintainer 50=Owner),required"`
	ExpiresAt   string               `json:"expires_at,omitempty" jsonschema:"Expiration date for the invitation (YYYY-MM-DD)"`
}

// Output types.

// PendingInviteOutput represents a single pending invitation.
type PendingInviteOutput struct {
	ID            int64  `json:"id"`
	InviteEmail   string `json:"invite_email"`
	CreatedAt     string `json:"created_at,omitempty"`
	AccessLevel   int    `json:"access_level"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	UserName      string `json:"user_name,omitempty"`
	CreatedByName string `json:"created_by_name,omitempty"`
}

// ListPendingInvitationsOutput holds a paginated list of pending invitations.
type ListPendingInvitationsOutput struct {
	toolutil.HintableOutput
	Invitations []PendingInviteOutput     `json:"invitations"`
	Pagination  toolutil.PaginationOutput `json:"pagination"`
}

// InviteResultOutput represents the result of an invitation operation.
type InviteResultOutput struct {
	toolutil.HintableOutput
	Status  string            `json:"status"`
	Message map[string]string `json:"message,omitempty"`
}

// Handlers.

// ListPendingProjectInvitations returns pending invitations for a project.
func ListPendingProjectInvitations(ctx context.Context, client *gitlabclient.Client, input ListPendingProjectInvitationsInput) (ListPendingInvitationsOutput, error) {
	if input.ProjectID == "" {
		return ListPendingInvitationsOutput{}, toolutil.WrapErrWithMessage("project_invite_list_pending", toolutil.ErrFieldRequired("project_id"))
	}

	opts := &gl.ListPendingInvitationsOptions{
		ListOptions: gl.ListOptions{
			Page:    input.Page,
			PerPage: input.PerPage,
		},
	}
	if input.Query != "" {
		opts.Query = new(input.Query)
	}

	invites, resp, err := client.GL().Invites.ListPendingProjectInvitations(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListPendingInvitationsOutput{}, toolutil.WrapErrWithMessage("project_invite_list_pending", err)
	}

	out := ListPendingInvitationsOutput{
		Invitations: make([]PendingInviteOutput, 0, len(invites)),
		Pagination:  toolutil.PaginationFromResponse(resp),
	}
	for _, inv := range invites {
		out.Invitations = append(out.Invitations, toPendingInviteOutput(inv))
	}
	return out, nil
}

// ListPendingGroupInvitations returns pending invitations for a group.
func ListPendingGroupInvitations(ctx context.Context, client *gitlabclient.Client, input ListPendingGroupInvitationsInput) (ListPendingInvitationsOutput, error) {
	if input.GroupID == "" {
		return ListPendingInvitationsOutput{}, toolutil.WrapErrWithMessage("group_invite_list_pending", toolutil.ErrFieldRequired("group_id"))
	}

	opts := &gl.ListPendingInvitationsOptions{
		ListOptions: gl.ListOptions{
			Page:    input.Page,
			PerPage: input.PerPage,
		},
	}
	if input.Query != "" {
		opts.Query = new(input.Query)
	}

	invites, resp, err := client.GL().Invites.ListPendingGroupInvitations(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListPendingInvitationsOutput{}, toolutil.WrapErrWithMessage("group_invite_list_pending", err)
	}

	out := ListPendingInvitationsOutput{
		Invitations: make([]PendingInviteOutput, 0, len(invites)),
		Pagination:  toolutil.PaginationFromResponse(resp),
	}
	for _, inv := range invites {
		out.Invitations = append(out.Invitations, toPendingInviteOutput(inv))
	}
	return out, nil
}

// ProjectInvites invites a user to a project by email or user ID.
func ProjectInvites(ctx context.Context, client *gitlabclient.Client, input ProjectInvitesInput) (InviteResultOutput, error) {
	if input.ProjectID == "" {
		return InviteResultOutput{}, toolutil.WrapErrWithMessage("project_invite", toolutil.ErrFieldRequired("project_id"))
	}
	if input.Email == "" && input.UserID == 0 {
		return InviteResultOutput{}, toolutil.WrapErrWithMessage("project_invite", errors.New("either email or user_id is required"))
	}

	accessLevel := gl.AccessLevelValue(input.AccessLevel)
	opts := &gl.InvitesOptions{
		AccessLevel: &accessLevel,
	}
	if input.Email != "" {
		opts.Email = new(input.Email)
	}
	if input.UserID != 0 {
		opts.UserID = input.UserID
	}
	if input.ExpiresAt != "" {
		if t, err := time.Parse("2006-01-02", input.ExpiresAt); err == nil {
			d := gl.ISOTime(t)
			opts.ExpiresAt = &d
		}
	}

	result, _, err := client.GL().Invites.ProjectInvites(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return InviteResultOutput{}, toolutil.WrapErrWithMessage("project_invite", err)
	}

	return toInviteResultOutput(result), nil
}

// GroupInvites invites a user to a group by email or user ID.
func GroupInvites(ctx context.Context, client *gitlabclient.Client, input GroupInvitesInput) (InviteResultOutput, error) {
	if input.GroupID == "" {
		return InviteResultOutput{}, toolutil.WrapErrWithMessage("group_invite", toolutil.ErrFieldRequired("group_id"))
	}
	if input.Email == "" && input.UserID == 0 {
		return InviteResultOutput{}, toolutil.WrapErrWithMessage("group_invite", errors.New("either email or user_id is required"))
	}

	accessLevel := gl.AccessLevelValue(input.AccessLevel)
	opts := &gl.InvitesOptions{
		AccessLevel: &accessLevel,
	}
	if input.Email != "" {
		opts.Email = new(input.Email)
	}
	if input.UserID != 0 {
		opts.UserID = input.UserID
	}
	if input.ExpiresAt != "" {
		if t, err := time.Parse("2006-01-02", input.ExpiresAt); err == nil {
			d := gl.ISOTime(t)
			opts.ExpiresAt = &d
		}
	}

	result, _, err := client.GL().Invites.GroupInvites(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return InviteResultOutput{}, toolutil.WrapErrWithMessage("group_invite", err)
	}

	return toInviteResultOutput(result), nil
}

// Converters.

// toPendingInviteOutput converts the GitLab API response to the tool output format.
func toPendingInviteOutput(inv *gl.PendingInvite) PendingInviteOutput {
	out := PendingInviteOutput{
		ID:            inv.ID,
		InviteEmail:   inv.InviteEmail,
		AccessLevel:   int(inv.AccessLevel),
		UserName:      inv.UserName,
		CreatedByName: inv.CreatedByName,
	}
	if inv.CreatedAt != nil {
		out.CreatedAt = inv.CreatedAt.Format(time.RFC3339)
	}
	if inv.ExpiresAt != nil {
		out.ExpiresAt = inv.ExpiresAt.Format(time.RFC3339)
	}
	return out
}

// toInviteResultOutput converts the GitLab API response to the tool output format.
func toInviteResultOutput(r *gl.InvitesResult) InviteResultOutput {
	return InviteResultOutput{
		Status:  r.Status,
		Message: r.Message,
	}
}

// Formatters.
