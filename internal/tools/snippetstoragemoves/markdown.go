package snippetstoragemoves

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders one snippet storage move as the shared card.
//
// The hints name the canonical action IDs declared in action_specs.go, the
// same strings the specs cross-link with, so a reader following one reaches an
// action gitlab_execute_action holds.
func FormatOutputMarkdown(o Output) string {
	return toolutil.FormatStorageMoveDetailMarkdown(
		storageMoveMarkdown(o), "Snippet Storage Move",
		toolutil.HintAction(actionRetrieveAllSnippet, "see every snippet storage move on the instance"),
		toolutil.HintAction(actionScheduleSnippet, "schedule another move for this snippet"),
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
	c.End(toolutil.HintAction(actionRetrieveAllSnippet, "watch the scheduled moves progress"))
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
