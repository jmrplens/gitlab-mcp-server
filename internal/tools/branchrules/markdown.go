package branchrules

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionBranchGetProtected = "branch.get_protected"
	actionBranchProtect      = "branch.protect"
)

// absentCell is what a cell shows for a value the response does not carry: a
// rule with no protection, or a setting the Community document never asks
// about.
const absentCell = "-"

// FormatListMarkdown renders a page of branch rules as a Markdown table,
// followed by one section per rule that carries grants, settings, approval
// rules or external status checks.
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
		"Name", "Default", "Protected", "Branches", "Push", "Merge", "Force Push", "CODEOWNERS", "Approval Rules", "Status Checks",
	))
	for _, r := range out.Rules {
		push, merge, forcePush, codeOwners := absentCell, absentCell, absentCell, absentCell
		if p := r.BranchProtection; p != nil {
			push = formatAccessSummary(pushAccessGrants(p.PushAccessLevels))
			merge = formatAccessSummary(p.MergeAccessLevels)
			forcePush = toolutil.BoolEmoji(p.AllowForcePush)
			codeOwners = boolPtrCell(p.CodeOwnerApprovalRequired)
		}
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(r.Name),
			toolutil.BoolEmoji(r.IsDefault),
			toolutil.BoolEmoji(r.IsProtected),
			strconv.Itoa(r.MatchingBranchesCount),
			push,
			merge,
			forcePush,
			codeOwners,
			formatApprovalRulesSummary(r.ApprovalRules),
			formatStatusChecksSummary(r.ExternalStatusChecks),
		))
	}
	for _, r := range out.Rules {
		writeProtectionSection(&b, r)
		writeApprovalRuleSection(&b, r)
		writeStatusCheckSection(&b, r)
	}
	toolutil.WriteGraphQLPagination(&b, toolutil.GraphQLPaginationOutput{
		HasNextPage: out.Pagination.HasNextPage,
		EndCursor:   out.Pagination.EndCursor,
	}, len(out.Rules))
	// The hints are written once, after the tables, rather than opening the
	// response above its own table header, and they ask for the links to be
	// kept only when a section below the table rendered one.
	hints := []string{
		toolutil.HintAction(actionBranchGetProtected, "see one rule's protection settings in full"),
		toolutil.HintAction(actionBranchProtect, "change what a branch pattern requires"),
	}
	if slices.ContainsFunc(out.Rules, rendersLink) {
		hints = toolutil.ListHints(hints...)
	}
	toolutil.WriteHints(&b, hints...)
	return b.String()
}

// rendersLink reports whether the sections under one rule link anything: a
// user, a group, an eligible approver or a status check's URL.
func rendersLink(r BranchRuleItem) bool {
	if len(r.ExternalStatusChecks) > 0 {
		return true
	}
	for _, rule := range r.ApprovalRules {
		if len(rule.EligibleApprovers) > 0 {
			return true
		}
	}
	for _, g := range ruleGrants(r.BranchProtection) {
		if g.access.User != nil || g.access.Group != nil || g.key != nil {
			return true
		}
	}
	return false
}

// setting is one row of a rule's settings table: a label and its rendered
// value.
type setting struct {
	label string
	value string
}

// ruleSettings lists the settings of one rule that say something: the squash
// option when GitLab sent one, and each flag that is on. A flag that is off
// is the default every rule starts from, so a row for it would only repeat
// the absence of a restriction.
func ruleSettings(r BranchRuleItem) []setting {
	var settings []setting
	if r.SquashOption != nil {
		settings = append(settings, setting{"Squash option", toolutil.EscapeMdTableCell(r.SquashOption.Option)})
	}
	on := toolutil.BoolEmoji(true)
	if r.IsGroupLevel != nil && *r.IsGroupLevel {
		settings = append(settings, setting{"Created at the group level", on})
	}
	p := r.BranchProtection
	if p == nil {
		return settings
	}
	for _, flag := range []struct {
		label string
		value *bool
	}{
		{"Protection created at the group level", p.IsGroupLevel},
		{"Modification blocked by a security policy", p.ModificationBlockedByPolicy},
		{"Push blocked by a security policy", p.ProtectedFromPushBySecurityPolicy},
		{"Modification would be blocked by a warn-mode policy", p.WarnModificationBlockedByPolicy},
		{"Push would be blocked by a warn-mode policy", p.WarnProtectedFromPushBySecurityPolicy},
	} {
		if flag.value != nil && *flag.value {
			settings = append(settings, setting{flag.label, on})
		}
	}
	return settings
}

// grant is one row of a rule's grants table: what it allows and the grant.
type grant struct {
	action string
	access Access
	key    *AccessDeployKey
}

// ruleGrants lists every grant of one rule in the order a reader asks about
// them: push, then merge, then unprotect.
func ruleGrants(p *BranchProtection) []grant {
	if p == nil {
		return nil
	}
	var grants []grant
	for _, a := range p.PushAccessLevels {
		grants = append(grants, grant{"Push", a.Access, a.DeployKey})
	}
	for _, a := range p.MergeAccessLevels {
		grants = append(grants, grant{"Merge", a, nil})
	}
	for _, a := range p.UnprotectAccessLevels {
		grants = append(grants, grant{"Unprotect", a, nil})
	}
	return grants
}

