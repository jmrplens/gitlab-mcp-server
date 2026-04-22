// register.go wires runner controller scope MCP tools to the MCP server.

package runnercontrollerscopes

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all runner controller scope tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_scope_list",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_scope_list"),
		Description: "List all scopes for a runner controller. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with array of scopes for the controller.\n\nSee also: gitlab_runner_controller_list, gitlab_runner_controller_token_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ScopesOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_scope_list", start, err)
		return toolutil.WithHints(FormatScopesResult(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_scope_add_instance",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_scope_add_instance"),
		Description: "Add an instance-level scope to a runner controller. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with the added instance-level scope details.\n\nSee also: gitlab_runner_controller_scope_list, gitlab_runner_controller_scope_remove_instance",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddInstanceScopeInput) (*mcp.CallToolResult, InstanceScopeOutput, error) {
		start := time.Now()
		out, err := AddInstanceScope(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_scope_add_instance", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatInstanceScopeMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_scope_remove_instance",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_scope_remove_instance"),
		Description: "Remove the instance-level scope from a runner controller. Admin only. Experimental: may change or be removed.\n\nReturns: JSON confirmation of scope removal.\n\nSee also: gitlab_runner_controller_scope_list, gitlab_runner_controller_scope_add_instance",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RemoveInstanceScopeInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Remove instance-level scope from controller %d?", input.ControllerID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := RemoveInstanceScope(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_scope_remove_instance", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("instance-level scope")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_scope_add_runner",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_scope_add_runner"),
		Description: "Add a runner scope to a runner controller. The runner must be instance-level. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with the added runner scope details.\n\nSee also: gitlab_runner_controller_scope_list, gitlab_runner_controller_scope_remove_runner",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddRunnerScopeInput) (*mcp.CallToolResult, RunnerScopeOutput, error) {
		start := time.Now()
		out, err := AddRunnerScope(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_scope_add_runner", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRunnerScopeMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_scope_remove_runner",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_scope_remove_runner"),
		Description: "Remove a runner scope from a runner controller. Admin only. Experimental: may change or be removed.\n\nReturns: JSON confirmation of scope removal.\n\nSee also: gitlab_runner_controller_scope_list, gitlab_runner_controller_scope_add_runner",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconRunner,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RemoveRunnerScopeInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Remove runner %d scope from controller %d?", input.RunnerID, input.ControllerID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := RemoveRunnerScope(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_scope_remove_runner", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("runner scope")
	})
}

// RegisterMeta registers the gitlab_runner_controller_scope meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list":            toolutil.RouteAction(client, List),
		"add_instance":    toolutil.RouteAction(client, AddInstanceScope),
		"remove_instance": toolutil.DestructiveVoidAction(client, RemoveInstanceScope),
		"add_runner":      toolutil.RouteAction(client, AddRunnerScope),
		"remove_runner":   toolutil.DestructiveVoidAction(client, RemoveRunnerScope),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_runner_controller_scope",
		Title: toolutil.TitleFromName("gitlab_runner_controller_scope"),
		Description: `Manage GitLab runner controller scopes (admin only, experimental). Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List all scopes for a controller. Params: controller_id (required, int)
- add_instance: Add instance-level scope. Params: controller_id (required, int)
- remove_instance: Remove instance-level scope. Params: controller_id (required, int)
- add_runner: Add runner scope (runner must be instance-level). Params: controller_id (required, int), runner_id (required, int)
- remove_runner: Remove runner scope. Params: controller_id (required, int), runner_id (required, int)`,
		Annotations: toolutil.DeriveAnnotations(routes),
		Icons:       toolutil.IconRunner,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_runner_controller_scope", routes, nil))
}
