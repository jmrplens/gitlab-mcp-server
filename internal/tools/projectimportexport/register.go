// register.go wires projectimportexport MCP tools to the MCP server.

package projectimportexport

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all project import/export tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_schedule_project_export",
		Title:       toolutil.TitleFromName("gitlab_schedule_project_export"),
		Description: "Schedule an asynchronous export of a project. After scheduling, use gitlab_get_project_export_status to check progress.\n\nReturns: JSON confirmation that the export was scheduled.\n\nSee also: gitlab_get_project_export_status, gitlab_schedule_group_export",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ScheduleExportInput) (*mcp.CallToolResult, ScheduleExportOutput, error) {
		start := time.Now()
		out, err := ScheduleExport(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_schedule_project_export", start, err)
		return toolutil.WithHints(FormatScheduleExportMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_project_export_status",
		Title:       toolutil.TitleFromName("gitlab_get_project_export_status"),
		Description: "Get the export status of a project, including download links when the export is finished.\n\nReturns: JSON with export status and download links when finished.\n\nSee also: gitlab_schedule_project_export, gitlab_download_project_export",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ExportStatusInput) (*mcp.CallToolResult, ExportStatusOutput, error) {
		start := time.Now()
		out, err := GetExportStatus(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_export_status", start, err)
		return toolutil.WithHints(FormatExportStatusMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_download_project_export",
		Title:       toolutil.TitleFromName("gitlab_download_project_export"),
		Description: "Download the finished export archive of a project. Returns the archive as base64-encoded content.\n\nReturns: JSON with the base64-encoded export archive.\n\nSee also: gitlab_get_project_export_status, gitlab_import_project_from_file",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ExportDownloadInput) (*mcp.CallToolResult, ExportDownloadOutput, error) {
		start := time.Now()
		out, err := ExportDownload(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_download_project_export", start, err)
		return toolutil.WithHints(FormatExportDownloadMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_import_project_from_file",
		Title:       toolutil.TitleFromName("gitlab_import_project_from_file"),
		Description: "Import a project from an export archive file. Accepts either a local file_path or base64-encoded content.\n\nReturns: JSON with import status details.\n\nSee also: gitlab_get_project_import_status, gitlab_import_from_github",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ImportFromFileInput) (*mcp.CallToolResult, ImportStatusOutput, error) {
		start := time.Now()
		out, err := ImportFromFile(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_import_project_from_file", start, err)
		return toolutil.WithHints(FormatImportStatusMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_project_import_status",
		Title:       toolutil.TitleFromName("gitlab_get_project_import_status"),
		Description: "Get the import status of a project.\n\nReturns: JSON with import status details.\n\nSee also: gitlab_import_project_from_file, gitlab_schedule_project_export",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetImportStatusInput) (*mcp.CallToolResult, ImportStatusOutput, error) {
		start := time.Now()
		out, err := GetImportStatus(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_import_status", start, err)
		return toolutil.WithHints(FormatImportStatusMarkdown(out), out, err)
	})
}

// RegisterMeta registers the gitlab_project_import_export meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"schedule_export":  toolutil.RouteAction(client, ScheduleExport),
		"export_status":    toolutil.RouteAction(client, GetExportStatus),
		"export_download":  toolutil.RouteAction(client, ExportDownload),
		"import_from_file": toolutil.RouteAction(client, ImportFromFile),
		"import_status":    toolutil.RouteAction(client, GetImportStatus),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_project_import_export",
		Title: toolutil.TitleFromName("gitlab_project_import_export"),
		Description: `Manage project import/export operations. Use 'action' to specify the operation.

Actions:
- schedule_export: Schedule an async export. Params: project_id (required), description, upload_url, upload_http_method
- export_status: Get export status. Params: project_id (required)
- export_download: Download finished export archive as base64. Params: project_id (required)
- import_from_file: Import project from archive. Params: file_path or content_base64 (required), namespace, name, path, overwrite
- import_status: Get import status. Params: project_id (required)`,
		Annotations: toolutil.DeriveAnnotations(routes),
		Icons:       toolutil.IconImport,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_project_import_export", routes, nil))
}
