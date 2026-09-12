package projectstoragemoves

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical catalog action IDs the hints name. The catalog domain is
// storage_move, the group all three storage-move packages register under, which
// is not this package's name.
const (
	hintActionRetrieveAll = "storage_move.retrieve_all_project"
	hintActionSchedule    = "storage_move.schedule_project"
)

// FormatOutputMarkdown renders one project storage move as a card, through the
// renderer the group and snippet storage-move packages already share.
//
// It used to keep a hand copy of that renderer, which opened a "| Field |
// Value |" table, printed the creation time in a zone-less layout no other
// formatter uses, and wrote the project as escaped text where the shared cell
// builds a link.
func FormatOutputMarkdown(o Output) string {
	return toolutil.FormatStorageMoveDetailMarkdown(
		storageMoveMarkdown(o), "Project Storage Move",
		toolutil.HintAction(hintActionRetrieveAll, "see every project storage move on the instance"),
		toolutil.HintAction(hintActionSchedule, "schedule another move for this project"),
	)
}

// FormatListMarkdown renders a page of project storage moves as the shared
// table: a collection of objects that share columns.
func FormatListMarkdown(o ListOutput) string {
	return toolutil.FormatStorageMoveCollectionMarkdown(o.Moves, o.Pagination, storageMoveMarkdown,
		"Project Storage Moves", toolutil.EmptyMessage("project storage moves"), "Project")
}

// FormatScheduleAllMarkdown renders the bulk schedule confirmation as a card.
func FormatScheduleAllMarkdown(o ScheduleAllOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Schedule All Project Storage Moves")
	c.Field("Result", o.Message)
	c.End(toolutil.HintAction(hintActionRetrieveAll, "watch the scheduled moves progress"))
	return b.String()
}

// storageMoveMarkdown maps one move onto the shared view model.
func storageMoveMarkdown(o Output) toolutil.StorageMoveMarkdown {
	return toolutil.NewStorageMoveMarkdown(o.ID, o.State, o.SourceStorageName, o.DestinationStorageName, o.CreatedAt, projectStorageMoveEntity(o.Project))
}

// projectStorageMoveEntity names the moved project. ProjectOutput carries no
// web URL, so the entity links to nothing and renders as its escaped path.
func projectStorageMoveEntity(project *ProjectOutput) *toolutil.StorageMoveEntityMarkdown {
	if project == nil {
		return nil
	}
	return toolutil.NewStorageMoveEntityMarkdown("Project", project.PathWithNamespace, "", project.ID)
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)      // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)        // ListOutput
	toolutil.RegisterMarkdown(FormatScheduleAllMarkdown) // ScheduleAllOutput
}
