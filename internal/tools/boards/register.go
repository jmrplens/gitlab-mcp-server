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
	specs := ActionSpecs(client)
	boardTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconBoard})
	}

	mcp.AddTool(server, boardTool("gitlab_board_list", "List all issue boards for a project\n\nSee also: gitlab_board_create, gitlab_issue_list\n\nReturns: JSON array of boards with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListBoardsInput) (*mcp.CallToolResult, ListBoardsOutput, error) {
		start := time.Now()
		out, err := ListBoards(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListBoardsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, boardTool("gitlab_board_get", "Get a single issue board\n\nSee also: gitlab_board_list, gitlab_board_list_lists\n\nReturns: JSON with board details."), func(ctx context.Context, req *mcp.CallToolRequest, input GetBoardInput) (*mcp.CallToolResult, BoardOutput, error) {
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

	mcp.AddTool(server, boardTool("gitlab_board_create", "Create a new issue board in a project\n\nSee also: gitlab_board_list, gitlab_board_list_create\n\nReturns: JSON with the board details."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateBoardInput) (*mcp.CallToolResult, BoardOutput, error) {
		start := time.Now()
		out, err := CreateBoard(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardMarkdown(out)), out, err)
	})

	mcp.AddTool(server, boardTool("gitlab_board_update", "Update an existing issue board\n\nSee also: gitlab_board_get, gitlab_board_list\n\nReturns: JSON with the board details."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateBoardInput) (*mcp.CallToolResult, BoardOutput, error) {
		start := time.Now()
		out, err := UpdateBoard(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardMarkdown(out)), out, err)
	})

	mcp.AddTool(server, boardTool("gitlab_board_delete", "Delete an issue board from a project. This action cannot be undone.\n\nSee also: gitlab_board_list, gitlab_board_create\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteBoardInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, boardTool("gitlab_board_list_lists", "List all lists in an issue board\n\nSee also: gitlab_board_list_create, gitlab_board_get\n\nReturns: JSON array of board lists with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListBoardListsInput) (*mcp.CallToolResult, ListBoardListsOutput, error) {
		start := time.Now()
		out, err := ListBoardLists(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_list_lists", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListBoardListsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, boardTool("gitlab_board_list_get", "Get a single list from an issue board\n\nSee also: gitlab_board_list_lists, gitlab_board_list_update\n\nReturns: JSON with board list details."), func(ctx context.Context, req *mcp.CallToolRequest, input GetBoardListInput) (*mcp.CallToolResult, BoardListOutput, error) {
		start := time.Now()
		out, err := GetBoardList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_list_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, boardTool("gitlab_board_list_create", "Create a new list in an issue board\n\nSee also: gitlab_board_list_lists, gitlab_label_list\n\nReturns: JSON with the board list details."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateBoardListInput) (*mcp.CallToolResult, BoardListOutput, error) {
		start := time.Now()
		out, err := CreateBoardList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_list_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, boardTool("gitlab_board_list_update", "Update (reorder) a list in an issue board\n\nSee also: gitlab_board_list_get, gitlab_board_list_lists\n\nReturns: JSON with the board list details."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateBoardListInput) (*mcp.CallToolResult, BoardListOutput, error) {
		start := time.Now()
		out, err := UpdateBoardList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_board_list_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBoardListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, boardTool("gitlab_board_list_delete", "Delete a list from an issue board. This action cannot be undone.\n\nSee also: gitlab_board_list_lists, gitlab_board_list_create\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteBoardListInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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
