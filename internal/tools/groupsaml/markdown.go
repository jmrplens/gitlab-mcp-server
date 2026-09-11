package groupsaml

import (
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// accessLevel renders a SAML link's numeric access level as the name GitLab
// gives it with the number beside it, "Developer (30)": the link's whole
// purpose is the role it grants, and the bare integer named it in a spelling
// only the API uses.
func accessLevel(level int) string {
	return fmt.Sprintf("%s (%d)", toolutil.AccessLevelDescription(gl.AccessLevelValue(level)), level)
}

// FormatOutputMarkdown renders a single group SAML link as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	// The SAML group name and the provider label are typed by the
	// administrator who configured the link.
	c := toolutil.NewCard(&b, "SAML Link: "+out.Name)
	c.Field("Name", out.Name)
	c.Field("Access Level", accessLevel(out.AccessLevel))
	c.Count("Member Role ID", out.MemberRoleID)
	c.Field("Provider", out.Provider)
	c.End(toolutil.HintAction("group.saml_link_delete", "remove this link"))
	return b.String()
}

// FormatListMarkdown renders a list of group SAML links as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Links) == 0 {
		return toolutil.EmptyMessage("SAML group links")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "SAML Group Links", len(out.Links), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Access Level", "Provider"))
	for _, l := range out.Links {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(l.Name),
			accessLevel(l.AccessLevel),
			toolutil.EscapeMdTableCell(l.Provider),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		"These map SAML group names to access levels",
		toolutil.HintAction("group.saml_users_list", "list the users provisioned through SAML SSO"),
	)
	return b.String()
}

// FormatSAMLUsersListMarkdown renders the SAML-provisioned users of a group as Markdown.
func FormatSAMLUsersListMarkdown(out SAMLUsersListOutput) string {
	if len(out.Users) == 0 {
		return toolutil.EmptyMessage("SAML users")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "SAML Users", len(out.Users), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Username", "Name", "State"))
	linked := false
	for _, u := range out.Users {
		linked = linked || u.WebURL != ""
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(u.ID, 10),
			toolutil.MdUserLink(u.Username, u.WebURL),
			toolutil.EscapeMdTableCell(u.Name),
			toolutil.EscapeMdTableCell(u.State),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, linked,
		toolutil.HintPreserveLinks,
		"These are users provisioned through SAML SSO",
		toolutil.HintAction("group.saml_link_list", "see the SAML group-to-access-level link mappings"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatSAMLUsersListMarkdown)
}
