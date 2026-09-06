package members

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatListMarkdownString renders a ListOutput as a Markdown table string.
func FormatListMarkdownString(v ListOutput) string {
	var b strings.Builder
	if len(v.Members) == 0 {
		b.WriteString("No members found.\n")
		return b.String()
	}
	b.WriteString("| Username | Name | Access Level | State |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, m := range v.Members {
		// MdTitleLink escapes the label itself, so the raw username goes in:
		// passing the already-escaped copy would escape it twice.
		username := toolutil.MdTitleLink(m.Username, m.WebURL)
		//gitlab:allow-unescaped m.State: a user state from GitLab's own closed set (active, blocked, deactivated, banned), never text anybody types.
		fmt.Fprintf(
			&b, "| %s | %s | %s | %s |\n",
			username,
			toolutil.EscapeMdTableCell(m.Name),
			toolutil.EscapeMdTableCell(toolutil.AccessLevelDescription(gl.AccessLevelValue(m.AccessLevel))),
			m.State,
		)
	}
	toolutil.WritePagination(&b, v.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use action 'get' with user_id to see member details",
		"Use action 'add' to add a new project member",
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
	fmt.Fprintf(&b, "## Member: %s\n\n", toolutil.EscapeMdHeading(v.Username))
	fmt.Fprintf(&b, toolutil.FmtMdID, v.ID)
	fmt.Fprintf(&b, toolutil.FmtMdName, toolutil.EscapeMdTableCell(v.Name))
	fmt.Fprintf(&b, toolutil.FmtMdUsername, toolutil.EscapeMdTableCell(v.Username))
	//gitlab:allow-unescaped v.State: a user state from GitLab's own closed set (active, blocked, deactivated, banned), never text anybody types.
	fmt.Fprintf(&b, toolutil.FmtMdState, v.State)
	fmt.Fprintf(&b, "- **Access Level**: %s (%d)\n", toolutil.AccessLevelDescription(gl.AccessLevelValue(v.AccessLevel)), v.AccessLevel)
	if v.WebURL != "" {
		toolutil.WriteMdURL(&b, v.WebURL)
	}
	if v.Email != "" {
		// GitLab validates an address with a regexp that forbids only '@' and
		// whitespace, so '|' and '<' both pass.
		fmt.Fprintf(&b, toolutil.FmtMdEmail, toolutil.EscapeMdTableCell(v.Email))
	}
	if v.MemberRole != nil {
		fmt.Fprintf(&b, "- **Member Role**: %s (%d)\n", toolutil.EscapeMdTableCell(v.MemberRole.Name), v.MemberRole.ID)
	}
	if v.CreatedBy != nil {
		fmt.Fprintf(&b, "- **Created By**: %s (@%s)\n",
			toolutil.EscapeMdTableCell(v.CreatedBy.Name), toolutil.EscapeMdTableCell(v.CreatedBy.Username))
	}
	if v.ExpiresAt != "" {
		fmt.Fprintf(&b, "- **Expires At**: %s\n", toolutil.FormatTime(v.ExpiresAt))
	}
	if v.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(v.CreatedAt))
	}
	toolutil.WriteHints(
		&b,
		"Use action 'update' to change this member's access level",
		"Use action 'member_delete' to remove this member from the project",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkdown)
}
