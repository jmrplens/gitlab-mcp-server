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
	specs := ActionSpecs(client)
	topicTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconLabel})
	}

	mcp.AddTool(server, topicTool("gitlab_list_topics", "List project topics. Can be filtered by search query.\n\nSee also: gitlab_create_topic, gitlab_project_list\n\nReturns: JSON with array of topics and pagination info."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_topics", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, topicTool("gitlab_get_topic", "Get a specific project topic by ID.\n\nSee also: gitlab_list_topics, gitlab_update_topic\n\nReturns: JSON with topic details (ID, name, title, description)."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_topic", start, err)
		return toolutil.WithHints(FormatTopicMarkdown(out.Topic), out, err)
	})

	mcp.AddTool(server, topicTool("gitlab_create_topic", "Create a new project topic. Requires admin access.\n\nSee also: gitlab_list_topics, gitlab_update_topic\n\nReturns: JSON with the created topic details."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, CreateOutput, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_topic", start, err)
		return toolutil.WithHints(FormatTopicMarkdown(out.Topic), out, err)
	})

	mcp.AddTool(server, topicTool("gitlab_update_topic", "Update a project topic. Requires admin access.\n\nSee also: gitlab_get_topic, gitlab_delete_topic\n\nReturns: JSON with the updated topic details."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, UpdateOutput, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_topic", start, err)
		return toolutil.WithHints(FormatTopicMarkdown(out.Topic), out, err)
	})

	mcp.AddTool(server, topicTool("gitlab_delete_topic", "Delete a project topic. Requires admin access.\n\nSee also: gitlab_list_topics, gitlab_create_topic\n\nReturns: JSON confirmation of deletion."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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
