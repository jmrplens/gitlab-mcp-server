// markdown.go provides Markdown formatting functions for snippet storage move
// MCP tool output.

package snippetstoragemoves

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown formats a single snippet storage move as a Markdown table.
func FormatOutputMarkdown(o Output) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Snippet Storage Move #%d\n\n", o.ID)
	fmt.Fprintf(&sb, "| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| **ID** | %d |\n", o.ID)
	fmt.Fprintf(&sb, "| **State** | %s |\n", o.State)
	fmt.Fprintf(&sb, "| **Source** | %s |\n", o.SourceStorageName)
	fmt.Fprintf(&sb, "| **Destination** | %s |\n", o.DestinationStorageName)
	fmt.Fprintf(&sb, "| **Created** | %s |\n", o.CreatedAt.Format("2006-01-02 15:04:05"))
	if o.Snippet != nil {
		fmt.Fprintf(&sb, "| **Snippet** | [%s](%s) (ID: %d) |\n", o.Snippet.Title, o.Snippet.WebURL, o.Snippet.ID)
	}
	toolutil.WriteHints(&sb,
		"Use `gitlab_retrieve_all_snippet_storage_moves` to view all moves",
	)
	return sb.String()
}

// FormatListMarkdown formats a paginated list of snippet storage moves as a Markdown table.
func FormatListMarkdown(o ListOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	fmt.Fprintf(&sb, "## Snippet Storage Moves\n\n")
	if len(o.Moves) == 0 {
		sb.WriteString("No snippet storage moves found.\n")
		return sb.String()
	}
	fmt.Fprintf(&sb, "| ID | State | Source | Destination | Snippet | Created |\n")
	fmt.Fprintf(&sb, "|---|---|---|---|---|---|\n")
	for _, m := range o.Moves {
		snippetInfo := ""
		if m.Snippet != nil {
			snippetInfo = fmt.Sprintf("[%s](%s)", m.Snippet.Title, m.Snippet.WebURL)
		}
		fmt.Fprintf(&sb, "| %d | %s | %s | %s | %s | %s |\n",
			m.ID, m.State, m.SourceStorageName, m.DestinationStorageName,
			snippetInfo, m.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if o.Pagination.Page != 0 {
		fmt.Fprintf(&sb, "\n_Page %d, %d moves shown._\n", o.Pagination.Page, len(o.Moves))
	}
	return sb.String()
}

// FormatScheduleAllMarkdown formats the schedule-all result.
func FormatScheduleAllMarkdown(o ScheduleAllOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Schedule All Snippet Storage Moves\n\n%s\n", o.Message)
	toolutil.WriteHints(&sb,
		"Use `gitlab_retrieve_all_snippet_storage_moves` to monitor progress",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)      // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)        // ListOutput
	toolutil.RegisterMarkdown(FormatScheduleAllMarkdown) // ScheduleAllOutput
}
