// register.go wires runner controller MCP tools to the MCP server.

package runnercontrollers

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runnercontrollerscopes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runnercontrollertokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all runner controller tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_list",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_list"),
		Description: "List all runner controllers. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with array of runner controllers and pagination info.\n\nSee also: gitlab_runner_controller_get, gitlab_runner_controller_scope_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_get",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_get"),
		Description: "Get detailed information about a runner controller. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with runner controller details (ID, description, state).\n\nSee also: gitlab_runner_controller_list, gitlab_runner_controller_token_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, DetailsOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_get", start, err)
		return toolutil.WithHints(FormatGetMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_create",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_create"),
		Description: "Register a new runner controller. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with the created runner controller details.\n\nSee also: gitlab_runner_controller_list, gitlab_runner_controller_scope_add_instance",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_update",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_update"),
		Description: "Update a runner controller's description or state. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with the updated runner controller details.\n\nSee also: gitlab_runner_controller_get, gitlab_runner_controller_delete",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_delete",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_delete"),
		Description: "Delete a runner controller. Admin only. This action cannot be undone. Experimental: may change or be removed.\n\nReturns: JSON confirmation of deletion.\n\nSee also: gitlab_runner_controller_list, gitlab_runner_controller_create",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete runner controller %d? This cannot be undone.", input.ControllerID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("runner controller")
	})
}

// RegisterMeta registers the gitlab_runner_controller meta-tool, consolidating
// controller CRUD, scope management, and token management into a single tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]toolutil.ActionFunc{
		// Controller CRUD
		"list":   toolutil.WrapAction(client, List),
		"get":    toolutil.WrapAction(client, Get),
		"create": toolutil.WrapAction(client, Create),
		"update": toolutil.WrapAction(client, Update),
		"delete": toolutil.WrapVoidAction(client, Delete),
		// Scope management
		"scope_list":            toolutil.WrapAction(client, runnercontrollerscopes.List),
		"scope_add_instance":    toolutil.WrapAction(client, runnercontrollerscopes.AddInstanceScope),
		"scope_remove_instance": toolutil.WrapVoidAction(client, runnercontrollerscopes.RemoveInstanceScope),
		"scope_add_runner":      toolutil.WrapAction(client, runnercontrollerscopes.AddRunnerScope),
		"scope_remove_runner":   toolutil.WrapVoidAction(client, runnercontrollerscopes.RemoveRunnerScope),
		// Token management
		"token_list":   toolutil.WrapAction(client, runnercontrollertokens.List),
		"token_get":    toolutil.WrapAction(client, runnercontrollertokens.Get),
		"token_create": toolutil.WrapAction(client, runnercontrollertokens.Create),
		"token_rotate": toolutil.WrapAction(client, runnercontrollertokens.Rotate),
		"token_revoke": toolutil.WrapVoidAction(client, runnercontrollertokens.Revoke),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_runner_controller",
		Title: toolutil.TitleFromName("gitlab_runner_controller"),
		Description: `Manage GitLab runner controllers, their scopes, and tokens (admin only, experimental). Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions (controller CRUD):
- list: List all runner controllers. Params: page, per_page
- get: Get runner controller details. Params: controller_id (required, int)
- create: Register a new runner controller. Params: description, state (enabled/disabled/dry_run)
- update: Update a runner controller. Params: controller_id (required, int), description, state (enabled/disabled/dry_run)
- delete: Delete a runner controller. Params: controller_id (required, int)

Actions (scope management):
- scope_list: List all scopes for a controller. Params: controller_id (required, int)
- scope_add_instance: Add instance-level scope. Params: controller_id (required, int)
- scope_remove_instance: Remove instance-level scope. Params: controller_id (required, int)
- scope_add_runner: Add runner scope (runner must be instance-level). Params: controller_id (required, int), runner_id (required, int)
- scope_remove_runner: Remove runner scope. Params: controller_id (required, int), runner_id (required, int)

Actions (token management):
- token_list: List all tokens for a controller. Params: controller_id (required, int), page, per_page
- token_get: Get a specific token. Params: controller_id (required, int), token_id (required, int)
- token_create: Create a new token. Params: controller_id (required, int), description
- token_rotate: Rotate a token. Params: controller_id (required, int), token_id (required, int)
- token_revoke: Revoke a token. Params: controller_id (required, int), token_id (required, int)`,
		Annotations: toolutil.MetaAnnotations,
		Icons:       toolutil.IconRunner,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_runner_controller", routes, nil))
}
