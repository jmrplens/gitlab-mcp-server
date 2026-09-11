package projectimportexport

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatScheduleExportMarkdown renders the one sentence a scheduled export
// answers with. The sentence is this package's own literal; the cell escaper
// is idempotent over it and is what keeps the line one line whatever a later
// GitLab-authored message holds.
func FormatScheduleExportMarkdown(out ScheduleExportOutput) *mcp.CallToolResult {
	if out.Message == "" {
		return nil
	}
	return toolutil.ToolResultWithMarkdown(toolutil.EscapeMdTableCell(out.Message) + "\n")
}

// FormatExportStatusMarkdown renders an export's status as a card: the
// project's identity, the status GitLab reports and the two addresses the
// archive is reachable at when the export has finished.
func FormatExportStatusMarkdown(out ExportStatusOutput) *mcp.CallToolResult {
	if out.ID == 0 {
		return nil
	}
	var b strings.Builder
	c := statusCard(&b, "Export Status", out.Name, out.ID, out.PathWithNamespace)
	c.Field("Status", out.ExportStatus)
	c.Field("Message", out.Message)
	c.Link("API URL", "", out.APIURL)
	c.Link("Web URL", "", out.WebURL)
	c.End(toolutil.HintAction(actionExportDownload, "download the archive once the export status is 'finished'"))
	return toolutil.ToolResultWithMarkdown(b.String())
}

// FormatImportStatusMarkdown renders an import's status as a card, with
// GitLab's own failure text when the import failed.
func FormatImportStatusMarkdown(out ImportStatusOutput) *mcp.CallToolResult {
	if out.ID == 0 {
		return nil
	}
	var b strings.Builder
	c := statusCard(&b, "Import Status", out.Name, out.ID, out.PathWithNamespace)
	c.Field("Status", out.ImportStatus)
	c.Field("Type", out.ImportType)
	c.Code("Correlation ID", out.CorrelationID)
	c.Text("Error", out.ImportError)
	c.End("Monitor import progress by checking status periodically")
	return toolutil.ToolResultWithMarkdown(b.String())
}

// statusCard opens the card both status answers share: the title names which
// of the two it is, and the project's name, id and path identify what the
// status is about.
func statusCard(b *strings.Builder, title, name string, id int64, path string) *toolutil.Card {
	// The title is a literal at both call sites; the project's name and path
	// are what whoever created it chose.
	c := toolutil.NewCard(b, title+": "+name)
	c.Int("ID", id)
	c.Field("Path", path)
	return c
}

// FormatExportDownloadMarkdown renders the one line a downloaded archive
// answers with: the size, and where the bytes are.
func FormatExportDownloadMarkdown(out ExportDownloadOutput) *mcp.CallToolResult {
	if out.SizeBytes == 0 {
		return nil
	}
	return toolutil.ToolResultWithMarkdown(fmt.Sprintf("Export archive downloaded: %d bytes (base64-encoded in content_base64 field)", out.SizeBytes))
}

func init() {
	toolutil.RegisterMarkdownResult(FormatScheduleExportMarkdown)
	toolutil.RegisterMarkdownResult(FormatExportStatusMarkdown)
	toolutil.RegisterMarkdownResult(FormatExportDownloadMarkdown)
	toolutil.RegisterMarkdownResult(FormatImportStatusMarkdown)
}
