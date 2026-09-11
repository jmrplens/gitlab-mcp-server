package members

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// accessLevel renders a membership's numeric access level as the name GitLab
// gives it with the number beside it, "Maintainer (40)". The number stays
// because it is what every write endpoint takes, and the name because a
// reader cannot be expected to know that 40 is a maintainer.
func accessLevel(level int) string {
	return fmt.Sprintf("%s (%d)", toolutil.AccessLevelDescription(gl.AccessLevelValue(level)), level)
}

// FormatListMarkdownString renders a ListOutput as a Markdown table string.
func FormatListMarkdownString(v ListOutput) string {
	if len(v.Members) == 0 {
		return toolutil.EmptyMessage("members")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Project Members", len(v.Members), v.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Username", "Name", "Access Level", "State", "Membership", "Expires"))
	linked := false
	for _, m := range v.Members {
		linked = linked || m.WebURL != ""
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdUserLink(m.Username, m.WebURL),
			toolutil.EscapeMdTableCell(m.Name),
			accessLevel(m.AccessLevel),
			toolutil.EscapeMdTableCell(m.State),
			toolutil.EscapeMdTableCell(m.MembershipState),
			toolutil.FormatTime(m.ExpiresAt),
		))
	}
	toolutil.WriteListFooter(&b, v.Pagination, linked,
		toolutil.HintPreserveLinks,
		toolutil.HintAction("project.member_get", "see one member's details"),
		toolutil.HintAction("project.member_add", "add a member to this project"),
	)
	return b.String()
}

// FormatListMarkdown returns a Markdown MCP tool result for a ListOutput.
func FormatListMarkdown(v ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(v))
}

// FormatMarkdown renders a single member Output as Markdown.
func FormatMarkdown(v Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Member: "+v.Username)
	c.Int("ID", v.ID)
	c.Field("Name", v.Name)
	c.Field("Username", v.Username)
	c.Field("State", v.State)
	// membership_state is what an Enterprise instance answers with beside the
	// account state: a member awaiting approval is active as a user and not
	// yet a member, and the card said only the first half.
	c.Field("Membership State", v.MembershipState)
	c.Warn("Locked", v.Locked)
	c.Field("Access Level", accessLevel(v.AccessLevel))
	c.URL(v.WebURL)
	c.Field("Email", v.Email)
	if v.MemberRole != nil {
		c.Field("Member Role", fmt.Sprintf("%s (%d)", v.MemberRole.Name, v.MemberRole.ID))
	}
	if v.CreatedBy != nil {
		author := c.Sub("Created By")
		author.Field("Name", v.CreatedBy.Name)
		author.Markdown("Username", toolutil.MdUserLink(v.CreatedBy.Username, v.CreatedBy.WebURL))
	}
	c.Time("Expires At", v.ExpiresAt)
	c.Time("Created", v.CreatedAt)
	c.End(
		toolutil.HintAction("project.member_edit", "change this member's access level"),
		toolutil.HintAction("project.member_delete", "remove this member from the project"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkdown)
}
