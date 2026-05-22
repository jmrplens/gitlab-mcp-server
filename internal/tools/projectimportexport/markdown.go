package projectimportexport

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatScheduleExportMarkdown coordinates format schedule export markdown for the projectimportexport package.
func FormatScheduleExportMarkdown(out ScheduleExportOutput) *mcp.CallToolResult {
	if out.Message == "" {
		return nil
	}
	return toolutil.ToolResultWithMarkdown(out.Message)
}

// FormatExportStatusMarkdown coordinates format export status markdown for the projectimportexport package.
func FormatExportStatusMarkdown(out ExportStatusOutput) *mcp.CallToolResult {
	if out.ID == 0 {
		return nil
	}
	rows := []statusRow{{"Status", out.ExportStatus}}
	appendNonEmptyStatusRow(&rows, "Message", out.Message)
	if out.APIURL != "" {
		appendNonEmptyStatusRow(&rows, "API URL", out.APIURL)
	}
	if out.WebURL != "" {
		appendNonEmptyStatusRow(&rows, "Web URL", out.WebURL)
	}
	return projectImportExportStatusResult("Export Status", out.Name, out.ID, out.PathWithNamespace, rows,
		"Use `gitlab_download_project_export` when the export status is 'finished'")
}

type statusRow struct {
	Field string
	Value string
}

func appendNonEmptyStatusRow(rows *[]statusRow, field, value string) {
	if value != "" {
		*rows = append(*rows, statusRow{Field: field, Value: value})
	}
}

func projectImportExportStatusResult(title, name string, id int64, path string, rows []statusRow, hint string) *mcp.CallToolResult {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s: %s\n\n", title, name)
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| ID | %d |\n", id)
	fmt.Fprintf(&sb, "| Path | %s |\n", path)
	for _, row := range rows {
		fmt.Fprintf(&sb, "| %s | %s |\n", row.Field, row.Value)
	}
	toolutil.WriteHints(&sb, hint)
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatExportDownloadMarkdown coordinates format export download markdown for the projectimportexport package.
func FormatExportDownloadMarkdown(out ExportDownloadOutput) *mcp.CallToolResult {
	if out.SizeBytes == 0 {
		return nil
	}
	return toolutil.ToolResultWithMarkdown(fmt.Sprintf("Export archive downloaded: %d bytes (base64-encoded in content_base64 field)", out.SizeBytes))
}

// FormatImportStatusMarkdown coordinates format import status markdown for the projectimportexport package.
func FormatImportStatusMarkdown(out ImportStatusOutput) *mcp.CallToolResult {
	if out.ID == 0 {
		return nil
	}
	rows := []statusRow{{"Status", out.ImportStatus}}
	appendNonEmptyStatusRow(&rows, "Type", out.ImportType)
	appendNonEmptyStatusRow(&rows, "Correlation ID", out.CorrelationID)
	appendNonEmptyStatusRow(&rows, "Error", out.ImportError)
	return projectImportExportStatusResult("Import Status", out.Name, out.ID, out.PathWithNamespace, rows,
		"Monitor import progress by checking status periodically")
}

func init() {
	toolutil.RegisterMarkdownResult(FormatScheduleExportMarkdown)
	toolutil.RegisterMarkdownResult(FormatExportStatusMarkdown)
	toolutil.RegisterMarkdownResult(FormatExportDownloadMarkdown)
	toolutil.RegisterMarkdownResult(FormatImportStatusMarkdown)
}
