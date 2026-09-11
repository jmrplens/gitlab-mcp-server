package groupimportexport

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The import/export routes a confirmation points at, by the canonical catalog
// ID every surface resolves; the bare names beside them in action_specs.go are
// the catalog's own cross-references, which are relative to the group.
const (
	hintActionExportDownload = "group." + actionGroupExportDownload
	hintActionImportFile     = "group." + actionGroupImportFile
)

// confirmation renders a one-line result: the server's own sentence as the
// card's heading, which terminates the line, collapses whatever line breaks it
// carries and defuses a heading inside it, then the next steps.
//
// The line used to be written with no newline of its own, so the guidance rule
// that followed turned it into a setext heading and the hints it opened never
// reached next_steps.
func confirmation(message string, hints ...string) *mcp.CallToolResult {
	var sb strings.Builder
	toolutil.NewCard(&sb, toolutil.EmojiSuccess+" "+message).End(hints...)
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatScheduleExportMarkdown formats the schedule export result.
func FormatScheduleExportMarkdown(out ScheduleExportOutput) *mcp.CallToolResult {
	if out.Message == "" {
		return nil
	}
	return confirmation(out.Message,
		toolutil.HintAction(hintActionExportDownload, "download the export once it is complete"),
	)
}

// FormatExportDownloadMarkdown formats the download result.
func FormatExportDownloadMarkdown(out ExportDownloadOutput) *mcp.CallToolResult {
	if out.SizeBytes == 0 {
		return nil
	}
	var sb strings.Builder
	c := toolutil.NewCard(&sb, toolutil.EmojiSuccess+" Group export archive downloaded")
	c.Int("Size (bytes)", int64(out.SizeBytes))
	c.Note("The archive is base64-encoded in the content_base64 field.")
	c.End(toolutil.HintAction(hintActionImportFile, "import the archive into another group"))
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatImportFileMarkdown formats the import result.
func FormatImportFileMarkdown(out ImportFileOutput) *mcp.CallToolResult {
	if out.Message == "" {
		return nil
	}
	return confirmation(out.Message,
		toolutil.HintAction(actionGroupList, "verify the imported group appears"),
	)
}

func init() {
	toolutil.RegisterMarkdownResult(FormatScheduleExportMarkdown)
	toolutil.RegisterMarkdownResult(FormatExportDownloadMarkdown)
	toolutil.RegisterMarkdownResult(FormatImportFileMarkdown)
}
