package groupprotectedenvs

import (
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionGet       = "group.protected_env_get"
	actionList      = "group.protected_env_list"
	actionProtect   = "group.protected_env_protect"
	actionUnprotect = "group.protected_env_unprotect"
	actionUpdate    = "group.protected_env_update"
)

// FormatOutputMarkdown renders one group-level protected environment as the
// card of a single object: the approvals it needs, then the deploy access
// levels and the approval rules as nested collections.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Protected Environment: "+out.Name)
	// A tier with approval rules of its own counts its approvals per rule, so
	// the headline number GitLab sends beside them is not what gates a
	// deployment.
	if len(out.ApprovalRules) > 0 {
		c.Field("Required Approvals", "per approval rule (see below)")
	} else {
		c.Int("Required Approvals", out.RequiredApprovalCount)
	}
	if len(out.DeployAccessLevels) > 0 {
		t := c.Table("Deploy Access Levels", "ID", "Level", "Grantee", "Description", "Inheritance")
		for _, l := range out.DeployAccessLevels {
			t.Row(
				strconv.FormatInt(l.ID, 10),
				levelCell(l.AccessLevel, l.UserID, l.GroupID),
				granteeCell(l.UserID, l.GroupID),
				// GitLab's access-level description is a role name for a plain
				// rule but a user's display name or a group's name for a
				// granular one.
				toolutil.EscapeMdTableCell(l.AccessLevelDescription),
				inheritanceCell(l.GroupID, l.GroupInheritanceType),
			)
		}
	}
	if len(out.ApprovalRules) > 0 {
		t := c.Table("Approval Rules", "ID", "Level", "Grantee", "Description", "Required", "Inheritance")
		for _, r := range out.ApprovalRules {
			t.Row(
				strconv.FormatInt(r.ID, 10),
				levelCell(r.AccessLevel, r.UserID, r.GroupID),
				granteeCell(r.UserID, r.GroupID),
				toolutil.EscapeMdTableCell(r.AccessLevelDescription),
				strconv.FormatInt(r.RequiredApprovalCount, 10),
				inheritanceCell(r.GroupID, r.GroupInheritanceType),
			)
		}
	}
	c.End(
		toolutil.HintAction(actionUpdate, "change the deploy access levels or approval rules"),
		toolutil.HintAction(actionUnprotect, "remove this protection from the group"),
	)
	return b.String()
}

// levelCell renders the role a rule grants, and "-" for a rule that names a
// user or a group instead, whose access_level is not what decides.
func levelCell(accessLevel int, userID, groupID int64) string {
	if granteeCell(userID, groupID) != "" {
		return "-"
	}
	return toolutil.AccessLevelDescription(gl.AccessLevelValue(accessLevel))
}

// granteeCell names who a granular rule is about, and nothing for a rule that
// grants a role to everyone who holds it. GitLab sends zero for the id it is
// not about, which used to be rendered as a numeric 0 in a column of its own.
func granteeCell(userID, groupID int64) string {
	switch {
	case userID != 0:
		return fmt.Sprintf("user #%d", userID)
	case groupID != 0:
		return fmt.Sprintf("group #%d", groupID)
	default:
		return ""
	}
}

// inheritanceCell says whether a group rule reaches the group's inherited
// members or only its direct ones, and nothing at all for a rule that is not
// about a group, where GitLab's zero means "not applicable" rather than
// "direct".
func inheritanceCell(groupID, inheritanceType int64) string {
	if groupID == 0 {
		return ""
	}
	if inheritanceType == 0 {
		return "direct"
	}
	return "inherited"
}

// FormatListMarkdown renders a page of a group's protected environments as a
// Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Environments) == 0 {
		return toolutil.EmptyMessage("group protected environments")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Protected Environments", len(out.Environments), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Required Approvals", "Deploy Access Levels", "Approval Rules"))
	for _, e := range out.Environments {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(e.Name),
			strconv.FormatInt(e.RequiredApprovalCount, 10),
			strconv.Itoa(len(e.DeployAccessLevels)),
			strconv.Itoa(len(e.ApprovalRules)),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionGet, "see one environment's rules in full"),
		toolutil.HintAction(actionProtect, "protect another environment tier"),
		toolutil.HintAction(actionList, "page through the rest of the group's protected environments"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
