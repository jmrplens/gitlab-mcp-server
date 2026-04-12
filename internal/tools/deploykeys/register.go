// register.go wires deploykeys MCP tools to the MCP server.

package deploykeys

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all deploy key MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_key_list_project",
		Title:       toolutil.TitleFromName("gitlab_deploy_key_list_project"),
		Description: "List all deploy keys for a GitLab project.\n\nSee also: gitlab_deploy_key_add, gitlab_deploy_token_list_project\n\nReturns: JSON array of deploy keys with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_list_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_key_get",
		Title:       toolutil.TitleFromName("gitlab_deploy_key_get"),
		Description: "Get a specific deploy key for a project by its ID.\n\nSee also: gitlab_deploy_key_list_project, gitlab_deploy_key_update\n\nReturns: JSON with deploy key details including title, key content, and permissions.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_key_add",
		Title:       toolutil.TitleFromName("gitlab_deploy_key_add"),
		Description: "Add a deploy key to a GitLab project with title, public SSH key, and optional push access and expiry date.\n\nSee also: gitlab_deploy_key_list_project, gitlab_deploy_key_enable\n\nReturns: JSON with the created deploy key details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Add(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_add", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_key_update",
		Title:       toolutil.TitleFromName("gitlab_deploy_key_update"),
		Description: "Update an existing deploy key's title or push access permission.\n\nSee also: gitlab_deploy_key_get, gitlab_deploy_key_list_project\n\nReturns: JSON with the updated deploy key details.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_key_delete",
		Title:       toolutil.TitleFromName("gitlab_deploy_key_delete"),
		Description: "Remove a deploy key from a GitLab project.\n\nSee also: gitlab_deploy_key_list_project, gitlab_deploy_key_add\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_key_enable",
		Title:       toolutil.TitleFromName("gitlab_deploy_key_enable"),
		Description: "Enable an existing deploy key for a project (e.g., a key shared from another project).\n\nSee also: gitlab_deploy_key_list_project, gitlab_deploy_key_add\n\nReturns: JSON with the enabled deploy key details.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EnableInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Enable(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_enable", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_key_list_all",
		Title:       toolutil.TitleFromName("gitlab_deploy_key_list_all"),
		Description: "List all instance-level deploy keys. Requires admin access. Filter by public keys.\n\nSee also: gitlab_deploy_key_add_instance, gitlab_deploy_key_list_project\n\nReturns: JSON array of instance deploy keys with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListAllInput) (*mcp.CallToolResult, InstanceListOutput, error) {
		start := time.Now()
		out, err := ListAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_list_all", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatInstanceListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_key_add_instance",
		Title:       toolutil.TitleFromName("gitlab_deploy_key_add_instance"),
		Description: "Create an instance-level deploy key with title, public SSH key, and optional expiry date. Requires admin access.\n\nSee also: gitlab_deploy_key_list_all, gitlab_deploy_key_add\n\nReturns: JSON with the created instance deploy key details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddInstanceInput) (*mcp.CallToolResult, InstanceOutput, error) {
		start := time.Now()
		out, err := AddInstance(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_add_instance", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatInstanceOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deploy_key_list_user_project",
		Title:       toolutil.TitleFromName("gitlab_deploy_key_list_user_project"),
		Description: "List all deploy keys across projects for a specific user.\n\nSee also: gitlab_deploy_key_list_project, gitlab_deploy_key_list_all\n\nReturns: JSON array of deploy keys with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListUserProjectInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListUserProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deploy_key_list_user_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})
}

// RegisterMeta registers the gitlab_deploy_key meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]toolutil.ActionFunc{
		"list_project":      toolutil.WrapAction(client, ListProject),
		"get":               toolutil.WrapAction(client, Get),
		"add":               toolutil.WrapAction(client, Add),
		"update":            toolutil.WrapAction(client, Update),
		"delete":            toolutil.WrapVoidAction(client, Delete),
		"enable":            toolutil.WrapAction(client, Enable),
		"list_all":          toolutil.WrapAction(client, ListAll),
		"add_instance":      toolutil.WrapAction(client, AddInstance),
		"list_user_project": toolutil.WrapAction(client, ListUserProject),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_deploy_key",
		Title: toolutil.TitleFromName("gitlab_deploy_key"),
		Description: `Manage deploy keys in GitLab (project-level, instance-level, and per-user). Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list_project: List project deploy keys. Params: project_id (required), page, per_page
- get: Get a deploy key. Params: project_id (required), deploy_key_id (required, int)
- add: Add a deploy key to a project. Params: project_id (required), title (required), key (required), can_push (bool), expires_at (YYYY-MM-DD)
- update: Update a deploy key. Params: project_id (required), deploy_key_id (required, int), title, can_push (bool)
- delete: Delete a deploy key. Params: project_id (required), deploy_key_id (required, int)
- enable: Enable a deploy key for a project. Params: project_id (required), deploy_key_id (required, int)
- list_all: List all instance-level deploy keys (admin). Params: public (bool), page, per_page
- add_instance: Create instance-level deploy key (admin). Params: title (required), key (required), expires_at (YYYY-MM-DD)
- list_user_project: List deploy keys for a user's projects. Params: user_id (required), page, per_page`,
		Annotations: toolutil.MetaAnnotations,
		Icons:       toolutil.IconKey,
	}, toolutil.MakeMetaHandler("gitlab_deploy_key", routes, nil))
}
