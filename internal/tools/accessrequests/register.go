// register.go wires accessrequests MCP tools to the MCP server.

package accessrequests

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all access request MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_access_request_list_project",
		Title:       toolutil.TitleFromName("gitlab_access_request_list_project"),
		Description: "List access requests for a GitLab project.\n\nSee also: gitlab_access_request_approve_project, gitlab_project_member_add\n\nReturns: JSON array of access requests with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_access_request_list_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_access_request_list_group",
		Title:       toolutil.TitleFromName("gitlab_access_request_list_group"),
		Description: "List access requests for a GitLab group.\n\nSee also: gitlab_access_request_approve_group, gitlab_group_member_add\n\nReturns: JSON array of access requests with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_access_request_list_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_access_request_request_project",
		Title:       toolutil.TitleFromName("gitlab_access_request_request_project"),
		Description: "Request access to a GitLab project for the authenticated user.\n\nSee also: gitlab_access_request_list_project, gitlab_project_member_add\n\nReturns: JSON with the access request details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RequestProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := RequestProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_access_request_request_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_access_request_request_group",
		Title:       toolutil.TitleFromName("gitlab_access_request_request_group"),
		Description: "Request access to a GitLab group for the authenticated user.\n\nSee also: gitlab_access_request_list_group, gitlab_group_member_add\n\nReturns: JSON with the access request details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RequestGroupInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := RequestGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_access_request_request_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_access_request_approve_project",
		Title:       toolutil.TitleFromName("gitlab_access_request_approve_project"),
		Description: "Approve a project access request. Optionally set the access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer).\n\nSee also: gitlab_access_request_list_project, gitlab_project_member_add\n\nReturns: JSON with the approved access request details.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ApproveProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := ApproveProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_access_request_approve_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_access_request_approve_group",
		Title:       toolutil.TitleFromName("gitlab_access_request_approve_group"),
		Description: "Approve a group access request. Optionally set the access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer).\n\nSee also: gitlab_access_request_list_group, gitlab_group_member_add\n\nReturns: JSON with the approved access request details.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ApproveGroupInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := ApproveGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_access_request_approve_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_access_request_deny_project",
		Title:       toolutil.TitleFromName("gitlab_access_request_deny_project"),
		Description: "Deny a project access request. This action cannot be undone.\n\nSee also: gitlab_access_request_approve_project, gitlab_access_request_list_project\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DenyProjectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Deny access request from user %d for project %s?", input.UserID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DenyProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_access_request_deny_project", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("project access request")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_access_request_deny_group",
		Title:       toolutil.TitleFromName("gitlab_access_request_deny_group"),
		Description: "Deny a group access request. This action cannot be undone.\n\nSee also: gitlab_access_request_approve_group, gitlab_access_request_list_group\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DenyGroupInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Deny access request from user %d for group %s?", input.UserID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DenyGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_access_request_deny_group", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group access request")
	})
}

// RegisterMeta registers the gitlab_access_request meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list_project":    toolutil.RouteAction(client, ListProject),
		"list_group":      toolutil.RouteAction(client, ListGroup),
		"request_project": toolutil.RouteAction(client, RequestProject),
		"request_group":   toolutil.RouteAction(client, RequestGroup),
		"approve_project": toolutil.RouteAction(client, ApproveProject),
		"approve_group":   toolutil.RouteAction(client, ApproveGroup),
		"deny_project":    toolutil.DestructiveVoidAction(client, DenyProject),
		"deny_group":      toolutil.DestructiveVoidAction(client, DenyGroup),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_access_request",
		Title: toolutil.TitleFromName("gitlab_access_request"),
		Description: `Manage access requests for GitLab projects and groups. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list_project: List project access requests. Params: project_id (required), page, per_page
- list_group: List group access requests. Params: group_id (required), page, per_page
- request_project: Request access to a project. Params: project_id (required)
- request_group: Request access to a group. Params: group_id (required)
- approve_project: Approve project access request. Params: project_id (required), user_id (required, int), access_level (optional, int)
- approve_group: Approve group access request. Params: group_id (required), user_id (required, int), access_level (optional, int)
- deny_project: Deny project access request. Params: project_id (required), user_id (required, int)
- deny_group: Deny group access request. Params: group_id (required), user_id (required, int)`,
		Annotations:  toolutil.DeriveAnnotationsWithTitle("gitlab_access_request", routes),
		Icons:        toolutil.IconUser,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_access_request", routes, nil))
}
