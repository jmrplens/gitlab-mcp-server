// register.go wires project iteration MCP tools to the MCP server.

package projectiterations

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers individual project iteration tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_project_iterations",
		Title:       toolutil.TitleFromName("gitlab_list_project_iterations"),
		Description: "List iterations for a project. Iterations provide time-boxed planning periods.\n\nReturns: JSON array of iterations with pagination. Fields include id, iid, title, state, start_date, due_date, web_url.\n\nSee also: gitlab_list_group_iterations",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_project_iterations", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})
}
