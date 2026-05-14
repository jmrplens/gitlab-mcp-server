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
	specs := ActionSpecs(client)
	deployTokenTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconToken})
	}

	mcp.AddTool(server, deployTokenTool("gitlab_deploy_token_list_all", "List all instance-level deploy tokens. Requires admin access.\n\nSee also: gitlab_deploy_token_list_project, gitlab_deploy_token_list_group\n\nReturns: JSON array of deploy tokens with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListAllInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_list_all", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, deployTokenTool("gitlab_deploy_token_list_project", "List all deploy tokens for a GitLab project.\n\nSee also: gitlab_deploy_token_create_project, gitlab_deploy_key_list_project\n\nReturns: JSON array of deploy tokens with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_list_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, deployTokenTool("gitlab_deploy_token_list_group", "List all deploy tokens for a GitLab group.\n\nSee also: gitlab_deploy_token_create_group, gitlab_deploy_token_list_project\n\nReturns: JSON array of deploy tokens with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_list_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, deployTokenTool("gitlab_deploy_token_get_project", "Get a specific deploy token for a project.\n\nSee also: gitlab_deploy_token_list_project, gitlab_deploy_token_get_group\n\nReturns: JSON with deploy token details including ID, name, scopes, and expiration."), func(ctx context.Context, req *mcp.CallToolRequest, input GetProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_get_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, deployTokenTool("gitlab_deploy_token_get_group", "Get a specific deploy token for a group.\n\nSee also: gitlab_deploy_token_list_group, gitlab_deploy_token_get_project\n\nReturns: JSON with deploy token details including ID, name, scopes, and expiration."), func(ctx context.Context, req *mcp.CallToolRequest, input GetGroupInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_get_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, deployTokenTool("gitlab_deploy_token_create_project", "Create a deploy token for a project with name, scopes, optional username and expiry date.\n\nSee also: gitlab_deploy_token_list_project, gitlab_deploy_token_create_group\n\nReturns: JSON with the created deploy token including the token value (shown only once)."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_create_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, deployTokenTool("gitlab_deploy_token_create_group", "Create a deploy token for a group with name, scopes, optional username and expiry date.\n\nSee also: gitlab_deploy_token_list_group, gitlab_deploy_token_create_project\n\nReturns: JSON with the created deploy token including the token value (shown only once)."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateGroupInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_token_create_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, deployTokenTool("gitlab_deploy_token_delete_project", "Delete a deploy token from a project. This action cannot be undone.\n\nSee also: gitlab_deploy_token_list_project, gitlab_deploy_token_create_project\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteProjectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, deployTokenTool("gitlab_deploy_token_delete_group", "Delete a deploy token from a group. This action cannot be undone.\n\nSee also: gitlab_deploy_token_list_group, gitlab_deploy_token_create_group\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteGroupInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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
