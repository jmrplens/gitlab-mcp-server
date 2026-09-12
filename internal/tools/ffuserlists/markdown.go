package ffuserlists

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionGet    = "feature_flags.ff_user_list_get"
	actionCreate = "feature_flags.ff_user_list_create"
	actionUpdate = "feature_flags.ff_user_list_update"
	actionDelete = "feature_flags.ff_user_list_delete"
)

// FormatUserListMarkdown renders one feature flag user list as the card of one
// object.
func FormatUserListMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Feature Flag User List: "+out.Name)
	c.Int("ID", out.ID)
	c.Int("IID", out.IID)
	c.Count("Project ID", out.ProjectID)
	c.Field("Name", out.Name)
	// The external user ids are a comma-separated list a person supplies.
	c.Field("User XIDs", out.UserXIDs)
	c.Time("Created", out.CreatedAt)
	c.Time("Updated", out.UpdatedAt)
	c.End(
		toolutil.HintAction(actionUpdate, "modify the user XIDs on this list"),
		toolutil.HintAction(actionDelete, "remove this user list"),
	)
	return b.String()
}

// FormatListUserListsMarkdown renders a page of feature flag user lists as a
// Markdown table: a collection of objects that share columns.
func FormatListUserListsMarkdown(out ListOutput) string {
	if len(out.UserLists) == 0 {
		return toolutil.EmptyMessage("feature flag user lists")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Feature Flag User Lists", len(out.UserLists), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Name", "User XIDs"))
	for _, l := range out.UserLists {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(l.IID, 10),
			toolutil.EscapeMdTableCell(l.Name),
			toolutil.EscapeMdTableCell(l.UserXIDs),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionGet, "read one list by its user_list_iid"),
		toolutil.HintAction(actionCreate, "add a new user list"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatUserListMarkdown)
	toolutil.RegisterMarkdown(FormatListUserListsMarkdown)
}
