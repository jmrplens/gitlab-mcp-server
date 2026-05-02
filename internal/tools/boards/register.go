package boards

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all project issue board individual tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	// ----- Board CRUD -----
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_board_list",
		Title:       toolutil.TitleFromName("gitlab_board_list"),
		Description: "List all issue boards for a project\n\nSee also: gitlab_board_create, gitlab_issue_list\n\nReturns: JSON array of boards with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListBoardsInput) (*mcp.CallToolResult, ListBoardsOutput, error) {
		start := time.Now()
		out, err := ListBoards(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListBoardsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_board_get",
		Title:       toolutil.TitleFromName("gitlab_board_get"),
		Description: "Get a single issue board\n\nSee also: gitlab_board_list, gitlab_board_list_lists\n\nReturns: JSON with board details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetBoardInput) (*mcp.CallToolResult, BoardOutput, error) {
		start := time.Now()
		out, err := GetBoard(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatBoardMarkdown(out))
		if err == nil && out.ID != 0 && string(input.ProjectID) != "" {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://project/%s/board/%d", url.PathEscape(string(input.ProjectID)), out.ID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_board_create",
		Title:       toolutil.TitleFromName("gitlab_board_create"),
		Description: "Create a new issue board in a project\n\nSee also: gitlab_board_list, gitlab_board_list_create\n\nReturns: JSON with the board details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateBoardInput) (*mcp.CallToolResult, BoardOutput, error) {
		start := time.Now()
		out, err := CreateBoard(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_board_update",
		Title:       toolutil.TitleFromName("gitlab_board_update"),
		Description: "Update an existing issue board\n\nSee also: gitlab_board_get, gitlab_board_list\n\nReturns: JSON with the board details.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateBoardInput) (*mcp.CallToolResult, BoardOutput, error) {
		start := time.Now()
		out, err := UpdateBoard(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_board_delete",
		Title:       toolutil.TitleFromName("gitlab_board_delete"),
		Description: "Delete an issue board from a project. This action cannot be undone.\n\nSee also: gitlab_board_list, gitlab_board_create\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteBoardInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete board %d from project %s?", input.BoardID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteBoard(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("board")
	})

	// ----- Board List CRUD -----
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_board_list_lists",
		Title:       toolutil.TitleFromName("gitlab_board_list_lists"),
		Description: "List all lists in an issue board\n\nSee also: gitlab_board_list_create, gitlab_board_get\n\nReturns: JSON array of board lists with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListBoardListsInput) (*mcp.CallToolResult, ListBoardListsOutput, error) {
		start := time.Now()
		out, err := ListBoardLists(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_list_lists", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListBoardListsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_board_list_get",
		Title:       toolutil.TitleFromName("gitlab_board_list_get"),
		Description: "Get a single list from an issue board\n\nSee also: gitlab_board_list_lists, gitlab_board_list_update\n\nReturns: JSON with board list details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetBoardListInput) (*mcp.CallToolResult, BoardListOutput, error) {
		start := time.Now()
		out, err := GetBoardList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_list_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_board_list_create",
		Title:       toolutil.TitleFromName("gitlab_board_list_create"),
		Description: "Create a new list in an issue board\n\nSee also: gitlab_board_list_lists, gitlab_label_list\n\nReturns: JSON with the board list details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateBoardListInput) (*mcp.CallToolResult, BoardListOutput, error) {
		start := time.Now()
		out, err := CreateBoardList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_list_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_board_list_update",
		Title:       toolutil.TitleFromName("gitlab_board_list_update"),
		Description: "Update (reorder) a list in an issue board\n\nSee also: gitlab_board_list_get, gitlab_board_list_lists\n\nReturns: JSON with the board list details.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateBoardListInput) (*mcp.CallToolResult, BoardListOutput, error) {
		start := time.Now()
		out, err := UpdateBoardList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_list_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_board_list_delete",
		Title:       toolutil.TitleFromName("gitlab_board_list_delete"),
		Description: "Delete a list from an issue board. This action cannot be undone.\n\nSee also: gitlab_board_list_lists, gitlab_board_list_create\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteBoardListInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete list %d from board %d in project %s?", input.ListID, input.BoardID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteBoardList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_list_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("board list")
	})
}

// RegisterMeta registers the gitlab_board meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list":        toolutil.RouteAction(client, ListBoards),
		"get":         toolutil.RouteAction(client, GetBoard),
		"create":      toolutil.RouteAction(client, CreateBoard),
		"update":      toolutil.RouteAction(client, UpdateBoard),
		"delete":      toolutil.DestructiveVoidAction(client, DeleteBoard),
		"list_lists":  toolutil.RouteAction(client, ListBoardLists),
		"get_list":    toolutil.RouteAction(client, GetBoardList),
		"create_list": toolutil.RouteAction(client, CreateBoardList),
		"update_list": toolutil.RouteAction(client, UpdateBoardList),
		"delete_list": toolutil.DestructiveVoidAction(client, DeleteBoardList),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_board",
		Title: toolutil.TitleFromName("gitlab_board"),
		Description: `Project issue board operations. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List all issue boards (project_id, page, per_page)
- get: Get a single board (project_id, board_id)
- create: Create a board (project_id, name)
- update: Update a board (project_id, board_id, name, assignee_id, milestone_id, labels, weight, hide_backlog_list, hide_closed_list)
- delete: Delete a board (project_id, board_id)
- list_lists: List all lists in a board (project_id, board_id, page, per_page)
- get_list: Get a single board list (project_id, board_id, list_id)
- create_list: Create a board list (project_id, board_id, label_id, assignee_id, milestone_id, iteration_id)
- update_list: Reorder a board list (project_id, board_id, list_id, position)
- delete_list: Delete a board list (project_id, board_id, list_id)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconBoard,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_board", routes, nil))
}
