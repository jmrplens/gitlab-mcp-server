// register.go wires projecttemplates MCP tools to the MCP server.
package projecttemplates

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all project template MCP tools on the given server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_project_templates",
		Title:       toolutil.TitleFromName("gitlab_list_project_templates"),
		Description: "List project templates of a given type (dockerfiles, gitignores, gitlab_ci_ymls, licenses).\n\nReturns: JSON array of templates with pagination.\n\nSee also: gitlab_get_project_template, gitlab_list_ci_yml_templates",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconTemplate,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_project_templates", start, err)
		if err != nil {
			return nil, ListOutput{}, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_project_template",
		Title:       toolutil.TitleFromName("gitlab_get_project_template"),
		Description: "Get a single project template by type and key.\n\nReturns: JSON with the template details.\n\nSee also: gitlab_list_project_templates, gitlab_list_dockerfile_templates",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconTemplate,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_template", start, err)
		if err != nil {
			return nil, GetOutput{}, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGetMarkdown(out)), out, nil)
	})
}
