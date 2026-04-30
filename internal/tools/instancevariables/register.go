// register.go wires instancevariables MCP tools to the MCP server.
package instancevariables

import (
	"context"
	"fmt"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers the five instance CI/CD variable management tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlab.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_instance_variable_list",
		Title:       toolutil.TitleFromName("gitlab_instance_variable_list"),
		Description: "List CI/CD variables at the GitLab instance level. Returns paginated results with variable key, type, protection, and masking.\n\nReturns: JSON with array of instance-level CI/CD variables and pagination info. See also: gitlab_instance_variable_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_instance_variable_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_instance_variable_get",
		Title:       toolutil.TitleFromName("gitlab_instance_variable_get"),
		Description: "Get a specific CI/CD variable by key from the GitLab instance level.\n\nReturns: JSON with variable details (key, value, type, protection, masking). See also: gitlab_instance_variable_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_instance_variable_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_instance_variable_create",
		Title:       toolutil.TitleFromName("gitlab_instance_variable_create"),
		Description: "Create a new CI/CD variable at the GitLab instance level. Requires key and value. Optionally set type (env_var/file), protection, and masking.\n\nReturns: JSON with the created variable details. See also: gitlab_instance_variable_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_instance_variable_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_instance_variable_update",
		Title:       toolutil.TitleFromName("gitlab_instance_variable_update"),
		Description: "Update an existing CI/CD variable at the GitLab instance level. Specify the key to update and any fields to change.\n\nReturns: JSON with the updated variable details. See also: gitlab_instance_variable_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_instance_variable_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_instance_variable_delete",
		Title:       toolutil.TitleFromName("gitlab_instance_variable_delete"),
		Description: "Delete a CI/CD variable from the GitLab instance level by key. This action cannot be undone.\n\nReturns: JSON confirmation of deletion. See also: gitlab_instance_variable_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete instance CI/CD variable %q?", input.Key)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_instance_variable_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("instance CI/CD variable")
	})
}

// RegisterMeta registers the gitlab_instance_variable meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlab.Client) {
	routes := toolutil.ActionMap{
		"list":   toolutil.RouteAction(client, List),
		"get":    toolutil.RouteAction(client, Get),
		"create": toolutil.RouteAction(client, Create),
		"update": toolutil.RouteAction(client, Update),
		"delete": toolutil.DestructiveVoidAction(client, Delete),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_instance_variable",
		Title: toolutil.TitleFromName("gitlab_instance_variable"),
		Description: `Manage CI/CD variables at the GitLab instance level. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List variables. Params: page, per_page
- get: Get variable by key. Params: key (required)
- create: Create variable. Params: key (required), value (required), description, variable_type, protected (bool), masked (bool), raw (bool)
- update: Update variable. Params: key (required), value, description, variable_type, protected (bool), masked (bool), raw (bool)
- delete: Delete variable. Params: key (required)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconVariable,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_instance_variable", routes, nil))
}
