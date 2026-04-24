// markdown.go provides Markdown formatting for dependency outputs.

package dependencies

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown renders a paginated list of dependencies as Markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("## Project Dependencies\n\n")
	if len(out.Dependencies) == 0 {
		sb.WriteString("No dependencies found.\n")
		return sb.String()
	}
	sb.WriteString("| Name | Version | Package Manager | Vulns | Licenses |\n")
	sb.WriteString("|------|---------|-----------------|-------|----------|\n")
	for _, d := range out.Dependencies {
		fmt.Fprintf(&sb, "| %s | %s | %s | %d | %d |\n",
			toolutil.EscapeMdTableCell(d.Name),
			toolutil.EscapeMdTableCell(d.Version),
			d.PackageManager,
			len(d.Vulnerabilities),
			len(d.Licenses),
		)
	}
	toolutil.WriteListSummary(&sb, len(out.Dependencies), out.Pagination)
	return sb.String()
}

// FormatExportMarkdown renders a dependency list export status as Markdown.
func FormatExportMarkdown(e ExportOutput) string {
	var sb strings.Builder
	sb.WriteString("## Dependency List Export\n\n")
	sb.WriteString("| Field | Value |\n|-------|-------|\n")
	fmt.Fprintf(&sb, "| ID | %d |\n", e.ID)
	fmt.Fprintf(&sb, "| Finished | %s |\n", toolutil.BoolEmoji(e.HasFinished))
	if e.Self != "" {
		fmt.Fprintf(&sb, "| Self | %s |\n", e.Self)
	}
	if e.Download != "" {
		fmt.Fprintf(&sb, "| Download | %s |\n", e.Download)
	}
	return sb.String()
}

// FormatDownloadMarkdown renders the downloaded SBOM content as Markdown.
func FormatDownloadMarkdown(d DownloadOutput) string {
	var sb strings.Builder
	sb.WriteString("## Dependency List Export (CycloneDX SBOM)\n\n")
	sb.WriteString("```json\n")
	sb.WriteString(d.Content)
	sb.WriteString("\n```\n")
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatExportMarkdown)
	toolutil.RegisterMarkdown(FormatDownloadMarkdown)
}
