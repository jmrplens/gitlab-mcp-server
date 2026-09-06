package workitemsavedviews

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatGetMarkdown renders one saved view, filters included.
func FormatGetMarkdown(out GetOutput) string {
	var sb strings.Builder
	// A saved view's name is free text whoever saved it typed.
	fmt.Fprintf(&sb, "## Saved View: %s\n\n", toolutil.EscapeMdHeading(out.SavedView.Name))
	writeViewDetails(&sb, out.SavedView)
	if out.SavedView.Filters != nil {
		fmt.Fprintf(&sb, "\n### Filters\n\n```json\n%s\n```\n", prettyJSON(out.SavedView.Filters))
	}
	if out.SavedView.DisplaySettings != nil {
		fmt.Fprintf(&sb, "\n### Display Settings\n\n```json\n%s\n```\n", prettyJSON(out.SavedView.DisplaySettings))
	}
	toolutil.WriteHints(&sb, "Use `work_item_saved_view.update` to change this view, or `work_item_saved_view.subscribe` to follow it")
	return sb.String()
}

// FormatListMarkdown renders a page of saved views as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	fmt.Fprintf(&sb, "## Saved Views: %s\n\n", toolutil.EscapeMdHeading(out.NamespacePath))

	if len(out.SavedViews) == 0 {
		sb.WriteString("No saved views found.\n")
		return sb.String()
	}

	sb.WriteString("| ID | Name | Private | Subscribed | Sort | Description |\n")
	sb.WriteString("|----|------|---------|------------|------|-------------|\n")
	for _, view := range out.SavedViews {
		fmt.Fprintf(&sb, "| %d | %s | %v | %v | %s | %s |\n",
			view.ID,
			toolutil.EscapeMdTableCell(view.Name),
			view.IsPrivate,
			view.Subscribed,
			toolutil.EscapeMdTableCell(view.Sort),
			toolutil.EscapeMdTableCell(view.Description),
		)
	}
	if out.Pagination.HasNextPage {
		fmt.Fprintf(&sb, "\n> Next page cursor: `%s`\n", out.Pagination.EndCursor)
	}
	toolutil.WriteHints(&sb, "Filters are omitted here, so use `work_item_saved_view.get` with an ID from the table to read them")
	return sb.String()
}

// FormatMutateMarkdown renders the confirmation shared by create, update,
// subscribe, and unsubscribe.
func FormatMutateMarkdown(out MutateOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Saved View\n\n%s\n\n", out.Message)
	writeViewDetails(&sb, out.SavedView)
	return sb.String()
}

// writeViewDetails renders the scalar fields shared by the detail and mutation
// renderings.
func writeViewDetails(sb *strings.Builder, view Item) {
	fmt.Fprintf(sb, "- **ID**: %d\n", view.ID)
	if view.GID != "" {
		//gitlab:allow-unescaped view.GID: a GraphQL global id GitLab mints, gid://gitlab/ and a type name and a number.
		fmt.Fprintf(sb, "- **Global ID**: `%s`\n", view.GID)
	}
	if view.Description != "" {
		fmt.Fprintf(sb, "- **Description**: %s\n", toolutil.EscapeMdTableCell(view.Description))
	}
	fmt.Fprintf(sb, "- **Private**: %v\n", view.IsPrivate)
	fmt.Fprintf(sb, "- **Subscribed**: %v\n", view.Subscribed)
	if view.Sort != "" {
		//gitlab:allow-unescaped view.Sort: a work item sort key GitLab picks from its own enum (CREATED_DESC, TITLE_ASC and the rest).
		fmt.Fprintf(sb, "- **Sort**: %s\n", view.Sort)
	}
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
