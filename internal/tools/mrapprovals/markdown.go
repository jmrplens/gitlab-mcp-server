package mrapprovals

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// actionMRMerge is the one canonical action ID these hints name that the
// action specs beside them do not already declare.
const actionMRMerge = "merge_request.merge"

// userCell renders one approver: the "@handle" linked to the profile, and the
// display name when GitLab sent no username. Both halves are escaped, which the
// comma-joined list of raw names this replaced was not.
func userCell(u *BasicUserOutput) string {
	if u == nil {
		return ""
	}
	if handle := toolutil.MdUserLink(u.Username, u.WebURL); handle != "" {
		return handle
	}
	return toolutil.MdTitleLink(u.Name, u.WebURL)
}

// userList renders a set of approvers as the comma-joined list a row shows, and
// nothing at all when there are none.
func userList(users []*BasicUserOutput) string {
	cells := make([]string, 0, len(users))
	for _, u := range users {
		if cell := userCell(u); cell != "" {
			cells = append(cells, cell)
		}
	}
	return strings.Join(cells, ", ")
}

// groupPaths renders approval groups by their full path, each escaped.
func groupPaths(groups []*GroupOutput) string {
	paths := make([]string, 0, len(groups))
	for _, g := range groups {
		if g == nil {
			continue
		}
		path := g.FullPath
		if path == "" {
			path = g.Name
		}
		if path != "" {
			paths = append(paths, toolutil.EscapeMdTableCell(path))
		}
	}
	return strings.Join(paths, ", ")
}

// approverList renders the users who approved, each with the moment they
// approved in the display form every other timestamp takes. The names used to
// be joined raw, and the timestamp printed as GitLab sent it.
func approverList(users []*MergeRequestApproverUserOutput) string {
	cells := make([]string, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		cell := userCell(u.User)
		if cell == "" {
			continue
		}
		if at := toolutil.FormatTime(u.ApprovedAt); at != "" {
			cell += " (" + at + ")"
		}
		cells = append(cells, cell)
	}
	return strings.Join(cells, ", ")
}

// FormatStateMarkdown renders the approval state of a merge request: the card
// of one object, with the rules it is judged by as a nested collection.
func FormatStateMarkdown(s StateOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "MR Approval State")
	c.Bool("Rules overwritten", s.ApprovalRulesOverwritten)
	c.Int("Rules", int64(len(s.Rules)))
	if len(s.Rules) == 0 {
		c.Note("No approval rules are configured for this merge request.")
		c.End(
			toolutil.HintAction(actionApprovalRules, "list the rules configured on this merge request"),
			toolutil.HintAction(actionApprovalRuleCreate, "add an approval rule"),
		)
		return b.String()
	}
	t := c.Table("Rules", "ID", "Name", "Type", "Required", "Approved", "Approved By")
	for _, r := range s.Rules {
		t.Row(
			strconv.FormatInt(r.ID, 10),
			toolutil.EscapeMdTableCell(r.Name),
			toolutil.EscapeMdTableCell(r.RuleType),
			strconv.Itoa(r.ApprovalsRequired),
			toolutil.BoolEmoji(r.Approved),
			userList(r.ApprovedBy),
		)
	}
	c.End(
		toolutil.HintAction(actionMRApprove, "approve this merge request"),
		toolutil.HintAction(actionMRUnapprove, "withdraw an approval"),
	)
	return b.String()
}

// anyProfileLink reports whether any eligible approver carries a profile URL,
// which decides whether the table has a link to preserve. The instruction to
// keep the links of a table that has none is noise a model has to read past,
// so it is asked rather than assumed: an instance that hides profile URLs
// renders these rules as plain names.
func anyProfileLink(rules []RuleOutput) bool {
	for _, r := range rules {
		for _, u := range r.EligibleApprovers {
			if u != nil && u.WebURL != "" {
				return true
			}
		}
	}
	return false
}

// FormatRulesMarkdown renders the approval rules of a merge request as a
// Markdown table: a collection of objects that share columns.
//
// There is no Approved column here: the three approval_rules routes present
// MergeRequestApprovalRule, which does not expose it. Only the approval_state
// route does, and [FormatStateMarkdown] renders that one.
func FormatRulesMarkdown(out RulesOutput) string {
	if len(out.Rules) == 0 {
		return toolutil.EmptyMessage("approval rules")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "MR Approval Rules", len(out.Rules), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Type", "Required", "Eligible"))
	for _, r := range out.Rules {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(r.ID, 10),
			toolutil.EscapeMdTableCell(r.Name),
			toolutil.EscapeMdTableCell(r.RuleType),
			strconv.Itoa(r.ApprovalsRequired),
			userList(r.EligibleApprovers),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, anyProfileLink(out.Rules),
		toolutil.HintAction(actionApprovalRuleCreate, "add a rule"),
		toolutil.HintAction(actionApprovalRuleUpdate, "change an existing rule"),
		toolutil.HintAction(actionApprovalRuleDelete, "remove a rule"),
	)
	return b.String()
}

// FormatConfigMarkdown renders a merge request's approvals as the card of one
// object.
//
// The rows it used to print for approvals required, approvals left and whether
// rules exist are gone with the fields behind them: GitLab answers none of them
// at this endpoint, so every one of those rows printed a zero. What answers
// those questions is action 'approval_state', which the hints point at.
func FormatConfigMarkdown(c ConfigOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "MR Approvals")
	card.Bool("Approved", c.Approved)
	card.Bool("You have approved", c.UserHasApproved)
	card.Bool("You can approve", c.UserCanApprove)
	card.Markdown("Approved By", approverList(c.ApprovedBy))
	card.End(
		toolutil.HintAction(actionMRApprove, "approve this merge request"),
		toolutil.HintAction(actionMRUnapprove, "withdraw your approval"),
		toolutil.HintAction(actionApprovalState, "see how many approvals are required and left"),
		toolutil.HintAction(actionApprovalRules, "see every configured rule"),
	)
	return b.String()
}

// FormatRuleMarkdown renders one approval rule as the card of one object.
func FormatRuleMarkdown(r RuleOutput) string {
	var b strings.Builder
	// An approval rule's name is free text a maintainer types.
	c := toolutil.NewCard(&b, "Approval Rule: "+r.Name)
	c.Int("ID", r.ID)
	c.Field("Type", r.RuleType)
	c.Field("Report Type", r.ReportType)
	c.Field("Section", r.Section)
	c.Int("Approvals Required", int64(r.ApprovalsRequired))
	c.Bool("Overridden", r.Overridden)
	c.Warn("Contains groups you cannot see", r.ContainsHiddenGroups)
	c.Markdown("Eligible", userList(r.EligibleApprovers))
	c.Markdown("Users", userList(r.Users))
	c.Markdown("Groups", groupPaths(r.Groups))
	c.End(
		toolutil.HintAction(actionApprovalRuleUpdate, "modify this rule"),
		toolutil.HintAction(actionApprovalRuleDelete, "remove this rule"),
		toolutil.HintAction(actionMRMerge, "merge once the rule is satisfied"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatStateMarkdown)
	toolutil.RegisterMarkdown(FormatRulesMarkdown)
	toolutil.RegisterMarkdown(FormatConfigMarkdown)
	toolutil.RegisterMarkdown(FormatRuleMarkdown)
}
