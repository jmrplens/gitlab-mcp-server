package groupsaml

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single group SAML link as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## SAML Link: %s\n\n", toolutil.EscapeMdHeading(out.Name))
	fmt.Fprintf(&b, toolutil.FmtMdName, out.Name)
	fmt.Fprintf(&b, "- **Access Level**: %d\n", out.AccessLevel)
	if out.MemberRoleID != 0 {
		fmt.Fprintf(&b, "- **Member Role ID**: %d\n", out.MemberRoleID)
	}
	if out.Provider != "" {
		fmt.Fprintf(&b, "- **Provider**: %s\n", out.Provider)
	}
	toolutil.WriteHints(&b,
		"Use gitlab_group_saml_link_delete to remove this link",
	)
	return b.String()
}

// FormatListMarkdown renders a list of group SAML links as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Links) == 0 {
		return "No SAML group links found.\n"
	}
	var b strings.Builder
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	fmt.Fprintf(&b, "**%d SAML link(s)**\n\n", len(out.Links))
	b.WriteString("| Name | Access Level | Provider |\n| --- | --- | --- |\n")
	for _, l := range out.Links {
		fmt.Fprintf(&b, "| %s | %d | %s |\n",
			toolutil.EscapeMdTableCell(l.Name),
			l.AccessLevel,
			toolutil.EscapeMdTableCell(l.Provider),
		)
	}
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
