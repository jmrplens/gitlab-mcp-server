package deploykeys

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all deploy key MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	deployKeyTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconKey})
	}

	mcp.AddTool(server, deployKeyTool("gitlab_deploy_key_list_project", "List all deploy keys for a GitLab project.\n\nSee also: gitlab_deploy_key_add, gitlab_deploy_token_list_project\n\nReturns: JSON array of deploy keys with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_list_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, deployKeyTool("gitlab_deploy_key_get", "Get a specific deploy key for a project by its ID.\n\nSee also: gitlab_deploy_key_list_project, gitlab_deploy_key_update\n\nReturns: JSON with deploy key details including title, key content, and permissions."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out))
		if err == nil && out.ID != 0 && string(input.ProjectID) != "" {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://project/%s/deploy_key/%d", url.PathEscape(string(input.ProjectID)), out.ID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, deployKeyTool("gitlab_deploy_key_add", "Add a deploy key to a GitLab project with title, public SSH key, and optional push access and expiry date.\n\nSee also: gitlab_deploy_key_list_project, gitlab_deploy_key_enable\n\nReturns: JSON with the created deploy key details."), func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Add(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_add", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, deployKeyTool("gitlab_deploy_key_update", "Update an existing deploy key's title or push access permission.\n\nSee also: gitlab_deploy_key_get, gitlab_deploy_key_list_project\n\nReturns: JSON with the updated deploy key details."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, deployKeyTool("gitlab_deploy_key_delete", "Remove a deploy key from a GitLab project.\n\nSee also: gitlab_deploy_key_list_project, gitlab_deploy_key_add\n\nReturns: confirmation message."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete deploy key %d from project %q?", input.DeployKeyID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("deploy key")
	})

	mcp.AddTool(server, deployKeyTool("gitlab_deploy_key_enable", "Enable an existing deploy key for a project (e.g., a key shared from another project).\n\nSee also: gitlab_deploy_key_list_project, gitlab_deploy_key_add\n\nReturns: JSON with the enabled deploy key details."), func(ctx context.Context, req *mcp.CallToolRequest, input EnableInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Enable(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_enable", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, deployKeyTool("gitlab_deploy_key_list_all", "List all instance-level deploy keys. Requires admin access. Filter by public keys.\n\nSee also: gitlab_deploy_key_add_instance, gitlab_deploy_key_list_project\n\nReturns: JSON array of instance deploy keys with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListAllInput) (*mcp.CallToolResult, InstanceListOutput, error) {
		start := time.Now()
		out, err := ListAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_list_all", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatInstanceListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, deployKeyTool("gitlab_deploy_key_add_instance", "Create an instance-level deploy key with title, public SSH key, and optional expiry date. Requires admin access.\n\nSee also: gitlab_deploy_key_list_all, gitlab_deploy_key_add\n\nReturns: JSON with the created instance deploy key details."), func(ctx context.Context, req *mcp.CallToolRequest, input AddInstanceInput) (*mcp.CallToolResult, InstanceOutput, error) {
		start := time.Now()
		out, err := AddInstance(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_add_instance", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatInstanceOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, deployKeyTool("gitlab_deploy_key_list_user_project", "List all deploy keys across projects for a specific user.\n\nSee also: gitlab_deploy_key_list_project, gitlab_deploy_key_list_all\n\nReturns: JSON array of deploy keys with pagination."), func(ctx context.Context, req *mcp.CallToolRequest, input ListUserProjectInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListUserProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_list_user_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})
}
