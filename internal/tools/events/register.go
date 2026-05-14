package events

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers individual event tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := UserActionSpecs(client)
	eventTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconEvent})
	}

	mcp.AddTool(server, eventTool("gitlab_project_event_list", "List all visible events for a project. Supports filtering by action type, target type, date range, sort order, and pagination.\n\nReturns: JSON array of events with pagination.\n\nSee also: gitlab_user_contribution_event_list, gitlab_commit_list"), func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectEventsInput) (*mcp.CallToolResult, ListProjectEventsOutput, error) {
		start := time.Now()
		out, err := ListProjectEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_event_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, eventTool("gitlab_user_contribution_event_list", "List contribution events for the authenticated user. Supports filtering by action type, target type, date range, sort order, scope, and pagination.\n\nReturns: JSON array of contribution events with pagination.\n\nSee also: gitlab_project_event_list, gitlab_get_current_user"), func(ctx context.Context, req *mcp.CallToolRequest, input ListContributionEventsInput) (*mcp.CallToolResult, ListContributionEventsOutput, error) {
		start := time.Now()
		out, err := ListCurrentUserContributionEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_user_contribution_event_list", start, err)
		return toolutil.WithHints(FormatContributionListMarkdown(out), out, err)
	})
}
