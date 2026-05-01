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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_retrieve_all_group_storage_moves",
		Title:       toolutil.TitleFromName("gitlab_retrieve_all_group_storage_moves"),
		Description: "Retrieve all group repository storage moves (admin only).\n\nReturns: JSON with array of storage moves and pagination.\n\nSee also: gitlab_retrieve_group_storage_moves, gitlab_get_group_storage_move",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := RetrieveAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_retrieve_all_group_storage_moves", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_retrieve_group_storage_moves",
		Title:       toolutil.TitleFromName("gitlab_retrieve_group_storage_moves"),
		Description: "Retrieve all repository storage moves for a specific group (admin only).\n\nReturns: JSON with array of storage moves and pagination.\n\nSee also: gitlab_retrieve_all_group_storage_moves",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListForGroupInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := RetrieveForGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_retrieve_group_storage_moves", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_group_storage_move",
		Title:       toolutil.TitleFromName("gitlab_get_group_storage_move"),
		Description: "Get a single group repository storage move by ID (admin only).\n\nReturns: JSON with storage move details.\n\nSee also: gitlab_get_group_storage_move_for_group",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input IDInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_group_storage_move", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_group_storage_move_for_group",
		Title:       toolutil.TitleFromName("gitlab_get_group_storage_move_for_group"),
		Description: "Get a single repository storage move for a specific group (admin only).\n\nReturns: JSON with storage move details.\n\nSee also: gitlab_get_group_storage_move",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GroupMoveInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetForGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_group_storage_move_for_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_schedule_group_storage_move",
		Title:       toolutil.TitleFromName("gitlab_schedule_group_storage_move"),
		Description: "Schedule a repository storage move for a group (admin only).\n\nReturns: JSON with the scheduled storage move.\n\nSee also: gitlab_schedule_all_group_storage_moves",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ScheduleInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Schedule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_schedule_group_storage_move", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_schedule_all_group_storage_moves",
		Title:       toolutil.TitleFromName("gitlab_schedule_all_group_storage_moves"),
		Description: "Schedule repository storage moves for all groups on a storage shard (admin only).\n\nReturns: JSON with confirmation message.\n\nSee also: gitlab_schedule_group_storage_move",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ScheduleAllInput) (*mcp.CallToolResult, ScheduleAllOutput, error) {
		start := time.Now()
		out, err := ScheduleAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_schedule_all_group_storage_moves", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatScheduleAllMarkdown(out)), out, err)
	})
}
