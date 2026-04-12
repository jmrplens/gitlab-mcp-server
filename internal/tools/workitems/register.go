// register.go wires workitems MCP tools to the MCP server.

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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_work_item",
		Title:       toolutil.TitleFromName("gitlab_get_work_item"),
		Description: "Get a single work item by IID. Experimental: the Work Items API may introduce breaking changes between minor versions.\n\nReturns: JSON with work item details (IID, title, state, type, description, assignees, labels, dates).\n\nSee also: gitlab_list_work_items, gitlab_update_work_item",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconIssue,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_work_item", start, err)
		result := FormatGetMarkdown(out)
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_work_items",
		Title:       toolutil.TitleFromName("gitlab_list_work_items"),
		Description: "List work items for a project or group. Supports filtering by state, type, labels, author, search. Experimental: the Work Items API may introduce breaking changes between minor versions.\n\nReturns: JSON with array of work items and pagination info.\n\nSee also: gitlab_get_work_item, gitlab_create_work_item",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconIssue,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_work_items", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_work_item",
		Title:       toolutil.TitleFromName("gitlab_create_work_item"),
		Description: "Create a new work item. Requires full_path, work_item_type_id, and title. Experimental: the Work Items API may introduce breaking changes between minor versions.\n\nReturns: JSON with the created work item details.\n\nSee also: gitlab_list_work_items, gitlab_update_work_item",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconIssue,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_work_item", start, err)
		result := FormatGetMarkdown(out)
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_work_item",
		Title:       toolutil.TitleFromName("gitlab_update_work_item"),
		Description: "Update an existing work item by IID. Supports changing title, state (CLOSE/REOPEN), description, assignees, milestone, labels (add/remove), dates, weight, health status, iteration, color. Experimental: the Work Items API may introduce breaking changes between minor versions.\n\nReturns: JSON with the updated work item details.\n\nSee also: gitlab_get_work_item, gitlab_delete_work_item",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconIssue,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_work_item", start, err)
		result := FormatGetMarkdown(out)
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_work_item",
		Title:       toolutil.TitleFromName("gitlab_delete_work_item"),
		Description: "Permanently delete a work item by IID. This action cannot be undone. Experimental: the Work Items API may introduce breaking changes between minor versions.\n\nReturns: JSON confirmation of deletion.\n\nSee also: gitlab_list_work_items, gitlab_create_work_item",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconIssue,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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
