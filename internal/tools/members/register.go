// register.go wires members MCP tools to the MCP server.

package members

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers member-related tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_members_list",
		Title:       toolutil.TitleFromName("gitlab_project_members_list"),
		Description: "List all members of a GitLab project including inherited members from parent groups. Returns user ID, username, name, state, access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner), and web URL. Supports filtering by name/username query.\n\nReturns: JSON array of members with pagination. See also: gitlab_project_member_get, gitlab_project_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_members_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_member_get",
		Title:       toolutil.TitleFromName("gitlab_project_member_get"),
		Description: "Get details of a specific project member by user ID. Returns access level, state, username, and membership info.\n\nReturns: JSON with member details. See also: gitlab_project_members_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_member_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_member_get_inherited",
		Title:       toolutil.TitleFromName("gitlab_project_member_get_inherited"),
		Description: "Get a project member including inherited membership from parent groups. Returns access level, state, and membership origin.\n\nReturns: JSON with member details. See also: gitlab_project_member_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetInherited(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_member_get_inherited", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_member_add",
		Title:       toolutil.TitleFromName("gitlab_project_member_add"),
		Description: "Add a user as a project member. Requires user_id (from gitlab_search_users or gitlab_project_members_list) and access_level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner). Optionally set expires_at and member_role_id. Returns: username, access level, state, and web URL. See also: gitlab_project_members_list, gitlab_get_user.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Add(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_member_add", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_member_edit",
		Title:       toolutil.TitleFromName("gitlab_project_member_edit"),
		Description: "Edit a project member's access level or expiration. Requires access_level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner). Returns: updated username, access level, state, and web URL. See also: gitlab_project_member_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EditInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Edit(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_member_edit", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_member_delete",
		Title:       toolutil.TitleFromName("gitlab_project_member_delete"),
		Description: "Remove a member from a project.\n\nReturns: confirmation message.\n\nSee also: gitlab_project_member_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Remove member (user %d) from project %q?", input.UserID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_member_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("project member")
	})
}
