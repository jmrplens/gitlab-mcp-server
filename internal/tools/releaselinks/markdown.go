package releaselinks

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single release asset link as Markdown.
func FormatOutputMarkdown(l Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Release Link: %s\n\n", toolutil.EscapeMdHeading(l.Name))
	fmt.Fprintf(&b, toolutil.FmtMdID, l.ID)
	fmt.Fprintf(&b, toolutil.FmtMdURL, l.URL)
	fmt.Fprintf(&b, "- **Type**: %s\n", l.LinkType)
	fmt.Fprintf(&b, "- **External**: %v\n", l.External)
	toolutil.WriteHints(&b,
		"Use action 'link_update' to modify this link",
		"Use action 'link_delete' to remove this link",
	)
	return b.String()
}

// FormatBatchMarkdown renders the result of a batch link creation as Markdown.
func FormatBatchMarkdown(out CreateBatchOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Release Links Created (%d)\n\n", len(out.Created))
	if len(out.Created) > 0 {
		b.WriteString("| ID | Name | Type | URL |\n")
		b.WriteString(toolutil.TblSep4Col)
		for _, l := range out.Created {
			fmt.Fprintf(&b, "| %d | %s | %s | %s |\n", l.ID, toolutil.EscapeMdTableCell(l.Name), toolutil.EscapeMdTableCell(l.LinkType), toolutil.MdTitleLink(l.Name, l.URL))
		}
	}
	if len(out.Failed) > 0 {
		fmt.Fprintf(&b, "\n### Failures (%d)\n\n", len(out.Failed))
		for _, f := range out.Failed {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'link_list' to view all links for this release",
	)
	return b.String()
}

// FormatListMarkdown renders a list of release asset links as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Release Links (%d)\n\n", len(out.Links))
	toolutil.WriteListSummary(&b, len(out.Links), out.Pagination)
	if len(out.Links) == 0 {
		b.WriteString("No release links found.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | Type | URL |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, l := range out.Links {
		fmt.Fprintf(&b, "| %d | %s | %s | %s |\n", l.ID, toolutil.EscapeMdTableCell(l.Name), toolutil.EscapeMdTableCell(l.LinkType), toolutil.MdTitleLink(l.Name, l.URL))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'link_create' to add a new release asset link",
		"Use action 'link_create_batch' to add multiple asset links in one call",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatBatchMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
