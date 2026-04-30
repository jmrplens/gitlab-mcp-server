// register.go wires events MCP tools to the MCP server.
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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_event_list",
		Title:       toolutil.TitleFromName("gitlab_project_event_list"),
		Description: "List all visible events for a project. Supports filtering by action type, target type, date range, sort order, and pagination.\n\nReturns: JSON array of events with pagination.\n\nSee also: gitlab_user_contribution_event_list, gitlab_commit_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectEventsInput) (*mcp.CallToolResult, ListProjectEventsOutput, error) {
		start := time.Now()
		out, err := ListProjectEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_event_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_user_contribution_event_list",
		Title:       toolutil.TitleFromName("gitlab_user_contribution_event_list"),
		Description: "List contribution events for the authenticated user. Supports filtering by action type, target type, date range, sort order, scope, and pagination.\n\nReturns: JSON array of contribution events with pagination.\n\nSee also: gitlab_project_event_list, gitlab_get_current_user",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListContributionEventsInput) (*mcp.CallToolResult, ListContributionEventsOutput, error) {
		start := time.Now()
		out, err := ListCurrentUserContributionEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_user_contribution_event_list", start, err)
		return toolutil.WithHints(FormatContributionListMarkdown(out), out, err)
	})
}

// RegisterMeta registers the gitlab_event meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list_project":            toolutil.RouteAction(client, ListProjectEvents),
		"list_user_contributions": toolutil.RouteAction(client, ListCurrentUserContributionEvents),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_event",
		Title: toolutil.TitleFromName("gitlab_event"),
		Description: `Manage GitLab events. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list_project: List visible events for a project. Params: project_id (required), action, target_type, before (YYYY-MM-DD), after (YYYY-MM-DD), sort, page, per_page
- list_user_contributions: List contribution events for the authenticated user. Params: action, target_type, before (YYYY-MM-DD), after (YYYY-MM-DD), sort, scope, page, per_page`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconEvent,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_event", routes, nil))
}
