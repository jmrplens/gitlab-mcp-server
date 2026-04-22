// register.go wires grouplabels MCP tools to the MCP server.

package grouplabels

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers group label tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_label_list",
		Title:       toolutil.TitleFromName("gitlab_group_label_list"),
		Description: "List all labels for a GitLab group. Supports filtering by search keyword, including issue/MR counts (with_counts), ancestor/descendant groups, and group-only labels. Returns label name, color, description, open/closed issue counts, and MR counts with pagination.\n\nReturns: JSON array of group labels with pagination. See also: gitlab_group_label_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_label_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_label_get",
		Title:       toolutil.TitleFromName("gitlab_group_label_get"),
		Description: "Get details of a single group label by ID or name, including color, description, priority, and issue/MR counts.\n\nReturns: JSON with label details including color, description, priority, and counts. See also: gitlab_group_label_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_label_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_label_create",
		Title:       toolutil.TitleFromName("gitlab_group_label_create"),
		Description: "Create a new label in a GitLab group with a name, color (hex), optional description, and optional priority.\n\nReturns: JSON with the created label details. See also: gitlab_group_label_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_label_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_label_update",
		Title:       toolutil.TitleFromName("gitlab_group_label_update"),
		Description: "Update an existing group label. Can change name, color, description, or priority. Only specified fields are modified.\n\nReturns: JSON with the updated label details. See also: gitlab_group_label_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_label_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_label_delete",
		Title:       toolutil.TitleFromName("gitlab_group_label_delete"),
		Description: "Delete a group label by ID or name.\n\nReturns: JSON confirming deletion. See also: gitlab_group_label_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_label_subscribe",
		Title:       toolutil.TitleFromName("gitlab_group_label_subscribe"),
		Description: "Subscribe to a group label to receive notifications when the label is applied to issues or merge requests.\n\nReturns: JSON with the subscribed label details. See also: gitlab_group_label_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SubscribeInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Subscribe(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_label_subscribe", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_label_unsubscribe",
		Title:       toolutil.TitleFromName("gitlab_group_label_unsubscribe"),
		Description: "Unsubscribe from a group label to stop receiving notifications.\n\nReturns: JSON confirming unsubscription. See also: gitlab_group_label_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SubscribeInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Unsubscribe(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_label_unsubscribe", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group label subscription")
	})
}

// RegisterMeta registers group label meta-tool on the MCP server.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list":        toolutil.RouteAction(client, List),
		"get":         toolutil.RouteAction(client, Get),
		"create":      toolutil.RouteAction(client, Create),
		"update":      toolutil.RouteAction(client, Update),
		"delete":      toolutil.DestructiveVoidAction(client, Delete),
		"subscribe":   toolutil.RouteAction(client, Subscribe),
		"unsubscribe": toolutil.RouteVoidAction(client, Unsubscribe),
	}

	desc := `Manage GitLab group labels (list, get, create, update, delete, subscribe, unsubscribe).

Actions:
- list: List group labels. Params: group_id (required), search, with_counts (bool), include_ancestor_groups (bool), include_descendant_groups (bool), only_group_labels (bool), page, per_page
- get: Get a group label. Params: group_id (required), label_id (required)
- create: Create a group label. Params: group_id (required), name (required), color (required, hex), description, priority
- update: Update a group label. Params: group_id (required), label_id (required), new_name, color, description, priority
- delete: Delete a group label. Params: group_id (required), label_id (required)
- subscribe: Subscribe to a group label. Params: group_id (required), label_id (required)
- unsubscribe: Unsubscribe from a group label. Params: group_id (required), label_id (required)`

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_label",
		Title:       toolutil.TitleFromName("gitlab_group_label"),
		Description: desc,
		Annotations: toolutil.DeriveAnnotations(routes),
		Icons:       toolutil.IconLabel,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_group_label", routes, nil))
}
