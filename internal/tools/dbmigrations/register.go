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
	specs := ActionSpecs(client)
	databaseMigrationTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconConfig})
	}

	mcp.AddTool(server, databaseMigrationTool("gitlab_mark_migration", "Mark a pending database migration as successfully executed (admin). Params: version (required), database (optional).\n\nReturns: JSON with the migration mark confirmation.\n\nSee also: gitlab_server_status."), func(ctx context.Context, req *mcp.CallToolRequest, input MarkInput) (*mcp.CallToolResult, MarkOutput, error) {
		start := time.Now()
		out, err := Mark(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mark_migration", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkMarkdown(out)), out, nil)
	})
}
