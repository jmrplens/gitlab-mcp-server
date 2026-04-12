// markdown.go provides Markdown formatting for Branch Rules outputs.

package branchrules

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown renders a paginated list of branch rules as Markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("## Branch Rules\n\n")

	if len(out.Rules) == 0 {
		sb.WriteString("No branch rules found.\n")
		return sb.String()
	}

	sb.WriteString("| Name | Default | Protected | Branches | Force Push | CODEOWNERS | Approval Rules | Status Checks |\n")
	sb.WriteString("|------|---------|-----------|----------|------------|------------|----------------|---------------|\n")

	for _, r := range out.Rules {
		forcePush := "-"
		codeOwners := "-"
		if r.BranchProtection != nil {
			forcePush = boolIcon(r.BranchProtection.AllowForcePush)
			codeOwners = boolIcon(r.BranchProtection.CodeOwnerApprovalRequired)
		}

		approvals := formatApprovalRulesSummary(r.ApprovalRules)
		checks := formatStatusChecksSummary(r.ExternalStatusChecks)

		fmt.Fprintf(&sb, "| %s | %s | %s | %d | %s | %s | %s | %s |\n",
			toolutil.EscapeMdTableCell(r.Name),
			boolIcon(r.IsDefault),
			boolIcon(r.IsProtected),
			r.MatchingBranchesCount,
			forcePush,
			codeOwners,
			approvals,
			checks,
		)
	}

	sb.WriteString("\n")

	// Render detailed sections for rules with approval rules or external status checks.
	for _, r := range out.Rules {
		if len(r.ApprovalRules) > 0 {
			fmt.Fprintf(&sb, "### Approval Rules for `%s`\n\n", r.Name)
			sb.WriteString("| Name | Approvals Required | Type |\n")
			sb.WriteString("|------|--------------------|------|\n")
			for _, ar := range r.ApprovalRules {
				fmt.Fprintf(&sb, "| %s | %d | %s |\n",
					toolutil.EscapeMdTableCell(ar.Name),
					ar.ApprovalsRequired,
					toolutil.EscapeMdTableCell(ar.Type),
				)
			}
			sb.WriteString("\n")
		}
		if len(r.ExternalStatusChecks) > 0 {
			fmt.Fprintf(&sb, "### External Status Checks for `%s`\n\n", r.Name)
			sb.WriteString("| Name | URL |\n")
			sb.WriteString("|------|-----|\n")
			for _, esc := range r.ExternalStatusChecks {
				fmt.Fprintf(&sb, "| %s | %s |\n",
					toolutil.EscapeMdTableCell(esc.Name),
					toolutil.EscapeMdTableCell(esc.ExternalURL),
				)
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString(toolutil.FormatGraphQLPagination(out.Pagination, len(out.Rules)))
	sb.WriteString("\n")
	return sb.String()
}

// boolIcon returns "Yes" or "No" for use in Markdown table cells.
func boolIcon(v bool) string {
	if v {
		return "Yes"
	}
	return "No"
}

// formatApprovalRulesSummary returns a Markdown-safe summary of approval rules,
// showing the count and comma-separated names, or "None" if empty.
func formatApprovalRulesSummary(rules []ApprovalRule) string {
	if len(rules) == 0 {
		return "None"
	}
	names := make([]string, 0, len(rules))
	for _, r := range rules {
		names = append(names, r.Name)
	}
	return toolutil.EscapeMdTableCell(fmt.Sprintf("%d (%s)", len(rules), strings.Join(names, ", ")))
}

// formatStatusChecksSummary returns a Markdown-safe summary of external status
// checks, showing the count and comma-separated names, or "None" if empty.
func formatStatusChecksSummary(checks []ExternalStatusCheck) string {
	if len(checks) == 0 {
		return "None"
	}
	names := make([]string, 0, len(checks))
	for _, c := range checks {
		names = append(names, c.Name)
	}
	return toolutil.EscapeMdTableCell(fmt.Sprintf("%d (%s)", len(checks), strings.Join(names, ", ")))
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
