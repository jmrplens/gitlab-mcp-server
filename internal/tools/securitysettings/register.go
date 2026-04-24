// register.go wires security settings MCP tools to the MCP server.

package securitysettings

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab security settings operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_project_security_settings",
		Title:       toolutil.TitleFromName("gitlab_get_project_security_settings"),
		Description: "Get security settings for a GitLab project. Returns auto-fix, vulnerability scanning, and secret push protection status.\n\nReturns: JSON with project security settings. See also: gitlab_update_project_secret_push_protection.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetProjectInput) (*mcp.CallToolResult, ProjectOutput, error) {
		start := time.Now()
		out, err := GetProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_security_settings", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProjectMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_project_secret_push_protection",
		Title:       toolutil.TitleFromName("gitlab_update_project_secret_push_protection"),
		Description: "Enable or disable secret push protection for a GitLab project.\n\nReturns: JSON with updated security settings. See also: gitlab_get_project_security_settings.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateProjectInput) (*mcp.CallToolResult, ProjectOutput, error) {
		start := time.Now()
		out, err := UpdateProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_project_secret_push_protection", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProjectMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_group_secret_push_protection",
		Title:       toolutil.TitleFromName("gitlab_update_group_secret_push_protection"),
		Description: "Enable or disable secret push protection for a GitLab group. Optionally exclude specific projects.\n\nReturns: JSON with updated group security settings. See also: gitlab_get_project_security_settings.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateGroupInput) (*mcp.CallToolResult, GroupOutput, error) {
		start := time.Now()
		out, err := UpdateGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_group_secret_push_protection", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGroupMarkdown(out)), out, err)
	})
}
