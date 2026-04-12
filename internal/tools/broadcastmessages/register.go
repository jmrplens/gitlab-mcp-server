// register.go wires broadcastmessages MCP tools to the MCP server.

package broadcastmessages

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all broadcast message tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_broadcast_messages",
		Title:       toolutil.TitleFromName("gitlab_list_broadcast_messages"),
		Description: "List all broadcast messages. Requires admin access.\n\nReturns: JSON with array of broadcast messages and pagination info.\n\nSee also: gitlab_create_broadcast_message, gitlab_get_appearance",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconNotify,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_broadcast_messages", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_broadcast_message",
		Title:       toolutil.TitleFromName("gitlab_get_broadcast_message"),
		Description: "Get a specific broadcast message by ID. Requires admin access.\n\nReturns: JSON with broadcast message details (ID, message, type, dates, theme).\n\nSee also: gitlab_list_broadcast_messages, gitlab_update_broadcast_message",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconNotify,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_broadcast_message", start, err)
		return toolutil.WithHints(FormatMessageMarkdown(out.Message), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_broadcast_message",
		Title:       toolutil.TitleFromName("gitlab_create_broadcast_message"),
		Description: "Create a broadcast message. Requires admin access.\n\nReturns: JSON with the created broadcast message details.\n\nSee also: gitlab_list_broadcast_messages, gitlab_update_broadcast_message",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconNotify,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, CreateOutput, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_broadcast_message", start, err)
		return toolutil.WithHints(FormatMessageMarkdown(out.Message), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_broadcast_message",
		Title:       toolutil.TitleFromName("gitlab_update_broadcast_message"),
		Description: "Update a broadcast message. Requires admin access.\n\nReturns: JSON with the updated broadcast message details.\n\nSee also: gitlab_get_broadcast_message, gitlab_delete_broadcast_message",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconNotify,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, UpdateOutput, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_broadcast_message", start, err)
		return toolutil.WithHints(FormatMessageMarkdown(out.Message), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_broadcast_message",
		Title:       toolutil.TitleFromName("gitlab_delete_broadcast_message"),
		Description: "Delete a broadcast message. Requires admin access.\n\nReturns: JSON confirmation of deletion.\n\nSee also: gitlab_list_broadcast_messages, gitlab_create_broadcast_message",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconNotify,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete broadcast message %d?", input.ID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_broadcast_message", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		r, o, _ := toolutil.DeleteResult("broadcast_message")
		return r, o, nil
	})
}
