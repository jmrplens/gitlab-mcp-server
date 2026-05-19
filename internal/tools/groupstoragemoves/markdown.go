package groupstoragemoves

import (
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatOutputMarkdown formats a single group storage move as a Markdown table.
func FormatOutputMarkdown(o Output) string {
	return toolutil.FormatStorageMoveDetailMarkdown(storageMoveMarkdown(o), "Group Storage Move")
}

// FormatListMarkdown formats a paginated list of group storage moves as a Markdown table.
func FormatListMarkdown(o ListOutput) string {
	return toolutil.FormatStorageMoveCollectionMarkdown(o.Moves, o.Pagination, storageMoveMarkdown,
		"Group Storage Moves", "No group storage moves found.", "Group")
}

// FormatScheduleAllMarkdown formats the schedule-all result.
func FormatScheduleAllMarkdown(o ScheduleAllOutput) string {
	return fmt.Sprintf("## Schedule All Group Storage Moves\n\n%s\n", o.Message)
}

func storageMoveMarkdown(o Output) toolutil.StorageMoveMarkdown {
	return toolutil.NewStorageMoveMarkdown(o.ID, o.State, o.SourceStorageName, o.DestinationStorageName, o.CreatedAt, groupStorageMoveEntity(o.Group))
}

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
