package runnercontrollertokens

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all runner controller token tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	runnerControllerTokenTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconToken})
	}

	mcp.AddTool(server, runnerControllerTokenTool("gitlab_runner_controller_token_list", "List all tokens for a runner controller. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with tokens array including ID, description, and status.\n\nSee also: gitlab_runner_controller_token_create, gitlab_runner_controller_list"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_token_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerControllerTokenTool("gitlab_runner_controller_token_get", "Get a specific runner controller token. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with token details including ID, description, and status.\n\nSee also: gitlab_runner_controller_token_list, gitlab_runner_controller_token_rotate"), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_token_get", start, err)
		return toolutil.WithHints(FormatGetMarkdown(out), out, err)
	})

	mcp.AddTool(server, runnerControllerTokenTool("gitlab_runner_controller_token_create", "Create a new runner controller token. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with created token including ID and token value.\n\nSee also: gitlab_runner_controller_token_list, gitlab_runner_controller_token_revoke"), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_token_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerControllerTokenTool("gitlab_runner_controller_token_rotate", "Rotate a runner controller token. Returns a new token replacing the old one. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with new token including ID and rotated token value.\n\nSee also: gitlab_runner_controller_token_get, gitlab_runner_controller_token_revoke"), func(ctx context.Context, req *mcp.CallToolRequest, input RotateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Rotate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_token_rotate", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, runnerControllerTokenTool("gitlab_runner_controller_token_revoke", "Revoke a runner controller token. This action cannot be undone. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with deletion confirmation.\n\nSee also: gitlab_runner_controller_token_list, gitlab_runner_controller_token_create"), func(ctx context.Context, req *mcp.CallToolRequest, input RevokeInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Revoke runner controller token %d? This cannot be undone.", input.TokenID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Revoke(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_token_revoke", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("runner controller token")
	})
}
