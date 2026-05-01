package groupmembers

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatMemberMarkdown formats a single group member as markdown.
func FormatMemberMarkdown(out Output) string {
	var b strings.Builder
	b.WriteString("## Group Member\n\n")
	b.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| ID | %d |\n", out.ID)
	b.WriteString("| Username | " + toolutil.EscapeMdTableCell(out.Username) + " |\n")
	b.WriteString("| Name | " + toolutil.EscapeMdTableCell(out.Name) + " |\n")
	b.WriteString("| State | " + out.State + " |\n")
	fmt.Fprintf(&b, "| Access Level | %s (%d) |\n", out.AccessLevelDescription, out.AccessLevel)
	if out.ExpiresAt != "" {
		b.WriteString("| Expires | " + toolutil.FormatTime(out.ExpiresAt) + " |\n")
	}
	if out.WebURL != "" {
		b.WriteString("| URL | " + toolutil.MdTitleLink(out.Username, out.WebURL) + " |\n")
	}
	toolutil.WriteHints(&b,
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
	b.WriteString("| Name | " + toolutil.EscapeMdTableCell(out.Name) + " |\n")
	b.WriteString("| Path | " + toolutil.EscapeMdTableCell(out.Path) + " |\n")
	if out.WebURL != "" {
		b.WriteString("| URL | " + toolutil.MdTitleLink(out.Name, out.WebURL) + " |\n")
	}
	toolutil.WriteHints(&b,
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
