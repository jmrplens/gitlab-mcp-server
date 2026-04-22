// register.go wires errortracking MCP tools to the MCP server.

package errortracking

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all error tracking tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_error_tracking_settings",
		Title:       toolutil.TitleFromName("gitlab_get_error_tracking_settings"),
		Description: "Get error tracking settings for a GitLab project.\n\nReturns: JSON with error tracking settings including active status and integration mode.\n\nSee also: gitlab_enable_disable_error_tracking, gitlab_list_error_tracking_client_keys",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAlert,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetSettingsInput) (*mcp.CallToolResult, SettingsOutput, error) {
		start := time.Now()
		out, err := GetSettings(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_error_tracking_settings", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatSettingsMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_enable_disable_error_tracking",
		Title:       toolutil.TitleFromName("gitlab_enable_disable_error_tracking"),
		Description: "Enable or disable error tracking for a GitLab project.\n\nReturns: JSON with updated error tracking settings including active status.\n\nSee also: gitlab_get_error_tracking_settings, gitlab_list_error_tracking_client_keys",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconAlert,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EnableDisableInput) (*mcp.CallToolResult, SettingsOutput, error) {
		start := time.Now()
		out, err := EnableDisable(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_enable_disable_error_tracking", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatSettingsMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_error_tracking_client_keys",
		Title:       toolutil.TitleFromName("gitlab_list_error_tracking_client_keys"),
		Description: "List error tracking client keys for a GitLab project.\n\nReturns: JSON with client keys array including ID and public key.\n\nSee also: gitlab_get_error_tracking_settings, gitlab_create_error_tracking_client_key",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAlert,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListClientKeysInput) (*mcp.CallToolResult, ListClientKeysOutput, error) {
		start := time.Now()
		out, err := ListClientKeys(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_error_tracking_client_keys", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListKeysMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_error_tracking_client_key",
		Title:       toolutil.TitleFromName("gitlab_create_error_tracking_client_key"),
		Description: "Create a new error tracking client key for a GitLab project.\n\nReturns: JSON with created client key including ID and public key.\n\nSee also: gitlab_list_error_tracking_client_keys, gitlab_delete_error_tracking_client_key",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconAlert,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateClientKeyInput) (*mcp.CallToolResult, ClientKeyItem, error) {
		start := time.Now()
		out, err := CreateClientKey(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_error_tracking_client_key", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatKeyMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_error_tracking_client_key",
		Title:       toolutil.TitleFromName("gitlab_delete_error_tracking_client_key"),
		Description: "Delete an error tracking client key for a GitLab project.\n\nReturns: JSON with deletion confirmation.\n\nSee also: gitlab_list_error_tracking_client_keys, gitlab_create_error_tracking_client_key",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconAlert,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteClientKeyInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete error tracking client key %d from project %s?", input.KeyID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteClientKey(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_error_tracking_client_key", start, err)
		r, o, _ := toolutil.DeleteResult("error tracking client key")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})
}

// RegisterMeta registers the gitlab_error_tracking meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"get_settings":      toolutil.RouteAction(client, GetSettings),
		"enable_disable":    toolutil.RouteAction(client, EnableDisable),
		"list_client_keys":  toolutil.RouteAction(client, ListClientKeys),
		"create_client_key": toolutil.RouteAction(client, CreateClientKey),
		"delete_client_key": toolutil.DestructiveVoidAction(client, DeleteClientKey),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_error_tracking",
		Title: toolutil.TitleFromName("gitlab_error_tracking"),
		Description: `Manage error tracking settings and client keys in GitLab. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- get_settings: Get error tracking settings. Params: project_id (required)
- enable_disable: Enable or disable error tracking. Params: project_id (required), active (required, bool), integrated (bool)
- list_client_keys: List error tracking client keys. Params: project_id (required)
- create_client_key: Create error tracking client key. Params: project_id (required)
- delete_client_key: Delete error tracking client key. Params: project_id (required), key_id (required, int)`,
		Annotations: toolutil.MetaAnnotations,
		Icons:       toolutil.IconAlert,
	}, toolutil.MakeMetaHandler("gitlab_error_tracking", routes, nil))
}
