// register.go wires merge train MCP tools to the MCP server.
package mergetrains

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers individual merge train tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_project_merge_trains",
		Title:       toolutil.TitleFromName("gitlab_list_project_merge_trains"),
		Description: "List all merge trains for a project.\n\nReturns: JSON array of merge train entries with pagination. Fields include id, merge_request, user, pipeline, target_branch, status, duration.\n\nSee also: gitlab_list_merge_request_in_merge_train, gitlab_get_merge_request_on_merge_train",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListProjectMergeTrains(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_project_merge_trains", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_merge_request_in_merge_train",
		Title:       toolutil.TitleFromName("gitlab_list_merge_request_in_merge_train"),
		Description: "List merge requests in a merge train for a specific target branch.\n\nReturns: JSON array of merge train entries with pagination.\n\nSee also: gitlab_list_project_merge_trains, gitlab_add_merge_request_to_merge_train",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListBranchInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListMergeRequestInMergeTrain(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_merge_request_in_merge_train", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_merge_request_on_merge_train",
		Title:       toolutil.TitleFromName("gitlab_get_merge_request_on_merge_train"),
		Description: "Get the merge train status for a specific merge request.\n\nReturns: JSON with merge train entry details including id, merge_request, status, target_branch, duration.\n\nSee also: gitlab_add_merge_request_to_merge_train, gitlab_list_project_merge_trains",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetMergeRequestOnMergeTrain(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_merge_request_on_merge_train", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_add_merge_request_to_merge_train",
		Title:       toolutil.TitleFromName("gitlab_add_merge_request_to_merge_train"),
		Description: "Add a merge request to a merge train.\n\nReturns: JSON array of merge train entries after the addition.\n\nSee also: gitlab_get_merge_request_on_merge_train, gitlab_list_project_merge_trains",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := AddMergeRequestToMergeTrain(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_merge_request_to_merge_train", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})
}
