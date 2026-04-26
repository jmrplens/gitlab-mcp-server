// register.go wires groupboards MCP tools to the MCP server.

package groupboards

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all group issue board individual tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	// ----- Group Board CRUD -----
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_board_list",
		Title:       toolutil.TitleFromName("gitlab_group_board_list"),
		Description: "List all issue boards for a group\n\nSee also: gitlab_group_board_create, gitlab_list_group_issues\n\nReturns: JSON array of boards with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupBoardsInput) (*mcp.CallToolResult, ListGroupBoardsOutput, error) {
		start := time.Now()
		out, err := ListGroupBoards(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListGroupBoardsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_board_get",
		Title:       toolutil.TitleFromName("gitlab_group_board_get"),
		Description: "Get a single group issue board\n\nSee also: gitlab_group_board_list, gitlab_group_board_list_lists\n\nReturns: JSON with board details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetGroupBoardInput) (*mcp.CallToolResult, GroupBoardOutput, error) {
		start := time.Now()
		out, err := GetGroupBoard(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGroupBoardMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_board_create",
		Title:       toolutil.TitleFromName("gitlab_group_board_create"),
		Description: "Create a new issue board in a group\n\nSee also: gitlab_group_board_list, gitlab_group_board_list_create\n\nReturns: JSON with the board details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateGroupBoardInput) (*mcp.CallToolResult, GroupBoardOutput, error) {
		start := time.Now()
		out, err := CreateGroupBoard(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGroupBoardMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_board_update",
		Title:       toolutil.TitleFromName("gitlab_group_board_update"),
		Description: "Update an existing group issue board\n\nSee also: gitlab_group_board_get, gitlab_group_board_list\n\nReturns: JSON with the board details.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateGroupBoardInput) (*mcp.CallToolResult, GroupBoardOutput, error) {
		start := time.Now()
		out, err := UpdateGroupBoard(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGroupBoardMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_board_delete",
		Title:       toolutil.TitleFromName("gitlab_group_board_delete"),
		Description: "Delete a group issue board. This action cannot be undone.\n\nSee also: gitlab_group_board_list, gitlab_group_board_create\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteGroupBoardInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete board %d from group %s?", input.BoardID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteGroupBoard(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group board")
	})

	// ----- Group Board List CRUD -----
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_board_list_lists",
		Title:       toolutil.TitleFromName("gitlab_group_board_list_lists"),
		Description: "List all lists in a group issue board\n\nSee also: gitlab_group_board_list_create, gitlab_group_board_get\n\nReturns: JSON array of board lists with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupBoardListsInput) (*mcp.CallToolResult, ListBoardListsOutput, error) {
		start := time.Now()
		out, err := ListGroupBoardLists(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_list_lists", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListBoardListsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_board_list_get",
		Title:       toolutil.TitleFromName("gitlab_group_board_list_get"),
		Description: "Get a single list from a group issue board\n\nSee also: gitlab_group_board_list_lists, gitlab_group_board_list_update\n\nReturns: JSON with board list details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetGroupBoardListInput) (*mcp.CallToolResult, BoardListOutput, error) {
		start := time.Now()
		out, err := GetGroupBoardList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_list_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_board_list_create",
		Title:       toolutil.TitleFromName("gitlab_group_board_list_create"),
		Description: "Create a new list in a group issue board\n\nSee also: gitlab_group_board_list_lists, gitlab_group_label_list\n\nReturns: JSON with the board list details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateGroupBoardListInput) (*mcp.CallToolResult, BoardListOutput, error) {
		start := time.Now()
		out, err := CreateGroupBoardList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_list_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_board_list_update",
		Title:       toolutil.TitleFromName("gitlab_group_board_list_update"),
		Description: "Update (reorder) a list in a group issue board\n\nSee also: gitlab_group_board_list_get, gitlab_group_board_list_lists\n\nReturns: JSON with the board list details.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateGroupBoardListInput) (*mcp.CallToolResult, BoardListOutput, error) {
		start := time.Now()
		out, err := UpdateGroupBoardList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_list_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_board_list_delete",
		Title:       toolutil.TitleFromName("gitlab_group_board_list_delete"),
		Description: "Delete a list from a group issue board. This action cannot be undone.\n\nSee also: gitlab_group_board_list_lists, gitlab_group_board_list_create\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteGroupBoardListInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete list %d from board %d in group %s?", input.ListID, input.BoardID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteGroupBoardList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_list_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group board list")
	})
}

// RegisterMeta registers the gitlab_group_board meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list":        toolutil.RouteAction(client, ListGroupBoards),
		"get":         toolutil.RouteAction(client, GetGroupBoard),
		"create":      toolutil.RouteAction(client, CreateGroupBoard),
		"update":      toolutil.RouteAction(client, UpdateGroupBoard),
		"delete":      toolutil.DestructiveVoidAction(client, DeleteGroupBoard),
		"list_lists":  toolutil.RouteAction(client, ListGroupBoardLists),
		"get_list":    toolutil.RouteAction(client, GetGroupBoardList),
		"create_list": toolutil.RouteAction(client, CreateGroupBoardList),
		"update_list": toolutil.RouteAction(client, UpdateGroupBoardList),
		"delete_list": toolutil.DestructiveVoidAction(client, DeleteGroupBoardList),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_group_board",
		Title: toolutil.TitleFromName("gitlab_group_board"),
		Description: `Group issue board operations. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List all group issue boards (group_id, page, per_page)
- get: Get a single group board (group_id, board_id)
- create: Create a group board (group_id, name)
- update: Update a group board (group_id, board_id, name, assignee_id, milestone_id, labels, weight)
- delete: Delete a group board (group_id, board_id)
- list_lists: List all lists in a group board (group_id, board_id, page, per_page)
- get_list: Get a single group board list (group_id, board_id, list_id)
- create_list: Create a group board list (group_id, board_id, label_id)
- update_list: Reorder a group board list (group_id, board_id, list_id, position)
- delete_list: Delete a group board list (group_id, board_id, list_id)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconBoard,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_group_board", routes, nil))
}
