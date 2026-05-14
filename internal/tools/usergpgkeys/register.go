package usergpgkeys

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers GPG key management tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	userGPGTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconUser})
	}

	mcp.AddTool(server, userGPGTool("gitlab_list_gpg_keys", "List GPG keys for the currently authenticated GitLab user.\n\nSee also: gitlab_add_gpg_key, gitlab_get_gpg_key\n\nReturns: JSON array of GPG keys."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_gpg_keys", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, userGPGTool("gitlab_list_gpg_keys_for_user", "List GPG keys for a specific GitLab user by user ID.\n\nSee also: gitlab_list_gpg_keys, gitlab_get_gpg_key_for_user\n\nReturns: JSON array of GPG keys."), func(ctx context.Context, req *mcp.CallToolRequest, input ListForUserInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_gpg_keys_for_user", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, userGPGTool("gitlab_get_gpg_key", "Retrieve a specific GPG key by its ID for the current user.\n\nSee also: gitlab_list_gpg_keys, gitlab_add_gpg_key\n\nReturns: JSON with GPG key details."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_gpg_key", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, userGPGTool("gitlab_get_gpg_key_for_user", "Retrieve a specific GPG key for a specific user by user ID and key ID.\n\nSee also: gitlab_list_gpg_keys_for_user, gitlab_add_gpg_key_for_user\n\nReturns: JSON with GPG key details."), func(ctx context.Context, req *mcp.CallToolRequest, input GetForUserInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_gpg_key_for_user", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, userGPGTool("gitlab_add_gpg_key", "Add a GPG key to the currently authenticated GitLab user. Requires the armored GPG public key.\n\nSee also: gitlab_list_gpg_keys, gitlab_delete_gpg_key\n\nReturns: JSON with the created GPG key details."), func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Add(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_gpg_key", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, userGPGTool("gitlab_add_gpg_key_for_user", "Add a GPG key to a specific GitLab user (admin only). Requires user ID and GPG public key.\n\nSee also: gitlab_list_gpg_keys_for_user, gitlab_delete_gpg_key_for_user\n\nReturns: JSON with the created GPG key details."), func(ctx context.Context, req *mcp.CallToolRequest, input AddForUserInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := AddForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_gpg_key_for_user", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdownString(out)), out, err)
	})

	mcp.AddTool(server, userGPGTool("gitlab_delete_gpg_key", "Delete a GPG key from the currently authenticated GitLab user.\n\nSee also: gitlab_list_gpg_keys, gitlab_add_gpg_key\n\nReturns: JSON with deletion confirmation."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		out, err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_gpg_key", start, err)
		return toolutil.ToolResultWithMarkdown(
			fmt.Sprintf("## GPG Key Deleted\n\n"+toolutil.FmtMdID+"- **Deleted**: %s %v\n",
				out.KeyID, toolutil.EmojiSuccess, out.Deleted),
		), out, err
	})

	mcp.AddTool(server, userGPGTool("gitlab_delete_gpg_key_for_user", "Delete a GPG key from a specific GitLab user (admin only).\n\nSee also: gitlab_list_gpg_keys_for_user, gitlab_add_gpg_key_for_user\n\nReturns: JSON with deletion confirmation."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteForUserInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		out, err := DeleteForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_gpg_key_for_user", start, err)
		return toolutil.ToolResultWithMarkdown(
			fmt.Sprintf("## GPG Key Deleted\n\n"+toolutil.FmtMdID+"- **Deleted**: %s %v\n",
				out.KeyID, toolutil.EmojiSuccess, out.Deleted),
		), out, err
	})
}
