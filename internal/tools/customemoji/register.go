// register.go wires Custom Emoji MCP tools to the MCP server.

package customemoji

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers Custom Emoji tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_custom_emoji",
		Title:       toolutil.TitleFromName("gitlab_list_custom_emoji"),
		Description: "List custom emoji for a GitLab group via GraphQL API. Returns: paginated list with ID, name, URL, and created date. See also: gitlab_create_custom_emoji.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_custom_emoji", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_custom_emoji",
		Title:       toolutil.TitleFromName("gitlab_create_custom_emoji"),
		Description: "Create a custom emoji in a GitLab group via GraphQL API. Requires group path, emoji name (without colons), and image URL. Returns: created emoji with ID, name, URL, and created date. See also: gitlab_list_custom_emoji, gitlab_delete_custom_emoji.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, CreateOutput, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_custom_emoji", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatCreateMarkdown(out), toolutil.ContentMutate), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_custom_emoji",
		Title:       toolutil.TitleFromName("gitlab_delete_custom_emoji"),
		Description: "Delete a custom emoji from a GitLab group via GraphQL API. Requires the emoji GID. Returns: confirmation message. See also: gitlab_list_custom_emoji.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete custom emoji %q?", input.ID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_custom_emoji", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("custom emoji")
	})
}
