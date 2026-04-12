// register.go wires mrapprovalsettings MCP tools to the MCP server.

package mrapprovalsettings

import (
	"context"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers all MR approval settings tools on the given MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_group_mr_approval_settings",
		Title:       toolutil.TitleFromName("gitlab_get_group_mr_approval_settings"),
		Description: "Get group-level merge request approval settings: whether author/committer approval is allowed, approver list overrides, approval retention on push, and reauthentication requirements. Settings may be locked or inherited from parent groups.\n\nReturns: JSON with each setting's value, locked status, and inheritance source. Requires GitLab Premium.\n\nSee also: gitlab_update_group_mr_approval_settings, gitlab_get_project_mr_approval_settings.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GroupGetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetGroupSettings(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_group_mr_approval_settings", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out, "Group")), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_group_mr_approval_settings",
		Title:       toolutil.TitleFromName("gitlab_update_group_mr_approval_settings"),
		Description: "Update group-level merge request approval settings: control author/committer approval, approver list overrides, approval retention on push, and reauthentication. Only include settings you want to change.\n\nReturns: JSON with all updated settings. Requires GitLab Premium.\n\nSee also: gitlab_get_group_mr_approval_settings.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GroupUpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := UpdateGroupSettings(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_group_mr_approval_settings", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out, "Group")), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_project_mr_approval_settings",
		Title:       toolutil.TitleFromName("gitlab_get_project_mr_approval_settings"),
		Description: "Get project-level merge request approval settings: whether author/committer approval is allowed, approver list overrides, approval retention on push, selective code owner removals, and reauthentication requirements. Settings may be locked or inherited from parent group.\n\nReturns: JSON with each setting's value, locked status, and inheritance source. Requires GitLab Premium.\n\nSee also: gitlab_update_project_mr_approval_settings, gitlab_get_group_mr_approval_settings.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectGetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetProjectSettings(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_mr_approval_settings", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out, "Project")), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_project_mr_approval_settings",
		Title:       toolutil.TitleFromName("gitlab_update_project_mr_approval_settings"),
		Description: "Update project-level merge request approval settings: control author/committer approval, approver list overrides, approval retention on push, selective code owner removals, and reauthentication. Only include settings you want to change.\n\nReturns: JSON with all updated settings. Requires GitLab Premium.\n\nSee also: gitlab_get_project_mr_approval_settings.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectUpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := UpdateProjectSettings(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_project_mr_approval_settings", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out, "Project")), out, err)
	})
}
