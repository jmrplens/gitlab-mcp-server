package groupstoragemoves

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders one group storage move as the shared card.
//
// The hints are the ones its project and snippet siblings carry: the three
// scopes render the same object and used to disagree about whether a reader
// was told what to do next at all.
func FormatOutputMarkdown(o Output) string {
	return toolutil.FormatStorageMoveDetailMarkdown(
		storageMoveMarkdown(o), "Group Storage Move",
		toolutil.HintAction(actionRetrieveAllGroup, "see every group storage move on the instance"),
		toolutil.HintAction(actionScheduleGroup, "schedule another move for this group"),
	)
}

// FormatListMarkdown renders a page of group storage moves as the shared table.
func FormatListMarkdown(o ListOutput) string {
	return toolutil.FormatStorageMoveCollectionMarkdown(o.Moves, o.Pagination, storageMoveMarkdown,
		"Group Storage Moves", toolutil.EmptyMessage("group storage moves"), "Group")
}

// FormatScheduleAllMarkdown renders the bulk schedule confirmation as a card.
func FormatScheduleAllMarkdown(o ScheduleAllOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Schedule All Group Storage Moves")
	c.Field("Result", o.Message)
	c.End(toolutil.HintAction(actionRetrieveAllGroup, "watch the scheduled moves progress"))
	return b.String()
}

// storageMoveMarkdown maps one move onto the shared view model.
func storageMoveMarkdown(o Output) toolutil.StorageMoveMarkdown {
	return toolutil.NewStorageMoveMarkdown(o.ID, o.State, o.SourceStorageName, o.DestinationStorageName, o.CreatedAt, groupStorageMoveEntity(o.Group))
}

// groupStorageMoveEntity names the moved group, linked to its page.
func groupStorageMoveEntity(group *GroupOutput) *toolutil.StorageMoveEntityMarkdown {
	if group != nil {
		return toolutil.NewStorageMoveEntityMarkdown("Group", group.Name, group.WebURL, group.ID)
	}
	return nil
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)      // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)        // ListOutput
	toolutil.RegisterMarkdown(FormatScheduleAllMarkdown) // ScheduleAllOutput
}
