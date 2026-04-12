// register.go wires cilint MCP tools to the MCP server.

package cilint

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all CI lint MCP tools on the given server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_ci_lint_project",
		Title:       toolutil.TitleFromName("gitlab_ci_lint_project"),
		Description: "Validate a project's CI/CD configuration (.gitlab-ci.yml) from the repository. Returns validation status, errors, warnings, merged YAML, and includes.\n\nReturns: JSON with validation status, errors, warnings, and merged YAML.\n\nSee also: gitlab_ci_lint, gitlab_list_ci_yml_templates",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := LintProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ci_lint_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_ci_lint",
		Title:       toolutil.TitleFromName("gitlab_ci_lint"),
		Description: "Validate arbitrary CI/CD YAML content within a project's namespace context. Useful for testing CI configuration changes before committing. Returns validation status, errors, warnings, and merged YAML.\n\nReturns: JSON with validation status, errors, warnings, and merged YAML.\n\nSee also: gitlab_ci_lint_project, gitlab_list_ci_yml_templates",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ContentInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := LintContent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ci_lint", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})
}
