// markdown.go provides Markdown formatting functions for project storage move
// MCP tool output.
package projectstoragemoves

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown formats a single project storage move as Markdown.
func FormatOutputMarkdown(o Output) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Project Storage Move #%d\n\n", o.ID)
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| ID | %d |\n", o.ID)
	fmt.Fprintf(&sb, "| State | %s |\n", o.State)
	fmt.Fprintf(&sb, "| Source Storage | %s |\n", o.SourceStorageName)
	fmt.Fprintf(&sb, "| Destination Storage | %s |\n", o.DestinationStorageName)
	if !o.CreatedAt.IsZero() {
		fmt.Fprintf(&sb, "| Created At | %s |\n", o.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if o.Project != nil {
		fmt.Fprintf(&sb, "| Project | %s (ID: %d) |\n", o.Project.PathWithNamespace, o.Project.ID)
	}
	toolutil.WriteHints(&sb,
		"Use `gitlab_retrieve_all_project_storage_moves` to view all moves",
	)
	return sb.String()
}

// FormatListMarkdown formats a list of project storage moves as Markdown.
func FormatListMarkdown(o ListOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("## Project Storage Moves\n\n")
	sb.WriteString("| ID | State | Source | Destination | Project |\n|---|---|---|---|---|\n")
	for _, m := range o.Moves {
		project := ""
		if m.Project != nil {
			project = m.Project.PathWithNamespace
		}
		fmt.Fprintf(&sb, "| %d | %s | %s | %s | %s |\n",
			m.ID, m.State, m.SourceStorageName, m.DestinationStorageName, project)
	}
	if o.Pagination.Page != 0 {
		fmt.Fprintf(&sb, "\n_Page %d, %d moves shown._\n", o.Pagination.Page, len(o.Moves))
	}
	return sb.String()
}

// FormatScheduleAllMarkdown formats the schedule-all result as Markdown.
func FormatScheduleAllMarkdown(o ScheduleAllOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Schedule All Project Storage Moves\n\n%s\n", o.Message)
	toolutil.WriteHints(&sb,
		"Use `gitlab_retrieve_all_project_storage_moves` to monitor progress",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)      // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)        // ListOutput
	toolutil.RegisterMarkdown(FormatScheduleAllMarkdown) // ScheduleAllOutput
}
