// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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

// emojiText is a custom emoji's name the way GitLab shows it, ":name:", or
// nothing for an emoji GitLab sent no name for, so a cell never reads "::".
func emojiText(name string) string {
	if name == "" {
		return ""
	}
	return ":" + name + ":"
}

// emojiCell renders a custom emoji's ":name:" escaped for a cell. The whole
// form is escaped rather than the name inside it, because whether an '@'
// opens an email address depends on the character in front of it, and the
// first character of a name follows the colon, which markdown-it reads as
// part of the address: the name "@evil.com" escaped alone and put between
// the colons is ":@evil.com:", a mailto link there.
func emojiCell(name string) string {
	return toolutil.EscapeMdTableCell(emojiText(name))
}

// ancestorsHint is the step a subgroup listed without its ancestor groups
// carries: the emoji those groups define can be used there too, and leaving
// them out read as "this group has none", which led a model to create a
// duplicate (issue 1342).
var ancestorsHint = toolutil.HintAction(actionList,
	"list again with include_ancestor_groups=true: this subgroup can also use the custom emoji of its ancestor groups, which this list leaves out")

// FormatListMarkdown renders a paginated list of custom emoji as Markdown.
//
// The ID column carries the global ID the delete action takes: without it the
// list named every emoji and gave a reader no value to act on.
func FormatListMarkdown(out ListOutput) string {
	var ancestors string
	if out.ancestorsLeftOut {
		ancestors = ancestorsHint
	}
	if len(out.Emoji) == 0 {
		var sb strings.Builder
		sb.WriteString(toolutil.EmptyMessage("custom emoji"))
		toolutil.WriteHints(&sb, ancestors)
		return sb.String()
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
		ancestors,
		toolutil.PartialPageHint(out.partial),
		toolutil.HintAction(actionCreate, "add another emoji to the group"),
		toolutil.HintAction(actionDelete, "remove one by the ID in the first column"),
	)
	return sb.String()
}

// nameCell renders the emoji as its ":name:" form linked to the image GitLab
// serves for it, as the escaped form when GitLab sent no URL, and as nothing
// when it sent no name, since a link needs text to click.
//
// The form reaches [toolutil.MdTitleLink] raw, because the link helper
// escapes its label itself: a form escaped for a cell first has the
// backslash the cell escaper writes in front of an address's colon, dot or
// '@' escaped again into one the reader sees, so a name holding "www." would
// read "www\." in the link.
func nameCell(e Item) string {
	text := emojiText(e.Name)
	if text == "" {
		return ""
	}
	return toolutil.MdTitleLink(text, e.URL)
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
