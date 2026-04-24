// register.go wires audit event MCP tools to the MCP server.

package auditevents

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers audit event tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_instance_audit_events",
		Title:       toolutil.TitleFromName("gitlab_list_instance_audit_events"),
		Description: "List instance-level audit events (admin only). Supports filtering by date range. Returns: paginated list of audit events with ID, author, entity, event details. See also: gitlab_get_instance_audit_event, gitlab_list_group_audit_events, gitlab_list_project_audit_events.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInstanceInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListInstance(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_instance_audit_events", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_instance_audit_event",
		Title:       toolutil.TitleFromName("gitlab_get_instance_audit_event"),
		Description: "Get a single instance-level audit event by ID (admin only). Returns: audit event with ID, author, entity, event details, and created timestamp. See also: gitlab_list_instance_audit_events.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInstanceInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetInstance(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_instance_audit_event", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_group_audit_events",
		Title:       toolutil.TitleFromName("gitlab_list_group_audit_events"),
		Description: "List audit events for a GitLab group. Supports filtering by date range. Returns: paginated list of audit events with ID, author, entity, event details. See also: gitlab_get_group_audit_event, gitlab_list_instance_audit_events, gitlab_list_project_audit_events.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_group_audit_events", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_group_audit_event",
		Title:       toolutil.TitleFromName("gitlab_get_group_audit_event"),
		Description: "Get a single group-level audit event by ID. Returns: audit event with ID, author, entity, event details, and created timestamp. See also: gitlab_list_group_audit_events.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetGroupInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_group_audit_event", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_project_audit_events",
		Title:       toolutil.TitleFromName("gitlab_list_project_audit_events"),
		Description: "List audit events for a GitLab project. Supports filtering by date range. Returns: paginated list of audit events with ID, author, entity, event details. See also: gitlab_get_project_audit_event, gitlab_list_group_audit_events, gitlab_list_instance_audit_events.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_project_audit_events", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_project_audit_event",
		Title:       toolutil.TitleFromName("gitlab_get_project_audit_event"),
		Description: "Get a single project-level audit event by ID. Returns: audit event with ID, author, entity, event details, and created timestamp. See also: gitlab_list_project_audit_events.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEvent,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_audit_event", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})
}
