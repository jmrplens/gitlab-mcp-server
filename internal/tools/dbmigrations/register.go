// register.go wires dbmigrations MCP tools to the MCP server.

package dbmigrations

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all Database Migrations MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mark_migration",
		Title:       toolutil.TitleFromName("gitlab_mark_migration"),
		Description: "Mark a pending database migration as successfully executed (admin). Params: version (required), database (optional).\n\nReturns: JSON with the migration mark confirmation.\n\nSee also: gitlab_server_status.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MarkInput) (*mcp.CallToolResult, MarkOutput, error) {
		start := time.Now()
		out, err := Mark(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mark_migration", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkMarkdown(out)), out, nil)
	})
}
