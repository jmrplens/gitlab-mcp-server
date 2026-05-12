package invites

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers individual invite tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_invite_list_pending",
		Title:       toolutil.TitleFromName("gitlab_project_invite_list_pending"),
		Description: "List all pending invitations for a project. Supports filtering by query and pagination.\n\nReturns: JSON array of pending invitations with pagination.\n\nSee also: gitlab_project_invite, gitlab_project_members_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListPendingProjectInvitationsInput) (*mcp.CallToolResult, ListPendingInvitationsOutput, error) {
		start := time.Now()
		out, err := ListPendingProjectInvitations(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_invite_list_pending", start, err)
		return toolutil.WithHints(FormatListPendingMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_invite_list_pending",
		Title:       toolutil.TitleFromName("gitlab_group_invite_list_pending"),
		Description: "List all pending invitations for a group. Supports filtering by query and pagination.\n\nReturns: JSON array of pending invitations with pagination.\n\nSee also: gitlab_group_invite, gitlab_group_members_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListPendingGroupInvitationsInput) (*mcp.CallToolResult, ListPendingInvitationsOutput, error) {
		start := time.Now()
		out, err := ListPendingGroupInvitations(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_invite_list_pending", start, err)
		return toolutil.WithHints(FormatListPendingMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_invite",
		Title:       toolutil.TitleFromName("gitlab_project_invite"),
		Description: "Invite a user to a project by email or user ID. Requires access_level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner).\n\nReturns: JSON with the invitation result.\n\nSee also: gitlab_project_invite_list_pending, gitlab_project_member_add",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectInvitesInput) (*mcp.CallToolResult, InviteResultOutput, error) {
		start := time.Now()
		out, err := ProjectInvites(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_invite", start, err)
		return toolutil.WithHints(FormatInviteResultMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_invite",
		Title:       toolutil.TitleFromName("gitlab_group_invite"),
		Description: "Invite a user to a group by email or user ID. Requires access_level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner).\n\nReturns: JSON with the invitation result.\n\nSee also: gitlab_group_invite_list_pending, gitlab_group_member_add",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GroupInvitesInput) (*mcp.CallToolResult, InviteResultOutput, error) {
		start := time.Now()
		out, err := GroupInvites(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_invite", start, err)
		return toolutil.WithHints(FormatInviteResultMarkdown(out), out, err)
	})
}
