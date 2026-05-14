package projectstoragemoves

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab project repository storage move operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	storageMoveTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconServer})
	}

	mcp.AddTool(server, storageMoveTool("gitlab_retrieve_all_project_storage_moves", "Retrieve all project repository storage moves (admin only).\n\nReturns: JSON with array of storage moves and pagination.\n\nSee also: gitlab_retrieve_project_storage_moves, gitlab_get_project_storage_move"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := RetrieveAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_retrieve_all_project_storage_moves", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, storageMoveTool("gitlab_retrieve_project_storage_moves", "Retrieve all repository storage moves for a specific project (admin only).\n\nReturns: JSON with array of storage moves and pagination.\n\nSee also: gitlab_retrieve_all_project_storage_moves"), func(ctx context.Context, req *mcp.CallToolRequest, input ListForProjectInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := RetrieveForProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_retrieve_project_storage_moves", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, storageMoveTool("gitlab_get_project_storage_move", "Get a single project repository storage move by ID (admin only).\n\nReturns: JSON with storage move details.\n\nSee also: gitlab_get_project_storage_move_for_project"), func(ctx context.Context, req *mcp.CallToolRequest, input IDInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_storage_move", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, storageMoveTool("gitlab_get_project_storage_move_for_project", "Get a single repository storage move for a specific project (admin only).\n\nReturns: JSON with storage move details.\n\nSee also: gitlab_get_project_storage_move"), func(ctx context.Context, req *mcp.CallToolRequest, input ProjectMoveInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetForProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_storage_move_for_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, storageMoveTool("gitlab_schedule_project_storage_move", "Schedule a repository storage move for a project (admin only).\n\nReturns: JSON with the scheduled storage move.\n\nSee also: gitlab_schedule_all_project_storage_moves"), func(ctx context.Context, req *mcp.CallToolRequest, input ScheduleInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Schedule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_schedule_project_storage_move", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, storageMoveTool("gitlab_schedule_all_project_storage_moves", "Schedule repository storage moves for all projects on a storage shard (admin only).\n\nReturns: JSON with confirmation message.\n\nSee also: gitlab_schedule_project_storage_move"), func(ctx context.Context, req *mcp.CallToolRequest, input ScheduleAllInput) (*mcp.CallToolResult, ScheduleAllOutput, error) {
		start := time.Now()
		out, err := ScheduleAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_schedule_all_project_storage_moves", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatScheduleAllMarkdown(out)), out, err)
	})
}
