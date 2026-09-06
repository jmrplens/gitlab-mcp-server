package modelregistry

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatDownloadMarkdown formats a downloaded ML model package file as Markdown.
func FormatDownloadMarkdown(o DownloadOutput) string {
	var sb strings.Builder
	// Every one of these is echoed from the caller's own arguments: the
	// project, the model version, the package path and the file name inside it.
	fmt.Fprintf(&sb, "## ML Model Package: %s\n\n", toolutil.EscapeMdHeading(o.Filename))
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Project | %s |\n", toolutil.EscapeMdTableCell(o.ProjectID))
	fmt.Fprintf(&sb, "| Model Version | %s |\n", toolutil.EscapeMdTableCell(o.ModelVersionID))
	fmt.Fprintf(&sb, "| Path | %s |\n", toolutil.EscapeMdTableCell(o.Path))
	fmt.Fprintf(&sb, "| Filename | %s |\n", toolutil.EscapeMdTableCell(o.Filename))
	fmt.Fprintf(&sb, "| Size | %d bytes |\n", o.SizeBytes)
	sb.WriteString("\n_Content is base64-encoded in the structured JSON output._\n")
	toolutil.WriteHints(
		&sb,
		"Use `gitlab_package_list` to browse available model packages",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatDownloadMarkdown) // DownloadOutput
}
