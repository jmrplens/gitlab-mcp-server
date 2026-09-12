package groupmembers

import (
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// accessLevel renders a membership's numeric access level as the name GitLab
// gives it with the number beside it, "Maintainer (40)": the number is what
// every write endpoint takes, the name is what a reader can act on.
func accessLevel(level int) string {
	return fmt.Sprintf("%s (%d)", toolutil.AccessLevelDescription(gl.AccessLevelValue(level)), level)
}

// FormatMemberMarkdown formats a single group member as markdown.
func FormatMemberMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Group Member")
	c.Int("ID", out.ID)
	c.Field("Username", out.Username)
	c.Field("Name", out.Name)
	c.Field("State", out.State)
	// membership_state is the membership's own state, which an Enterprise
	// instance sends beside the account state: a member awaiting approval is
	// an active user and not yet a member, and the card said only the first
	// half.
	c.Field("Membership State", out.MembershipState)
	c.Warn("Locked", out.Locked)
	c.Field("Access Level", accessLevel(out.AccessLevel))
	if out.MemberRole != nil {
		c.Field("Member Role", out.MemberRole.Name)
	}
	c.Time("Expires", out.ExpiresAt)
	c.URL(out.WebURL)
	// No HintPreserveLinks here: it tells the model to keep the clickable links
	// "from the table", and this card has no table. It used to be written
	// unconditionally, so a member GitLab sent no web_url for carried an
	// instruction about links the card had not got.
	c.End(
		toolutil.HintAction("group.group_member_edit", "change this member's access level"),
		toolutil.HintAction("group.group_member_remove", "remove this member"),
	)
	return b.String()
}

// FormatShareMarkdown formats a group share result as markdown.
func FormatShareMarkdown(out ShareOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Group Shared")
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Field("Path", out.Path)
	c.Text("Description", out.Description)
	c.URL(out.WebURL)
	c.End(
		toolutil.HintAction("group.members", "see all members in the group"),
		toolutil.HintAction("group.group_member_unshare", "revoke this share"),
	)
	return b.String()
}

// FormatBillableMembersMarkdown formats a list of billable group members as
// markdown.
func FormatBillableMembersMarkdown(out BillableMembersOutput) string {
	if len(out.Members) == 0 {
		return toolutil.EmptyMessage("billable members")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Billable Group Members", len(out.Members), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Username", "Name", "State", "Membership Type", "Locked", "Removable", "Last Activity"))
	linked := false
	for _, m := range out.Members {
		linked = linked || m.WebURL != ""
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdUserLink(m.Username, m.WebURL),
			toolutil.EscapeMdTableCell(m.Name),
			toolutil.EscapeMdTableCell(m.State),
			toolutil.EscapeMdTableCell(m.MembershipType),
			toolutil.BoolEmoji(m.Locked),
			toolutil.BoolEmoji(m.Removable),
			toolutil.FormatTime(m.LastActivityOn),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, linked,
		toolutil.HintPreserveLinks,
		toolutil.HintAction("group.group_billable_member_memberships_list", "see why a member is billable"),
		toolutil.HintAction("group.group_billable_member_remove", "remove a removable billable member"),
	)
	return b.String()
}

// FormatBillableMembershipsMarkdown formats a billable member's memberships as
// markdown.
func FormatBillableMembershipsMarkdown(out BillableMembershipsOutput) string {
	if len(out.Memberships) == 0 {
		return toolutil.EmptyMessage("memberships")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Billable Member Memberships", len(out.Memberships), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Source", "Access Level", "Expires"))
	linked := false
	for _, m := range out.Memberships {
		linked = linked || m.SourceMembersURL != ""
		access := ""
		if m.AccessLevel != nil {
			access = fmt.Sprintf("%s (%d)", toolutil.EscapeMdTableCell(m.AccessLevel.StringValue), m.AccessLevel.IntegerValue)
		}
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(m.SourceFullName, m.SourceMembersURL),
			access,
			toolutil.FormatTime(m.ExpiresAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, linked,
		toolutil.HintPreserveLinks,
		toolutil.HintAction("group.members", "inspect the source group's membership"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMemberMarkdown)
	toolutil.RegisterMarkdown(FormatShareMarkdown)
	toolutil.RegisterMarkdown(FormatBillableMembersMarkdown)
	toolutil.RegisterMarkdown(FormatBillableMembershipsMarkdown)
}
