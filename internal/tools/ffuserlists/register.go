// register.go wires ffuserlists MCP tools to the MCP server.

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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_ff_user_list_list",
		Title:       toolutil.TitleFromName("gitlab_ff_user_list_list"),
		Description: "List feature flag user lists for a project.\n\nReturns: JSON with user lists array including name, IID, and user_xids.\n\nSee also: gitlab_ff_user_list_get, gitlab_feature_flag_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListUserLists(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ff_user_list_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListUserListsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_ff_user_list_get",
		Title:       toolutil.TitleFromName("gitlab_ff_user_list_get"),
		Description: "Get a single feature flag user list by IID.\n\nReturns: JSON with user list details including name, IID, and user_xids.\n\nSee also: gitlab_ff_user_list_list, gitlab_feature_flag_get",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetUserList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ff_user_list_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatUserListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_ff_user_list_create",
		Title:       toolutil.TitleFromName("gitlab_ff_user_list_create"),
		Description: "Create a new feature flag user list.\n\nReturns: JSON with created user list including name, IID, and user_xids.\n\nSee also: gitlab_ff_user_list_list, gitlab_feature_flag_create",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateUserList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ff_user_list_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatUserListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_ff_user_list_update",
		Title:       toolutil.TitleFromName("gitlab_ff_user_list_update"),
		Description: "Update a feature flag user list.\n\nReturns: JSON with updated user list including name, IID, and user_xids.\n\nSee also: gitlab_ff_user_list_get, gitlab_ff_user_list_delete",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := UpdateUserList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_ff_user_list_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatUserListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_ff_user_list_delete",
		Title:       toolutil.TitleFromName("gitlab_ff_user_list_delete"),
		Description: "Delete a feature flag user list.\n\nReturns: JSON with deletion confirmation.\n\nSee also: gitlab_ff_user_list_list, gitlab_feature_flag_list",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

// RegisterMeta registers the feature flag user list meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list":   toolutil.RouteAction(client, ListUserLists),
		"get":    toolutil.RouteAction(client, GetUserList),
		"create": toolutil.RouteAction(client, CreateUserList),
		"update": toolutil.RouteAction(client, UpdateUserList),
		"delete": toolutil.DestructiveVoidAction(client, DeleteUserList),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_ff_user_list",
		Title: toolutil.TitleFromName("gitlab_ff_user_list"),
		Description: `Feature flag user list operations. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List user lists (project_id, search, page, per_page)
- get: Get a user list (project_id, iid)
- create: Create a user list (project_id, name, user_xids)
- update: Update a user list (project_id, iid, name, user_xids)
- delete: Delete a user list (project_id, iid)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconUser,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_ff_user_list", routes, nil))
}
