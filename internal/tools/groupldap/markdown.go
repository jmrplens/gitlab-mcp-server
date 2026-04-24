// markdown.go provides Markdown formatting functions for group LDAP MCP tool output.

package groupldap

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single group LDAP link as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## LDAP Link: %s\n\n", toolutil.EscapeMdHeading(out.CN))
	fmt.Fprintf(&b, "- **CN**: %s\n", out.CN)
	if out.Filter != "" {
		fmt.Fprintf(&b, "- **Filter**: %s\n", out.Filter)
	}
	fmt.Fprintf(&b, "- **Access Level**: %d\n", out.GroupAccess)
	fmt.Fprintf(&b, "- **Provider**: %s\n", out.Provider)
	if out.MemberRoleID != 0 {
		fmt.Fprintf(&b, "- **Member Role ID**: %d\n", out.MemberRoleID)
	}
	toolutil.WriteHints(&b,
		"Use gitlab_group_ldap_link_delete to remove this link",
	)
	return b.String()
}

// FormatListMarkdown renders a list of group LDAP links as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Links) == 0 {
		return "No LDAP group links found.\n"
	}
	var b strings.Builder
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	fmt.Fprintf(&b, "**%d LDAP link(s)**\n\n", len(out.Links))
	b.WriteString("| CN | Filter | Access | Provider |\n| --- | --- | --- | --- |\n")
	for _, l := range out.Links {
		fmt.Fprintf(&b, "| %s | %s | %d | %s |\n",
			toolutil.EscapeMdTableCell(l.CN),
			toolutil.EscapeMdTableCell(l.Filter),
			l.GroupAccess,
			toolutil.EscapeMdTableCell(l.Provider),
		)
	}
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
