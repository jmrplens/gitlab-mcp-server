package notifications

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers individual notification settings tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_notification_global_get",
		Title:       toolutil.TitleFromName("gitlab_notification_global_get"),
		Description: "Get global notification settings for the authenticated user.\n\nReturns: JSON with global notification settings.\n\nSee also: gitlab_notification_global_update, gitlab_notification_project_get",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconNotify,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetGlobalInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetGlobalSettings(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_notification_global_get", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_notification_project_get",
		Title:       toolutil.TitleFromName("gitlab_notification_project_get"),
		Description: "Get notification settings for a specific project.\n\nReturns: JSON with project notification settings.\n\nSee also: gitlab_notification_project_update, gitlab_notification_global_get",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconNotify,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetSettingsForProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_notification_project_get", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_notification_group_get",
		Title:       toolutil.TitleFromName("gitlab_notification_group_get"),
		Description: "Get notification settings for a specific group.\n\nReturns: JSON with group notification settings.\n\nSee also: gitlab_notification_group_update, gitlab_notification_global_get",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconNotify,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetGroupInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetSettingsForGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_notification_group_get", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_notification_global_update",
		Title:       toolutil.TitleFromName("gitlab_notification_global_update"),
		Description: "Update global notification settings for the authenticated user.\n\nReturns: JSON with the updated global notification settings.\n\nSee also: gitlab_notification_global_get, gitlab_notification_project_update",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconNotify,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateGlobalInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := UpdateGlobalSettings(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_notification_global_update", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_notification_project_update",
		Title:       toolutil.TitleFromName("gitlab_notification_project_update"),
		Description: "Update notification settings for a specific project.\n\nReturns: JSON with the updated project notification settings.\n\nSee also: gitlab_notification_project_get, gitlab_notification_global_update",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconNotify,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := UpdateSettingsForProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_notification_project_update", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_notification_group_update",
		Title:       toolutil.TitleFromName("gitlab_notification_group_update"),
		Description: "Update notification settings for a specific group.\n\nReturns: JSON with the updated group notification settings.\n\nSee also: gitlab_notification_group_get, gitlab_notification_global_update",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconNotify,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateGroupInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := UpdateSettingsForGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_notification_group_update", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})
}
