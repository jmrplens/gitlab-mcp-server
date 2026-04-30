// register.go wires settings MCP tools to the MCP server.
package settings

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all application settings tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_settings",
		Title:       toolutil.TitleFromName("gitlab_get_settings"),
		Description: "Get current application settings. Requires admin access. Returns all instance-level settings as key-value pairs.\n\nReturns: JSON with all application settings.\n\nSee also: gitlab_update_settings.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_settings", start, err)
		return toolutil.WithHints(FormatGetMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_settings",
		Title:       toolutil.TitleFromName("gitlab_update_settings"),
		Description: "Update application settings. Requires admin access. Pass settings as key-value map with snake_case keys matching GitLab API (e.g. signup_enabled, default_project_visibility).\n\nReturns: JSON with the updated application settings.\n\nSee also: gitlab_get_settings.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, UpdateOutput, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_settings", start, err)
		return toolutil.WithHints(FormatUpdateMarkdown(out), out, err)
	})
}
