package groupmarkdownuploads

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintActionDeleteByID is the canonical catalog ID of the route that removes
// one of the uploads this list shows.
const hintActionDeleteByID = "group." + actionGroupUploadDeleteByID

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
}

// FormatListMarkdownString renders a list of group markdown uploads.
func FormatListMarkdownString(o ListOutput) string {
	if len(o.Uploads) == 0 {
		return toolutil.EmptyMessage("group markdown uploads")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Markdown Uploads", len(o.Uploads), o.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Filename", "Size (bytes)", "Created", "Uploaded By"))
	for _, u := range o.Uploads {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(u.ID, 10),
			toolutil.EscapeMdTableCell(u.Filename),
			strconv.FormatInt(u.Size, 10),
			toolutil.FormatTime(u.CreatedAt),
			toolutil.EscapeMdTableCell(uploadedByLabel(u.UploadedBy)),
		))
	}
	toolutil.WriteListFooter(&b, o.Pagination, false,
		toolutil.HintAction(hintActionDeleteByID, "remove one of these uploads"),
	)
	return b.String()
}
