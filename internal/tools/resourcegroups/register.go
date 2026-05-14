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
	specs := ActionSpecs(client)
	resourceGroupTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconQueue})
	}

	mcp.AddTool(server, resourceGroupTool("gitlab_list_resource_groups", "List resource groups for a GitLab project.\n\nReturns: JSON array of resource groups with pagination.\n\nSee also: gitlab_get_resource_group, gitlab_pipeline_list"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_resource_groups", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, resourceGroupTool("gitlab_get_resource_group", "Get details of a resource group.\n\nReturns: JSON with resource group details.\n\nSee also: gitlab_list_resource_groups, gitlab_edit_resource_group"), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, ResourceGroupItem, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_resource_group", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGroupMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, resourceGroupTool("gitlab_edit_resource_group", "Edit a resource group process mode.\n\nReturns: JSON with the updated resource group details.\n\nSee also: gitlab_get_resource_group, gitlab_list_resource_group_upcoming_jobs"), func(ctx context.Context, req *mcp.CallToolRequest, input EditInput) (*mcp.CallToolResult, ResourceGroupItem, error) {
		start := time.Now()
		out, err := Edit(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_edit_resource_group", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGroupMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, resourceGroupTool("gitlab_list_resource_group_upcoming_jobs", "List upcoming jobs for a resource group.\n\nReturns: JSON array of upcoming jobs with pagination.\n\nSee also: gitlab_get_resource_group, gitlab_list_resource_groups"), func(ctx context.Context, req *mcp.CallToolRequest, input ListUpcomingJobsInput) (*mcp.CallToolResult, ListUpcomingJobsOutput, error) {
		start := time.Now()
		out, err := ListUpcomingJobs(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_resource_group_upcoming_jobs", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatJobsMarkdown(out)), out, nil)
	})
}
