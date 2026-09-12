package grouprelationsexport

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintActionSchedule is the canonical catalog ID of the route that starts an
// export, which is what a reader looking at a finished status list does next.
const hintActionSchedule = "group.group_relations_schedule"

func init() {
	toolutil.RegisterMarkdown(FormatListExportStatusMarkdownString)
}

// FormatListExportStatusMarkdownString renders group relations export statuses.
func FormatListExportStatusMarkdownString(o ListExportStatusOutput) string {
	if len(o.Statuses) == 0 {
		return toolutil.EmptyMessage("export statuses")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Relations Export Status", len(o.Statuses), o.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Relation", "Status", "Batched", "Batches", "Error"))
	for _, s := range o.Statuses {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(s.Relation),
			exportStatusLabel(s.Status),
			toolutil.BoolEmoji(s.Batched),
			strconv.FormatInt(s.BatchesCount, 10),
			toolutil.EscapeMdTableCell(s.Error),
		))
	}
	toolutil.WriteListFooter(&b, o.Pagination, false,
		toolutil.HintAction(hintActionSchedule, "start a new export"),
	)
	return b.String()
}

// exportStatusLabel names the export state GitLab reports as a number, with
// the number kept beside it. The three values are the states of
// BulkImports::Export (started 0, finished 1, failed -1), which
// doc/api/group_relations_export.md shows on the status response; a value
// outside them is rendered as the number alone, since inventing a word for it
// would be a guess.
func exportStatusLabel(status int64) string {
	switch status {
	case -1:
		return "failed (-1)"
	case 0:
		return "started (0)"
	case 1:
		return "finished (1)"
	default:
		return strconv.FormatInt(status, 10)
	}
}
