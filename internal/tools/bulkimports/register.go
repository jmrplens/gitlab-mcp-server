// register.go wires bulkimports MCP tools to the MCP server.

package bulkimports

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all bulk import MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_start_bulk_import",
		Title:       toolutil.TitleFromName("gitlab_start_bulk_import"),
		Description: "Start a new group or project bulk import migration (admin). Requires source GitLab URL, access token, and entities to migrate.\n\nReturns: JSON with the migration details.\n\nSee also: gitlab_import_from_github, gitlab_schedule_group_export",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StartMigrationInput) (*mcp.CallToolResult, MigrationOutput, error) {
		start := time.Now()
		out, err := StartMigration(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_start_bulk_import", start, err)
		if err != nil {
			return nil, MigrationOutput{}, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatStartMigrationMarkdown(out)), out, nil)
	})
}
