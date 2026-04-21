// markdown.go provides Markdown formatting for security findings outputs.

package securityfindings

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown renders a paginated list of security findings as Markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("## Security Report Findings\n\n")

	if len(out.Findings) == 0 {
		sb.WriteString("No security findings found.\n")
		return sb.String()
	}

	sb.WriteString("| Severity | Confidence | Name | Report Type | Scanner | Location | State |\n")
	sb.WriteString("|----------|------------|------|-------------|---------|----------|-------|\n")

	for _, f := range out.Findings {
		scanner := ""
		if f.Scanner != nil {
			scanner = f.Scanner.Name
		}
		name := f.Name
		if f.Title != "" && f.Title != f.Name {
			name = f.Title
		}
		loc := formatLocation(f.Location)

		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s | %s |\n",
			severityBadge(f.Severity),
			toolutil.EscapeMdTableCell(f.Confidence),
			toolutil.EscapeMdTableCell(name),
			toolutil.EscapeMdTableCell(f.ReportType),
			toolutil.EscapeMdTableCell(scanner),
			toolutil.EscapeMdTableCell(loc),
			toolutil.EscapeMdTableCell(f.State),
		)
	}

	sb.WriteString("\n")
	sb.WriteString(toolutil.FormatGraphQLPagination(out.Pagination, len(out.Findings)))
	sb.WriteString("\n")
	return sb.String()
}

// formatLocation renders a security finding's file location as a
// human-readable string in the form "file:startLine-endLine".
func formatLocation(loc *LocationItem) string {
	if loc == nil {
		return ""
	}
	s := loc.File
	if loc.StartLine > 0 {
		s += fmt.Sprintf(":%d", loc.StartLine)
		if loc.EndLine > 0 && loc.EndLine != loc.StartLine {
			s += fmt.Sprintf("-%d", loc.EndLine)
		}
	}
	return s
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

// severityBadge returns an emoji-prefixed severity label for use in
// Markdown output (e.g., "🔴 CRITICAL", "🟠 HIGH").
func severityBadge(severity string) string {
	switch strings.ToUpper(severity) {
	case "CRITICAL":
		return "🔴 CRITICAL"
	case "HIGH":
		return "🟠 HIGH"
	case "MEDIUM":
		return "🟡 MEDIUM"
	case "LOW":
		return "🔵 LOW"
	case "INFO":
		return "ℹ️ INFO"
	default:
		return severity
	}
}
