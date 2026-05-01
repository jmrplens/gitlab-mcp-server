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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_token_list",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_token_list"),
		Description: "List all tokens for a runner controller. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with tokens array including ID, description, and status.\n\nSee also: gitlab_runner_controller_token_create, gitlab_runner_controller_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_token_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_token_get",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_token_get"),
		Description: "Get a specific runner controller token. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with token details including ID, description, and status.\n\nSee also: gitlab_runner_controller_token_list, gitlab_runner_controller_token_rotate",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_token_get", start, err)
		return toolutil.WithHints(FormatGetMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_token_create",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_token_create"),
		Description: "Create a new runner controller token. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with created token including ID and token value.\n\nSee also: gitlab_runner_controller_token_list, gitlab_runner_controller_token_revoke",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_token_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_token_rotate",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_token_rotate"),
		Description: "Rotate a runner controller token. Returns a new token replacing the old one. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with new token including ID and rotated token value.\n\nSee also: gitlab_runner_controller_token_get, gitlab_runner_controller_token_revoke",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RotateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Rotate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_runner_controller_token_rotate", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_runner_controller_token_revoke",
		Title:       toolutil.TitleFromName("gitlab_runner_controller_token_revoke"),
		Description: "Revoke a runner controller token. This action cannot be undone. Admin only. Experimental: may change or be removed.\n\nReturns: JSON with deletion confirmation.\n\nSee also: gitlab_runner_controller_token_list, gitlab_runner_controller_token_create",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RevokeInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

// RegisterMeta registers the gitlab_runner_controller_token meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list":   toolutil.RouteAction(client, List),
		"get":    toolutil.RouteAction(client, Get),
		"create": toolutil.RouteAction(client, Create),
		"rotate": toolutil.DestructiveAction(client, Rotate),
		"revoke": toolutil.DestructiveVoidAction(client, Revoke),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_runner_controller_token",
		Title: toolutil.TitleFromName("gitlab_runner_controller_token"),
		Description: `Manage GitLab runner controller tokens (admin only, experimental). Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List all tokens for a controller. Params: controller_id (required, int), page, per_page
- get: Get a specific token. Params: controller_id (required, int), token_id (required, int)
- create: Create a new token. Params: controller_id (required, int), description
- rotate: Rotate a token. Params: controller_id (required, int), token_id (required, int)
- revoke: Revoke a token. Params: controller_id (required, int), token_id (required, int)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconToken,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_runner_controller_token", routes, nil))
}
