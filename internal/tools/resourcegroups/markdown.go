// markdown.go provides Markdown formatting functions for resource group MCP tool output.

package resourcegroups

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown performs the format list markdown operation for the resourcegroups package.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	sb.WriteString("## Resource Groups\n\n")
	if len(out.Groups) == 0 {
		sb.WriteString("No resource groups found.\n")
		return sb.String()
	}
	sb.WriteString("| ID | Key | Process Mode |\n|----|-----|-----------|\n")
	for _, g := range out.Groups {
		fmt.Fprintf(&sb, "| %d | %s | %s |\n", g.ID, toolutil.EscapeMdTableCell(g.Key), g.ProcessMode)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_get_resource_group` to view details or edit process mode")
	return sb.String()
}

// FormatGroupMarkdown performs the format group markdown operation for the resourcegroups package.
func FormatGroupMarkdown(g ResourceGroupItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Resource Group\n\n- **ID**: %d\n- **Key**: %s\n- **Process Mode**: %s\n", g.ID, g.Key, g.ProcessMode)
	toolutil.WriteHints(&b, "Use `gitlab_list_resource_group_jobs` to see upcoming jobs for this group")
	return b.String()
}

// FormatJobsMarkdown performs the format jobs markdown operation for the resourcegroups package.
func FormatJobsMarkdown(out ListUpcomingJobsOutput) string {
	var sb strings.Builder
	sb.WriteString("## Upcoming Jobs\n\n")
	if len(out.Jobs) == 0 {
		sb.WriteString("No upcoming jobs.\n")
		return sb.String()
	}
	sb.WriteString("| ID | Name | Status | Stage |\n|----|------|--------|-------|\n")
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
