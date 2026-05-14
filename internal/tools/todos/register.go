package todos

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all to-do MCP tools on the given server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	todoTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconTodo})
	}

	mcp.AddTool(server, todoTool("gitlab_todo_list", "List pending to-do items for the authenticated user. Returns paginated results with action, target, type, and state. Use page and per_page for pagination.\n\nReturns: JSON array of to-do items with pagination.\n\nSee also: gitlab_todo_mark_done, gitlab_todo_mark_all_done"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_todo_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, todoTool("gitlab_todo_mark_done", "Mark a single pending to-do item as done by its ID. Use gitlab_todo_list to find to-do item IDs first.\n\nReturns: JSON with the marked to-do item details.\n\nSee also: gitlab_todo_list"), func(ctx context.Context, req *mcp.CallToolRequest, input MarkDoneInput) (*mcp.CallToolResult, MarkDoneOutput, error) {
		start := time.Now()
		out, err := MarkDone(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_todo_mark_done", start, err)
		return toolutil.WithHints(FormatMarkDoneMarkdown(out), out, err)
	})

	mcp.AddTool(server, todoTool("gitlab_todo_mark_all_done", "Mark ALL pending to-do items as done for the authenticated user. This affects all pending to-dos, not just those on a specific project.\n\nReturns: JSON with count of marked to-do items.\n\nSee also: gitlab_todo_list"), func(ctx context.Context, req *mcp.CallToolRequest, input MarkAllDoneInput) (*mcp.CallToolResult, MarkAllDoneOutput, error) {
		start := time.Now()
		out, err := MarkAllDone(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_todo_mark_all_done", start, err)
		return toolutil.WithHints(FormatMarkAllDoneMarkdown(out), out, err)
	})
}
