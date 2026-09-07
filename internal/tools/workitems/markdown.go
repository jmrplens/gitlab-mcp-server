package workitems

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatGetMarkdown formats a single work item as markdown.
func FormatGetMarkdown(out GetOutput) *mcp.CallToolResult {
	wi := out.WorkItem
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Work Item #%d: %s\n\n", wi.IID, toolutil.EscapeMdHeading(wi.Title))
	// The type name is a GraphQL String rather than an enum, and this file's
	// own type table already escapes the same value.
	fmt.Fprintf(&sb, "- **Type**: %s\n", toolutil.EscapeMdTableCell(wi.Type))
	//gitlab:allow-unescaped wi.State: a work item state from the GraphQL WorkItemState enum (OPEN, CLOSED), never text anybody types.
	fmt.Fprintf(&sb, toolutil.FmtMdState, wi.State)
	if wi.Status != "" {
		// The status widget carries the display name of a status in the
		// namespace's lifecycle, which an administrator can create and rename.
		fmt.Fprintf(&sb, "- **Status**: %s\n", toolutil.EscapeMdTableCell(wi.Status))
	}
	if name := authorName(wi.Author); name != "" {
		fmt.Fprintf(&sb, toolutil.FmtMdAuthor, toolutil.EscapeMdTableCell(name))
	}
	if len(wi.Assignees) > 0 {
		fmt.Fprintf(&sb, "- **Assignees**: %s\n", toolutil.EscapeMdTableCell(strings.Join(assigneeNames(wi.Assignees), ", ")))
	}
	if len(wi.Labels) > 0 {
		// A label title is free text: GitLab's only rule on one is that it
		// carries no comma.
		fmt.Fprintf(&sb, "- **Labels**: %s\n", toolutil.EscapeMdTableCell(strings.Join(labelNames(wi.Labels), ", ")))
	}
	writeWidgetLines(&sb, wi)
	if wi.WebURL != "" {
		toolutil.WriteMdURL(&sb, wi.WebURL)
	}
	if wi.Description != "" {
		fmt.Fprintf(&sb, "\n### Description\n\n%s\n", wi.Description)
	}
	if len(wi.LinkedItems) > 0 {
		sb.WriteString("\n### Linked Items\n\n")
		sb.WriteString("| IID | Link Type | Path |\n")
		sb.WriteString("|-----|-----------|------|\n")
		for _, li := range wi.LinkedItems {
			//gitlab:allow-unescaped li.LinkType: a link type from GitLab's own closed set (blocks, is_blocked_by, relates_to).
			fmt.Fprintf(&sb, "| %d | %s | %s |\n", li.IID, li.LinkType, toolutil.EscapeMdTableCell(li.Path))
		}
	}
	if len(wi.Children) > 0 {
		sb.WriteString("\n### Children\n\n")
		sb.WriteString("| IID | Path |\n")
		sb.WriteString("|-----|------|\n")
		for _, c := range wi.Children {
			fmt.Fprintf(&sb, "| %d | %s |\n", c.IID, toolutil.EscapeMdTableCell(c.Path))
		}
	}
	toolutil.WriteHints(&sb, "Use `gitlab_update_work_item` to modify this work item")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// authorName is the handle the rendered text names an author by, and the empty
// string for a work item whose author the query did not ask for.
func authorName(author *toolutil.BasicUserOutput) string {
	if author == nil {
		return ""
	}
	return author.Username
}

// assigneeNames flattens the assignee objects to the usernames the Markdown
// prints.
//
// The JSON keeps the whole objects, which is what the 1:1 norm asks for; a
// reader of the rendered text wants the handles, and seven fields per assignee
// on one bullet line would bury the item they belong to.
func assigneeNames(assignees []*toolutil.BasicUserOutput) []string {
	names := make([]string, 0, len(assignees))
	for _, assignee := range assignees {
		names = append(names, assignee.Username)
	}
	return names
}

// labelNames flattens the label objects to the titles the Markdown prints, for
// the same reason [assigneeNames] does.
func labelNames(labels []*toolutil.LabelDetailsOutput) []string {
	names := make([]string, 0, len(labels))
	for _, label := range labels {
		names = append(names, label.Name)
	}
	return names
}

// writeWidgetLines renders the widget-backed values of a work item, each of
// which is absent from a type that has no such widget and from a list answer
// that did not ask for it.
//
// The parent goes here rather than beside the Children table because it is one
// value, not a list, and a one-row table would read worse than a bullet.
func writeWidgetLines(sb *strings.Builder, wi WorkItemItem) {
	if wi.Parent != nil {
		fmt.Fprintf(sb, "- **Parent**: #%d in %s\n", wi.Parent.IID, toolutil.EscapeMdTableCell(wi.Parent.Path))
	}
	if wi.MilestoneID != 0 {
		fmt.Fprintf(sb, "- **Milestone ID**: %d\n", wi.MilestoneID)
	}
	if wi.IterationID != 0 {
		fmt.Fprintf(sb, "- **Iteration ID**: %d\n", wi.IterationID)
	}
	if wi.Weight != nil {
		fmt.Fprintf(sb, "- **Weight**: %d\n", *wi.Weight)
	}
	if wi.HealthStatus != "" {
		//gitlab:allow-unescaped wi.HealthStatus: a value of the GraphQL HealthStatus enum (onTrack, needsAttention, atRisk), never text anybody types.
		fmt.Fprintf(sb, "- **Health Status**: %s\n", wi.HealthStatus)
	}
	if wi.StartDate != "" {
		//gitlab:allow-unescaped wi.StartDate: a date this package formatted itself from a gl.ISOTime, so it is YYYY-MM-DD or nothing.
		fmt.Fprintf(sb, "- **Start Date**: %s\n", wi.StartDate)
	}
	if wi.DueDate != "" {
		//gitlab:allow-unescaped wi.DueDate: a date this package formatted itself from a gl.ISOTime, so it is YYYY-MM-DD or nothing.
		fmt.Fprintf(sb, "- **Due Date**: %s\n", wi.DueDate)
	}
	if wi.Color != "" {
		// GitLab accepts a named CSS color as well as a hex code, so this is
		// not the closed set the hex-only jsonschema example suggests.
		fmt.Fprintf(sb, "- **Color**: %s\n", toolutil.EscapeMdTableCell(wi.Color))
	}
}

// FormatListMarkdown formats a list of work items as markdown.
//
// The empty result carries its own hint because GitLab answers a namespace that
// does not exist, or that the token cannot see, with a null namespace rather
// than an error: the list handler cannot tell that apart from a namespace with
// no matching work items, so the reader is told to check the path.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	var sb strings.Builder
	if len(out.WorkItems) == 0 {
		sb.WriteString("No work items found.\n")
		toolutil.WriteHints(&sb, "If work items were expected, verify full_path with `gitlab_project_list` or `gitlab_group_list`: a namespace that does not exist, or that the token cannot read, also lists no work items")
		return toolutil.ToolResultWithMarkdown(sb.String())
	}
	fmt.Fprintf(&sb, "## Work Items (%d)\n\n", len(out.WorkItems))
	sb.WriteString("| IID | Type | State | Status | Title | Author |\n")
	sb.WriteString("|-----|------|-------|--------|-------|--------|\n")
	for _, wi := range out.WorkItems {
		fmt.Fprintf(&sb, "| %d | %s | %s | %s | %s | %s |\n",
			wi.IID, toolutil.EscapeMdTableCell(wi.Type), wi.State, toolutil.EscapeMdTableCell(wi.Status),
			toolutil.EscapeMdTableCell(wi.Title), toolutil.EscapeMdTableCell(authorName(wi.Author)))
	}
	if out.Pagination.HasNextPage {
		fmt.Fprintf(&sb, "\n> Next page cursor: `%s`\n", out.Pagination.EndCursor)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_get_work_item` to view full details of a specific item")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatWorkItemTypeListMarkdown formats a list of work item types as a Markdown table.
func FormatWorkItemTypeListMarkdown(out WorkItemTypeListOutput) *mcp.CallToolResult {
	if len(out.Types) == 0 {
		return toolutil.ToolResultWithMarkdown("No work item types found.\n")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Work Item Types (%d)\n\n", len(out.Types))
	sb.WriteString("| Name | ID | Enabled |\n")
	sb.WriteString("|------|----|---------|\n")
	for _, t := range out.Types {
		fmt.Fprintf(&sb, "| %s | `%s` | %v |\n",
			toolutil.EscapeMdTableCell(t.Name), toolutil.EscapeMdTableCell(t.ID), t.Enabled)
	}
	if out.Pagination.HasNextPage {
		fmt.Fprintf(&sb, "\n> Next page cursor: `%s`\n", out.Pagination.EndCursor)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_create_work_item` with work_item_type_id from the ID column to create work items of this type")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatGetMarkdown)
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatWorkItemTypeListMarkdown)
}
