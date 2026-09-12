package broadcastmessages

import (
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// targetAccessLevels names the roles a broadcast message is shown to, in the
// words GitLab's own UI uses, through the one table this server keeps. GitLab
// sends no list at all for a message that is not restricted by role, and the
// card then writes no row rather than claiming a restriction that is not
// there.
func targetAccessLevels(levels []int64) string {
	if len(levels) == 0 {
		return ""
	}
	names := make([]string, 0, len(levels))
	for _, level := range levels {
		names = append(names, toolutil.AccessLevelDescription(gl.AccessLevelValue(level)))
	}
	return strings.Join(names, ", ")
}

// FormatListMarkdown renders a page of broadcast messages as a Markdown table:
// a collection of objects that share columns.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	if len(out.Messages) == 0 {
		return toolutil.ToolResultWithMarkdown(toolutil.EmptyMessage("broadcast messages"))
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Broadcast Messages", len(out.Messages), out.Pagination)
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Message", "Type", "Active", "Starts", "Ends"))
	for _, m := range out.Messages {
		sb.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(m.ID, 10),
			toolutil.EscapeMdTableCell(m.Message),
			toolutil.EscapeMdTableCell(m.BroadcastType),
			toolutil.BoolEmoji(m.Active),
			// The two instants are RFC 3339 as toItem formatted them, and the
			// column shows the display form every other table in this server
			// shows rather than the wire form.
			toolutil.FormatTime(m.StartsAt),
			toolutil.FormatTime(m.EndsAt),
		))
	}
	// The table carries no link, so the footer carries no instruction to keep
	// links it does not have.
	toolutil.WriteListFooter(&sb, out.Pagination, false,
		"Use `gitlab_get_broadcast_message` to view details of a specific message")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatMessageMarkdown renders one broadcast message as the card of one
// object: what it says, when it is shown, how it looks, and who sees it.
func FormatMessageMarkdown(item MessageItem) *mcp.CallToolResult {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Broadcast Message #"+strconv.FormatInt(item.ID, 10))
	c.Int("ID", item.ID)
	// The message is Markdown an administrator typed and GitLab renders, so it
	// is quoted rather than written into the card's own list.
	c.Text("Message", item.Message)
	c.Field("Type", item.BroadcastType)
	c.Bool("Active", item.Active)
	c.Bool("Dismissable", item.Dismissable)
	c.Time("Starts At", item.StartsAt)
	c.Time("Ends At", item.EndsAt)
	c.Field("Theme", item.Theme)
	// The color and the font are what an administrator typed into the message
	// form: a hex value and a CSS font name.
	c.Field("Color", item.Color)
	c.Field("Font", item.Font)
	// A target path is a path glob an administrator types into the message.
	c.Field("Target Path", item.TargetPath)
	// GitLab decides who sees a message from the roles below, and the card used
	// to drop them, so a message shown to maintainers alone read as one shown
	// to everybody.
	c.Field("Target Access Levels", targetAccessLevels(item.TargetAccessLevels))
	c.End("Use `gitlab_update_broadcast_message` to modify this message")
	return toolutil.ToolResultWithMarkdown(b.String())
}

func formatGetOutput(out GetOutput) *mcp.CallToolResult {
	return FormatMessageMarkdown(out.Message)
}

func formatCreateOutput(out CreateOutput) *mcp.CallToolResult {
	return FormatMessageMarkdown(out.Message)
}

func formatUpdateOutput(out UpdateOutput) *mcp.CallToolResult {
	return FormatMessageMarkdown(out.Message)
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatMessageMarkdown)
	toolutil.RegisterMarkdownResult(formatGetOutput)
	toolutil.RegisterMarkdownResult(formatCreateOutput)
	toolutil.RegisterMarkdownResult(formatUpdateOutput)
}
