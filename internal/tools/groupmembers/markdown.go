package groupmembers

import (
	"fmt"
	"strings"

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
	fmt.Fprintf(&b, "| State | %s |\n", out.State)
	fmt.Fprintf(&b, "| Access Level | %s (%d) |\n", out.AccessLevelDescription, out.AccessLevel)
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

func init() {
	toolutil.RegisterMarkdown(FormatMemberMarkdown)
	toolutil.RegisterMarkdown(FormatShareMarkdown)
}
