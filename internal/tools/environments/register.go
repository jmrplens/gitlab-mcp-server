// register.go wires environments MCP tools to the MCP server.

package environments

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all environment-related MCP tools on the given server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_environment_list",
		Title:       toolutil.TitleFromName("gitlab_environment_list"),
		Description: "List environments for a GitLab project. Supports filtering by name, search term, and state. Returns paginated results with environment details including tier, state, and external URL.\n\nReturns: paginated list of environments with id, name, slug, state, tier, and external_url. See also: gitlab_environment_get, gitlab_deployment_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEnvironment,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_environment_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_environment_get",
		Title:       toolutil.TitleFromName("gitlab_environment_get"),
		Description: "Get details of a specific environment in a GitLab project by its ID. Returns environment name, state, tier, external URL, and timestamps.\n\nReturns: id, name, slug, state, tier, external_url, and timestamps. See also: gitlab_environment_update, gitlab_deployment_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEnvironment,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_environment_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_environment_create",
		Title:       toolutil.TitleFromName("gitlab_environment_create"),
		Description: "Create a new environment in a GitLab project. Specify name (required), description, external URL, and tier (production, staging, testing, development, other). Returns: id, name, slug, state, tier, external_url, created_at. See also: gitlab_environment_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconEnvironment,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_environment_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_environment_update",
		Title:       toolutil.TitleFromName("gitlab_environment_update"),
		Description: "Update an existing environment in a GitLab project. Can modify name, description, external URL, and tier. Returns: id, name, slug, state, tier, external_url, updated_at. See also: gitlab_environment_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconEnvironment,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_environment_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_environment_delete",
		Title:       toolutil.TitleFromName("gitlab_environment_delete"),
		Description: "Permanently delete an environment from a GitLab project. The environment must be stopped before it can be deleted.\n\nReturns: confirmation message. See also: gitlab_environment_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconEnvironment,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete environment %d from project %q?", input.EnvironmentID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_environment_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("environment %d from project %s", input.EnvironmentID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_environment_stop",
		Title:       toolutil.TitleFromName("gitlab_environment_stop"),
		Description: "Stop a running environment in a GitLab project. Triggers any on_stop actions defined in CI/CD. Use force=true to stop even if the environment has active deployments. Returns: id, name, slug, state, tier, external_url. See also: gitlab_environment_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconEnvironment,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StopInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Stop(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_environment_stop", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})
}
