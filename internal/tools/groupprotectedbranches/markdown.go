package groupprotectedbranches

import (
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders one group protected branch as the card of a
// single object: its own fields, then each set of access levels as a nested
// collection under a heading of its own.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Protected Branch: "+out.Name)
	c.Int("ID", out.ID)
	c.Bool("Allow Force Push", out.AllowForcePush)
	c.Bool("Code Owner Approval Required", out.CodeOwnerApprovalRequired)
	writeAccessLevels(c, "Push Access Levels", out.PushAccessLevels)
	writeAccessLevels(c, "Merge Access Levels", out.MergeAccessLevels)
	writeAccessLevels(c, "Unprotect Access Levels", out.UnprotectAccessLevels)
	c.End(
		toolutil.HintAction(actionUpdate, "add or remove an allowed user, group or role"),
		toolutil.HintAction(actionUnprotect, "remove this protection from every subgroup project"),
	)
	return b.String()
}

// writeAccessLevels writes one set of access levels as a table under the card.
//
// The grantee is a column of its own because the access level alone does not
// say who the rule is about: GitLab answers a per-user or per-group rule with
// the same access_level it answers a role rule with, and the description that
// used to be the only other column is the role name for one and a person's or
// a group's display name for the other, so two rows that grant entirely
// different things rendered alike.
func writeAccessLevels(c *toolutil.Card, heading string, levels []AccessLevelOutput) {
	if len(levels) == 0 {
		return
	}
	t := c.Table(heading, "ID", "Level", "Grantee", "Description")
	for _, l := range levels {
		t.Row(
			strconv.FormatInt(l.ID, 10),
			levelCell(l),
			granteeCell(l),
			// GitLab's access-level description is a role name for a plain rule
			// but a user's display name or a group's name for a granular one.
			toolutil.EscapeMdTableCell(l.AccessLevelDescription),
		)
	}
}

// levelCell renders the role a rule grants, and "-" for a rule that names a
// user, a group or a deploy key instead, whose access_level is not what
// decides.
func levelCell(l AccessLevelOutput) string {
	if granteeCell(l) != "" {
		return "-"
	}
	return toolutil.AccessLevelDescription(gl.AccessLevelValue(l.AccessLevel))
}

// granteeCell names who a granular rule is about, and nothing for a rule that
// grants a role to everyone who holds it.
func granteeCell(l AccessLevelOutput) string {
	switch {
	case l.UserID != 0:
		return fmt.Sprintf("user #%d", l.UserID)
	case l.GroupID != 0:
		return fmt.Sprintf("group #%d", l.GroupID)
	case l.DeployKeyID != 0:
		return fmt.Sprintf("deploy key #%d", l.DeployKeyID)
	default:
		return ""
	}
}

// FormatListMarkdown renders a page of a group's protected branches as a
// Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Branches) == 0 {
		return toolutil.EmptyMessage("group protected branches")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Protected Branches", len(out.Branches), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Force Push", "Code Owner"))
	for _, br := range out.Branches {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(br.ID, 10),
			toolutil.EscapeMdTableCell(br.Name),
			toolutil.BoolEmoji(br.AllowForcePush),
			toolutil.BoolEmoji(br.CodeOwnerApprovalRequired),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionGet, "see one branch's access levels in full"),
		toolutil.HintAction(actionProtect, "protect another branch or wildcard"),
		toolutil.HintAction(actionList, "page through the rest of the group's protected branches"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
