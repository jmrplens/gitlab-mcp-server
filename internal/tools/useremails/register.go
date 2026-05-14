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
	specs := ActionSpecs(client)
	userEmailTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconUser})
	}

	mcp.AddTool(server, userEmailTool("gitlab_list_emails_for_user", "List email addresses for a specific GitLab user by user ID.\n\nSee also: gitlab_get_email, gitlab_add_email_for_user\n\nReturns: JSON array of emails."), func(ctx context.Context, req *mcp.CallToolRequest, input ListForUserInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_emails_for_user", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, userEmailTool("gitlab_get_email", "Retrieve a specific email address by its ID.\n\nSee also: gitlab_list_emails_for_user, gitlab_add_email\n\nReturns: JSON with email details."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_email", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, userEmailTool("gitlab_add_email", "Add an email address to the currently authenticated GitLab user.\n\nSee also: gitlab_list_emails, gitlab_delete_email\n\nReturns: JSON with the created email details."), func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Add(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_email", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, userEmailTool("gitlab_add_email_for_user", "Add an email address to a specific GitLab user (admin only). Requires user ID and email address.\n\nSee also: gitlab_list_emails_for_user, gitlab_delete_email_for_user\n\nReturns: JSON with the created email details."), func(ctx context.Context, req *mcp.CallToolRequest, input AddForUserInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := AddForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_email_for_user", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, userEmailTool("gitlab_delete_email", "Delete an email address from the currently authenticated GitLab user.\n\nSee also: gitlab_list_emails, gitlab_add_email\n\nReturns: JSON with deletion confirmation."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		out, err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_email", start, err)
		return toolutil.ToolResultWithMarkdown(
			fmt.Sprintf("## Email Deleted\n\n"+toolutil.FmtMdID+"- **Deleted**: %s %v\n",
				out.EmailID, toolutil.EmojiSuccess, out.Deleted),
		), out, err
	})

	mcp.AddTool(server, userEmailTool("gitlab_delete_email_for_user", "Delete an email address from a specific GitLab user (admin only).\n\nSee also: gitlab_list_emails_for_user, gitlab_add_email_for_user\n\nReturns: JSON with deletion confirmation."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteForUserInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		out, err := DeleteForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_email_for_user", start, err)
		return toolutil.ToolResultWithMarkdown(
			fmt.Sprintf("## Email Deleted\n\n"+toolutil.FmtMdID+"- **Deleted**: %s %v\n",
				out.EmailID, toolutil.EmojiSuccess, out.Deleted),
		), out, err
	})
}
