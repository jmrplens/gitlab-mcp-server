// register.go wires resourcegroups MCP tools to the MCP server.

package resourcegroups

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all resource group tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_resource_groups",
		Title:       toolutil.TitleFromName("gitlab_list_resource_groups"),
		Description: "List resource groups for a GitLab project.\n\nReturns: JSON array of resource groups with pagination.\n\nSee also: gitlab_get_resource_group, gitlab_list_pipelines",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconQueue,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_resource_groups", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_resource_group",
		Title:       toolutil.TitleFromName("gitlab_get_resource_group"),
		Description: "Get details of a resource group.\n\nReturns: JSON with resource group details.\n\nSee also: gitlab_list_resource_groups, gitlab_edit_resource_group",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconQueue,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, ResourceGroupItem, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_resource_group", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGroupMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_edit_resource_group",
		Title:       toolutil.TitleFromName("gitlab_edit_resource_group"),
		Description: "Edit a resource group process mode.\n\nReturns: JSON with the updated resource group details.\n\nSee also: gitlab_get_resource_group, gitlab_list_resource_group_upcoming_jobs",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconQueue,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EditInput) (*mcp.CallToolResult, ResourceGroupItem, error) {
		start := time.Now()
		out, err := Edit(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_edit_resource_group", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGroupMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_resource_group_upcoming_jobs",
		Title:       toolutil.TitleFromName("gitlab_list_resource_group_upcoming_jobs"),
		Description: "List upcoming jobs for a resource group.\n\nReturns: JSON array of upcoming jobs with pagination.\n\nSee also: gitlab_get_resource_group, gitlab_list_resource_groups",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconQueue,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListUpcomingJobsInput) (*mcp.CallToolResult, ListUpcomingJobsOutput, error) {
		start := time.Now()
		out, err := ListUpcomingJobs(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_resource_group_upcoming_jobs", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatJobsMarkdown(out)), out, nil)
	})
}

// RegisterMeta registers the gitlab_resource_group meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list":               toolutil.RouteAction(client, ListAll),
		"get":                toolutil.RouteAction(client, Get),
		"edit":               toolutil.RouteAction(client, Edit),
		"list_upcoming_jobs": toolutil.RouteAction(client, ListUpcomingJobs),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_resource_group",
		Title: toolutil.TitleFromName("gitlab_resource_group"),
		Description: `Manage resource groups in GitLab CI/CD. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List resource groups for a project. Params: project_id (required)
- get: Get a single resource group. Params: project_id (required), key (required)
- edit: Edit a resource group process mode. Params: project_id (required), key (required), process_mode (required: unordered, oldest_first, newest_first)
- list_upcoming_jobs: List upcoming jobs for a resource group. Params: project_id (required), key (required)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconQueue,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_resource_group", routes, nil))
}
