package grouplabels

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers group label tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	groupLabelTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconLabel})
	}

	mcp.AddTool(server, groupLabelTool("gitlab_group_label_list", "List all labels for a GitLab group. Supports filtering by search keyword, including issue/MR counts (with_counts), ancestor/descendant groups, and group-only labels. Returns label name, color, description, open/closed issue counts, and MR counts with pagination.\n\nReturns: JSON array of group labels with pagination. See also: gitlab_group_label_get."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_label_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, groupLabelTool("gitlab_group_label_get", "Get details of a single group label by ID or name, including color, description, priority, and issue/MR counts.\n\nReturns: JSON with label details including color, description, priority, and counts. See also: gitlab_group_label_list."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_label_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		if err == nil && out.ID != 0 && string(input.GroupID) != "" {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://group/%s/label/%d", url.PathEscape(string(input.GroupID)), out.ID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, groupLabelTool("gitlab_group_label_create", "Create a new label in a GitLab group with a name, color (hex), optional description, and optional priority.\n\nReturns: JSON with the created label details. See also: gitlab_group_label_list."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_label_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupLabelTool("gitlab_group_label_update", "Update an existing group label. Can change name, color, description, or priority. Only specified fields are modified.\n\nReturns: JSON with the updated label details. See also: gitlab_group_label_get."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_label_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupLabelTool("gitlab_group_label_delete", "Delete a group label by ID or name.\n\nReturns: JSON confirming deletion. See also: gitlab_group_label_list."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete group label %q from group %q?", input.LabelID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_label_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group label")
	})

	mcp.AddTool(server, groupLabelTool("gitlab_group_label_subscribe", "Subscribe to a group label to receive notifications when the label is applied to issues or merge requests.\n\nReturns: JSON with the subscribed label details. See also: gitlab_group_label_get."), func(ctx context.Context, req *mcp.CallToolRequest, input SubscribeInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Subscribe(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_label_subscribe", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupLabelTool("gitlab_group_label_unsubscribe", "Unsubscribe from a group label to stop receiving notifications.\n\nReturns: JSON confirming unsubscription. See also: gitlab_group_label_get."), func(ctx context.Context, req *mcp.CallToolRequest, input SubscribeInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Unsubscribe(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_label_unsubscribe", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group label subscription")
	})
}
