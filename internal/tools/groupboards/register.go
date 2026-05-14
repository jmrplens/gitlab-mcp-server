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
	specs := ActionSpecs(client)
	groupBoardTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconBoard})
	}

	mcp.AddTool(server, groupBoardTool("gitlab_group_board_list", "List all issue boards for a group\n\nSee also: gitlab_group_board_create, gitlab_issue_list_group\n\nReturns: JSON array of boards with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupBoardsInput) (*mcp.CallToolResult, ListGroupBoardsOutput, error) {
		start := time.Now()
		out, err := ListGroupBoards(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListGroupBoardsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupBoardTool("gitlab_group_board_get", "Get a single group issue board\n\nSee also: gitlab_group_board_list, gitlab_group_board_list_lists\n\nReturns: JSON with board details."), func(ctx context.Context, req *mcp.CallToolRequest, input GetGroupBoardInput) (*mcp.CallToolResult, GroupBoardOutput, error) {
		start := time.Now()
		out, err := GetGroupBoard(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGroupBoardMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupBoardTool("gitlab_group_board_create", "Create a new issue board in a group\n\nSee also: gitlab_group_board_list, gitlab_group_board_list_create\n\nReturns: JSON with the board details."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateGroupBoardInput) (*mcp.CallToolResult, GroupBoardOutput, error) {
		start := time.Now()
		out, err := CreateGroupBoard(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGroupBoardMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupBoardTool("gitlab_group_board_update", "Update an existing group issue board\n\nSee also: gitlab_group_board_get, gitlab_group_board_list\n\nReturns: JSON with the board details."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateGroupBoardInput) (*mcp.CallToolResult, GroupBoardOutput, error) {
		start := time.Now()
		out, err := UpdateGroupBoard(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGroupBoardMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupBoardTool("gitlab_group_board_delete", "Delete a group issue board. This action cannot be undone.\n\nSee also: gitlab_group_board_list, gitlab_group_board_create\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteGroupBoardInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, groupBoardTool("gitlab_group_board_list_lists", "List all lists in a group issue board\n\nSee also: gitlab_group_board_list_create, gitlab_group_board_get\n\nReturns: JSON array of board lists with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupBoardListsInput) (*mcp.CallToolResult, ListBoardListsOutput, error) {
		start := time.Now()
		out, err := ListGroupBoardLists(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_list_lists", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListBoardListsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupBoardTool("gitlab_group_board_list_get", "Get a single list from a group issue board\n\nSee also: gitlab_group_board_list_lists, gitlab_group_board_list_update\n\nReturns: JSON with board list details."), func(ctx context.Context, req *mcp.CallToolRequest, input GetGroupBoardListInput) (*mcp.CallToolResult, BoardListOutput, error) {
		start := time.Now()
		out, err := GetGroupBoardList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_list_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupBoardTool("gitlab_group_board_list_create", "Create a new list in a group issue board\n\nSee also: gitlab_group_board_list_lists, gitlab_group_label_list\n\nReturns: JSON with the board list details."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateGroupBoardListInput) (*mcp.CallToolResult, BoardListOutput, error) {
		start := time.Now()
		out, err := CreateGroupBoardList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_list_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupBoardTool("gitlab_group_board_list_update", "Update (reorder) a list in a group issue board\n\nSee also: gitlab_group_board_list_get, gitlab_group_board_list_lists\n\nReturns: JSON with the board list details."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateGroupBoardListInput) (*mcp.CallToolResult, BoardListOutput, error) {
		start := time.Now()
		out, err := UpdateGroupBoardList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_board_list_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupBoardTool("gitlab_group_board_list_delete", "Delete a list from a group issue board. This action cannot be undone.\n\nSee also: gitlab_group_board_list_lists, gitlab_group_board_list_create\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteGroupBoardListInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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
