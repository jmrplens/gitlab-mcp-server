package runnercontrollertokens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders a runner controller token as a card. The secret
// is shown only on the result that mints it, and the advice to store it is
// added by the card from the row that showed one: a get answers with the same
// type and no token, and telling its reader to store a value the card does not
// hold sends them looking for one.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Runner Controller Token #%d", out.ID))
	c.Int("ID", out.ID)
	c.Int("Controller ID", out.RunnerControllerID)
	c.Text("Description", out.Description)
	c.Secret("Token", out.Token)
	c.Time("Last Used At", out.LastUsedAt)
	c.Time("Created At", out.CreatedAt)
	c.End()
	return b.String()
}

// FormatListMarkdown renders a list of runner controller tokens as a table.
// Its closing hint is built by canonicalID from the action name action_specs.go
// registers, rather than from a copy: the copy it used to hold was spelled with
// this package's own name, which names no action at all, so a model following
// the hint was told the action is unknown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Tokens) == 0 {
		return toolutil.EmptyMessage("runner controller tokens")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Runner Controller Tokens", len(out.Tokens), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Controller", "Description", "Last Used", "Created At"))
	for _, t := range out.Tokens {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(t.ID, 10),
			strconv.FormatInt(t.RunnerControllerID, 10),
			toolutil.EscapeMdTableCell(t.Description),
			toolutil.FormatTime(t.LastUsedAt),
			toolutil.FormatTime(t.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(canonicalID(actionNameTokenGet), "read one of these tokens in full"))
	return b.String()
}

// FormatGetMarkdown formats Get output as an MCP tool result.
func FormatGetMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out))
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
