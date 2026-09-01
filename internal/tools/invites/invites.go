package invites

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Input types.

// ListPendingProjectInvitationsInput contains parameters for listing pending project invitations.
type ListPendingProjectInvitationsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Query     string               `json:"query,omitempty" jsonschema:"Filter invitations by email or name"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort order for keyset-paginated results: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListPendingGroupInvitationsInput contains parameters for listing pending group invitations.
type ListPendingGroupInvitationsInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Query   string               `json:"query,omitempty" jsonschema:"Filter invitations by email or name"`
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by"`
	Sort    string               `json:"sort,omitempty" jsonschema:"Sort order for keyset-paginated results: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ProjectInvitesInput contains parameters for inviting a user to a project.
type ProjectInvitesInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ID          toolutil.StringOrInt `json:"id,omitempty" jsonschema:"Project ID or URL-encoded path sent in the request body (mirrors the GitLab id parameter. Usually equal to project_id)"`
	Email       string               `json:"email,omitempty" jsonschema:"Email address to invite (either email or user_id required)"`
	UserID      int64                `json:"user_id,omitempty" jsonschema:"User ID to invite (either email or user_id required)"`
	AccessLevel int                  `json:"access_level" jsonschema:"Access level (5=Minimal access, 10=Guest, 15=Planner (Premium/Ultimate), 20=Reporter, 25=Security Manager (Premium/Ultimate), 30=Developer, 40=Maintainer, 50=Owner, 60=Admin where supported),required"`
	ExpiresAt   string               `json:"expires_at,omitempty" jsonschema:"Expiration date for the invitation (YYYY-MM-DD)"`
}

// GroupInvitesInput contains parameters for inviting a user to a group.
type GroupInvitesInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	ID          toolutil.StringOrInt `json:"id,omitempty" jsonschema:"Group ID or URL-encoded path sent in the request body (mirrors the GitLab id parameter. Usually equal to group_id)"`
	Email       string               `json:"email,omitempty" jsonschema:"Email address to invite (either email or user_id required)"`
	UserID      int64                `json:"user_id,omitempty" jsonschema:"User ID to invite (either email or user_id required)"`
	AccessLevel int                  `json:"access_level" jsonschema:"Access level (5=Minimal access, 10=Guest, 15=Planner (Premium/Ultimate), 20=Reporter, 25=Security Manager (Premium/Ultimate), 30=Developer, 40=Maintainer, 50=Owner, 60=Admin where supported),required"`
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

// pendingInvitationsListArgs carries the resolved per-call inputs for the
// shared runPendingInvitationsList helper. The query/order_by/sort/pagination
// fields mirror the documented GitLab list-invitations query parameters
// one-for-one. The &gl.ListPendingInvitationsOptions{} literal itself is built
// by buildListPendingInvitationsOptions from the public
// ListPendingProjectInvitationsInput / ListPendingGroupInvitationsInput structs,
// which are the model-facing carriers and 1:1 source of these parameters.
type pendingInvitationsListArgs struct {
	scopeID       toolutil.StringOrInt
	operation     string
	requiredField string
	notFoundHint  string
	opts          *gl.ListPendingInvitationsOptions
	list          func(any, *gl.ListPendingInvitationsOptions, ...gl.RequestOptionFunc) ([]*gl.PendingInvite, *gl.Response, error)
}

func runPendingInvitationsList(ctx context.Context, args pendingInvitationsListArgs) (ListPendingInvitationsOutput, error) {
	if args.scopeID == "" {
		return ListPendingInvitationsOutput{}, toolutil.WrapErrWithMessage(args.operation, toolutil.ErrFieldRequired(args.requiredField))
	}

	invites, resp, err := args.list(string(args.scopeID), args.opts, gl.WithContext(ctx))
	if err != nil {
		return ListPendingInvitationsOutput{}, toolutil.WrapErrWithStatusHint(args.operation, err, http.StatusNotFound, args.notFoundHint)
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

// ListPendingProjectInvitations returns pending invitations for a project.
//
// The &gl.ListPendingInvitationsOptions{} literal is built here, in the handler
// that takes the model-facing ListPendingProjectInvitationsInput, so every
// documented list query parameter (query, order_by, sort, and the embedded
// page/per_page/page_token/pagination) is mapped 1:1 from a public input field.
func ListPendingProjectInvitations(ctx context.Context, client *gitlabclient.Client, input ListPendingProjectInvitationsInput) (ListPendingInvitationsOutput, error) {
	opts := &gl.ListPendingInvitationsOptions{}
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
	return runPendingInvitationsList(ctx, pendingInvitationsListArgs{
		scopeID:       input.ProjectID,
		operation:     "project_invite_list_pending",
		requiredField: "project_id",
		notFoundHint:  "verify project_id; listing pending invitations requires Maintainer or Owner role",
		opts:          opts,
		list:          client.GL().Invites.ListPendingProjectInvitations,
	})
}

// ListPendingGroupInvitations returns pending invitations for a group.
//
// The &gl.ListPendingInvitationsOptions{} literal is built here, in the handler
// that takes the model-facing ListPendingGroupInvitationsInput, so every
// documented list query parameter (query, order_by, sort, and the embedded
// page/per_page/page_token/pagination) is mapped 1:1 from a public input field.
func ListPendingGroupInvitations(ctx context.Context, client *gitlabclient.Client, input ListPendingGroupInvitationsInput) (ListPendingInvitationsOutput, error) {
	opts := &gl.ListPendingInvitationsOptions{}
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
	return runPendingInvitationsList(ctx, pendingInvitationsListArgs{
		scopeID:       input.GroupID,
		operation:     "group_invite_list_pending",
		requiredField: "group_id",
		notFoundHint:  "verify group_id; listing pending invitations requires Owner role",
		opts:          opts,
		list:          client.GL().Invites.ListPendingGroupInvitations,
	})
}

// applyInviteExpiresAt parses the date-only invitation expires_at parameter
// (YYYY-MM-DD ISOTime, not an RFC3339 timestamp) onto the SDK options.
func applyInviteExpiresAt(opts *gl.InvitesOptions, expiresAt string) {
	if expiresAt == "" {
		return
	}
	if t, err := time.Parse("2006-01-02", expiresAt); err == nil {
		d := gl.ISOTime(t)
		opts.ExpiresAt = &d
	}
}

type sendInvitationArgs struct {
	scopeID       toolutil.StringOrInt
	operation     string
	requiredField string
	forbiddenHint string
	email         string
	userID        int64
	opts          *gl.InvitesOptions
	invite        func(any, *gl.InvitesOptions, ...gl.RequestOptionFunc) (*gl.InvitesResult, *gl.Response, error)
}

func runInvitation(ctx context.Context, args sendInvitationArgs) (InviteResultOutput, error) {
	if args.scopeID == "" {
		return InviteResultOutput{}, toolutil.WrapErrWithMessage(args.operation, toolutil.ErrFieldRequired(args.requiredField))
	}
	if args.email == "" && args.userID == 0 {
		return InviteResultOutput{}, toolutil.WrapErrWithMessage(args.operation, errors.New("either email or user_id is required"))
	}

	result, _, err := args.invite(string(args.scopeID), args.opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return InviteResultOutput{}, toolutil.WrapErrWithHint(args.operation, err, args.forbiddenHint)
		}
		return InviteResultOutput{}, toolutil.WrapErrWithStatusHint(args.operation, err, http.StatusBadRequest,
			"valid access_level: 5 (Minimal access), 10 (Guest), 15 (Planner Premium/Ultimate), 20 (Reporter), 25 (Security Manager Premium/Ultimate), 30 (Developer), 40 (Maintainer), 50 (Owner), 60 (Admin where supported); expires_at format: YYYY-MM-DD; user may already be a member")
	}

	return toInviteResultOutput(result), nil
}

// ProjectInvites invites a user to a project by email or user ID.
//
// The &gl.InvitesOptions{} literal is built here, in the handler that takes the
// model-facing ProjectInvitesInput, so every documented add-a-member POST body
// parameter (id, email, user_id, access_level, expires_at) is mapped 1:1 from a
// public input field.
func ProjectInvites(ctx context.Context, client *gitlabclient.Client, input ProjectInvitesInput) (InviteResultOutput, error) {
	accessLevel := gl.AccessLevelValue(input.AccessLevel)
	opts := &gl.InvitesOptions{AccessLevel: &accessLevel}
	if input.ID != "" {
		opts.ID = string(input.ID)
	}
	if input.Email != "" {
		opts.Email = new(input.Email)
	}
	if input.UserID != 0 {
		opts.UserID = input.UserID
	}
	applyInviteExpiresAt(opts, input.ExpiresAt)
	return runInvitation(ctx, sendInvitationArgs{
		scopeID:       input.ProjectID,
		operation:     "project_invite",
		requiredField: "project_id",
		forbiddenHint: "inviting users requires Maintainer or Owner role on the project",
		email:         input.Email,
		userID:        input.UserID,
		opts:          opts,
		invite:        client.GL().Invites.ProjectInvites,
	})
}

// GroupInvites invites a user to a group by email or user ID.
//
// The &gl.InvitesOptions{} literal is built here, in the handler that takes the
// model-facing GroupInvitesInput, so every documented add-a-member POST body
// parameter (id, email, user_id, access_level, expires_at) is mapped 1:1 from a
// public input field.
func GroupInvites(ctx context.Context, client *gitlabclient.Client, input GroupInvitesInput) (InviteResultOutput, error) {
	accessLevel := gl.AccessLevelValue(input.AccessLevel)
	opts := &gl.InvitesOptions{AccessLevel: &accessLevel}
	if input.ID != "" {
		opts.ID = string(input.ID)
	}
	if input.Email != "" {
		opts.Email = new(input.Email)
	}
	if input.UserID != 0 {
		opts.UserID = input.UserID
	}
	applyInviteExpiresAt(opts, input.ExpiresAt)
	return runInvitation(ctx, sendInvitationArgs{
		scopeID:       input.GroupID,
		operation:     "group_invite",
		requiredField: "group_id",
		forbiddenHint: "inviting users requires Owner role on the group",
		email:         input.Email,
		userID:        input.UserID,
		opts:          opts,
		invite:        client.GL().Invites.GroupInvites,
	})
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
