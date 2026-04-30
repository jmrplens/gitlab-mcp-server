// register.go wires deploymentmergerequests MCP tools to the MCP server.
package deploymentmergerequests

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers the deployment merge requests tool on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_deployment_merge_requests",
		Title:       toolutil.TitleFromName("gitlab_list_deployment_merge_requests"),
		Description: "List merge requests associated with a specific deployment in a GitLab project.\n\nReturns: JSON array of merge requests with pagination.\n\nSee also: gitlab_deployment_get, gitlab_mr_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_deployment_merge_requests", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})
}
