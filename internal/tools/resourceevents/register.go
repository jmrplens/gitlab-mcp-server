// register.go wires resourceevents MCP tools to the MCP server.

package resourceevents

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers individual resource event tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	// Label Events.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_label_event_list",
		Title:       toolutil.TitleFromName("gitlab_issue_label_event_list"),
		Description: "List label events for a project issue. Shows when labels were added or removed.\n\nReturns: JSON array of label events with pagination. Fields include id, action, user, created_at.\n\nSee also: gitlab_issue_label_event_get, gitlab_issue_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListIssueLabelEventsInput) (*mcp.CallToolResult, ListLabelEventsOutput, error) {
		start := time.Now()
		out, err := ListIssueLabelEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_label_event_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatLabelEventsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_label_event_get",
		Title:       toolutil.TitleFromName("gitlab_issue_label_event_get"),
		Description: "Get a single label event for a project issue.\n\nReturns: JSON with label event details including id, action, user, created_at.\n\nSee also: gitlab_issue_label_event_list, gitlab_issue_milestone_event_get",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetIssueLabelEventInput) (*mcp.CallToolResult, LabelEventOutput, error) {
		start := time.Now()
		out, err := GetIssueLabelEvent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_label_event_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatLabelEventMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_label_event_list",
		Title:       toolutil.TitleFromName("gitlab_mr_label_event_list"),
		Description: "List label events for a merge request. Shows when labels were added or removed.\n\nReturns: JSON array of label events with pagination. Fields include id, action, user, created_at.\n\nSee also: gitlab_mr_label_event_get, gitlab_list_merge_requests",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListMRLabelEventsInput) (*mcp.CallToolResult, ListLabelEventsOutput, error) {
		start := time.Now()
		out, err := ListMRLabelEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_label_event_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatLabelEventsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_label_event_get",
		Title:       toolutil.TitleFromName("gitlab_mr_label_event_get"),
		Description: "Get a single label event for a merge request.\n\nReturns: JSON with label event details including id, action, user, created_at.\n\nSee also: gitlab_mr_label_event_list, gitlab_mr_milestone_event_get",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetMRLabelEventInput) (*mcp.CallToolResult, LabelEventOutput, error) {
		start := time.Now()
		out, err := GetMRLabelEvent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_label_event_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatLabelEventMarkdown(out)), out, err)
	})

	// Milestone Events.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_milestone_event_list",
		Title:       toolutil.TitleFromName("gitlab_issue_milestone_event_list"),
		Description: "List milestone events for a project issue. Shows when milestones were added or removed.\n\nReturns: JSON array of milestone events with pagination. Fields include id, action, user, created_at.\n\nSee also: gitlab_issue_milestone_event_get, gitlab_issue_label_event_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListIssueMilestoneEventsInput) (*mcp.CallToolResult, ListMilestoneEventsOutput, error) {
		start := time.Now()
		out, err := ListIssueMilestoneEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_milestone_event_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMilestoneEventsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_milestone_event_get",
		Title:       toolutil.TitleFromName("gitlab_issue_milestone_event_get"),
		Description: "Get a single milestone event for a project issue.\n\nReturns: JSON with milestone event details including id, action, user, created_at.\n\nSee also: gitlab_issue_milestone_event_list, gitlab_issue_state_event_get",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetIssueMilestoneEventInput) (*mcp.CallToolResult, MilestoneEventOutput, error) {
		start := time.Now()
		out, err := GetIssueMilestoneEvent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_milestone_event_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMilestoneEventMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_milestone_event_list",
		Title:       toolutil.TitleFromName("gitlab_mr_milestone_event_list"),
		Description: "List milestone events for a merge request. Shows when milestones were added or removed.\n\nReturns: JSON array of milestone events with pagination. Fields include id, action, user, created_at.\n\nSee also: gitlab_mr_milestone_event_get, gitlab_mr_label_event_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListMRMilestoneEventsInput) (*mcp.CallToolResult, ListMilestoneEventsOutput, error) {
		start := time.Now()
		out, err := ListMRMilestoneEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_milestone_event_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMilestoneEventsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_milestone_event_get",
		Title:       toolutil.TitleFromName("gitlab_mr_milestone_event_get"),
		Description: "Get a single milestone event for a merge request.\n\nReturns: JSON with milestone event details including id, action, user, created_at.\n\nSee also: gitlab_mr_milestone_event_list, gitlab_mr_state_event_get",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetMRMilestoneEventInput) (*mcp.CallToolResult, MilestoneEventOutput, error) {
		start := time.Now()
		out, err := GetMRMilestoneEvent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_milestone_event_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMilestoneEventMarkdown(out)), out, err)
	})

	// State Events.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_state_event_list",
		Title:       toolutil.TitleFromName("gitlab_issue_state_event_list"),
		Description: "List state events for a project issue. Shows when the issue was opened, closed, or reopened.\n\nReturns: JSON array of state events with pagination. Fields include id, state, user, created_at.\n\nSee also: gitlab_issue_state_event_get, gitlab_issue_label_event_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListIssueStateEventsInput) (*mcp.CallToolResult, ListStateEventsOutput, error) {
		start := time.Now()
		out, err := ListIssueStateEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_state_event_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatStateEventsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_state_event_get",
		Title:       toolutil.TitleFromName("gitlab_issue_state_event_get"),
		Description: "Get a single state event for a project issue.\n\nReturns: JSON with state event details including id, state, user, created_at.\n\nSee also: gitlab_issue_state_event_list, gitlab_issue_milestone_event_get",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetIssueStateEventInput) (*mcp.CallToolResult, StateEventOutput, error) {
		start := time.Now()
		out, err := GetIssueStateEvent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_state_event_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatStateEventMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_state_event_list",
		Title:       toolutil.TitleFromName("gitlab_mr_state_event_list"),
		Description: "List state events for a merge request. Shows when the MR was opened, closed, merged, or reopened.\n\nReturns: JSON array of state events with pagination. Fields include id, state, user, created_at.\n\nSee also: gitlab_mr_state_event_get, gitlab_mr_label_event_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListMRStateEventsInput) (*mcp.CallToolResult, ListStateEventsOutput, error) {
		start := time.Now()
		out, err := ListMRStateEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_state_event_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatStateEventsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_state_event_get",
		Title:       toolutil.TitleFromName("gitlab_mr_state_event_get"),
		Description: "Get a single state event for a merge request.\n\nReturns: JSON with state event details including id, state, user, created_at.\n\nSee also: gitlab_mr_state_event_list, gitlab_mr_milestone_event_get",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetMRStateEventInput) (*mcp.CallToolResult, StateEventOutput, error) {
		start := time.Now()
		out, err := GetMRStateEvent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_state_event_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatStateEventMarkdown(out)), out, err)
	})

	// Iteration Events.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_iteration_event_list",
		Title:       toolutil.TitleFromName("gitlab_issue_iteration_event_list"),
		Description: "List iteration events for a project issue. Shows when iterations were added or removed.\n\nReturns: JSON array of iteration events with pagination. Fields include id, action, user, iteration, created_at.\n\nSee also: gitlab_issue_iteration_event_get, gitlab_issue_label_event_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListIssueIterationEventsInput) (*mcp.CallToolResult, ListIterationEventsOutput, error) {
		start := time.Now()
		out, err := ListIssueIterationEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_iteration_event_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatIterationEventsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_iteration_event_get",
		Title:       toolutil.TitleFromName("gitlab_issue_iteration_event_get"),
		Description: "Get a single iteration event for a project issue.\n\nReturns: JSON with iteration event details including id, action, user, iteration, created_at.\n\nSee also: gitlab_issue_iteration_event_list, gitlab_issue_milestone_event_get",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetIssueIterationEventInput) (*mcp.CallToolResult, IterationEventOutput, error) {
		start := time.Now()
		out, err := GetIssueIterationEvent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_iteration_event_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatIterationEventMarkdown(out)), out, err)
	})

	// Weight Events.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_weight_event_list",
		Title:       toolutil.TitleFromName("gitlab_issue_weight_event_list"),
		Description: "List weight events for a project issue. Shows when weight was changed.\n\nReturns: JSON array of weight events with pagination. Fields include id, weight, user, created_at.\n\nSee also: gitlab_issue_label_event_list, gitlab_issue_state_event_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListIssueWeightEventsInput) (*mcp.CallToolResult, ListWeightEventsOutput, error) {
		start := time.Now()
		out, err := ListIssueWeightEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_weight_event_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatWeightEventsMarkdown(out)), out, err)
	})
}

