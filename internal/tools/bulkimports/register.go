// register.go wires bulkimports MCP tools to the MCP server.

package bulkimports

import (
	"context"
	"fmt"
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
		Description: "Start a new group or project bulk import migration (admin). Requires source GitLab URL, access token, and entities to migrate.\n\nReturns: JSON with the migration details.\n\nSee also: gitlab_bulk_import_list, gitlab_bulk_import_get, gitlab_import_from_github, gitlab_schedule_group_export",
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_bulk_import_list",
		Title:       toolutil.TitleFromName("gitlab_bulk_import_list"),
		Description: "List all group or project bulk import migrations visible to the caller. Optionally filter by status.\n\nReturns: paginated list of migrations with id, status, source_type, source_url, has_failures, and timestamps.\n\nSee also: gitlab_bulk_import_get, gitlab_bulk_import_entity_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_bulk_import_list", start, err)
		if err != nil {
			return nil, ListOutput{}, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_bulk_import_get",
		Title:       toolutil.TitleFromName("gitlab_bulk_import_get"),
		Description: "Get details of a single bulk import migration by ID.\n\nReturns: migration with id, status, source_type, source_url, has_failures, and timestamps.\n\nSee also: gitlab_bulk_import_list, gitlab_bulk_import_entity_list, gitlab_bulk_import_cancel.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, MigrationSummary, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_bulk_import_get", start, nil)
			return toolutil.NotFoundResult("Bulk Import", fmt.Sprintf("ID %d", input.ID),
				"Use gitlab_bulk_import_list to list visible migrations",
				"Verify the migration id is correct",
			), MigrationSummary{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_bulk_import_get", start, err)
		if err != nil {
			return nil, MigrationSummary{}, err
		}
		return toolutil.ToolResultWithMarkdown(FormatGetMarkdown(out)), out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_bulk_import_cancel",
		Title:       toolutil.TitleFromName("gitlab_bulk_import_cancel"),
		Description: "Cancel an in-progress bulk import migration. Returns the migration with updated status.\n\nReturns: migration summary with id and status.\n\nSee also: gitlab_bulk_import_get, gitlab_bulk_import_list.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CancelInput) (*mcp.CallToolResult, MigrationSummary, error) {
		start := time.Now()
		out, err := Cancel(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_bulk_import_cancel", start, err)
		if err != nil {
			return nil, MigrationSummary{}, err
		}
		return toolutil.ToolResultAnnotated(FormatGetMarkdown(out), toolutil.ContentMutate), out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_bulk_import_entity_list",
		Title:       toolutil.TitleFromName("gitlab_bulk_import_entity_list"),
		Description: "List bulk import migration entities. When bulk_import_id is provided, scopes to that import; otherwise returns all entities visible to the caller. Optionally filter by status.\n\nReturns: paginated list of entities with id, status, type, source/destination paths, and per-relation stats.\n\nSee also: gitlab_bulk_import_entity_get, gitlab_bulk_import_entity_failures.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListEntitiesInput) (*mcp.CallToolResult, ListEntitiesOutput, error) {
		start := time.Now()
		out, err := ListEntities(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_bulk_import_entity_list", start, err)
		if err != nil {
			return nil, ListEntitiesOutput{}, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListEntitiesMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_bulk_import_entity_get",
		Title:       toolutil.TitleFromName("gitlab_bulk_import_entity_get"),
		Description: "Get details of a single bulk import migration entity by bulk_import_id and entity_id.\n\nReturns: entity with id, status, type, source/destination paths, migration flags, and per-relation stats.\n\nSee also: gitlab_bulk_import_entity_list, gitlab_bulk_import_entity_failures.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetEntityInput) (*mcp.CallToolResult, EntitySummary, error) {
		start := time.Now()
		out, err := GetEntity(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_bulk_import_entity_get", start, nil)
			return toolutil.NotFoundResult("Bulk Import Entity", fmt.Sprintf("entity %d in import %d", input.EntityID, input.BulkImportID),
				"Use gitlab_bulk_import_entity_list with bulk_import_id to list entities",
				"Verify both bulk_import_id and entity_id are correct",
			), EntitySummary{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_bulk_import_entity_get", start, err)
		if err != nil {
			return nil, EntitySummary{}, err
		}
		return toolutil.ToolResultWithMarkdown(FormatGetEntityMarkdown(out)), out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_bulk_import_entity_failures",
		Title:       toolutil.TitleFromName("gitlab_bulk_import_entity_failures"),
		Description: "List failed import records for a bulk import migration entity. Useful for diagnosing failed migrations.\n\nReturns: list of failures with relation, exception class/message, pipeline class/step, and source url.\n\nSee also: gitlab_bulk_import_entity_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListEntityFailuresInput) (*mcp.CallToolResult, ListEntityFailuresOutput, error) {
		start := time.Now()
		out, err := ListEntityFailures(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_bulk_import_entity_failures", start, err)
		if err != nil {
			return nil, ListEntityFailuresOutput{}, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatEntityFailuresMarkdown(out)), out, nil)
	})
}
