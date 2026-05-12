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
		Description: "Import a group from an export archive file. Requires a local .tar.gz archive path under the current working directory, OS temp directory, or GITLAB_MCP_ALLOWED_IMPORT_DIRS after symlink resolution.\n\nReturns: JSON with the import details.\n\nSee also: gitlab_schedule_group_export, gitlab_start_bulk_import",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ImportFileInput) (*mcp.CallToolResult, ImportFileOutput, error) {
		start := time.Now()
		out, err := ImportFile(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_import_group_from_file", start, err)
		return toolutil.WithHints(FormatImportFileMarkdown(out), out, err)
	})
}
