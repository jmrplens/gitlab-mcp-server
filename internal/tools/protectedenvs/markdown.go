// markdown.go provides Markdown formatting functions for protected environment MCP tool output.
package protectedenvs

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single protected environment as Markdown.
func FormatOutputMarkdown(pe Output) string {
	if pe.Name == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Protected Environment: %s\n\n", pe.Name)
	fmt.Fprintf(&b, "- **Required Approvals**: %d\n\n", pe.RequiredApprovalCount)

	if len(pe.DeployAccessLevels) > 0 {
		b.WriteString("### Deploy Access Levels\n\n")
		b.WriteString("| ID | Access Level | Description | User ID | Group ID |\n")
		b.WriteString("| --- | --- | --- | --- | --- |\n")
		for _, a := range pe.DeployAccessLevels {
			fmt.Fprintf(&b, "| %d | %d | %s | %d | %d |\n",
				a.ID, a.AccessLevel, toolutil.EscapeMdTableCell(a.AccessLevelDescription), a.UserID, a.GroupID)
		}
		b.WriteString("\n")
	}

	if len(pe.ApprovalRules) > 0 {
		b.WriteString("### Approval Rules\n\n")
		b.WriteString("| ID | Access Level | Description | Required Approvals | User ID | Group ID |\n")
		b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
		for _, r := range pe.ApprovalRules {
			fmt.Fprintf(&b, "| %d | %d | %s | %d | %d | %d |\n",
				r.ID, r.AccessLevel, toolutil.EscapeMdTableCell(r.AccessLevelDescription), r.RequiredApprovalCount, r.UserID, r.GroupID)
		}
		b.WriteString("\n")
	}
	toolutil.WriteHints(&b,
		"Use action 'update' to modify protection rules",
		"Use action 'unprotect' to remove environment protection",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of protected environments as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Environments) == 0 {
		return "No protected environments found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Protected Environments (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Environments), out.Pagination)
	b.WriteString("| Name | Required Approvals | Deploy Access Levels | Approval Rules |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, pe := range out.Environments {
		fmt.Fprintf(&b, "| %s | %d | %d | %d |\n",
			toolutil.EscapeMdTableCell(pe.Name), pe.RequiredApprovalCount,
			len(pe.DeployAccessLevels), len(pe.ApprovalRules))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'get' with environment name for full details",
		"Use action 'protect' to add environment protection",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
