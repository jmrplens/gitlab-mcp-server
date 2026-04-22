// register.go wires groupmembers MCP tools to the MCP server.

package groupmembers

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all group member individual tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_member_get",
		Title:       toolutil.TitleFromName("gitlab_group_member_get"),
		Description: "Get a single member of a GitLab group by user ID. Returns user details including access level, state, and expiration date.\n\nReturns: JSON with member details including access level, state, and expiration. See also: gitlab_group_member_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetMember(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_member_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMemberMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_member_get_inherited",
		Title:       toolutil.TitleFromName("gitlab_group_member_get_inherited"),
		Description: "Get a single inherited member of a GitLab group by user ID. Returns member details including access level inherited from parent groups.\n\nReturns: JSON with inherited member details including access level. See also: gitlab_group_member_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetInheritedMember(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_member_get_inherited", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMemberMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_member_add",
		Title:       toolutil.TitleFromName("gitlab_group_member_add"),
		Description: "Add a member to a GitLab group. Specify user by user_id or username, and set access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner). Optionally set expiration date.\n\nReturns: JSON with the added member details. See also: gitlab_group_member_list, gitlab_get_user.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := AddMember(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_member_add", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMemberMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_member_edit",
		Title:       toolutil.TitleFromName("gitlab_group_member_edit"),
		Description: "Edit a member of a GitLab group. Update access level or expiration date for an existing member.\n\nReturns: JSON with the updated member details. See also: gitlab_group_member_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EditInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := EditMember(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_member_edit", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMemberMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_member_remove",
		Title:       toolutil.TitleFromName("gitlab_group_member_remove"),
		Description: "Remove a member from a GitLab group. Optionally skip subresource removal and unassign issuables.\n\nReturns: JSON confirming member removal. See also: gitlab_group_member_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RemoveInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Remove member (user %d) from group %q?", input.UserID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := RemoveMember(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_member_remove", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group member")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_share",
		Title:       toolutil.TitleFromName("gitlab_group_share"),
		Description: "Share a GitLab group with another group, granting the shared group a specified access level. Optionally set an expiration date.\n\nReturns: JSON with the group share details. See also: gitlab_group_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ShareInput) (*mcp.CallToolResult, ShareOutput, error) {
		start := time.Now()
		out, err := ShareGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_share", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatShareMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_unshare",
		Title:       toolutil.TitleFromName("gitlab_group_unshare"),
		Description: "Stop sharing a GitLab group with another group, removing the group-level access.\n\nReturns: JSON confirming group unshare.\n\nSee also: gitlab_group_share.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UnshareInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Stop sharing group %q with group %d?", input.GroupID, input.ShareGroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := UnshareGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_unshare", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group share")
	})
}

// RegisterMeta registers the gitlab_group_member meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"get":           toolutil.RouteAction(client, GetMember),
		"get_inherited": toolutil.RouteAction(client, GetInheritedMember),
		"add":           toolutil.RouteAction(client, AddMember),
		"edit":          toolutil.RouteAction(client, EditMember),
		"remove":        toolutil.DestructiveVoidAction(client, RemoveMember),
		"share":         toolutil.RouteAction(client, ShareGroup),
		"unshare":       toolutil.RouteVoidAction(client, UnshareGroup),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_group_member",
		Title: toolutil.TitleFromName("gitlab_group_member"),
		Description: `Group member operations. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- get: Get a group member (group_id, user_id)
- get_inherited: Get an inherited group member (group_id, user_id)
- add: Add a member to a group (group_id, user_id/username, access_level, expires_at)
- edit: Edit a group member (group_id, user_id, access_level, expires_at)
- remove: Remove a member from a group (group_id, user_id, skip_subresources, unassign_issuables)
- share: Share a group with another group (group_id, share_group_id, group_access, expires_at)
- unshare: Stop sharing a group (group_id, share_group_id)`,
		Annotations: toolutil.MetaAnnotations,
		Icons:       toolutil.IconUser,
	}, toolutil.MakeMetaHandler("gitlab_group_member", routes, nil))
}
