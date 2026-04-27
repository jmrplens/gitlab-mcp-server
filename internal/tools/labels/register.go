// register.go wires labels MCP tools to the MCP server.

package labels

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers label-related tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_label_list",
		Title:       toolutil.TitleFromName("gitlab_label_list"),
		Description: "List all labels for a GitLab project. Supports filtering by search keyword, including issue/MR counts (with_counts), and including labels from ancestor groups. Returns label name, color, description, open/closed issue counts, and merge request counts with pagination.\n\nReturns: paginated list of labels with id, name, color, description, open/closed issue counts, and MR counts. See also: gitlab_label_get, gitlab_label_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_label_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_label_get",
		Title:       toolutil.TitleFromName("gitlab_label_get"),
		Description: "Get details of a single project label by ID or name. Returns: ID, name, color, description, priority, open/closed issue counts, open MR count, and subscribed status. See also: gitlab_label_update.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_label_get", start, nil)
			return toolutil.NotFoundResult("Label", fmt.Sprintf("ID %s in project %s", input.LabelID, input.ProjectID),
				"Use gitlab_label_list with project_id to list labels",
				"Labels can be referenced by ID or name — verify the value is correct",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_label_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		if err == nil && out.ID > 0 && string(input.ProjectID) != "" {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://project/%s/label/%d", url.PathEscape(string(input.ProjectID)), out.ID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_label_create",
		Title:       toolutil.TitleFromName("gitlab_label_create"),
		Description: "Create a new label in a GitLab project with a name, color (hex), optional description, and optional priority. Returns: label ID, name, color, description, priority, and subscribed status. See also: gitlab_label_list, gitlab_issue_create.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_label_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_label_update",
		Title:       toolutil.TitleFromName("gitlab_label_update"),
		Description: "Update an existing project label. Can change name, color, description, or priority. Only specified fields are modified. Returns: updated label with ID, name, color, description, and priority. See also: gitlab_label_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_label_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_label_delete",
		Title:       toolutil.TitleFromName("gitlab_label_delete"),
		Description: "Delete a project label by ID or name.\n\nReturns: confirmation message. See also: gitlab_label_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete label %q from project %q?", input.LabelID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_label_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("label")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_label_subscribe",
		Title:       toolutil.TitleFromName("gitlab_label_subscribe"),
		Description: "Subscribe to a project label to receive notifications when the label is applied to issues or merge requests. Returns: label details with subscribed status set to true. See also: gitlab_label_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SubscribeInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Subscribe(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_label_subscribe", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_label_unsubscribe",
		Title:       toolutil.TitleFromName("gitlab_label_unsubscribe"),
		Description: "Unsubscribe from a project label to stop receiving notifications.\n\nReturns: confirmation message. See also: gitlab_label_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SubscribeInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Unsubscribe(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_label_unsubscribe", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("label subscription")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_label_promote",
		Title:       toolutil.TitleFromName("gitlab_label_promote"),
		Description: "Promote a project label to a group label, making it available to all projects in the group.\n\nReturns: confirmation message. See also: gitlab_label_get, gitlab_group_label_list.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PromoteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Promote(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_label_promote", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("label promotion")
	})
}
