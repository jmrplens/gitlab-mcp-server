package projectaliases

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders one project alias as a card. The alias name is a
// path segment the caller has to copy exactly, so it is a code span: a span is
// its own containment, where a cell escaper's entities would have rendered
// literally inside one.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	// An alias name is the string the caller of the create action supplied.
	c := toolutil.NewCard(&b, "Project Alias: "+out.Name)
	c.Int("ID", out.ID)
	c.Code("Name", out.Name)
	c.Int("Project ID", out.ProjectID)
	c.End(
		toolutil.HintAction("project_alias.delete", "remove this alias"),
		toolutil.HintAction("project_alias.list", "view all aliases"),
	)
	return b.String()
}

// FormatListMarkdown renders the project aliases as a table. The rows carry no
// link, so the footer drops the instruction to preserve links, and the hints
// follow the table rather than opening the response above it, where the table
// header lazily continued the last hint's list item and no table rendered at
// all.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Aliases) == 0 {
		return toolutil.EmptyMessage("project aliases")
	}
	var b strings.Builder
	var pagination toolutil.PaginationOutput
	toolutil.WriteListHeading(&b, "Project Aliases", len(out.Aliases), pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Project ID"))
	for _, a := range out.Aliases {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(a.ID, 10),
			toolutil.MdCodeSpanCell(a.Name),
			strconv.FormatInt(a.ProjectID, 10),
		))
	}
	toolutil.WriteListFooter(&b, pagination, false,
		toolutil.HintAction("project_alias.get", "see one alias"))
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown) // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)   // ListOutput
}
