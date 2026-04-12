// markdown.go provides Markdown formatting functions for feature flag user list MCP tool output.

package ffuserlists

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatUserListMarkdown formats a single feature flag user list as markdown.
func FormatUserListMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Feature Flag User List: %s\n\n", toolutil.EscapeMdHeading(out.Name))
	fmt.Fprintf(&b, "- **ID**: %d (IID: %d)\n", out.ID, out.IID)
	if out.UserXIDs != "" {
		fmt.Fprintf(&b, "- **User XIDs**: %s\n", out.UserXIDs)
	}
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(out.CreatedAt))
	}
	if out.UpdatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdUpdated, toolutil.FormatTime(out.UpdatedAt))
	}
	toolutil.WriteHints(&b,
		"Use action 'ff_user_list_update' to modify user XIDs",
		"Use action 'ff_user_list_delete' to remove this user list",
	)
	return b.String()
}

// FormatListUserListsMarkdown formats a list of feature flag user lists as markdown.
func FormatListUserListsMarkdown(out ListOutput) string {
	var b strings.Builder
	b.WriteString("## Feature Flag User Lists\n\n")
	toolutil.WriteListSummary(&b, len(out.UserLists), out.Pagination)
	if len(out.UserLists) == 0 {
		b.WriteString("No feature flag user lists found.\n")
		return b.String()
	}
	b.WriteString("| IID | Name | User XIDs |\n|---|---|---|\n")
	for _, l := range out.UserLists {
		fmt.Fprintf(&b, "| %d | %s | %s |\n",
			l.IID,
			toolutil.EscapeMdTableCell(l.Name),
			toolutil.EscapeMdTableCell(l.UserXIDs),
		)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'ff_user_list_get' with user_list_iid for full details",
		"Use action 'ff_user_list_create' to add a new user list",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatUserListMarkdown)
	toolutil.RegisterMarkdown(FormatListUserListsMarkdown)
}
