// register.go wires invites MCP tools to the MCP server.

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

// RegisterMeta registers the gitlab_invite meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list_pending_project": toolutil.RouteAction(client, ListPendingProjectInvitations),
		"list_pending_group":   toolutil.RouteAction(client, ListPendingGroupInvitations),
		"project":              toolutil.RouteAction(client, ProjectInvites),
		"group":                toolutil.RouteAction(client, GroupInvites),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_invite",
		Title: toolutil.TitleFromName("gitlab_invite"),
		Description: `Manage GitLab invitations. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list_pending_project: List pending project invitations. Params: project_id (required), query, page, per_page
- list_pending_group: List pending group invitations. Params: group_id (required), query, page, per_page
- project: Invite user to a project. Params: project_id (required), email or user_id (required), access_level (required), expires_at
- group: Invite user to a group. Params: group_id (required), email or user_id (required), access_level (required), expires_at`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconUser,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_invite", routes, nil))
}
