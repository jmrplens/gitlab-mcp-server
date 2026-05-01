package externalstatuschecks

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatMergeCheckMarkdown renders a single merge status check as Markdown.
func FormatMergeCheckMarkdown(out MergeStatusCheckOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## External Status Check: %s\n\n", toolutil.EscapeMdHeading(out.Name))
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&b, "- **External URL**: %s\n", out.ExternalURL)
	fmt.Fprintf(&b, toolutil.FmtMdStatus, out.Status)
	toolutil.WriteHints(&b,
		"Use gitlab_set_project_mr_external_status_check_status to update the status",
		"Use gitlab_retry_failed_external_status_check_for_project_mr to retry a failed check",
	)
	return b.String()
}

// FormatProjectCheckMarkdown renders a single project status check as Markdown.
func FormatProjectCheckMarkdown(out ProjectStatusCheckOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## External Status Check: %s\n\n", toolutil.EscapeMdHeading(out.Name))
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&b, "- **Project ID**: %d\n", out.ProjectID)
	fmt.Fprintf(&b, "- **External URL**: %s\n", out.ExternalURL)
	fmt.Fprintf(&b, "- **HMAC**: %s\n", toolutil.BoolEmoji(out.HMAC))
	if len(out.ProtectedBranches) > 0 {
		fmt.Fprintf(&b, "- **Protected Branches**: %d\n", len(out.ProtectedBranches))
		for _, pb := range out.ProtectedBranches {
			fmt.Fprintf(&b, "  - %s (ID: %d)\n", pb.Name, pb.ID)
		}
	}
	toolutil.WriteHints(&b,
		"Use gitlab_update_project_external_status_check to modify this check",
		"Use gitlab_delete_project_external_status_check to remove this check",
		"Use gitlab_list_project_external_status_checks to see all checks",
	)
	return b.String()
}

// FormatListMergeMarkdown renders a list of merge status checks as a Markdown table.
func FormatListMergeMarkdown(out ListMergeStatusCheckOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Merge Status Checks (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Items), out.Pagination)
	if len(out.Items) == 0 {
		b.WriteString("No merge status checks found.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | External URL | Status |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, c := range out.Items {
		fmt.Fprintf(&b, "| %d | %s | %s | %s |\n",
			c.ID,
			toolutil.EscapeMdTableCell(c.Name),
			toolutil.EscapeMdTableCell(c.ExternalURL),
			toolutil.EscapeMdTableCell(c.Status),
		)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use gitlab_set_project_mr_external_status_check_status to update a check status",
		"Use gitlab_retry_failed_external_status_check_for_project_mr to retry a failed check",
	)
	return b.String()
}

// FormatListProjectMarkdown renders a list of project status checks as a Markdown table.
func FormatListProjectMarkdown(out ListProjectStatusCheckOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Project External Status Checks (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Items), out.Pagination)
	if len(out.Items) == 0 {
		b.WriteString("No project external status checks found.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | External URL | HMAC | Protected Branches |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, c := range out.Items {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %d |\n",
			c.ID,
			toolutil.EscapeMdTableCell(c.Name),
			toolutil.EscapeMdTableCell(c.ExternalURL),
			toolutil.BoolEmoji(c.HMAC),
			len(c.ProtectedBranches),
		)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use gitlab_create_project_external_status_check to add a new check",
		"Use gitlab_update_project_external_status_check to modify a check",
		"Use gitlab_delete_project_external_status_check to remove a check",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMergeCheckMarkdown)
	toolutil.RegisterMarkdown(FormatProjectCheckMarkdown)
	toolutil.RegisterMarkdown(FormatListMergeMarkdown)
	toolutil.RegisterMarkdown(FormatListProjectMarkdown)
}
