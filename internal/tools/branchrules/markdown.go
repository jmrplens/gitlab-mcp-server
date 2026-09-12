package branchrules

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionBranchGetProtected = "branch.get_protected"
	actionBranchProtect      = "branch.protect"
)

// FormatListMarkdown renders a page of branch rules as a Markdown table,
// followed by one section per rule that carries approval rules or external
// status checks.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Rules) == 0 {
		return toolutil.EmptyMessage("branch rules")
	}
	var b strings.Builder
	// A cursor-paginated connection sends no total, so the heading counts what
	// is shown and says whether more follows, which is all the response knows.
	toolutil.WriteListHeading(&b, "Branch Rules", len(out.Rules),
		toolutil.PaginationOutput{HasMore: out.Pagination.HasNextPage})
	b.WriteString(toolutil.MarkdownTableHeader(
		"Name", "Default", "Protected", "Branches", "Force Push", "CODEOWNERS", "Approval Rules", "Status Checks",
	))
	for _, r := range out.Rules {
		forcePush := "-"
		codeOwners := "-"
		if r.BranchProtection != nil {
			forcePush = toolutil.BoolEmoji(r.BranchProtection.AllowForcePush)
			codeOwners = toolutil.BoolEmoji(r.BranchProtection.CodeOwnerApprovalRequired)
		}
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(r.Name),
			toolutil.BoolEmoji(r.IsDefault),
			toolutil.BoolEmoji(r.IsProtected),
			strconv.Itoa(r.MatchingBranchesCount),
			forcePush,
			codeOwners,
			formatApprovalRulesSummary(r.ApprovalRules),
			formatStatusChecksSummary(r.ExternalStatusChecks),
		))
	}
	for _, r := range out.Rules {
		writeApprovalRuleSection(&b, r)
		writeStatusCheckSection(&b, r)
	}
	toolutil.WriteGraphQLPagination(&b, toolutil.GraphQLPaginationOutput{
		HasNextPage: out.Pagination.HasNextPage,
		EndCursor:   out.Pagination.EndCursor,
	}, len(out.Rules))
	// The table carries no link, so the hints carry no instruction to preserve
	// one, and they are written once, after the tables, rather than opening the
	// response above its own table header.
	toolutil.WriteHints(&b,
		toolutil.HintAction(actionBranchGetProtected, "see one rule's protection settings in full"),
		toolutil.HintAction(actionBranchProtect, "change what a branch pattern requires"),
	)
	return b.String()
}

// writeApprovalRuleSection writes the approval rules of one branch rule as a
// table under its own heading, and nothing when the rule has none. The rule's
// name is the heading's subject rather than a hand-written code span, since a
// backtick in the name would end the span and the rest would render as
// Markdown.
func writeApprovalRuleSection(b *strings.Builder, r BranchRuleItem) {
	if len(r.ApprovalRules) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### Approval Rules for %s\n\n", toolutil.EscapeMdHeading(r.Name))
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Approvals Required", "Type"))
	for _, ar := range r.ApprovalRules {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(ar.Name),
			strconv.Itoa(ar.ApprovalsRequired),
			toolutil.EscapeMdTableCell(ar.Type),
		))
	}
}

// writeStatusCheckSection writes the external status checks of one branch rule
// as a table under its own heading, and nothing when the rule has none.
func writeStatusCheckSection(b *strings.Builder, r BranchRuleItem) {
	if len(r.ExternalStatusChecks) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### External Status Checks for %s\n\n", toolutil.EscapeMdHeading(r.Name))
	b.WriteString(toolutil.MarkdownTableHeader("Name", "URL"))
	for _, esc := range r.ExternalStatusChecks {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(esc.Name),
			toolutil.MdTitleLink(esc.ExternalURL, esc.ExternalURL),
		))
	}
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
