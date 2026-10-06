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
	toolutil.WriteListHeading(&b, "MR Approval Rules", len(out.Rules), out.Pagination)
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
	toolutil.WriteListFooter(&b, out.Pagination, anyProfileLink(out.Rules),
		toolutil.HintAction(actionApprovalRuleCreate, "add a rule"),
		toolutil.HintAction(actionApprovalRuleUpdate, "change an existing rule"),
		toolutil.HintAction(actionApprovalRuleDelete, "remove a rule"),
	)
	return b.String()
}

// userBasicCell renders one whole user the way [userCell] renders the subset:
// the "@handle" linked to the profile, or the display name when GitLab sent
// no username.
func userBasicCell(u toolutil.UserBasicOutput) string {
	if handle := toolutil.MdUserLink(u.Username, u.WebURL); handle != "" {
		return handle
	}
	return toolutil.MdTitleLink(u.Name, u.WebURL)
}

// userBasicList renders a set of whole users as the comma-joined list a row
// shows, and nothing at all when there are none.
func userBasicList(users []toolutil.UserBasicOutput) string {
	cells := make([]string, 0, len(users))
	for _, u := range users {
		if cell := userBasicCell(u); cell != "" {
			cells = append(cells, cell)
		}
	}
	return strings.Join(cells, ", ")
}

// ruleNames renders the rules a short reference names, each by its name
// escaped, or by its ID when it has none.
func ruleNames(rules []ApprovalRuleShortOutput) string {
	names := make([]string, 0, len(rules))
	for _, r := range rules {
		if r.Name != "" {
			names = append(names, toolutil.EscapeMdTableCell(r.Name))
			continue
		}
		names = append(names, "#"+strconv.FormatInt(r.ID, 10))
	}
	return strings.Join(names, ", ")
}

// countRow writes a count an Enterprise Edition answer sends, zero included,
// and nothing when GitLab did not send it.
func countRow(card *toolutil.Card, label string, v *int64) {
	if v == nil {
		return
	}
	card.Int(label, *v)
}

// textRow writes a value an Enterprise Edition answer sends, and nothing when
// GitLab did not send it or sent it blank.
func textRow(card *toolutil.Card, label string, v *string) {
	if v == nil {
		return
	}
	card.Field(label, *v)
}

// WriteEnterpriseRows writes onto card the rows of the approval state an
// Enterprise Edition instance answers with, each only when GitLab sent its
// key, so a Community Edition answer adds none. The deprecated approvers and
// approver groups are left to the structured result, since they repeat the
// first rule's users and groups.
func WriteEnterpriseRows(card *toolutil.Card, s EnterpriseApprovalState) {
	textRow(card, "Title", s.Title)
	textRow(card, "State", s.State)
	countRow(card, "Approvals Required", s.ApprovalsRequired)
	countRow(card, "Approvals Left", s.ApprovalsLeft)
	card.Markdown("Rules Left", ruleNames(s.ApprovalRulesLeft))
	card.Markdown("Rules Nobody Can Satisfy", ruleNames(s.InvalidApproversRules))
	card.BoolPtr("Has Approval Rules", s.HasApprovalRules)
	card.BoolPtr("Approval Rules Available", s.MergeRequestApproversAvailable)
	card.BoolPtr("Password Required To Approve", s.RequirePasswordToApprove)
	card.Markdown("Suggested Approvers", userBasicList(s.SuggestedApprovers))
}

// FormatConfigMarkdown renders a merge request's approvals as the card of one
// object: the four rows every edition answers with, then the approval state
// an Enterprise Edition instance adds, row by row as GitLab sent it.
func FormatConfigMarkdown(c ConfigOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "MR Approvals")
	card.Bool("Approved", c.Approved)
	card.Bool("You have approved", c.UserHasApproved)
	card.Bool("You can approve", c.UserCanApprove)
	card.Markdown("Approved By", approverList(c.ApprovedBy))
	WriteEnterpriseRows(card, c.EnterpriseApprovalState)
	card.End(
		toolutil.HintAction(actionMRApprove, "approve this merge request"),
		toolutil.HintAction(actionMRUnapprove, "withdraw your approval"),
		toolutil.HintAction(actionApprovalState, "see each approval rule and whether it is satisfied"),
		toolutil.HintAction(actionApprovalRules, "list the configured rules"),
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
