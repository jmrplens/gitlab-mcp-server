package snippetstoragemoves

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical catalog action IDs the hints name. The catalog domain is
// storage_move, the group all three storage-move packages register under.
const (
	hintActionRetrieveAll = "storage_move.retrieve_all_snippet"
	hintActionSchedule    = "storage_move.schedule_snippet"
)

// FormatOutputMarkdown renders one snippet storage move as the shared card.
func FormatOutputMarkdown(o Output) string {
	return toolutil.FormatStorageMoveDetailMarkdown(
		storageMoveMarkdown(o), "Snippet Storage Move",
		toolutil.HintAction(hintActionRetrieveAll, "see every snippet storage move on the instance"),
		toolutil.HintAction(hintActionSchedule, "schedule another move for this snippet"),
	)
}

// FormatListMarkdown renders a page of snippet storage moves as the shared
// table.
func FormatListMarkdown(o ListOutput) string {
	return toolutil.FormatStorageMoveCollectionMarkdown(o.Moves, o.Pagination, storageMoveMarkdown,
		"Snippet Storage Moves", toolutil.EmptyMessage("snippet storage moves"), "Snippet")
}

// FormatScheduleAllMarkdown renders the bulk schedule confirmation as a card.
func FormatScheduleAllMarkdown(o ScheduleAllOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Schedule All Snippet Storage Moves")
	c.Field("Result", o.Message)
	c.End(toolutil.HintAction(hintActionRetrieveAll, "watch the scheduled moves progress"))
	return b.String()
}

// storageMoveMarkdown maps one move onto the shared view model.
func storageMoveMarkdown(o Output) toolutil.StorageMoveMarkdown {
	return toolutil.NewStorageMoveMarkdown(o.ID, o.State, o.SourceStorageName, o.DestinationStorageName, o.CreatedAt, snippetStorageMoveEntity(o.Snippet))
}

// snippetStorageMoveEntity names the moved snippet, linked to its page.
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
