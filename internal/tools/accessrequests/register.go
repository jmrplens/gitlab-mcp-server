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
	specs := ActionSpecs(client)
	accessRequestTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconUser})
	}

	mcp.AddTool(server, accessRequestTool("gitlab_access_request_list_project", "List access requests for a GitLab project.\n\nSee also: gitlab_access_request_approve_project, gitlab_project_member_add\n\nReturns: JSON array of access requests with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_access_request_list_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, accessRequestTool("gitlab_access_request_list_group", "List access requests for a GitLab group.\n\nSee also: gitlab_access_request_approve_group, gitlab_group_member_add\n\nReturns: JSON array of access requests with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_access_request_list_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, accessRequestTool("gitlab_access_request_request_project", "Request access to a GitLab project for the authenticated user.\n\nSee also: gitlab_access_request_list_project, gitlab_project_member_add\n\nReturns: JSON with the access request details."), func(ctx context.Context, req *mcp.CallToolRequest, input RequestProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := RequestProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_access_request_request_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, accessRequestTool("gitlab_access_request_request_group", "Request access to a GitLab group for the authenticated user.\n\nSee also: gitlab_access_request_list_group, gitlab_group_member_add\n\nReturns: JSON with the access request details."), func(ctx context.Context, req *mcp.CallToolRequest, input RequestGroupInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := RequestGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_access_request_request_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, accessRequestTool("gitlab_access_request_approve_project", "Approve a project access request. Optionally set the access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer).\n\nSee also: gitlab_access_request_list_project, gitlab_project_member_add\n\nReturns: JSON with the approved access request details."), func(ctx context.Context, req *mcp.CallToolRequest, input ApproveProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := ApproveProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_access_request_approve_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, accessRequestTool("gitlab_access_request_approve_group", "Approve a group access request. Optionally set the access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer).\n\nSee also: gitlab_access_request_list_group, gitlab_group_member_add\n\nReturns: JSON with the approved access request details."), func(ctx context.Context, req *mcp.CallToolRequest, input ApproveGroupInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := ApproveGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_access_request_approve_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, accessRequestTool("gitlab_access_request_deny_project", "Deny a project access request. This action cannot be undone.\n\nSee also: gitlab_access_request_approve_project, gitlab_access_request_list_project\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DenyProjectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, accessRequestTool("gitlab_access_request_deny_group", "Deny a group access request. This action cannot be undone.\n\nSee also: gitlab_access_request_approve_group, gitlab_access_request_list_group\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DenyGroupInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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
