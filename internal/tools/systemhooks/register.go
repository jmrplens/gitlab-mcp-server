package systemhooks

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all system hooks tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	systemHookTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconIntegration})
	}

	mcp.AddTool(server, systemHookTool("gitlab_list_system_hooks", "List all system hooks (admin). Returns ID, URL and event subscriptions.\n\nReturns: JSON with array of system hooks and pagination info.\n\nSee also: gitlab_add_system_hook, gitlab_list_integrations"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_system_hooks", start, err)
		if err != nil {
			return nil, ListOutput{}, err
		}
		return toolutil.WithHints(FormatListMarkdown(out), out, nil)
	})

	mcp.AddTool(server, systemHookTool("gitlab_get_system_hook", "Get a system hook by ID (admin).\n\nReturns: JSON with system hook details (ID, URL, event subscriptions, SSL settings).\n\nSee also: gitlab_list_system_hooks, gitlab_test_system_hook"), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_system_hook", start, err)
		if err != nil {
			return nil, GetOutput{}, err
		}
		return toolutil.WithHints(FormatHookMarkdown(out.Hook), out, nil)
	})

	mcp.AddTool(server, systemHookTool("gitlab_add_system_hook", "Add a new system hook (admin). Requires URL. Optionally configure event subscriptions and SSL verification.\n\nReturns: JSON with the created system hook details.\n\nSee also: gitlab_list_system_hooks, gitlab_delete_system_hook"), func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, AddOutput, error) {
		start := time.Now()
		out, err := Add(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_system_hook", start, err)
		if err != nil {
			return nil, AddOutput{}, err
		}
		return toolutil.WithHints(FormatHookMarkdown(out.Hook), out, nil)
	})

	mcp.AddTool(server, systemHookTool("gitlab_test_system_hook", "Test a system hook by ID (admin). Triggers a test event and returns the result.\n\nReturns: JSON with test event result.\n\nSee also: gitlab_get_system_hook, gitlab_add_system_hook"), func(ctx context.Context, req *mcp.CallToolRequest, input TestInput) (*mcp.CallToolResult, TestOutput, error) {
		start := time.Now()
		out, err := Test(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_test_system_hook", start, err)
		if err != nil {
			return nil, TestOutput{}, err
		}
		return toolutil.WithHints(FormatTestMarkdown(out), out, nil)
	})

	mcp.AddTool(server, systemHookTool("gitlab_delete_system_hook", "Delete a system hook by ID (admin).\n\nReturns: JSON confirmation of deletion.\n\nSee also: gitlab_list_system_hooks, gitlab_add_system_hook"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete system hook %d?", input.ID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_system_hook", start, err)
		r, o, _ := toolutil.DeleteResult("system hook")
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return r, o, nil
	})
}
