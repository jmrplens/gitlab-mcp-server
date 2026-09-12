package iterationdata

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatListMarkdown renders a page of iterations as a Markdown table: a
// collection of objects that share columns. The title and the empty-list
// sentence come from the scope, since a group iteration list and a project one
// differ in nothing else.
//
// The hints were written before the table and the summary line after its last
// row, which put a guidance section where no reader looks for one and made the
// header lazily continue the paragraph above it, so no table rendered at all.
// Heading, summary, table, pagination and hints now run in that order, which is
// what [toolutil.WriteListHeading] and [toolutil.WriteListFooter] exist for.
func FormatListMarkdown(title, emptyText string, iterations []Output, pagination toolutil.PaginationOutput, hints ...string) string {
	if len(iterations) == 0 {
		return strings.TrimRight(emptyText, "\r\n") + "\n"
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, title, len(iterations), pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "IID", "Title", "State", "Start", "Due"))
	for _, iteration := range iterations {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(iteration.ID, 10),
			strconv.FormatInt(iteration.IID, 10),
			titleCell(iteration),
			StateName(iteration.State),
			toolutil.FormatTime(iteration.StartDate),
			toolutil.FormatTime(iteration.DueDate),
		))
	}
	toolutil.WriteListFooter(&b, pagination, true, hints...)
	return b.String()
}

// titleCell links an iteration's title to its page. The table used to carry a
// URL column whose link text was the state word, which named the link after a
// value that is not the iteration's identity and left the title unlinked; an
// iteration GitLab sent no title for links its own address.
func titleCell(iteration Output) string {
	title := iteration.Title
	if strings.TrimSpace(title) == "" {
		title = iteration.WebURL
	}
	return toolutil.MdTitleLink(title, iteration.WebURL)
}

// FormatOutputMarkdown renders one iteration as the card of one object, with
// the caller's hints last.
//
// It used to open a "| Property | Value |" table and then write list rows into
// it (the ID row, the URL row, the created row), which ended the table with no
// body and left every later "| State | … |" on the page as literal pipes. A
// card row is a complete block wherever it lands, which is why this is a card.
func FormatOutputMarkdown(output Output, hints ...string) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Iteration #%d: %s", output.IID, output.Title))
	c.Int("ID", output.ID)
	c.Int("IID", output.IID)
	c.Count("Sequence", output.Sequence)
	c.Field("State", StateName(output.State))
	// Every iteration belongs to a group, so a zero group id is GitLab not
	// saying rather than group number zero.
	c.Count("Group ID", output.GroupID)
	c.Time("Start", output.StartDate)
	c.Time("Due", output.DueDate)
	c.URL(output.WebURL)
	c.Time("Created", output.CreatedAt)
	c.Time("Updated", output.UpdatedAt)
	c.Text("Description", output.Description)
	c.End(hints...)
	return b.String()
}

// StateName names a GitLab iteration state as GitLab's own enum spells it:
// upcoming, current, closed.
//
// It used to map 1 to "opened", which is not a state GitLab stores at all — it
// is a list filter meaning upcoming plus current — and that shifted every
// later value by one, so a current iteration read as upcoming and a closed one
// as current.
func StateName(state int64) string {
	switch state {
	case 1:
		return "upcoming"
	case 2:
		return "current"
	case 3:
		return "closed"
	default:
		return fmt.Sprintf("unknown(%d)", state)
	}
}
