// markdown.go provides Markdown formatting functions for project import/export MCP tool output.
package projectimportexport

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatScheduleExportMarkdown performs the format schedule export markdown operation for the projectimportexport package.
func FormatScheduleExportMarkdown(out ScheduleExportOutput) *mcp.CallToolResult {
	if out.Message == "" {
		return nil
	}
	return toolutil.ToolResultWithMarkdown(out.Message)
}

// FormatExportStatusMarkdown performs the format export status markdown operation for the projectimportexport package.
func FormatExportStatusMarkdown(out ExportStatusOutput) *mcp.CallToolResult {
	if out.ID == 0 {
		return nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Export Status: %s\n\n", out.Name)
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&sb, "| Path | %s |\n", out.PathWithNamespace)
	fmt.Fprintf(&sb, "| Status | %s |\n", out.ExportStatus)
	if out.Message != "" {
		fmt.Fprintf(&sb, "| Message | %s |\n", out.Message)
	}
	if out.APIURL != "" {
		fmt.Fprintf(&sb, "| API URL | %s |\n", out.APIURL)
	}
	if out.WebURL != "" {
		fmt.Fprintf(&sb, "| Web URL | %s |\n", out.WebURL)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_download_project_export` when the export status is 'finished'")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatExportDownloadMarkdown performs the format export download markdown operation for the projectimportexport package.
func FormatExportDownloadMarkdown(out ExportDownloadOutput) *mcp.CallToolResult {
	if out.SizeBytes == 0 {
		return nil
	}
	return toolutil.ToolResultWithMarkdown(fmt.Sprintf("Export archive downloaded: %d bytes (base64-encoded in content_base64 field)", out.SizeBytes))
}

// FormatImportStatusMarkdown performs the format import status markdown operation for the projectimportexport package.
func FormatImportStatusMarkdown(out ImportStatusOutput) *mcp.CallToolResult {
	if out.ID == 0 {
		return nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Import Status: %s\n\n", out.Name)
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&sb, "| Path | %s |\n", out.PathWithNamespace)
	fmt.Fprintf(&sb, "| Status | %s |\n", out.ImportStatus)
	if out.ImportType != "" {
		fmt.Fprintf(&sb, "| Type | %s |\n", out.ImportType)
	}
	if out.CorrelationID != "" {
		fmt.Fprintf(&sb, "| Correlation ID | %s |\n", out.CorrelationID)
	}
	if out.ImportError != "" {
		fmt.Fprintf(&sb, "| Error | %s |\n", out.ImportError)
	}
	toolutil.WriteHints(&sb, "Monitor import progress by checking status periodically")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatScheduleExportMarkdown)
	toolutil.RegisterMarkdownResult(FormatExportStatusMarkdown)
	toolutil.RegisterMarkdownResult(FormatExportDownloadMarkdown)
	toolutil.RegisterMarkdownResult(FormatImportStatusMarkdown)
}
