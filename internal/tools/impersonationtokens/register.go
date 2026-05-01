package impersonationtokens

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers impersonation token and PAT management tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_impersonation_tokens",
		Title:       toolutil.TitleFromName("gitlab_list_impersonation_tokens"),
		Description: "List all impersonation tokens for a GitLab user by user ID. Optionally filter by state (all/active/inactive).\n\nSee also: gitlab_get_impersonation_token, gitlab_create_impersonation_token\n\nReturns: JSON array of impersonation tokens.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_impersonation_tokens", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_impersonation_token",
		Title:       toolutil.TitleFromName("gitlab_get_impersonation_token"),
		Description: "Retrieve a specific impersonation token by user ID and token ID.\n\nSee also: gitlab_list_impersonation_tokens, gitlab_create_impersonation_token\n\nReturns: JSON with token details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_impersonation_token", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_impersonation_token",
		Title:       toolutil.TitleFromName("gitlab_create_impersonation_token"),
		Description: "Create an impersonation token for a GitLab user (admin only). Requires user ID, token name, and scopes.\n\nSee also: gitlab_list_impersonation_tokens, gitlab_revoke_impersonation_token\n\nReturns: JSON with the created token (includes the token value).",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_impersonation_token", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_revoke_impersonation_token",
		Title:       toolutil.TitleFromName("gitlab_revoke_impersonation_token"),
		Description: "Revoke an impersonation token for a GitLab user (admin only).\n\nSee also: gitlab_list_impersonation_tokens, gitlab_create_impersonation_token\n\nReturns: JSON with revocation confirmation.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RevokeInput) (*mcp.CallToolResult, RevokeOutput, error) {
		start := time.Now()
		out, err := Revoke(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_revoke_impersonation_token", start, err)
		return toolutil.ToolResultWithMarkdown(
			fmt.Sprintf("## Impersonation Token Revoked\n\n"+toolutil.FmtMdID+"- **User ID**: %d\n- **Revoked**: %s %v\n",
				out.TokenID, out.UserID, toolutil.EmojiSuccess, out.Revoked),
		), out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_personal_access_token",
		Title:       toolutil.TitleFromName("gitlab_create_personal_access_token"),
		Description: "Create a personal access token for a specific GitLab user (admin only). Requires user ID, token name, and scopes.\n\nSee also: gitlab_create_impersonation_token\n\nReturns: JSON with the created token (includes the token value).",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreatePATInput) (*mcp.CallToolResult, PATOutput, error) {
		start := time.Now()
		out, err := CreatePAT(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_personal_access_token", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPATMarkdownString(out)), out, err)
	})
}