// RegisterMeta registers the gitlab_resource_event meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list_issue_label_events":     toolutil.RouteAction(client, ListIssueLabelEvents),
		"get_issue_label_event":       toolutil.RouteAction(client, GetIssueLabelEvent),
		"list_mr_label_events":        toolutil.RouteAction(client, ListMRLabelEvents),
		"get_mr_label_event":          toolutil.RouteAction(client, GetMRLabelEvent),
		"list_issue_milestone_events": toolutil.RouteAction(client, ListIssueMilestoneEvents),
		"get_issue_milestone_event":   toolutil.RouteAction(client, GetIssueMilestoneEvent),
		"list_mr_milestone_events":    toolutil.RouteAction(client, ListMRMilestoneEvents),
		"get_mr_milestone_event":      toolutil.RouteAction(client, GetMRMilestoneEvent),
		"list_issue_state_events":     toolutil.RouteAction(client, ListIssueStateEvents),
		"get_issue_state_event":       toolutil.RouteAction(client, GetIssueStateEvent),
		"list_mr_state_events":        toolutil.RouteAction(client, ListMRStateEvents),
		"get_mr_state_event":          toolutil.RouteAction(client, GetMRStateEvent),
		"list_issue_iteration_events": toolutil.RouteAction(client, ListIssueIterationEvents),
		"get_issue_iteration_event":   toolutil.RouteAction(client, GetIssueIterationEvent),
		"list_issue_weight_events":    toolutil.RouteAction(client, ListIssueWeightEvents),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_resource_event",
		Title: toolutil.TitleFromName("gitlab_resource_event"),
		Description: `Manage GitLab resource events (label, milestone, state, iteration, weight changes on issues and MRs). Use 'action' to specify the operation.

Actions:
- list_issue_label_events: List label events for an issue. Params: project_id, issue_iid (required), page, per_page
- get_issue_label_event: Get a single issue label event. Params: project_id, issue_iid, label_event_id (all required)
- list_mr_label_events: List label events for a MR. Params: project_id, merge_request_iid (required), page, per_page
- get_mr_label_event: Get a single MR label event. Params: project_id, merge_request_iid, label_event_id (all required)
- list_issue_milestone_events: List milestone events for an issue. Params: project_id, issue_iid (required), page, per_page
- get_issue_milestone_event: Get a single issue milestone event. Params: project_id, issue_iid, milestone_event_id (all required)
- list_mr_milestone_events: List milestone events for a MR. Params: project_id, merge_request_iid (required), page, per_page
- get_mr_milestone_event: Get a single MR milestone event. Params: project_id, merge_request_iid, milestone_event_id (all required)
- list_issue_state_events: List state events for an issue. Params: project_id, issue_iid (required), page, per_page
- get_issue_state_event: Get a single issue state event. Params: project_id, issue_iid, state_event_id (all required)
- list_mr_state_events: List state events for a MR. Params: project_id, merge_request_iid (required), page, per_page
- get_mr_state_event: Get a single MR state event. Params: project_id, merge_request_iid, state_event_id (all required)
- list_issue_iteration_events: List iteration events for an issue. Params: project_id, issue_iid (required), page, per_page
- get_issue_iteration_event: Get a single issue iteration event. Params: project_id, issue_iid, iteration_event_id (all required)
- list_issue_weight_events: List weight events for an issue. Params: project_id, issue_iid (required), page, per_page`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconEvent,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_resource_event", routes, nil))
}
