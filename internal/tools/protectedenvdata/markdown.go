// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package protectedenvdata

import (
	"fmt"
	"strconv"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// labelRequiredApprovals is the label the card row and the list column both
// carry.
const labelRequiredApprovals = "Required Approvals"

// WriteRules writes what a protected environment's card carries between its
// heading and its hints: the approvals it needs, then the deploy access levels
// and the approval rules as nested collections, each table left out when
// GitLab sent no entry for it. The project and the group cards differ in
// nothing else.
//
// An environment with approval rules of its own counts its approvals per
// rule, so the single number GitLab sends beside them is not what gates a
// deployment, and printing it alone said a deployment needed two approvals
// when it needed one from each of three rules.
func WriteRules(c *toolutil.Card, requiredApprovalCount int64, deployAccessLevels []AccessLevel, approvalRules []ApprovalRule) {
	if len(approvalRules) > 0 {
		c.Field(labelRequiredApprovals, "per approval rule (see below)")
	} else {
		c.Int(labelRequiredApprovals, requiredApprovalCount)
	}
	if len(deployAccessLevels) > 0 {
		t := c.Table("Deploy Access Levels", "ID", "Level", "Grantee", "Description", "Inheritance")
		for _, a := range deployAccessLevels {
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
	if len(approvalRules) > 0 {
		t := c.Table("Approval Rules", "ID", "Level", "Grantee", "Description", "Required", "Inheritance")
		for _, r := range approvalRules {
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
}

// ListHeader is the header of the table a page of protected environments is
// listed in, a project's or a group's.
func ListHeader() string {
	return toolutil.MarkdownTableHeader("Name", labelRequiredApprovals, "Deploy Access Levels", "Approval Rules")
}

// ListRow is one protected environment's row of the table [ListHeader]
// opens: its name, the approvals GitLab sent for it, and how many deploy
// access levels and approval rules it carries.
func ListRow(name string, requiredApprovalCount int64, deployAccessLevels []AccessLevel, approvalRules []ApprovalRule) string {
	return toolutil.MarkdownTableRow(
		toolutil.EscapeMdTableCell(name),
		strconv.FormatInt(requiredApprovalCount, 10),
		strconv.Itoa(len(deployAccessLevels)),
		strconv.Itoa(len(approvalRules)),
	)
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
//
// The conditions are early returns rather than the cases of a tagless switch,
// whose expressions no coverage profile places, so gremlins reported their
// mutants not covered although the tests render both kinds of rule.
func granteeCell(userID, groupID int64) string {
	if userID != 0 {
		return fmt.Sprintf("user #%d", userID)
	}
	if groupID != 0 {
		return fmt.Sprintf("group #%d", groupID)
	}
	return ""
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
