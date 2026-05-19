package resourcegroups

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatListMarkdown renders resource groups as a compact Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	sb.WriteString("## Resource Groups\n\n")
	if len(out.Groups) == 0 {
		sb.WriteString("No resource groups found.\n")
		return sb.String()
	}
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Key", "Process Mode"))
	for _, g := range out.Groups {
		fmt.Fprintf(&sb, "| %d | %s | %s |\n", g.ID, toolutil.EscapeMdTableCell(g.Key), g.ProcessMode)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_get_resource_group` to view details or edit process mode")
	return sb.String()
}

// FormatGroupMarkdown renders a single resource group summary.
func FormatGroupMarkdown(g ResourceGroupItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Resource Group\n\n- **ID**: %d\n- **Key**: %s\n- **Process Mode**: %s\n", g.ID, g.Key, g.ProcessMode)
	toolutil.WriteHints(&b, "Use `gitlab_list_resource_group_jobs` to see upcoming jobs for this group")
	return b.String()
}

// FormatJobsMarkdown renders upcoming resource-group jobs as a Markdown table.
func FormatJobsMarkdown(out ListUpcomingJobsOutput) string {
	var sb strings.Builder
	sb.WriteString("## Upcoming Jobs\n\n")
	if len(out.Jobs) == 0 {
		sb.WriteString("No upcoming jobs.\n")
		return sb.String()
	}
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Status", "Stage"))
	for _, j := range out.Jobs {
		fmt.Fprintf(&sb, "| %d | %s | %s | %s |\n", j.ID, toolutil.EscapeMdTableCell(j.Name), j.Status, j.Stage)
	}
	toolutil.WriteHints(&sb, "Use job tools to view logs or retry specific jobs")
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGroupMarkdown)
	toolutil.RegisterMarkdown(FormatJobsMarkdown)
}
