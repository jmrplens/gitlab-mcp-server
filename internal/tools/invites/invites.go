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
	ID          toolutil.StringOrInt `json:"id,omitempty" jsonschema:"Project ID or URL-encoded path sent in the request body (mirrors the GitLab id parameter; usually equal to project_id)"`
	Email       string               `json:"email,omitempty" jsonschema:"Email address to invite (either email or user_id required)"`
	UserID      int64                `json:"user_id,omitempty" jsonschema:"User ID to invite (either email or user_id required)"`
	AccessLevel int                  `json:"access_level" jsonschema:"Access level (5=Minimal access, 10=Guest, 15=Planner (Premium/Ultimate), 20=Reporter, 25=Security Manager (Premium/Ultimate), 30=Developer, 40=Maintainer, 50=Owner, 60=Admin where supported),required"`
	ExpiresAt   string               `json:"expires_at,omitempty" jsonschema:"Expiration date for the invitation (YYYY-MM-DD)"`
}

// GroupInvitesInput contains parameters for inviting a user to a group.
type GroupInvitesInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	ID          toolutil.StringOrInt `json:"id,omitempty" jsonschema:"Group ID or URL-encoded path sent in the request body (mirrors the GitLab id parameter; usually equal to group_id)"`
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

type pendingInvitationsListArgs struct {
	scopeID       toolutil.StringOrInt
	operation     string
	requiredField string
	notFoundHint  string
	query         string
	orderBy       string
	sort          string
	pagination    toolutil.PaginationInput
	keyset        toolutil.KeysetPaginationInput
	list          func(any, *gl.ListPendingInvitationsOptions, ...gl.RequestOptionFunc) ([]*gl.PendingInvite, *gl.Response, error)
}

func listPendingInvitations(ctx context.Context, args pendingInvitationsListArgs) (ListPendingInvitationsOutput, error) {
	if args.scopeID == "" {
		return ListPendingInvitationsOutput{}, toolutil.WrapErrWithMessage(args.operation, toolutil.ErrFieldRequired(args.requiredField))
	}

	opts := &gl.ListPendingInvitationsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, args.pagination, args.keyset)
	if args.orderBy != "" {
		opts.OrderBy = args.orderBy
	}
	if args.sort != "" {
		opts.Sort = args.sort
	}
	if args.query != "" {
		opts.Query = new(args.query)
	}

	invites, resp, err := args.list(string(args.scopeID), opts, gl.WithContext(ctx))
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
func ListPendingProjectInvitations(ctx context.Context, client *gitlabclient.Client, input ListPendingProjectInvitationsInput) (ListPendingInvitationsOutput, error) {
	return listPendingInvitations(ctx, pendingInvitationsListArgs{
		scopeID:       input.ProjectID,
		operation:     "project_invite_list_pending",
		requiredField: "project_id",
		notFoundHint:  "verify project_id; listing pending invitations requires Maintainer or Owner role",
		query:         input.Query,
		orderBy:       input.OrderBy,
		sort:          input.Sort,
		pagination:    input.PaginationInput,
		keyset:        input.KeysetPaginationInput,
		list:          client.GL().Invites.ListPendingProjectInvitations,
	})
}

// ListPendingGroupInvitations returns pending invitations for a group.
func ListPendingGroupInvitations(ctx context.Context, client *gitlabclient.Client, input ListPendingGroupInvitationsInput) (ListPendingInvitationsOutput, error) {
	return listPendingInvitations(ctx, pendingInvitationsListArgs{
		scopeID:       input.GroupID,
		operation:     "group_invite_list_pending",
		requiredField: "group_id",
		notFoundHint:  "verify group_id; listing pending invitations requires Owner role",
		query:         input.Query,
		orderBy:       input.OrderBy,
		sort:          input.Sort,
		pagination:    input.PaginationInput,
		keyset:        input.KeysetPaginationInput,
		list:          client.GL().Invites.ListPendingGroupInvitations,
	})
}

func buildInviteOptions(input inviteRequest) *gl.InvitesOptions {
	accessLevel := gl.AccessLevelValue(input.accessLevel)
	opts := &gl.InvitesOptions{AccessLevel: &accessLevel}
	if input.id != "" {
		opts.ID = string(input.id)
	}
	if input.email != "" {
		opts.Email = new(input.email)
	}
	if input.userID != 0 {
		opts.UserID = input.userID
	}
	if input.expiresAt != "" {
		// GitLab's invitation expires_at parameter is a date-only ISOTime
		// (YYYY-MM-DD), not an RFC3339 timestamp, so it is parsed directly
		// rather than via toolutil.ParseOptionalTime.
		if t, err := time.Parse("2006-01-02", input.expiresAt); err == nil {
			d := gl.ISOTime(t)
			opts.ExpiresAt = &d
		}
	}
	return opts
}

type inviteRequest struct {
	email       string
	expiresAt   string
	id          toolutil.StringOrInt
	accessLevel int
	userID      int64
}

func sendInvitation(ctx context.Context, scopeID toolutil.StringOrInt, operation, requiredField, forbiddenHint string, request inviteRequest, invite func(any, *gl.InvitesOptions, ...gl.RequestOptionFunc) (*gl.InvitesResult, *gl.Response, error)) (InviteResultOutput, error) {
	if scopeID == "" {
		return InviteResultOutput{}, toolutil.WrapErrWithMessage(operation, toolutil.ErrFieldRequired(requiredField))
	}
	if request.email == "" && request.userID == 0 {
		return InviteResultOutput{}, toolutil.WrapErrWithMessage(operation, errors.New("either email or user_id is required"))
	}

	result, _, err := invite(string(scopeID), buildInviteOptions(request), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return InviteResultOutput{}, toolutil.WrapErrWithHint(operation, err, forbiddenHint)
		}
		return InviteResultOutput{}, toolutil.WrapErrWithStatusHint(operation, err, http.StatusBadRequest,
			"valid access_level: 5 (Minimal access), 10 (Guest), 15 (Planner Premium/Ultimate), 20 (Reporter), 25 (Security Manager Premium/Ultimate), 30 (Developer), 40 (Maintainer), 50 (Owner), 60 (Admin where supported); expires_at format: YYYY-MM-DD; user may already be a member")
	}

	return toInviteResultOutput(result), nil
}

// ProjectInvites invites a user to a project by email or user ID.
func ProjectInvites(ctx context.Context, client *gitlabclient.Client, input ProjectInvitesInput) (InviteResultOutput, error) {
	return sendInvitation(ctx, input.ProjectID, "project_invite", "project_id",
		"inviting users requires Maintainer or Owner role on the project",
		inviteRequest{email: input.Email, userID: input.UserID, id: input.ID, expiresAt: input.ExpiresAt, accessLevel: input.AccessLevel},
		client.GL().Invites.ProjectInvites)
}

// GroupInvites invites a user to a group by email or user ID.
func GroupInvites(ctx context.Context, client *gitlabclient.Client, input GroupInvitesInput) (InviteResultOutput, error) {
	return sendInvitation(ctx, input.GroupID, "group_invite", "group_id",
		"inviting users requires Owner role on the group",
		inviteRequest{email: input.Email, userID: input.UserID, id: input.ID, expiresAt: input.ExpiresAt, accessLevel: input.AccessLevel},
		client.GL().Invites.GroupInvites)
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
