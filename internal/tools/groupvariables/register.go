// register.go wires groupvariables MCP tools to the MCP server.

package groupvariables

import (
	"context"
	"fmt"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers the five group CI/CD variable management tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlab.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_variable_list",
		Title:       toolutil.TitleFromName("gitlab_group_variable_list"),
		Description: "List CI/CD variables for a GitLab group. Returns paginated results with variable key, type, protection, masking, and environment scope.\n\nReturns: paginated list of variables with key, variable_type, protected, masked, and environment_scope. See also: gitlab_group_variable_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_variable_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_variable_get",
		Title:       toolutil.TitleFromName("gitlab_group_variable_get"),
		Description: "Get a specific CI/CD variable by key from a GitLab group. Optionally filter by environment scope when duplicate keys exist. Returns: key, value, variable_type, protected, masked, environment_scope. See also: gitlab_group_variable_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_variable_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_variable_create",
		Title:       toolutil.TitleFromName("gitlab_group_variable_create"),
		Description: "Create a new CI/CD variable in a GitLab group. Requires key and value. Optionally set type (env_var/file), protection, masking, and environment scope. Returns: key, value, variable_type, protected, masked, environment_scope. See also: gitlab_group_variable_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_variable_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_variable_update",
		Title:       toolutil.TitleFromName("gitlab_group_variable_update"),
		Description: "Update an existing CI/CD variable in a GitLab group. Specify the key to update and any fields to change: value, type, protection, masking, environment scope. Returns: key, value, variable_type, protected, masked, environment_scope. See also: gitlab_group_variable_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_variable_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_variable_delete",
		Title:       toolutil.TitleFromName("gitlab_group_variable_delete"),
		Description: "Delete a CI/CD variable from a GitLab group by key. Optionally filter by environment scope. This action cannot be undone.\n\nReturns: confirmation message. See also: gitlab_group_variable_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconVariable,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete group CI/CD variable %q from group %q?", input.Key, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_variable_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group CI/CD variable")
	})
}

// RegisterMeta registers the gitlab_group_variable meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlab.Client) {
	routes := toolutil.ActionMap{
		"list":   toolutil.RouteAction(client, List),
		"get":    toolutil.RouteAction(client, Get),
		"create": toolutil.RouteAction(client, Create),
		"update": toolutil.RouteAction(client, Update),
		"delete": toolutil.DestructiveVoidAction(client, Delete),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_group_variable",
		Title: toolutil.TitleFromName("gitlab_group_variable"),
		Description: `Manage CI/CD variables in a GitLab group. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List variables. Params: group_id (required), page, per_page
- get: Get variable by key. Params: group_id (required), key (required), environment_scope
- create: Create variable. Params: group_id (required), key (required), value (required), description, variable_type, protected (bool), masked (bool), masked_and_hidden (bool), raw (bool), environment_scope
- update: Update variable. Params: group_id (required), key (required), value, description, variable_type, protected (bool), masked (bool), raw (bool), environment_scope
- delete: Delete variable. Params: group_id (required), key (required), environment_scope`,
		Annotations: toolutil.DeriveAnnotations(routes),
		Icons:       toolutil.IconVariable,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_group_variable", routes, nil))
}
