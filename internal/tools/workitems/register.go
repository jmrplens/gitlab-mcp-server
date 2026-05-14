package workitems

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all work item tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	workItemTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconIssue})
	}

	mcp.AddTool(server, workItemTool("gitlab_get_work_item", "Get a single work item by IID. Experimental: the Work Items API may introduce breaking changes between minor versions.\n\nReturns: JSON with work item details (IID, title, state, type, description, assignees, labels, dates).\n\nSee also: gitlab_list_work_items, gitlab_update_work_item"), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_work_item", start, err)
		result := FormatGetMarkdown(out)
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, workItemTool("gitlab_list_work_items", "List work items for a project or group. Supports filtering by state, type, labels, author, search. Experimental: the Work Items API may introduce breaking changes between minor versions.\n\nReturns: JSON with array of work items and pagination info.\n\nSee also: gitlab_get_work_item, gitlab_create_work_item"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_work_items", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, workItemTool("gitlab_create_work_item", "Create a new work item. Requires full_path, work_item_type_id, and title. Supports linked_items to link other work items on creation. Experimental: the Work Items API may introduce breaking changes between minor versions.\n\nReturns: JSON with the created work item details.\n\nSee also: gitlab_list_work_items, gitlab_update_work_item"), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_work_item", start, err)
		result := FormatGetMarkdown(out)
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, workItemTool("gitlab_update_work_item", "Update an existing work item by IID. Supports changing title, state (CLOSE/REOPEN), description, assignees, milestone, labels (add/remove), dates, weight, health status, iteration, color, and status (TODO/IN_PROGRESS/DONE/WONT_DO/DUPLICATE). Experimental: the Work Items API may introduce breaking changes between minor versions.\n\nReturns: JSON with the updated work item details.\n\nSee also: gitlab_get_work_item, gitlab_delete_work_item"), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_work_item", start, err)
		result := FormatGetMarkdown(out)
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, workItemTool("gitlab_delete_work_item", "Permanently delete a work item by IID. This action cannot be undone. Experimental: the Work Items API may introduce breaking changes between minor versions.\n\nReturns: JSON confirmation of deletion.\n\nSee also: gitlab_list_work_items, gitlab_create_work_item"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Permanently delete work item #%d in %q? This action cannot be undone.", input.IID, input.FullPath)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_work_item", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("work item #%d from %s", input.IID, input.FullPath))
	})
}
