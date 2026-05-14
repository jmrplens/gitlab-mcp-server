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
	specs := ActionSpecs(client)
	freezePeriodTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconSchedule})
	}

	mcp.AddTool(server, freezePeriodTool("gitlab_list_freeze_periods", "List deploy freeze periods for a GitLab project.\n\nReturns: JSON with freeze periods array including cron schedule, timezone, and status.\n\nSee also: gitlab_create_freeze_period, gitlab_environment_list"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_freeze_periods", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, freezePeriodTool("gitlab_get_freeze_period", "Get a single deploy freeze period by ID.\n\nReturns: JSON with freeze period details including cron schedule, timezone, and status.\n\nSee also: gitlab_list_freeze_periods, gitlab_update_freeze_period"), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_freeze_period", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, freezePeriodTool("gitlab_create_freeze_period", "Create a deploy freeze period with cron-based start and end times.\n\nReturns: JSON with created freeze period including ID, cron schedule, and timezone.\n\nSee also: gitlab_list_freeze_periods, gitlab_deployment_list"), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_freeze_period", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, freezePeriodTool("gitlab_update_freeze_period", "Update a deploy freeze period's cron schedule or timezone.\n\nReturns: JSON with updated freeze period including ID, cron schedule, and timezone.\n\nSee also: gitlab_get_freeze_period, gitlab_delete_freeze_period"), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_freeze_period", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})

	mcp.AddTool(server, freezePeriodTool("gitlab_delete_freeze_period", "Delete a deploy freeze period from a project.\n\nReturns: JSON with deletion confirmation.\n\nSee also: gitlab_list_freeze_periods, gitlab_create_freeze_period"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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
