package groupstoragemoves

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab group repository storage move operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	storageMoveTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconServer})
	}

	mcp.AddTool(server, storageMoveTool("gitlab_retrieve_all_group_storage_moves", "Retrieve all group repository storage moves (admin only).\n\nReturns: JSON with array of storage moves and pagination.\n\nSee also: gitlab_retrieve_group_storage_moves, gitlab_get_group_storage_move"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := RetrieveAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_retrieve_all_group_storage_moves", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, storageMoveTool("gitlab_retrieve_group_storage_moves", "Retrieve all repository storage moves for a specific group (admin only).\n\nReturns: JSON with array of storage moves and pagination.\n\nSee also: gitlab_retrieve_all_group_storage_moves"), func(ctx context.Context, req *mcp.CallToolRequest, input ListForGroupInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := RetrieveForGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_retrieve_group_storage_moves", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, storageMoveTool("gitlab_get_group_storage_move", "Get a single group repository storage move by ID (admin only).\n\nReturns: JSON with storage move details.\n\nSee also: gitlab_get_group_storage_move_for_group"), func(ctx context.Context, req *mcp.CallToolRequest, input IDInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_group_storage_move", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, storageMoveTool("gitlab_get_group_storage_move_for_group", "Get a single repository storage move for a specific group (admin only).\n\nReturns: JSON with storage move details.\n\nSee also: gitlab_get_group_storage_move"), func(ctx context.Context, req *mcp.CallToolRequest, input GroupMoveInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetForGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_group_storage_move_for_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, storageMoveTool("gitlab_schedule_group_storage_move", "Schedule a repository storage move for a group (admin only).\n\nReturns: JSON with the scheduled storage move.\n\nSee also: gitlab_schedule_all_group_storage_moves"), func(ctx context.Context, req *mcp.CallToolRequest, input ScheduleInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Schedule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_schedule_group_storage_move", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, storageMoveTool("gitlab_schedule_all_group_storage_moves", "Schedule repository storage moves for all groups on a storage shard (admin only).\n\nReturns: JSON with confirmation message.\n\nSee also: gitlab_schedule_group_storage_move"), func(ctx context.Context, req *mcp.CallToolRequest, input ScheduleAllInput) (*mcp.CallToolResult, ScheduleAllOutput, error) {
		start := time.Now()
		out, err := ScheduleAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_schedule_all_group_storage_moves", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatScheduleAllMarkdown(out)), out, err)
	})
}
