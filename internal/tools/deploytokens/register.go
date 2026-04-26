// register.go wires deploytokens MCP tools to the MCP server.

package deploytokens

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all deploy token MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_token_list_all",
		Title:       toolutil.TitleFromName("gitlab_deploy_token_list_all"),
		Description: "List all instance-level deploy tokens. Requires admin access.\n\nSee also: gitlab_deploy_token_list_project, gitlab_deploy_token_list_group\n\nReturns: JSON array of deploy tokens with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListAllInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_list_all", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_token_list_project",
		Title:       toolutil.TitleFromName("gitlab_deploy_token_list_project"),
		Description: "List all deploy tokens for a GitLab project.\n\nSee also: gitlab_deploy_token_create_project, gitlab_deploy_key_list_project\n\nReturns: JSON array of deploy tokens with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_list_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_token_list_group",
		Title:       toolutil.TitleFromName("gitlab_deploy_token_list_group"),
		Description: "List all deploy tokens for a GitLab group.\n\nSee also: gitlab_deploy_token_create_group, gitlab_deploy_token_list_project\n\nReturns: JSON array of deploy tokens with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_list_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_token_get_project",
		Title:       toolutil.TitleFromName("gitlab_deploy_token_get_project"),
		Description: "Get a specific deploy token for a project.\n\nSee also: gitlab_deploy_token_list_project, gitlab_deploy_token_get_group\n\nReturns: JSON with deploy token details including ID, name, scopes, and expiration.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_get_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_token_get_group",
		Title:       toolutil.TitleFromName("gitlab_deploy_token_get_group"),
		Description: "Get a specific deploy token for a group.\n\nSee also: gitlab_deploy_token_list_group, gitlab_deploy_token_get_project\n\nReturns: JSON with deploy token details including ID, name, scopes, and expiration.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetGroupInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_get_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_token_create_project",
		Title:       toolutil.TitleFromName("gitlab_deploy_token_create_project"),
		Description: "Create a deploy token for a project with name, scopes, optional username and expiry date.\n\nSee also: gitlab_deploy_token_list_project, gitlab_deploy_token_create_group\n\nReturns: JSON with the created deploy token including the token value (shown only once).",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_create_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_token_create_group",
		Title:       toolutil.TitleFromName("gitlab_deploy_token_create_group"),
		Description: "Create a deploy token for a group with name, scopes, optional username and expiry date.\n\nSee also: gitlab_deploy_token_list_group, gitlab_deploy_token_create_project\n\nReturns: JSON with the created deploy token including the token value (shown only once).",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateGroupInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_create_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_token_delete_project",
		Title:       toolutil.TitleFromName("gitlab_deploy_token_delete_project"),
		Description: "Delete a deploy token from a project. This action cannot be undone.\n\nSee also: gitlab_deploy_token_list_project, gitlab_deploy_token_create_project\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteProjectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete deploy token %d from project %q? This cannot be undone.", input.DeployTokenID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := DeleteProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_delete_project", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("project deploy token")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_token_delete_group",
		Title:       toolutil.TitleFromName("gitlab_deploy_token_delete_group"),
		Description: "Delete a deploy token from a group. This action cannot be undone.\n\nSee also: gitlab_deploy_token_list_group, gitlab_deploy_token_create_group\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteGroupInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete deploy token %d from group %q? This cannot be undone.", input.DeployTokenID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := DeleteGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_delete_group", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group deploy token")
	})
}

// RegisterMeta registers the gitlab_deploy_token meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list_all":       toolutil.RouteAction(client, ListAll),
		"list_project":   toolutil.RouteAction(client, ListProject),
		"list_group":     toolutil.RouteAction(client, ListGroup),
		"get_project":    toolutil.RouteAction(client, GetProject),
		"get_group":      toolutil.RouteAction(client, GetGroup),
		"create_project": toolutil.RouteAction(client, CreateProject),
		"create_group":   toolutil.RouteAction(client, CreateGroup),
		"delete_project": toolutil.DestructiveVoidAction(client, DeleteProject),
		"delete_group":   toolutil.DestructiveVoidAction(client, DeleteGroup),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_deploy_token",
		Title: toolutil.TitleFromName("gitlab_deploy_token"),
		Description: `Manage deploy tokens in GitLab (project, group, and instance level). Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list_all: List all instance deploy tokens (admin). No params required
- list_project: List project deploy tokens. Params: project_id (required), page, per_page
- list_group: List group deploy tokens. Params: group_id (required), page, per_page
- get_project: Get a project deploy token. Params: project_id (required), deploy_token_id (required, int)
- get_group: Get a group deploy token. Params: group_id (required), deploy_token_id (required, int)
- create_project: Create project deploy token. Params: project_id (required), name (required), scopes (required, array), username, expires_at (YYYY-MM-DD)
- create_group: Create group deploy token. Params: group_id (required), name (required), scopes (required, array), username, expires_at (YYYY-MM-DD)
- delete_project: Delete project deploy token. Params: project_id (required), deploy_token_id (required, int)
- delete_group: Delete group deploy token. Params: group_id (required), deploy_token_id (required, int)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconToken,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_deploy_token", routes, nil))
}