// writeProtectionSection writes who may push, merge and unprotect under one
// rule, and the settings of it that are on, and nothing when the rule has
// neither.
func writeProtectionSection(b *strings.Builder, r BranchRuleItem) {
	settings := ruleSettings(r)
	grants := ruleGrants(r.BranchProtection)
	if len(settings) == 0 && len(grants) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### Protection for %s\n\n", toolutil.EscapeMdHeading(r.Name))
	if len(settings) > 0 {
		b.WriteString(toolutil.MarkdownTableHeader("Setting", "Value"))
		for _, s := range settings {
			b.WriteString(toolutil.MarkdownTableRow(s.label, s.value))
		}
	}
	if len(grants) == 0 {
		return
	}
	if len(settings) > 0 {
		b.WriteString("\n")
	}
	b.WriteString(toolutil.MarkdownTableHeader("Grant", "Allowed", "Detail"))
	for _, g := range grants {
		b.WriteString(toolutil.MarkdownTableRow(
			g.action,
			toolutil.EscapeMdTableCell(g.access.AccessLevelDescription),
			grantDetail(g),
		))
	}
}

// grantDetail names who or what a grant is for beyond its role: the user or
// group linked to its page, the deploy key, or the custom role. A grant of a
// role alone has no detail.
func grantDetail(g grant) string {
	var details []string
	if u := g.access.User; u != nil {
		details = append(details, "user "+userCell(*u))
	}
	if grp := g.access.Group; grp != nil {
		details = append(details, "group "+toolutil.MdTitleLink(grp.Name, grp.WebURL))
	}
	if role := g.access.MemberRole; role != nil {
		details = append(details, "custom role "+toolutil.EscapeMdTableCell(role.Name))
	}
	if g.key != nil {
		details = append(details, deployKeyCell(*g.key))
	}
	if len(details) == 0 {
		return absentCell
	}
	return strings.Join(details, "; ")
}

// deployKeyCell names a deploy key, the user it is assigned to and, when it
// has one, the date it expires.
func deployKeyCell(key AccessDeployKey) string {
	cell := "deploy key " + toolutil.EscapeMdTableCell(key.Title) + " of " + userCell(key.User)
	if key.ExpiresAt != "" {
		cell += ", expires " + toolutil.EscapeMdTableCell(key.ExpiresAt)
	}
	return cell
}

// userCell links a user reference by username to its profile.
func userCell(u UserRef) string {
	return toolutil.MdTitleLink("@"+u.Username, u.WebURL)
}

// boolPtrCell renders a flag the Community document never asks about: the
// dash for one GitLab was not asked, the flag otherwise.
func boolPtrCell(v *bool) string {
	if v == nil {
		return absentCell
	}
	return toolutil.BoolEmoji(*v)
}

// pushAccessGrants reads the grants of a list of push grants, so the table
// summarizes them the way it summarizes every other kind.
func pushAccessGrants(levels []PushAccess) []Access {
	grants := make([]Access, 0, len(levels))
	for _, level := range levels {
		grants = append(grants, level.Access)
	}
	return grants
}

// formatAccessSummary returns the Markdown-safe list of who a kind of grant
// allows, in GitLab's own words, or the dash when GitLab sent none.
func formatAccessSummary(grants []Access) string {
	if len(grants) == 0 {
		return absentCell
	}
	descriptions := make([]string, 0, len(grants))
	for _, g := range grants {
		descriptions = append(descriptions, g.AccessLevelDescription)
	}
	return toolutil.EscapeMdTableCell(strings.Join(descriptions, ", "))
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
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Approvals Required", "Type", "Coverage Threshold", "Eligible Approvers"))
	for _, ar := range r.ApprovalRules {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(ar.Name),
			strconv.Itoa(ar.ApprovalsRequired),
			toolutil.EscapeMdTableCell(ar.Type),
			coverageCell(ar.CoverageMinimumThreshold),
			approversCell(ar.EligibleApprovers),
		))
	}
}

// coverageCell renders the coverage an approval rule requires, a percentage
// GitLab sends only for a coverage-check rule.
func coverageCell(threshold *float64) string {
	if threshold == nil {
		return absentCell
	}
	return strconv.FormatFloat(*threshold, 'f', -1, 64) + "%"
}

// approversCell lists the users an approval rule allows to approve, each
// linked to its profile.
func approversCell(approvers []UserRef) string {
	if len(approvers) == 0 {
		return absentCell
	}
	cells := make([]string, 0, len(approvers))
	for _, u := range approvers {
		cells = append(cells, userCell(u))
	}
	return strings.Join(cells, ", ")
}

// writeStatusCheckSection writes the external status checks of one branch rule
// as a table under its own heading, and nothing when the rule has none.
func writeStatusCheckSection(b *strings.Builder, r BranchRuleItem) {
	if len(r.ExternalStatusChecks) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### External Status Checks for %s\n\n", toolutil.EscapeMdHeading(r.Name))
	b.WriteString(toolutil.MarkdownTableHeader("Name", "URL", "HMAC"))
	for _, esc := range r.ExternalStatusChecks {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(esc.Name),
			toolutil.MdTitleLink(esc.ExternalURL, esc.ExternalURL),
			toolutil.BoolEmoji(esc.HMAC),
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
