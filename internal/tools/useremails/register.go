// register.go wires email management MCP tools to the MCP server.

package useremails

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers email management tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_emails_for_user",
		Title:       toolutil.TitleFromName("gitlab_list_emails_for_user"),
		Description: "List email addresses for a specific GitLab user by user ID.\n\nSee also: gitlab_get_email, gitlab_add_email_for_user\n\nReturns: JSON array of emails.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListForUserInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_emails_for_user", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_email",
		Title:       toolutil.TitleFromName("gitlab_get_email"),
		Description: "Retrieve a specific email address by its ID.\n\nSee also: gitlab_list_emails_for_user, gitlab_add_email\n\nReturns: JSON with email details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_email", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_add_email",
		Title:       toolutil.TitleFromName("gitlab_add_email"),
		Description: "Add an email address to the currently authenticated GitLab user.\n\nSee also: gitlab_list_emails, gitlab_delete_email\n\nReturns: JSON with the created email details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Add(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_email", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_add_email_for_user",
		Title:       toolutil.TitleFromName("gitlab_add_email_for_user"),
		Description: "Add an email address to a specific GitLab user (admin only). Requires user ID and email address.\n\nSee also: gitlab_list_emails_for_user, gitlab_delete_email_for_user\n\nReturns: JSON with the created email details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddForUserInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := AddForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_email_for_user", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_email",
		Title:       toolutil.TitleFromName("gitlab_delete_email"),
		Description: "Delete an email address from the currently authenticated GitLab user.\n\nSee also: gitlab_list_emails, gitlab_add_email\n\nReturns: JSON with deletion confirmation.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		out, err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_email", start, err)
		return toolutil.ToolResultWithMarkdown(
			fmt.Sprintf("## Email Deleted\n\n"+toolutil.FmtMdID+"- **Deleted**: %s %v\n",
				out.EmailID, toolutil.EmojiSuccess, out.Deleted),
		), out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_email_for_user",
		Title:       toolutil.TitleFromName("gitlab_delete_email_for_user"),
		Description: "Delete an email address from a specific GitLab user (admin only).\n\nSee also: gitlab_list_emails_for_user, gitlab_add_email_for_user\n\nReturns: JSON with deletion confirmation.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteForUserInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		out, err := DeleteForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_email_for_user", start, err)
		return toolutil.ToolResultWithMarkdown(
			fmt.Sprintf("## Email Deleted\n\n"+toolutil.FmtMdID+"- **Deleted**: %s %v\n",
				out.EmailID, toolutil.EmojiSuccess, out.Deleted),
		), out, err
	})
}
