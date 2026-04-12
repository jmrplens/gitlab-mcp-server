// markdown.go provides Markdown formatting functions for merge request approval MCP tool output.

package mrapprovals

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatStateMarkdown renders the MR approval state as Markdown.
func FormatStateMarkdown(s StateOutput) string {
	var b strings.Builder
	overwritten := "No"
	if s.ApprovalRulesOverwritten {
		overwritten = "Yes"
	}
	fmt.Fprintf(&b, "## MR Approval State\n\n**Rules overwritten**: %s\n\n", overwritten)
	if len(s.Rules) == 0 {
		b.WriteString("No approval rules configured.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | Type | Required | Approved | Approved By |\n")
	b.WriteString("| -- | ---- | ---- | -------- | -------- | ----------- |\n")
	for _, r := range s.Rules {
		approved := toolutil.BoolEmoji(r.Approved)
		approvedBy := strings.Join(r.ApprovedByNames, ", ")
		fmt.Fprintf(&b, "| %d | %s | %s | %d | %s | %s |\n", r.ID, toolutil.EscapeMdTableCell(r.Name), r.RuleType, r.ApprovalsRequired, approved, toolutil.EscapeMdTableCell(approvedBy))
	}
	toolutil.WriteHints(&b,
		"Use action 'approve' to approve this MR",
		"Use action 'unapprove' to withdraw approval",
	)
	return b.String()
}

// FormatRulesMarkdown renders a list of MR approval rules as Markdown.
func FormatRulesMarkdown(out RulesOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Approval Rules (%d)\n\n", len(out.Rules))
	if len(out.Rules) == 0 {
		b.WriteString("No approval rules configured.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | Type | Required | Approved | Eligible |\n")
	b.WriteString("| -- | ---- | ---- | -------- | -------- | -------- |\n")
	for _, r := range out.Rules {
		approved := toolutil.BoolEmoji(r.Approved)
		eligible := strings.Join(r.EligibleNames, ", ")
		fmt.Fprintf(&b, "| %d | %s | %s | %d | %s | %s |\n", r.ID, toolutil.EscapeMdTableCell(r.Name), r.RuleType, r.ApprovalsRequired, approved, toolutil.EscapeMdTableCell(eligible))
	}
	toolutil.WriteHints(&b,
		"Use action 'approval_rule_create' to add new rules",
		"Use action 'approval_rule_update' or 'approval_rule_delete' to manage existing rules",
	)
	return b.String()
}

// FormatConfigMarkdown renders the MR approval configuration as Markdown.
func FormatConfigMarkdown(c ConfigOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Approval Configuration\n\n")
	fmt.Fprintf(&b, "| Field | Value |\n| ----- | ----- |\n")
	fmt.Fprintf(&b, "| MR | !%d |\n", c.IID)
	fmt.Fprintf(&b, "| State | %s |\n", c.State)
	fmt.Fprintf(&b, "| Approved | %v |\n", c.Approved)
	fmt.Fprintf(&b, "| Approvals Required | %d |\n", c.ApprovalsRequired)
	fmt.Fprintf(&b, "| Approvals Left | %d |\n", c.ApprovalsLeft)
	fmt.Fprintf(&b, "| Has Approval Rules | %v |\n", c.HasApprovalRules)
	fmt.Fprintf(&b, "| User Has Approved | %v |\n", c.UserHasApproved)
	fmt.Fprintf(&b, "| User Can Approve | %v |\n", c.UserCanApprove)
	if len(c.ApprovedBy) > 0 {
		names := make([]string, 0, len(c.ApprovedBy))
		for _, a := range c.ApprovedBy {
			if a.ApprovedAt != "" {
				names = append(names, fmt.Sprintf("%s (%s)", a.Name, a.ApprovedAt))
			} else {
				names = append(names, a.Name)
			}
		}
		fmt.Fprintf(&b, "\n**Approved by**: %s\n", strings.Join(names, ", "))
	}
	if len(c.SuggestedNames) > 0 {
		fmt.Fprintf(&b, "\n**Suggested approvers**: %s\n", strings.Join(c.SuggestedNames, ", "))
	}
	toolutil.WriteHints(&b,
		"Use action 'approve' or 'unapprove' to change approval status",
		"Use action 'approval_rules' to see all configured rules",
	)
	return b.String()
}

// FormatRuleMarkdown renders a single MR approval rule as Markdown.
func FormatRuleMarkdown(r RuleOutput) string {
	var b strings.Builder
	approved := toolutil.BoolEmoji(r.Approved)
	fmt.Fprintf(&b, "## Approval Rule: %s\n\n", r.Name)
	fmt.Fprintf(&b, "| Field | Value |\n| ----- | ----- |\n")
	fmt.Fprintf(&b, "| ID | %d |\n", r.ID)
	fmt.Fprintf(&b, "| Type | %s |\n", r.RuleType)
	fmt.Fprintf(&b, "| Approvals Required | %d |\n", r.ApprovalsRequired)
	fmt.Fprintf(&b, "| Approved | %s |\n", approved)
	if len(r.EligibleNames) > 0 {
		fmt.Fprintf(&b, "| Eligible | %s |\n", strings.Join(r.EligibleNames, ", "))
	}
	if len(r.UserNames) > 0 {
		fmt.Fprintf(&b, "| Users | %s |\n", strings.Join(r.UserNames, ", "))
	}
	if len(r.GroupNames) > 0 {
		fmt.Fprintf(&b, "| Groups | %s |\n", strings.Join(r.GroupNames, ", "))
	}
	toolutil.WriteHints(&b,
		"Use action 'approval_rule_update' to modify this rule",
		"Use action 'approval_rule_delete' to remove this rule",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatStateMarkdown)
	toolutil.RegisterMarkdown(FormatRulesMarkdown)
	toolutil.RegisterMarkdown(FormatConfigMarkdown)
	toolutil.RegisterMarkdown(FormatRuleMarkdown)
}
