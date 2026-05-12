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
		Description: "Import a project from an export archive file. Accepts either base64-encoded content or a local .tar.gz file_path under the current working directory, OS temp directory, or GITLAB_MCP_ALLOWED_IMPORT_DIRS after symlink resolution.\n\nReturns: JSON with import status details.\n\nSee also: gitlab_get_project_import_status, gitlab_import_from_github",
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
