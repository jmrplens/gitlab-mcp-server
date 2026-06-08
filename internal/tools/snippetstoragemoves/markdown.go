package snippetstoragemoves

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatOutputMarkdown formats a single snippet storage move as a Markdown table.
func FormatOutputMarkdown(o Output) string {
	return toolutil.FormatStorageMoveDetailMarkdown(
		storageMoveMarkdown(o), "Snippet Storage Move",
		"Use `gitlab_retrieve_all_snippet_storage_moves` to view all moves",
	)
}

// FormatListMarkdown formats a paginated list of snippet storage moves as a Markdown table.
func FormatListMarkdown(o ListOutput) string {
	return toolutil.FormatStorageMoveCollectionMarkdown(o.Moves, o.Pagination, storageMoveMarkdown,
		"Snippet Storage Moves", "No snippet storage moves found.", "Snippet")
}

// FormatScheduleAllMarkdown formats the schedule-all result.
func FormatScheduleAllMarkdown(o ScheduleAllOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Schedule All Snippet Storage Moves\n\n%s\n", o.Message)
	toolutil.WriteHints(
		&sb,
		"Use `gitlab_retrieve_all_snippet_storage_moves` to monitor progress",
	)
	return sb.String()
}

func storageMoveMarkdown(o Output) toolutil.StorageMoveMarkdown {
	return toolutil.NewStorageMoveMarkdown(o.ID, o.State, o.SourceStorageName, o.DestinationStorageName, o.CreatedAt, snippetStorageMoveEntity(o.Snippet))
}

func snippetStorageMoveEntity(snippet *SnippetOutput) *toolutil.StorageMoveEntityMarkdown {
	if snippet != nil {
		return toolutil.NewStorageMoveEntityMarkdown("Snippet", snippet.Title, snippet.WebURL, snippet.ID)
	}
	return nil
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)      // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)        // ListOutput
	toolutil.RegisterMarkdown(FormatScheduleAllMarkdown) // ScheduleAllOutput
}
