package groupmembers

import (
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatMemberMarkdown formats a single group member as markdown.
func FormatMemberMarkdown(out Output) string {
	var b strings.Builder
	b.WriteString("## Group Member\n\n")
	b.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&b, "| Username | %s |\n", toolutil.EscapeMdTableCell(out.Username))
	fmt.Fprintf(&b, "| Name | %s |\n", toolutil.EscapeMdTableCell(out.Name))
	//gitlab:allow-unescaped out.State: a membership state GitLab picks from a fixed set (active, awaiting and the rest).
	fmt.Fprintf(&b, "| State | %s |\n", out.State)
	fmt.Fprintf(&b, "| Access Level | %s (%d) |\n", toolutil.AccessLevelDescription(gl.AccessLevelValue(out.AccessLevel)), out.AccessLevel)
	if out.MemberRole != nil {
		fmt.Fprintf(&b, "| Member Role | %s |\n", toolutil.EscapeMdTableCell(out.MemberRole.Name))
	}
	if out.ExpiresAt != "" {
		fmt.Fprintf(&b, "| Expires | %s |\n", toolutil.FormatTime(out.ExpiresAt))
	}
	if out.WebURL != "" {
		fmt.Fprintf(&b, "| URL | %s |\n", toolutil.MdTitleLink(out.Username, out.WebURL))
	}
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use action 'group_member_edit' to change access level",
		"Use action 'group_member_remove' to remove this member",
	)
	return b.String()
}

// FormatShareMarkdown formats a group share result as markdown.
func FormatShareMarkdown(out ShareOutput) string {
	var b strings.Builder
	b.WriteString("## Group Shared\n\n")
	b.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&b, "| Name | %s |\n", toolutil.EscapeMdTableCell(out.Name))
	fmt.Fprintf(&b, "| Path | %s |\n", toolutil.EscapeMdTableCell(out.Path))
	if out.WebURL != "" {
		fmt.Fprintf(&b, "| URL | %s |\n", toolutil.MdTitleLink(out.Name, out.WebURL))
	}
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use action 'members' to see all members in the group",
		"Use action 'group_member_unshare' to revoke this share",
	)
	return b.String()
}

// FormatBillableMembersMarkdown formats a list of billable group members as
// markdown.
func FormatBillableMembersMarkdown(out BillableMembersOutput) string {
	var b strings.Builder
	if len(out.Members) == 0 {
		b.WriteString("No billable members found.\n")
		return b.String()
	}
	b.WriteString("## Billable Group Members\n\n")
	b.WriteString("| Username | Name | State | Membership Type | Removable | Last Activity |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, m := range out.Members {
		username := toolutil.EscapeMdTableCell(m.Username)
		if m.WebURL != "" {
			username = toolutil.MdTitleLink(m.Username, m.WebURL)
		}
		fmt.Fprintf(
			&b, "| %s | %s | %s | %s | %t | %s |\n",
			username,
			toolutil.EscapeMdTableCell(m.Name),
			//gitlab:allow-unescaped m.State: a membership state GitLab picks from a fixed set (active, awaiting and the rest).
			m.State,
			toolutil.EscapeMdTableCell(m.MembershipType),
			m.Removable,
			toolutil.EscapeMdTableCell(m.LastActivityOn),
		)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use action 'group_billable_member_memberships_list' with user_id to see why a member is billable",
		"Use action 'group_billable_member_remove' to remove a removable billable member",
	)
	return b.String()
}

// FormatBillableMembershipsMarkdown formats a billable member's memberships as
// markdown.
func FormatBillableMembershipsMarkdown(out BillableMembershipsOutput) string {
	var b strings.Builder
	if len(out.Memberships) == 0 {
		b.WriteString("No memberships found for this billable member.\n")
		return b.String()
	}
	b.WriteString("## Billable Member Memberships\n\n")
	b.WriteString("| Source | Access Level | Expires |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, m := range out.Memberships {
		source := toolutil.EscapeMdTableCell(m.SourceFullName)
		if m.SourceMembersURL != "" {
			source = toolutil.MdTitleLink(m.SourceFullName, m.SourceMembersURL)
		}
		access := ""
		if m.AccessLevel != nil {
			access = fmt.Sprintf("%s (%d)", toolutil.EscapeMdTableCell(m.AccessLevel.StringValue), m.AccessLevel.IntegerValue)
		}
		expires := ""
		if m.ExpiresAt != "" {
			expires = toolutil.FormatTime(m.ExpiresAt)
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", source, access, expires)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use action 'members' on the source group/project to inspect that membership",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMemberMarkdown)
	toolutil.RegisterMarkdown(FormatShareMarkdown)
	toolutil.RegisterMarkdown(FormatBillableMembersMarkdown)
	toolutil.RegisterMarkdown(FormatBillableMembershipsMarkdown)
}
