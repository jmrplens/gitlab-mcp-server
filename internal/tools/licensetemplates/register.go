// register.go wires licensetemplates MCP tools to the MCP server.
package licensetemplates

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all license template MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_license_templates",
		Title:       toolutil.TitleFromName("gitlab_list_license_templates"),
		Description: "List all available open-source license templates. Optionally filter by popular.\n\nReturns: JSON array of license templates with pagination.\n\nSee also: gitlab_get_license_template, gitlab_list_gitignore_templates",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconTemplate,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_license_templates", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_license_template",
		Title:       toolutil.TitleFromName("gitlab_get_license_template"),
		Description: "Get a single license template by key. Optionally substitute project name and full name.\n\nReturns: JSON with the license template content.\n\nSee also: gitlab_list_license_templates, gitlab_list_project_templates",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconTemplate,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_license_template", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGetMarkdown(out)), out, nil)
	})
}
