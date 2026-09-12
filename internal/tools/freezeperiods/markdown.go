package freezeperiods

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name.
const (
	actionFreezeGet    = "environment.freeze_get"
	actionFreezeList   = "environment.freeze_list"
	actionFreezeCreate = "environment.freeze_create"
	actionFreezeUpdate = "environment.freeze_update"
	actionFreezeDelete = "environment.freeze_delete"
)

// FormatListMarkdown formats a list of freeze periods as Markdown.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatListMarkdownString renders a page of freeze periods as a Markdown
// table: a collection of objects that share columns.
//
// The heading counts the total GitLab reports rather than the page length, and
// the pagination footer is written before the guidance section rather than
// after the rows of the next block.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.FreezePeriods) == 0 {
		return toolutil.EmptyMessage("freeze periods")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Freeze Periods", len(out.FreezePeriods), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Start", "End", "Timezone"))
	for _, fp := range out.FreezePeriods {
		// A freeze window is two cron expressions and a timezone a maintainer
		// types, and GitLab validates only that the cron parses.
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(fp.ID, 10),
			toolutil.MdCodeSpanCell(fp.FreezeStart),
			toolutil.MdCodeSpanCell(fp.FreezeEnd),
			toolutil.EscapeMdTableCell(fp.CronTimezone),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionFreezeGet, "see one freeze period in full"),
		toolutil.HintAction(actionFreezeCreate, "add a freeze window"),
	)
	return b.String()
}

// FormatMarkdown formats a single freeze period as Markdown.
func FormatMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(out))
}

// FormatMarkdownString renders one freeze period as the card of a single
// object.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Freeze Period #%d", out.ID))
	c.Int("ID", out.ID)
	// The two cron expressions and the timezone are a maintainer's own text.
	c.Code("Start", out.FreezeStart)
	c.Code("End", out.FreezeEnd)
	c.Field("Timezone", out.CronTimezone)
	c.Time("Created", out.CreatedAt)
	c.Time("Updated", out.UpdatedAt)
	c.End(
		toolutil.HintAction(actionFreezeUpdate, "change this freeze window"),
		toolutil.HintAction(actionFreezeDelete, "remove it"),
		toolutil.HintAction(actionFreezeList, "see every freeze window of this project"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkdownString)
}
