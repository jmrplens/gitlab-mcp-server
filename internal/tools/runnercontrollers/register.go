package runnercontrollers

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all runner controller tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	runnerControllerTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconRunner})
	}

	mcp.AddTool(server, runnerControllerTool("gitlab_runner_controller_list", "List all runner controllers. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with array of runner controllers and pagination info.\n\nSee also: gitlab_runner_controller_get, gitlab_runner_controller_scope_list"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerControllerTool("gitlab_runner_controller_get", "Get detailed information about a runner controller. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with runner controller details (ID, description, state).\n\nSee also: gitlab_runner_controller_list, gitlab_runner_controller_token_list"), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, DetailsOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_get", start, err)
		return toolutil.WithHints(FormatGetMarkdown(out), out, err)
	})

	mcp.AddTool(server, runnerControllerTool("gitlab_runner_controller_create", "Register a new runner controller. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with the created runner controller details.\n\nSee also: gitlab_runner_controller_list, gitlab_runner_controller_scope_add_instance"), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerControllerTool("gitlab_runner_controller_update", "Update a runner controller's description or state. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with the updated runner controller details.\n\nSee also: gitlab_runner_controller_get, gitlab_runner_controller_delete"), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerControllerTool("gitlab_runner_controller_delete", "Delete a runner controller. Admin only. This action cannot be undone. Experimental: may change or be removed.\n\nReturns: JSON confirmation of deletion.\n\nSee also: gitlab_runner_controller_list, gitlab_runner_controller_create"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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
