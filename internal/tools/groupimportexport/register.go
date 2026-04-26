// register.go wires groupimportexport MCP tools to the MCP server.

package groupimportexport

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all group import/export tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_schedule_group_export",
		Title:       toolutil.TitleFromName("gitlab_schedule_group_export"),
		Description: "Schedule an asynchronous export of a group.\n\nReturns: JSON with the export schedule confirmation.\n\nSee also: gitlab_download_group_export, gitlab_schedule_project_export",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ScheduleExportInput) (*mcp.CallToolResult, ScheduleExportOutput, error) {
		start := time.Now()
		out, err := ScheduleExport(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_schedule_group_export", start, err)
		return toolutil.WithHints(FormatScheduleExportMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_download_group_export",
		Title:       toolutil.TitleFromName("gitlab_download_group_export"),
		Description: "Download the finished export archive of a group. Returns the archive as base64-encoded content.\n\nReturns: JSON with the base64-encoded export archive.\n\nSee also: gitlab_schedule_group_export, gitlab_import_group_from_file",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ExportDownloadInput) (*mcp.CallToolResult, ExportDownloadOutput, error) {
		start := time.Now()
		out, err := ExportDownload(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_download_group_export", start, err)
		return toolutil.WithHints(FormatExportDownloadMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_import_group_from_file",
		Title:       toolutil.TitleFromName("gitlab_import_group_from_file"),
		Description: "Import a group from an export archive file. Requires a local file path to the .tar.gz archive.\n\nReturns: JSON with the import details.\n\nSee also: gitlab_schedule_group_export, gitlab_start_bulk_import",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ImportFileInput) (*mcp.CallToolResult, ImportFileOutput, error) {
		start := time.Now()
		out, err := ImportFile(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_import_group_from_file", start, err)
		return toolutil.WithHints(FormatImportFileMarkdown(out), out, err)
	})
}

// RegisterMeta registers the gitlab_group_import_export meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"schedule_export": toolutil.RouteAction(client, ScheduleExport),
		"export_download": toolutil.RouteAction(client, ExportDownload),
		"import_file":     toolutil.RouteAction(client, ImportFile),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_group_import_export",
		Title: toolutil.TitleFromName("gitlab_group_import_export"),
		Description: `Manage group import/export operations. Use 'action' to specify the operation.

Actions:
- schedule_export: Schedule an async group export. Params: group_id (required)
- export_download: Download finished group export archive as base64. Params: group_id (required)
- import_file: Import group from archive. Params: name, path, file (required), parent_id`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconImport,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_group_import_export", routes, nil))
}
