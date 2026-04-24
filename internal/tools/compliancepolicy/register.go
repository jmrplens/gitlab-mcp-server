// register.go wires compliance policy MCP tools to the MCP server.

package compliancepolicy

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab admin compliance policy settings.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_compliance_policy_settings",
		Title:       toolutil.TitleFromName("gitlab_get_compliance_policy_settings"),
		Description: "Get the admin-level compliance policy settings for the GitLab instance.\n\nReturns: JSON with compliance policy settings.\n\nSee also: gitlab_update_compliance_policy_settings",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_compliance_policy_settings", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_compliance_policy_settings",
		Title:       toolutil.TitleFromName("gitlab_update_compliance_policy_settings"),
		Description: "Update the admin-level compliance policy settings for the GitLab instance.\n\nReturns: JSON with updated compliance policy settings.\n\nSee also: gitlab_get_compliance_policy_settings",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_compliance_policy_settings", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})
}
