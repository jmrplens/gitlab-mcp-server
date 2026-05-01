package ciyamltemplates

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all CI YAML template MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_ci_yml_templates",
		Title:       toolutil.TitleFromName("gitlab_list_ci_yml_templates"),
		Description: "List all available GitLab CI YAML templates. Returns key and name for each template.\n\nReturns: JSON array of CI YAML templates with pagination.\n\nSee also: gitlab_get_ci_yml_template, gitlab_ci_lint",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconTemplate,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_ci_yml_templates", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_ci_yml_template",
		Title:       toolutil.TitleFromName("gitlab_get_ci_yml_template"),
		Description: "Get a single GitLab CI YAML template by key. Returns the template name and content.\n\nReturns: JSON with the template name and content.\n\nSee also: gitlab_list_ci_yml_templates, gitlab_ci_lint",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconTemplate,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_ci_yml_template", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGetMarkdown(out)), out, nil)
	})
}
