// markdown.go provides Markdown formatting functions for project member MCP tool output.
package members

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
		username := toolutil.EscapeMdTableCell(m.Username)
		if m.WebURL != "" {
			username = fmt.Sprintf("[%s](%s)", username, m.WebURL)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
			username,
			toolutil.EscapeMdTableCell(m.Name),
			toolutil.EscapeMdTableCell(m.AccessLevelDescription),
			m.State,
		)
	}
	toolutil.WritePagination(&b, v.Pagination)
	toolutil.WriteHints(&b,
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
	fmt.Fprintf(&b, toolutil.FmtMdName, v.Name)
	fmt.Fprintf(&b, toolutil.FmtMdUsername, v.Username)
	fmt.Fprintf(&b, toolutil.FmtMdState, v.State)
	fmt.Fprintf(&b, "- **Access Level**: %s (%d)\n", v.AccessLevelDescription, v.AccessLevel)
	if v.WebURL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURL, v.WebURL)
	}
	if v.Email != "" {
		fmt.Fprintf(&b, toolutil.FmtMdEmail, v.Email)
	}
	if v.MemberRoleName != "" {
		fmt.Fprintf(&b, "- **Member Role**: %s\n", v.MemberRoleName)
	}
	if v.ExpiresAt != "" {
		fmt.Fprintf(&b, "- **Expires At**: %s\n", toolutil.FormatTime(v.ExpiresAt))
	}
	if v.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(v.CreatedAt))
	}
	toolutil.WriteHints(&b,
		"Use action 'update' to change this member's access level",
		"Use action 'remove' to remove this member from the project",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkdown)
}
