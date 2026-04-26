// register.go wires freezeperiods MCP tools to the MCP server.

package freezeperiods

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all freeze period tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_freeze_periods",
		Title:       toolutil.TitleFromName("gitlab_list_freeze_periods"),
		Description: "List deploy freeze periods for a GitLab project.\n\nReturns: JSON with freeze periods array including cron schedule, timezone, and status.\n\nSee also: gitlab_create_freeze_period, gitlab_list_environments",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_freeze_periods", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_freeze_period",
		Title:       toolutil.TitleFromName("gitlab_get_freeze_period"),
		Description: "Get a single deploy freeze period by ID.\n\nReturns: JSON with freeze period details including cron schedule, timezone, and status.\n\nSee also: gitlab_list_freeze_periods, gitlab_update_freeze_period",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_freeze_period", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_freeze_period",
		Title:       toolutil.TitleFromName("gitlab_create_freeze_period"),
		Description: "Create a deploy freeze period with cron-based start and end times.\n\nReturns: JSON with created freeze period including ID, cron schedule, and timezone.\n\nSee also: gitlab_list_freeze_periods, gitlab_list_deployments",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_freeze_period", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_freeze_period",
		Title:       toolutil.TitleFromName("gitlab_update_freeze_period"),
		Description: "Update a deploy freeze period's cron schedule or timezone.\n\nReturns: JSON with updated freeze period including ID, cron schedule, and timezone.\n\nSee also: gitlab_get_freeze_period, gitlab_delete_freeze_period",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_freeze_period", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_freeze_period",
		Title:       toolutil.TitleFromName("gitlab_delete_freeze_period"),
		Description: "Delete a deploy freeze period from a project.\n\nReturns: JSON with deletion confirmation.\n\nSee also: gitlab_list_freeze_periods, gitlab_create_freeze_period",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, "delete freeze period"); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_freeze_period", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("freeze period")
	})
}

// RegisterMeta registers the gitlab_freeze_period meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list_freeze_periods":  toolutil.RouteAction(client, List),
		"get_freeze_period":    toolutil.RouteAction(client, Get),
		"create_freeze_period": toolutil.RouteAction(client, Create),
		"update_freeze_period": toolutil.RouteAction(client, Update),
		"delete_freeze_period": toolutil.DestructiveVoidAction(client, Delete),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_freeze_period",
		Title: toolutil.TitleFromName("gitlab_freeze_period"),
		Description: `Manage GitLab deploy freeze periods. Use 'action' to specify the operation.

Actions:
- list_freeze_periods: List freeze periods for a project. Params: project_id (required), page, per_page
- get_freeze_period: Get a freeze period by ID. Params: project_id (required), freeze_period_id (required)
- create_freeze_period: Create a freeze period. Params: project_id (required), freeze_start (required, cron), freeze_end (required, cron), cron_timezone
- update_freeze_period: Update a freeze period. Params: project_id (required), freeze_period_id (required), freeze_start, freeze_end, cron_timezone
- delete_freeze_period: Delete a freeze period. Params: project_id (required), freeze_period_id (required)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconSchedule,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_freeze_period", routes, nil))
}
