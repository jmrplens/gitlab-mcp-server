// register.go wires groupmilestones MCP tools to the MCP server.

package groupmilestones

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers group milestone tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_milestone_list",
		Title:       toolutil.TitleFromName("gitlab_group_milestone_list"),
		Description: "List all milestones for a GitLab group. Supports filtering by state, title, search, IIDs, date ranges, and ancestor/descendant groups. Returns milestone title, state, dates, and pagination.\n\nReturns: JSON array of group milestones with pagination. See also: gitlab_group_milestone_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_milestone_get",
		Title:       toolutil.TitleFromName("gitlab_group_milestone_get"),
		Description: "Get details of a single group milestone by IID, including title, state, start/due dates, and timestamps.\n\nReturns: JSON with group milestone details. See also: gitlab_group_milestone_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_milestone_create",
		Title:       toolutil.TitleFromName("gitlab_group_milestone_create"),
		Description: "Create a new milestone in a GitLab group with a title, optional description, start date and due date (YYYY-MM-DD).\n\nReturns: JSON with the created milestone details. See also: gitlab_group_milestone_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_milestone_update",
		Title:       toolutil.TitleFromName("gitlab_group_milestone_update"),
		Description: "Update an existing group milestone. Can change title, description, dates, or state (activate/close). Only specified fields are modified.\n\nReturns: JSON with the updated milestone details. See also: gitlab_group_milestone_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_milestone_delete",
		Title:       toolutil.TitleFromName("gitlab_group_milestone_delete"),
		Description: "Delete a group milestone by IID.\n\nReturns: confirmation message. See also: gitlab_group_milestone_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete group milestone %d from group %q?", input.MilestoneIID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group milestone")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_milestone_issues",
		Title:       toolutil.TitleFromName("gitlab_group_milestone_issues"),
		Description: "List all issues assigned to a group milestone. Returns issue ID, IID, title, state, and web URL with pagination.\n\nReturns: JSON array of issues for the milestone with pagination. See also: gitlab_group_milestone_get, gitlab_issue_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetIssuesInput) (*mcp.CallToolResult, IssuesOutput, error) {
		start := time.Now()
		out, err := GetIssues(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_issues", start, err)
		return toolutil.WithHints(FormatIssuesMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_milestone_merge_requests",
		Title:       toolutil.TitleFromName("gitlab_group_milestone_merge_requests"),
		Description: "List all merge requests assigned to a group milestone. Returns MR ID, IID, title, state, source/target branches with pagination.\n\nReturns: JSON array of merge requests for the milestone with pagination. See also: gitlab_group_milestone_get, gitlab_mr_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetMergeRequestsInput) (*mcp.CallToolResult, MergeRequestsOutput, error) {
		start := time.Now()
		out, err := GetMergeRequests(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_merge_requests", start, err)
		return toolutil.WithHints(FormatMergeRequestsMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_milestone_burndown_events",
		Title:       toolutil.TitleFromName("gitlab_group_milestone_burndown_events"),
		Description: "List all burndown chart events for a group milestone. Returns event timestamps, weights, and actions with pagination.\n\nReturns: JSON array of burndown chart events. See also: gitlab_group_milestone_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetBurndownChartEventsInput) (*mcp.CallToolResult, BurndownChartEventsOutput, error) {
		start := time.Now()
		out, err := GetBurndownChartEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_burndown_events", start, err)
		return toolutil.WithHints(FormatBurndownChartEventsMarkdown(out), out, err)
	})
}

// RegisterMeta registers the group milestone meta-tool on the MCP server.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list":            toolutil.RouteAction(client, List),
		"get":             toolutil.RouteAction(client, Get),
		"create":          toolutil.RouteAction(client, Create),
		"update":          toolutil.RouteAction(client, Update),
		"delete":          toolutil.DestructiveVoidAction(client, Delete),
		"issues":          toolutil.RouteAction(client, GetIssues),
		"merge_requests":  toolutil.RouteAction(client, GetMergeRequests),
		"burndown_events": toolutil.RouteAction(client, GetBurndownChartEvents),
	}

	desc := `Manage GitLab group milestones (list, get, create, update, delete, issues, merge_requests, burndown_events).

Actions:
- list: List group milestones. Params: group_id (required), state (active/closed), title, search, search_title, include_ancestors (bool), include_descendants (bool), iids ([]int), updated_before/updated_after/containing_date (YYYY-MM-DD), page, per_page
- get: Get a group milestone. Params: group_id (required), milestone_iid (required)
- create: Create a group milestone. Params: group_id (required), title (required), description, start_date (YYYY-MM-DD), due_date (YYYY-MM-DD)
- update: Update a group milestone. Params: group_id (required), milestone_iid (required), title, description, start_date, due_date, state_event (activate/close)
- delete: Delete a group milestone. Params: group_id (required), milestone_iid (required)
- issues: List issues assigned to a group milestone. Params: group_id (required), milestone_iid (required), page, per_page
- merge_requests: List merge requests assigned to a group milestone. Params: group_id (required), milestone_iid (required), page, per_page
- burndown_events: List burndown chart events for a group milestone. Params: group_id (required), milestone_iid (required), page, per_page`

	mcp.AddTool(server, &mcp.Tool{
		Name:         "gitlab_group_milestone",
		Title:        toolutil.TitleFromName("gitlab_group_milestone"),
		Description:  desc,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconMilestone,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_group_milestone", routes, nil))
}
