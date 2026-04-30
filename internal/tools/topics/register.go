// register.go wires topics MCP tools to the MCP server.
package topics

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all topic tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_topics",
		Title:       toolutil.TitleFromName("gitlab_list_topics"),
		Description: "List project topics. Can be filtered by search query.\n\nSee also: gitlab_create_topic, gitlab_project_list\n\nReturns: JSON with array of topics and pagination info.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_topics", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_topic",
		Title:       toolutil.TitleFromName("gitlab_get_topic"),
		Description: "Get a specific project topic by ID.\n\nSee also: gitlab_list_topics, gitlab_update_topic\n\nReturns: JSON with topic details (ID, name, title, description).",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_topic", start, err)
		return toolutil.WithHints(FormatTopicMarkdown(out.Topic), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_topic",
		Title:       toolutil.TitleFromName("gitlab_create_topic"),
		Description: "Create a new project topic. Requires admin access.\n\nSee also: gitlab_list_topics, gitlab_update_topic\n\nReturns: JSON with the created topic details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, CreateOutput, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_topic", start, err)
		return toolutil.WithHints(FormatTopicMarkdown(out.Topic), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_topic",
		Title:       toolutil.TitleFromName("gitlab_update_topic"),
		Description: "Update a project topic. Requires admin access.\n\nSee also: gitlab_get_topic, gitlab_delete_topic\n\nReturns: JSON with the updated topic details.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, UpdateOutput, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_topic", start, err)
		return toolutil.WithHints(FormatTopicMarkdown(out.Topic), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_topic",
		Title:       toolutil.TitleFromName("gitlab_delete_topic"),
		Description: "Delete a project topic. Requires admin access.\n\nSee also: gitlab_list_topics, gitlab_create_topic\n\nReturns: JSON confirmation of deletion.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete topic %d?", input.TopicID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_topic", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		r, o, _ := toolutil.DeleteResult("topic")
		return r, o, nil
	})
}
