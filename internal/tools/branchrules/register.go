// register.go wires Branch Rules MCP tools to the MCP server.

package branchrules

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers Branch Rules tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_branch_rules",
		Title:       toolutil.TitleFromName("gitlab_list_branch_rules"),
		Description: "List branch rules for a project via GraphQL API. Provides an aggregated read-only view of branch protections, approval rules, and external status checks. Returns: paginated list with name, protection status, force push, code owner approval, approval rules, and external status checks.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_branch_rules", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})
}
