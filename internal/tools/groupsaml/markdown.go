package groupsaml

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatOutputMarkdown renders a single group SAML link as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## SAML Link: %s\n\n", toolutil.EscapeMdHeading(out.Name))
	// The SAML group name and the provider label are typed by the
	// administrator who configured the link.
	fmt.Fprintf(&b, toolutil.FmtMdName, toolutil.EscapeMdTableCell(out.Name))
	fmt.Fprintf(&b, "- **Access Level**: %d\n", out.AccessLevel)
	if out.MemberRoleID != 0 {
		fmt.Fprintf(&b, "- **Member Role ID**: %d\n", out.MemberRoleID)
	}
	if out.Provider != "" {
		fmt.Fprintf(&b, "- **Provider**: %s\n", toolutil.EscapeMdTableCell(out.Provider))
	}
	toolutil.WriteHints(
		&b,
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
	fmt.Fprintf(&b, "**%d SAML link(s)**\n\n", len(out.Links))
	b.WriteString("| Name | Access Level | Provider |\n| --- | --- | --- |\n")
	for _, l := range out.Links {
		fmt.Fprintf(
			&b, "| %s | %d | %s |\n",
			toolutil.EscapeMdTableCell(l.Name),
			l.AccessLevel,
			toolutil.EscapeMdTableCell(l.Provider),
		)
	}
	toolutil.WriteHints(
		&b,
		"These map SAML group names to access levels; use action 'saml_users_list' to list the users provisioned via SAML SSO",
	)
	return b.String()
}

// FormatSAMLUsersListMarkdown renders the SAML-provisioned users of a group as Markdown.
func FormatSAMLUsersListMarkdown(out SAMLUsersListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## SAML Users (%d)\n\n", len(out.Users))
	toolutil.WriteListSummary(&b, len(out.Users), out.Pagination)
	if len(out.Users) == 0 {
		b.WriteString("No SAML users found.\n")
		toolutil.WritePagination(&b, out.Pagination)
		return b.String()
	}
	b.WriteString("| ID | Username | Name | State |\n| --- | --- | --- | --- |\n")
	for _, u := range out.Users {
		username := toolutil.MdTitleLink(u.Username, u.WebURL)
		//gitlab:allow-unescaped u.State: a user account state, one of GitLab's fixed set (active, blocked, deactivated, banned).
		fmt.Fprintf(&b, "| %d | %s | %s | %s |\n",
			u.ID, username, toolutil.EscapeMdTableCell(u.Name), u.State)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"These are users provisioned through SAML SSO; use action 'saml_link_list' to see the SAML group-to-access-level link mappings",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatSAMLUsersListMarkdown)
}
