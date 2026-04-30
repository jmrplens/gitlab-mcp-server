// markdown.go provides Markdown formatting functions for group import/export MCP tool output.
package groupimportexport

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatScheduleExportMarkdown formats the schedule export result.
func FormatScheduleExportMarkdown(out ScheduleExportOutput) *mcp.CallToolResult {
	if out.Message == "" {
		return nil
	}
	var sb strings.Builder
	sb.WriteString(out.Message)
	toolutil.WriteHints(&sb,
		"Use `gitlab_download_group_export` to download the export once complete",
	)
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatExportDownloadMarkdown formats the download result.
func FormatExportDownloadMarkdown(out ExportDownloadOutput) *mcp.CallToolResult {
	if out.SizeBytes == 0 {
		return nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Group export archive downloaded: %d bytes (base64-encoded in content_base64 field)", out.SizeBytes)
	toolutil.WriteHints(&sb,
		"Use `gitlab_import_group_from_file` to import the archive into another group",
	)
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatImportFileMarkdown formats the import result.
func FormatImportFileMarkdown(out ImportFileOutput) *mcp.CallToolResult {
	if out.Message == "" {
		return nil
	}
	var sb strings.Builder
	sb.WriteString(out.Message)
	toolutil.WriteHints(&sb,
		"Use `gitlab_group_list` to verify the imported group appears",
	)
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatMarkdown dispatches markdown formatting for group import/export results.
func FormatMarkdown(result any) *mcp.CallToolResult {
	switch v := result.(type) {
	case ScheduleExportOutput:
		return FormatScheduleExportMarkdown(v)
	case ExportDownloadOutput:
		return FormatExportDownloadMarkdown(v)
	case ImportFileOutput:
		return FormatImportFileMarkdown(v)
	default:
		return nil
	}
}

func init() {
	toolutil.RegisterMarkdownResult(FormatScheduleExportMarkdown)
	toolutil.RegisterMarkdownResult(FormatExportDownloadMarkdown)
	toolutil.RegisterMarkdownResult(FormatImportFileMarkdown)
	toolutil.RegisterMarkdownResult(FormatMarkdown)
}
