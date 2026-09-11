package protectedenvs

import (
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name that the specs do not already spell.
const (
	actionEnvProtectedList    = "environment.protected_list"
	actionEnvProtectedProtect = "environment.protected_protect"
	actionEnvProtectedUpdate  = "environment.protected_update"
)

// FormatOutputMarkdown renders one protected environment as the card of a
// single object: the approvals it needs, then the deploy access levels and the
// approval rules as nested collections.
func FormatOutputMarkdown(pe Output) string {
	if pe.Name == "" {
		return ""
	}
	var b strings.Builder
	// An environment name is written in .gitlab-ci.yml or typed in the UI.
	c := toolutil.NewCard(&b, "Protected Environment: "+pe.Name)
	// An environment with approval rules of its own counts its approvals per
	// rule, so the single number GitLab sends beside them is not what gates a
	// deployment, and printing it alone said a deployment needed two approvals
	// when it needed one from each of three rules.
	if len(pe.ApprovalRules) > 0 {
		c.Field("Required Approvals", "per approval rule (see below)")
	} else {
		c.Int("Required Approvals", pe.RequiredApprovalCount)
	}
	if len(pe.DeployAccessLevels) > 0 {
		t := c.Table("Deploy Access Levels", "ID", "Level", "Grantee", "Description", "Inheritance")
		for _, a := range pe.DeployAccessLevels {
			t.Row(
				strconv.FormatInt(a.ID, 10),
				levelCell(a.AccessLevel, a.UserID, a.GroupID),
				granteeCell(a.UserID, a.GroupID),
				// GitLab's access-level description is a role name for a plain
				// rule but a user's display name or a group's name for a
				// granular one.
				toolutil.EscapeMdTableCell(a.AccessLevelDescription),
				inheritanceCell(a.GroupID, a.GroupInheritanceType),
			)
		}
	}
	if len(pe.ApprovalRules) > 0 {
		t := c.Table("Approval Rules", "ID", "Level", "Grantee", "Description", "Required", "Inheritance")
		for _, r := range pe.ApprovalRules {
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
		toolutil.HintAction(actionEnvProtectedUpdate, "change the deploy access levels or approval rules"),
		toolutil.HintAction(actionEnvProtectedUnprotect, "remove this environment's protection"),
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
// grants a role to everyone who holds it. GitLab sends zero for the id a rule
// is not about, which used to be rendered as a numeric 0 in a column of its
// own, so every role rule claimed to be about user 0 and group 0.
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

// FormatListMarkdown renders a page of a project's protected environments as a
// Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Environments) == 0 {
		return toolutil.EmptyMessage("protected environments")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Protected Environments", len(out.Environments), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Required Approvals", "Deploy Access Levels", "Approval Rules"))
	for _, pe := range out.Environments {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(pe.Name),
			strconv.FormatInt(pe.RequiredApprovalCount, 10),
			strconv.Itoa(len(pe.DeployAccessLevels)),
			strconv.Itoa(len(pe.ApprovalRules)),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionEnvProtectedGet, "see one environment's rules in full"),
		toolutil.HintAction(actionEnvProtectedProtect, "protect another environment or wildcard"),
		toolutil.HintAction(actionEnvProtectedList, "page through the rest of the project's protected environments"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
