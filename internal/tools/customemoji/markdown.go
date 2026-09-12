package customemoji

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionList   = "custom_emoji.list"
	actionCreate = "custom_emoji.create"
	actionDelete = "custom_emoji.delete"
)

// emojiCell renders a custom emoji's name the way GitLab shows it, ":name:",
// with the name escaped for a cell.
func emojiCell(name string) string {
	if name == "" {
		return ""
	}
	return ":" + toolutil.EscapeMdTableCell(name) + ":"
}

// FormatListMarkdown renders a paginated list of custom emoji as Markdown.
//
// The ID column carries the global ID the delete action takes: without it the
// list named every emoji and gave a reader no value to act on.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Emoji) == 0 {
		return toolutil.EmptyMessage("custom emoji")
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Custom Emoji", len(out.Emoji), toolutil.PaginationOutput{})
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "External", "Created"))
	linked := false
	for _, e := range out.Emoji {
		linked = linked || e.URL != ""
		sb.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdCodeSpanCell(e.ID),
			nameCell(e),
			toolutil.BoolEmoji(e.External),
			toolutil.FormatTime(e.CreatedAt),
		))
	}
	toolutil.WriteGraphQLPagination(&sb, out.Pagination, len(out.Emoji))
	toolutil.WriteListFooter(&sb, toolutil.PaginationOutput{}, linked,
		toolutil.HintAction(actionCreate, "add another emoji to the group"),
		toolutil.HintAction(actionDelete, "remove one by the ID in the first column"),
	)
	return sb.String()
}

// nameCell renders the emoji as its ":name:" form linked to the image GitLab
// serves for it, or as the bare form when GitLab sent no URL.
func nameCell(e Item) string {
	cell := emojiCell(e.Name)
	if cell == "" || e.URL == "" {
		return cell
	}
	return toolutil.MdTitleLink(cell, e.URL)
}

// FormatCreateMarkdown renders a created custom emoji as the card of one
// object. The global ID is a code span because it is the value the delete
// action takes and a reader has to copy it exactly.
func FormatCreateMarkdown(out CreateOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, toolutil.EmojiSuccess+" Custom Emoji Created")
	c.Code("ID", out.Emoji.ID)
	c.Markdown("Name", emojiCell(out.Emoji.Name))
	c.URL(out.Emoji.URL)
	c.Bool("External", out.Emoji.External)
	c.Time("Created", out.Emoji.CreatedAt)
	c.End(
		toolutil.HintAction(actionList, "see every custom emoji in the group"),
		toolutil.HintAction(actionDelete, "remove this emoji"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatCreateMarkdown)
}
