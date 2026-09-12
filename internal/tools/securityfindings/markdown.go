package securityfindings

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatListMarkdown renders a page of pipeline security findings as a
// Markdown table: a collection of objects that share columns. The severity is
// the badge every security domain here shares, so a finding and the
// vulnerability it becomes read the same way.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Findings) == 0 {
		return toolutil.EmptyMessage("security findings")
	}
	var b strings.Builder
	// A cursor-paginated connection sends no total, so the heading counts what
	// is shown and says whether more follows, which is all the response knows.
	toolutil.WriteListHeading(&b, "Security Report Findings", len(out.Findings),
		toolutil.PaginationOutput{HasMore: out.Pagination.HasNextPage})
	b.WriteString(toolutil.MarkdownTableHeader("Severity", "Title", "Report Type", "Scanner", "Location", "State"))
	for _, f := range out.Findings {
		scanner := ""
		if f.Scanner != nil {
			scanner = f.Scanner.Name
		}
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.SeverityBadge(f.Severity),
			toolutil.EscapeMdTableCell(f.Title),
			toolutil.EscapeMdTableCell(f.ReportType),
			toolutil.EscapeMdTableCell(scanner),
			toolutil.EscapeMdTableCell(formatLocation(f.Location)),
			toolutil.EscapeMdTableCell(f.State),
		))
	}
	toolutil.WriteGraphQLPagination(&b, out.Pagination, len(out.Findings))
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(actionVulnList, "read the project's vulnerabilities, which is what a confirmed finding becomes"),
		toolutil.HintAction(actionVulnPipelineSummary, "see how many findings each scanner reported in this pipeline"),
	)
	return b.String()
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
