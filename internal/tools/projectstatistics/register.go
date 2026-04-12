// register.go wires projectstatistics MCP tools to the MCP server.

package projectstatistics

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all project statistics MCP tools on the given server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_project_statistics",
		Title:       toolutil.TitleFromName("gitlab_get_project_statistics"),
		Description: "Get project fetch statistics for the last 30 days.\n\nReturns: JSON with project fetch statistics.\n\nSee also: gitlab_project_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_statistics", start, err)
		if err != nil {
			return nil, GetOutput{}, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, nil)
	})
}

// RegisterMeta registers the gitlab_project_statistics meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]toolutil.ActionFunc{
		"get": toolutil.WrapAction(client, Get),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_project_statistics",
		Title: toolutil.TitleFromName("gitlab_project_statistics"),
		Description: `Get project fetch statistics from GitLab. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- get: Get project fetch statistics for the last 30 days. Params: project_id (required)`,
		Annotations: toolutil.MetaAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, toolutil.MakeMetaHandler("gitlab_project_statistics", routes, nil))
}
