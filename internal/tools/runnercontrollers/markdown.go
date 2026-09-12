package runnercontrollers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves: a
// controller action is projected under the runner domain.
const (
	actionControllerGet       = "runner.controller_get"
	actionControllerList      = "runner.controller_list"
	actionControllerUpdate    = "runner.controller_update"
	actionControllerTokenList = "runner.controller_token_list"
	actionControllerScopeList = "runner.controller_scope_list"
)

// writeController writes the fields every runner controller carries, so the
// summary and the detail card cannot drift apart.
func writeController(c *toolutil.Card, out Output) {
	c.Int("ID", out.ID)
	c.Text("Description", out.Description)
	// A runner controller state, a typed enum client-go renders from GitLab's
	// own fixed set.
	c.Field("State", out.State)
	c.Time("Created", out.CreatedAt)
	c.Time("Updated", out.UpdatedAt)
}

// FormatOutputMarkdown renders one runner controller as the card of a single
// object.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Runner Controller #%d", out.ID))
	writeController(c, out)
	c.End(
		toolutil.HintAction(actionControllerGet, "see this controller's full details"),
		toolutil.HintAction(actionControllerTokenList, "manage the tokens it authenticates with"),
	)
	return b.String()
}

// FormatDetailsMarkdown renders one runner controller in full: the same fields
// with whether it is connected right now.
func FormatDetailsMarkdown(out DetailsOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Runner Controller #%d: Details", out.ID))
	writeController(c, out.Output)
	c.Bool("Connected", out.Connected)
	c.End(
		toolutil.HintAction(actionControllerScopeList, "see the runners this controller is scoped to"),
		toolutil.HintAction(actionControllerUpdate, "change its description or state"),
	)
	return b.String()
}

// FormatListMarkdown renders a page of runner controllers as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Controllers) == 0 {
		return toolutil.EmptyMessage("runner controllers")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Runner Controllers", len(out.Controllers), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Description", "State", "Created"))
	for _, rc := range out.Controllers {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(rc.ID, 10),
			toolutil.EscapeMdTableCell(rc.Description),
			toolutil.EscapeMdTableCell(rc.State),
			toolutil.FormatTime(rc.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionControllerGet, "see one controller in full"),
		toolutil.HintAction(actionControllerList, "page through the rest of the controllers"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatDetailsMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
