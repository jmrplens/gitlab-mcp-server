package workitemsavedviews

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatGetMarkdown renders one saved view as a card, filters included.
func FormatGetMarkdown(out GetOutput) string {
	var b strings.Builder
	// A saved view's name is free text whoever saved it typed.
	c := toolutil.NewCard(&b, "Saved View: "+out.SavedView.Name)
	writeViewDetails(c, out.SavedView)
	// Both documents are what whoever saved the view typed, rendered back as
	// JSON, and JSON escaping leaves a backtick alone: the fence has to be
	// sized to the document rather than written as three, which is what
	// [toolutil.Card.Fence] does.
	if out.SavedView.Filters != nil {
		c.Fence("Filters", "json", prettyJSON(out.SavedView.Filters))
	}
	if out.SavedView.DisplaySettings != nil {
		c.Fence("Display Settings", "json", prettyJSON(out.SavedView.DisplaySettings))
	}
	c.End(
		toolutil.HintAction(actionUpdate, "change this view"),
		toolutil.HintAction(actionSubscribe, "follow it"),
	)
	return b.String()
}

// FormatListMarkdown renders a page of saved views as a Markdown table.
//
// The hints are written once, at the end. The leading call this had opened a
// guidance section above the heading, so the response carried two of them and
// only the first reached next_steps.
func FormatListMarkdown(out ListOutput) string {
	if len(out.SavedViews) == 0 {
		return toolutil.EmptyMessage("saved views")
	}
	var b strings.Builder
	// A cursor connection counts nothing it has not walked, so the heading
	// carries the count shown and the cursor line says whether more follow.
	toolutil.WriteListHeading(&b, "Saved Views: "+out.NamespacePath, len(out.SavedViews), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Private", "Subscribed", "Sort", "Description"))
	for _, view := range out.SavedViews {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(view.ID, 10),
			toolutil.EscapeMdTableCell(view.Name),
			toolutil.BoolEmoji(view.IsPrivate),
			toolutil.BoolEmoji(view.Subscribed),
			toolutil.EscapeMdTableCell(view.Sort),
			toolutil.EscapeMdTableCell(view.Description),
		))
	}
	toolutil.WriteGraphQLPagination(&b, out.Pagination, len(out.SavedViews))
	toolutil.WriteHints(&b, toolutil.HintAction(actionGet, "read the filters this table omits, with an ID from it"))
	return b.String()
}

// FormatMutateMarkdown renders the confirmation shared by create, update,
// subscribe, and unsubscribe as the card of the view that changed: the heading
// names the view, and the confirmation sentence sits under it as the note.
//
// The sentence is one this package composed, but it reaches the formatter as a
// field of the result rather than as a literal, so it is escaped like any
// other value read off an output: a note is one line of inline content.
func FormatMutateMarkdown(out MutateOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Saved View: "+out.SavedView.Name)
	writeViewDetails(c, out.SavedView)
	c.Note(toolutil.EscapeMdTableCell(out.Message))
	c.End()
	return b.String()
}

// writeViewDetails writes the scalar fields shared by the detail and mutation
// renderings.
func writeViewDetails(c *toolutil.Card, view Item) {
	c.Int("ID", view.ID)
	c.Code("Global ID", view.GID)
	c.Text("Description", view.Description)
	c.Bool("Private", view.IsPrivate)
	c.Bool("Subscribed", view.Subscribed)
	c.Field("Sort", view.Sort)
}

// prettyJSON renders a decoded opaque scalar for display, falling back to the
// Go rendering when the value cannot be marshaled back.
func prettyJSON(value any) string {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}

func init() {
	toolutil.RegisterMarkdown(FormatGetMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatMutateMarkdown)
}
