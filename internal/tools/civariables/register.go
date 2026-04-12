// register.go wires civariables MCP tools to the MCP server.

package civariables

import (
	"context"
	"fmt"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers the five CI/CD variable management tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlab.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_ci_variable_list",
		Title:       toolutil.TitleFromName("gitlab_ci_variable_list"),
		Description: "List CI/CD variables for a GitLab project. Returns paginated results with variable key, type, protection, masking, and environment scope.\n\nReturns: paginated list of variables with key, variable_type, protected, masked, and environment_scope. See also: gitlab_ci_variable_get, gitlab_pipeline_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ci_variable_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_ci_variable_get",
		Title:       toolutil.TitleFromName("gitlab_ci_variable_get"),
		Description: "Get a specific CI/CD variable by key from a GitLab project. Optionally filter by environment scope when duplicate keys exist. Returns: key, value, variable_type, protected, masked, environment_scope. See also: gitlab_ci_variable_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ci_variable_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_ci_variable_create",
		Title:       toolutil.TitleFromName("gitlab_ci_variable_create"),
		Description: "Create a new CI/CD variable in a GitLab project. Requires key and value. Optionally set type (env_var/file), protection, masking, and environment scope. Returns: key, value, variable_type, protected, masked, environment_scope. See also: gitlab_ci_variable_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ci_variable_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_ci_variable_update",
		Title:       toolutil.TitleFromName("gitlab_ci_variable_update"),
		Description: "Update an existing CI/CD variable in a GitLab project. Specify the key to update and any fields to change: value, type, protection, masking, environment scope. Returns: key, value, variable_type, protected, masked, environment_scope. See also: gitlab_ci_variable_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ci_variable_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_ci_variable_delete",
		Title:       toolutil.TitleFromName("gitlab_ci_variable_delete"),
		Description: "Delete a CI/CD variable from a GitLab project by key. Optionally filter by environment scope. This action cannot be undone.\n\nReturns: confirmation message. See also: gitlab_ci_variable_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete CI/CD variable %q from project %q?", input.Key, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ci_variable_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("CI/CD variable")
	})
}
