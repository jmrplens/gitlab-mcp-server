package enterpriseusers

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab enterprise user operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_enterprise_users",
		Title:       toolutil.TitleFromName("gitlab_list_enterprise_users"),
		Description: "List all enterprise users for a GitLab group.\n\nReturns: JSON with users array and pagination. See also: gitlab_get_enterprise_user.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_enterprise_users", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_enterprise_user",
		Title:       toolutil.TitleFromName("gitlab_get_enterprise_user"),
		Description: "Get details of a specific enterprise user.\n\nReturns: JSON with user details. See also: gitlab_list_enterprise_users.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_enterprise_user", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_disable_2fa_enterprise_user",
		Title:       toolutil.TitleFromName("gitlab_disable_2fa_enterprise_user"),
		Description: "Disable two-factor authentication for an enterprise user.\n\nReturns: JSON with confirmation. See also: gitlab_get_enterprise_user.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input Disable2FAInput) (*mcp.CallToolResult, toolutil.VoidOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Disable 2FA for enterprise user %d in group %q?", input.UserID, input.GroupID)); r != nil {
			return r, toolutil.VoidOutput{}, nil
		}
		start := time.Now()
		err := Disable2FA(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_disable_2fa_enterprise_user", start, err)
		if err != nil {
			return nil, toolutil.VoidOutput{}, err
		}
		return toolutil.VoidResult(fmt.Sprintf("Disabled 2FA for enterprise user %d in group %s.", input.UserID, input.GroupID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_enterprise_user",
		Title:       toolutil.TitleFromName("gitlab_delete_enterprise_user"),
		Description: "Delete an enterprise user from a group.\n\nReturns: JSON with deletion confirmation. See also: gitlab_list_enterprise_users.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete enterprise user %d from group %q?", input.UserID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_enterprise_user", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("enterprise user %d from group %s", input.UserID, input.GroupID))
	})
}
