package ffuserlists

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all feature flag user list individual tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	userListTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconUser})
	}

	mcp.AddTool(server, userListTool("gitlab_ff_user_list_list", "List feature flag user lists for a project.\n\nReturns: JSON with user lists array including name, IID, and user_xids.\n\nSee also: gitlab_ff_user_list_get, gitlab_feature_flag_list"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListUserLists(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ff_user_list_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListUserListsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, userListTool("gitlab_ff_user_list_get", "Get a single feature flag user list by IID.\n\nReturns: JSON with user list details including name, IID, and user_xids.\n\nSee also: gitlab_ff_user_list_list, gitlab_feature_flag_get"), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetUserList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ff_user_list_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatUserListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, userListTool("gitlab_ff_user_list_create", "Create a new feature flag user list.\n\nReturns: JSON with created user list including name, IID, and user_xids.\n\nSee also: gitlab_ff_user_list_list, gitlab_feature_flag_create"), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateUserList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ff_user_list_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatUserListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, userListTool("gitlab_ff_user_list_update", "Update a feature flag user list.\n\nReturns: JSON with updated user list including name, IID, and user_xids.\n\nSee also: gitlab_ff_user_list_get, gitlab_ff_user_list_delete"), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := UpdateUserList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ff_user_list_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatUserListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, userListTool("gitlab_ff_user_list_delete", "Delete a feature flag user list.\n\nReturns: JSON with deletion confirmation.\n\nSee also: gitlab_ff_user_list_list, gitlab_feature_flag_list"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete feature flag user list %d from project %s?", input.IID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteUserList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ff_user_list_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("feature flag user list")
	})
}
