package groupldap

import (
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// accessLevel renders an LDAP link's numeric group access as the name GitLab
// gives it with the number beside it, "Developer (30)": the link exists for
// the role it grants, and the bare integer named it in a spelling only the API
// uses.
func accessLevel(level int) string {
	return fmt.Sprintf("%s (%d)", toolutil.AccessLevelDescription(gl.AccessLevelValue(level)), level)
}

// linkHeading names the link in the card's heading. A link is defined either
// by a common name or by a filter, never by both, so a filter-based link has
// an empty CN and used to be headed "LDAP Link: " with nothing after the
// colon.
func linkHeading(out Output) string {
	switch {
	case out.CN != "":
		return "LDAP Link: " + out.CN
	case out.Filter != "":
		return "LDAP Link: " + out.Filter
	default:
		return "LDAP Link"
	}
}

// FormatOutputMarkdown renders a single group LDAP link as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	// The common name, the LDAP filter and the provider label are all typed by
	// the administrator who configured the link, and a filter is an expression
	// whose own syntax uses parentheses and vertical bars.
	c := toolutil.NewCard(&b, linkHeading(out))
	c.Field("CN", out.CN)
	c.Field("Filter", out.Filter)
	c.Field("Access Level", accessLevel(out.GroupAccess))
	c.Field("Provider", out.Provider)
	c.Count("Member Role ID", out.MemberRoleID)
	c.End(toolutil.HintAction("group.ldap_link_delete", "remove this link"))
	return b.String()
}

// FormatListMarkdown renders a list of group LDAP links as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Links) == 0 {
		return toolutil.EmptyMessage("LDAP group links")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "LDAP Group Links", len(out.Links), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("CN", "Filter", "Access Level", "Provider", "Member Role ID"))
	for _, l := range out.Links {
		role := "-"
		if l.MemberRoleID != 0 {
			role = strconv.FormatInt(l.MemberRoleID, 10)
		}
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(l.CN),
			toolutil.EscapeMdTableCell(l.Filter),
			accessLevel(l.GroupAccess),
			toolutil.EscapeMdTableCell(l.Provider),
			role,
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction("group.ldap_link_add", "add another LDAP group link"),
		toolutil.HintAction("group.ldap_sync", "trigger an LDAP sync for this group"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
