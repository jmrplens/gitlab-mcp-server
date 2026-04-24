// markdown.go provides Markdown formatting functions for group storage move
// MCP tool output.

package groupstoragemoves

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown formats a single group storage move as a Markdown table.
func FormatOutputMarkdown(o Output) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Group Storage Move #%d\n\n", o.ID)
	fmt.Fprintf(&sb, "| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| **ID** | %d |\n", o.ID)
	fmt.Fprintf(&sb, "| **State** | %s |\n", o.State)
	fmt.Fprintf(&sb, "| **Source** | %s |\n", o.SourceStorageName)
	fmt.Fprintf(&sb, "| **Destination** | %s |\n", o.DestinationStorageName)
	fmt.Fprintf(&sb, "| **Created** | %s |\n", o.CreatedAt.Format("2006-01-02 15:04:05"))
	if o.Group != nil {
		fmt.Fprintf(&sb, "| **Group** | [%s](%s) (ID: %d) |\n", o.Group.Name, o.Group.WebURL, o.Group.ID)
	}
	return sb.String()
}

// FormatListMarkdown formats a paginated list of group storage moves as a Markdown table.
func FormatListMarkdown(o ListOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	fmt.Fprintf(&sb, "## Group Storage Moves\n\n")
	if len(o.Moves) == 0 {
		sb.WriteString("No group storage moves found.\n")
		return sb.String()
	}
	fmt.Fprintf(&sb, "| ID | State | Source | Destination | Group | Created |\n")
	fmt.Fprintf(&sb, "|---|---|---|---|---|---|\n")
	for _, m := range o.Moves {
		groupInfo := ""
		if m.Group != nil {
			groupInfo = fmt.Sprintf("[%s](%s)", m.Group.Name, m.Group.WebURL)
		}
		fmt.Fprintf(&sb, "| %d | %s | %s | %s | %s | %s |\n",
			m.ID, m.State, m.SourceStorageName, m.DestinationStorageName,
			groupInfo, m.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if o.Pagination.Page != 0 {
		fmt.Fprintf(&sb, "\n_Page %d, %d moves shown._\n", o.Pagination.Page, len(o.Moves))
	}
	return sb.String()
}

// FormatScheduleAllMarkdown formats the schedule-all result.
func FormatScheduleAllMarkdown(o ScheduleAllOutput) string {
	return fmt.Sprintf("## Schedule All Group Storage Moves\n\n%s\n", o.Message)
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)      // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)        // ListOutput
	toolutil.RegisterMarkdown(FormatScheduleAllMarkdown) // ScheduleAllOutput
}
