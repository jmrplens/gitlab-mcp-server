package groupmilestones

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers group milestone tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	groupMilestoneTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconMilestone})
	}

	mcp.AddTool(server, groupMilestoneTool("gitlab_group_milestone_list", "List all milestones for a GitLab group. Supports filtering by state, title, search, IIDs, date ranges, and ancestor/descendant groups. Returns milestone title, state, dates, and pagination.\n\nReturns: JSON array of group milestones with pagination. See also: gitlab_group_milestone_get."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, groupMilestoneTool("gitlab_group_milestone_get", "Get details of a single group milestone by IID, including title, state, start/due dates, and timestamps.\n\nReturns: JSON with group milestone details. See also: gitlab_group_milestone_list."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		if err == nil && out.IID != 0 && string(input.GroupID) != "" {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://group/%s/milestone/%d", url.PathEscape(string(input.GroupID)), out.IID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, groupMilestoneTool("gitlab_group_milestone_create", "Create a new milestone in a GitLab group with a title, optional description, start date and due date (YYYY-MM-DD).\n\nReturns: JSON with the created milestone details. See also: gitlab_group_milestone_get."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupMilestoneTool("gitlab_group_milestone_update", "Update an existing group milestone. Can change title, description, dates, or state (activate/close). Only specified fields are modified.\n\nReturns: JSON with the updated milestone details. See also: gitlab_group_milestone_get."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupMilestoneTool("gitlab_group_milestone_delete", "Delete a group milestone by IID.\n\nReturns: confirmation message. See also: gitlab_group_milestone_list."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, groupMilestoneTool("gitlab_group_milestone_issues", "List all issues assigned to a group milestone. Returns issue ID, IID, title, state, and web URL with pagination.\n\nReturns: JSON array of issues for the milestone with pagination. See also: gitlab_group_milestone_get, gitlab_issue_list."), func(ctx context.Context, req *mcp.CallToolRequest, input GetIssuesInput) (*mcp.CallToolResult, IssuesOutput, error) {
		start := time.Now()
		out, err := GetIssues(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_issues", start, err)
		return toolutil.WithHints(FormatIssuesMarkdown(out), out, err)
	})

	mcp.AddTool(server, groupMilestoneTool("gitlab_group_milestone_merge_requests", "List all merge requests assigned to a group milestone. Returns MR ID, IID, title, state, source/target branches with pagination.\n\nReturns: JSON array of merge requests for the milestone with pagination. See also: gitlab_group_milestone_get, gitlab_mr_list."), func(ctx context.Context, req *mcp.CallToolRequest, input GetMergeRequestsInput) (*mcp.CallToolResult, MergeRequestsOutput, error) {
		start := time.Now()
		out, err := GetMergeRequests(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_merge_requests", start, err)
		return toolutil.WithHints(FormatMergeRequestsMarkdown(out), out, err)
	})

	mcp.AddTool(server, groupMilestoneTool("gitlab_group_milestone_burndown_events", "List all burndown chart events for a group milestone. Returns event timestamps, weights, and actions with pagination.\n\nReturns: JSON array of burndown chart events. See also: gitlab_group_milestone_get."), func(ctx context.Context, req *mcp.CallToolRequest, input GetBurndownChartEventsInput) (*mcp.CallToolResult, BurndownChartEventsOutput, error) {
		start := time.Now()
		out, err := GetBurndownChartEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_milestone_burndown_events", start, err)
		return toolutil.WithHints(FormatBurndownChartEventsMarkdown(out), out, err)
	})
}
