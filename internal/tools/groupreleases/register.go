// register.go wires group release MCP tools to the MCP server.
package groupreleases

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers group releases tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_release_list",
		Title:       toolutil.TitleFromName("gitlab_group_release_list"),
		Description: "List releases across all projects in a GitLab group.\n\nReturns: paginated list of releases with tag, name, dates, and author. See also: gitlab_release_list, gitlab_group_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRelease,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_release_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})
}
