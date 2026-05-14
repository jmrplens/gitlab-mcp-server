package planlimits

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all Plan Limits MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	planLimitTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconConfig})
	}

	mcp.AddTool(server, planLimitTool("gitlab_get_plan_limits", "Get current plan limits (admin). Optionally filter by plan name (default, free, bronze, silver, gold, premium, ultimate).\n\nReturns: JSON with current plan limits.\n\nSee also: gitlab_change_plan_limits."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_plan_limits", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGetMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, planLimitTool("gitlab_change_plan_limits", "Change plan limits (admin). Requires plan_name; optionally set individual file size limits.\n\nReturns: JSON with the updated plan limits.\n\nSee also: gitlab_get_plan_limits."), func(ctx context.Context, req *mcp.CallToolRequest, input ChangeInput) (*mcp.CallToolResult, ChangeOutput, error) {
		start := time.Now()
		out, err := Change(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_change_plan_limits", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatChangeMarkdown(out)), out, nil)
	})
}
