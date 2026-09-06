package mrapprovals

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// userNames returns the display names of a slice of basic-user outputs,
// skipping nil entries. Used by the Markdown formatters to render the
// approver/eligible/user object lists as comma-separated names.
func userNames(users []*BasicUserOutput) []string {
	names := make([]string, 0, len(users))
	for _, u := range users {
		if u != nil {
			names = append(names, u.Name)
		}
	}
	return names
}

// groupNames returns the display names of a slice of group outputs, skipping
// nil entries.
func groupNames(groups []*GroupOutput) []string {
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		if g != nil {
			names = append(names, g.Name)
		}
	}
	return names
}

// approverNames returns the display names of merge-request approver users,
// appending the approval timestamp in parentheses when present. Skips entries
// with a nil user object.
func approverNames(users []*MergeRequestApproverUserOutput) []string {
	names := make([]string, 0, len(users))
	for _, u := range users {
		if u == nil || u.User == nil {
			continue
		}
		if u.ApprovedAt != "" {
			names = append(names, fmt.Sprintf("%s (%s)", u.User.Name, u.ApprovedAt))
		} else {
			names = append(names, u.User.Name)
		}
	}
	return names
}

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
		approvedBy := strings.Join(userNames(r.ApprovedBy), ", ")
		//gitlab:allow-unescaped r.RuleType: an approval rule type GitLab picks from a fixed set (regular, any_approver, code_owner, report_approver).
		fmt.Fprintf(&b, "| %d | %s | %s | %d | %s | %s |\n", r.ID, toolutil.EscapeMdTableCell(r.Name), r.RuleType, r.ApprovalsRequired, approved, toolutil.EscapeMdTableCell(approvedBy))
	}
	toolutil.WriteHints(
		&b,
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
		eligible := strings.Join(userNames(r.EligibleApprovers), ", ")
		fmt.Fprintf(&b, "| %d | %s | %s | %d | %s | %s |\n", r.ID, toolutil.EscapeMdTableCell(r.Name), r.RuleType, r.ApprovalsRequired, approved, toolutil.EscapeMdTableCell(eligible))
	}
	toolutil.WriteHints(
		&b,
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
	//gitlab:allow-unescaped c.State: a merge request state, one of GitLab's fixed set (opened, closed, locked, merged).
	fmt.Fprintf(&b, "| State | %s |\n", c.State)
	fmt.Fprintf(&b, "| Approved | %v |\n", c.Approved)
	fmt.Fprintf(&b, "| Approvals Required | %d |\n", c.ApprovalsRequired)
	fmt.Fprintf(&b, "| Approvals Left | %d |\n", c.ApprovalsLeft)
	fmt.Fprintf(&b, "| Has Approval Rules | %v |\n", c.HasApprovalRules)
	fmt.Fprintf(&b, "| User Has Approved | %v |\n", c.UserHasApproved)
	fmt.Fprintf(&b, "| User Can Approve | %v |\n", c.UserCanApprove)
	if names := approverNames(c.ApprovedBy); len(names) > 0 {
		fmt.Fprintf(&b, "\n**Approved by**: %s\n", strings.Join(names, ", "))
	}
	if names := userNames(c.SuggestedApprovers); len(names) > 0 {
		fmt.Fprintf(&b, "\n**Suggested approvers**: %s\n", strings.Join(names, ", "))
	}
	toolutil.WriteHints(
		&b,
		"Use action 'approve' or 'unapprove' to change approval status",
		"Use action 'approval_rules' to see all configured rules",
	)
	return b.String()
}

// FormatRuleMarkdown renders a single MR approval rule as Markdown.
func FormatRuleMarkdown(r RuleOutput) string {
	var b strings.Builder
	approved := toolutil.BoolEmoji(r.Approved)
	// An approval rule's name is free text a maintainer types.
	fmt.Fprintf(&b, "## Approval Rule: %s\n\n", toolutil.EscapeMdHeading(r.Name))
	fmt.Fprintf(&b, "| Field | Value |\n| ----- | ----- |\n")
	fmt.Fprintf(&b, "| ID | %d |\n", r.ID)
	fmt.Fprintf(&b, "| Type | %s |\n", r.RuleType)
	fmt.Fprintf(&b, "| Approvals Required | %d |\n", r.ApprovalsRequired)
	fmt.Fprintf(&b, "| Approved | %s |\n", approved)
	if eligible := userNames(r.EligibleApprovers); len(eligible) > 0 {
		fmt.Fprintf(&b, "| Eligible | %s |\n", strings.Join(eligible, ", "))
	}
	if users := userNames(r.Users); len(users) > 0 {
		fmt.Fprintf(&b, "| Users | %s |\n", strings.Join(users, ", "))
	}
	if groups := groupNames(r.Groups); len(groups) > 0 {
		fmt.Fprintf(&b, "| Groups | %s |\n", strings.Join(groups, ", "))
	}
	toolutil.WriteHints(
		&b,
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
