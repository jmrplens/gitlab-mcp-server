package workitems

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name: the one form every surface resolves,
// where an individual tool name is a name two of the three surfaces do not
// register.
// Work items are routes on the issue catalog group, so every ID is namespaced
// under the issue domain, which is what a caller passes to
// gitlab_execute_action and what the meta and individual surfaces resolve to
// their own names.
const (
	hintActionWorkItemGet    = "issue.work_item_get"
	hintActionWorkItemUpdate = "issue.work_item_update"
	hintActionWorkItemCreate = "issue.work_item_create"
)

// FormatGetMarkdown renders one work item as a card: its own fields, then the
// description as quoted prose, then the hierarchy and the links it carries as
// nested collections.
func FormatGetMarkdown(out GetOutput) *mcp.CallToolResult {
	wi := out.WorkItem
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Work Item #%d: %s", wi.IID, wi.Title))
	c.Field("Type", wi.Type)
	c.Markdown("State", stateCell(wi.State))
	// The status widget carries the display name of a status in the
	// namespace's lifecycle, which an administrator can create and rename.
	c.Field("Status", wi.Status)
	c.Flag(toolutil.EmojiConfidential, "Confidential", wi.Confidential)
	c.Markdown("Author", toolutil.MdUserHandle(authorName(wi.Author)))
	c.Markdown("Assignees", handleList(assigneeNames(wi.Assignees)))
	// A label title is free text: GitLab's only rule on one is that it carries
	// no comma.
	c.Field("Labels", strings.Join(labelNames(wi.Labels), ", "))
	writeWidgetRows(c, wi)
	c.URL(wi.WebURL)
	c.Text("Description", wi.Description)
	writeLinkedItems(c, wi)
	writeChildren(c, wi)
	c.End(toolutil.HintAction(hintActionWorkItemUpdate, "modify this work item"))
	return toolutil.ToolResultWithMarkdown(b.String())
}

// writeLinkedItems writes the items linked to this one as a nested collection.
func writeLinkedItems(c *toolutil.Card, wi WorkItemItem) {
	if len(wi.LinkedItems) == 0 {
		return
	}
	t := c.Table("Linked Items", "IID", "Link Type", "Path")
	for _, li := range wi.LinkedItems {
		t.Row(
			strconv.FormatInt(li.IID, 10),
			toolutil.EscapeMdTableCell(li.LinkType),
			toolutil.EscapeMdTableCell(li.Path),
		)
	}
}

// writeChildren writes the work items under this one as a nested collection.
func writeChildren(c *toolutil.Card, wi WorkItemItem) {
	if len(wi.Children) == 0 {
		return
	}
	t := c.Table("Children", "IID", "Path")
	for _, child := range wi.Children {
		t.Row(strconv.FormatInt(child.IID, 10), toolutil.EscapeMdTableCell(child.Path))
	}
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

// handleList renders usernames as the "@handle" list a card row shows, each
// escaped, and nothing at all when there are none.
func handleList(names []string) string {
	handles := make([]string, 0, len(names))
	for _, name := range names {
		if handle := toolutil.MdUserHandle(name); handle != "" {
			handles = append(handles, handle)
		}
	}
	return strings.Join(handles, ", ")
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

// stateCell renders a work item state with the emoji every issue row in the
// tree shows.
//
// GitLab spells a work item's state in the capitals of the GraphQL
// WorkItemState enum (OPEN, CLOSED) where a REST issue sends opened and closed,
// and the shared emoji table reads the REST spelling. The lookup is therefore
// made on the normalized spelling while the value is shown as GitLab sent it,
// so the rendered text never claims a state GitLab did not.
func stateCell(state string) string {
	if strings.TrimSpace(state) == "" {
		return ""
	}
	return toolutil.IssueStateEmoji(normalizedState(state)) + " " + toolutil.EscapeMdTableCell(state)
}

// normalizedState maps either spelling of a work item state onto the REST one
// the emoji table is keyed by, and leaves anything else alone.
func normalizedState(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "OPEN", "OPENED":
		return "opened"
	case "CLOSED":
		return "closed"
	default:
		return state
	}
}

// writeWidgetRows writes the widget-backed values of a work item, each of
// which is absent from a type that has no such widget and from a list answer
// that did not ask for it.
//
// The parent is a nested object rather than a reference with a sigil: a work
// item's parent may be an epic, which GitLab writes &N, or an issue, which it
// writes #N, and the query does not say which, so the card names the two
// values it does have instead of picking a sigil that is wrong half the time.
func writeWidgetRows(c *toolutil.Card, wi WorkItemItem) {
	if wi.Parent != nil {
		parent := c.Sub("Parent")
		parent.Int("IID", wi.Parent.IID)
		parent.Field("Path", wi.Parent.Path)
	}
	c.Count("Milestone ID", wi.MilestoneID)
	c.Count("Iteration ID", wi.IterationID)
	if wi.Weight != nil {
		c.Int("Weight", *wi.Weight)
	}
	c.Field("Health Status", wi.HealthStatus)
	c.Time("Start Date", wi.StartDate)
	c.Time("Due Date", wi.DueDate)
	// GitLab accepts a named CSS color as well as a hex code, so this is not
	// the closed set the hex-only jsonschema example suggests.
	c.Field("Color", wi.Color)
}

// FormatListMarkdown renders a page of work items as a Markdown table.
//
// The empty result carries its own hint because GitLab answers a namespace that
// does not exist, or that the token cannot see, with a null namespace rather
// than an error: the list handler cannot tell that apart from a namespace with
// no matching work items, so the reader is told to check the path.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	var sb strings.Builder
	if len(out.WorkItems) == 0 {
		sb.WriteString(toolutil.EmptyMessage("work items"))
		toolutil.WriteHints(&sb, "If work items were expected, verify full_path with `gitlab_project_list` or `gitlab_group_list`: a namespace that does not exist, or that the token cannot read, also lists no work items")
		return toolutil.ToolResultWithMarkdown(sb.String())
	}
	// A cursor connection counts nothing it has not walked, so the heading
	// carries the count shown and the cursor line says whether more follow.
	toolutil.WriteListHeading(&sb, "Work Items", len(out.WorkItems), toolutil.PaginationOutput{})
	sb.WriteString(toolutil.MarkdownTableHeader("IID", "Type", "State", "Status", "Title", "Author"))
	linked := false
	for _, wi := range out.WorkItems {
		linked = linked || wi.WebURL != ""
		sb.WriteString(toolutil.MarkdownTableRow(
			referenceCell(wi),
			toolutil.EscapeMdTableCell(wi.Type),
			stateCell(wi.State),
			toolutil.EscapeMdTableCell(wi.Status),
			toolutil.EscapeMdTableCell(wi.Title),
			toolutil.MdUserHandle(authorName(wi.Author)),
		))
	}
	toolutil.WriteGraphQLPagination(&sb, out.Pagination, len(out.WorkItems))
	writeListHints(&sb, linked, toolutil.HintAction(hintActionWorkItemGet, "view full details of a specific item"))
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// referenceCell renders a work item's reference, linked to it when the query
// asked for the address, with the confidential marker a restricted item
// carries: without it a reader cannot tell a restricted item from an open one.
func referenceCell(wi WorkItemItem) string {
	cell := toolutil.MdTitleLink(fmt.Sprintf("#%d", wi.IID), wi.WebURL)
	if wi.Confidential {
		cell += " " + toolutil.EmojiConfidential
	}
	return cell
}

// writeListHints closes a cursor-paginated list, asking the model to keep the
// links only when the table carried some.
func writeListHints(sb *strings.Builder, linked bool, hints ...string) {
	if linked {
		toolutil.WriteHints(sb, toolutil.ListHints(hints...)...)
		return
	}
	toolutil.WriteHints(sb, hints...)
}

// FormatWorkItemTypeListMarkdown renders a namespace's work item types as a
// Markdown table.
func FormatWorkItemTypeListMarkdown(out WorkItemTypeListOutput) *mcp.CallToolResult {
	if len(out.Types) == 0 {
		return toolutil.ToolResultWithMarkdown(toolutil.EmptyMessage("work item types"))
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Work Item Types", len(out.Types), toolutil.PaginationOutput{})
	sb.WriteString(toolutil.MarkdownTableHeader("Name", "ID", "Enabled"))
	for _, t := range out.Types {
		sb.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(t.Name),
			toolutil.MdCodeSpanCell(t.ID),
			toolutil.BoolEmoji(t.Enabled),
		))
	}
	toolutil.WriteGraphQLPagination(&sb, out.Pagination, len(out.Types))
	toolutil.WriteHints(&sb, toolutil.HintAction(hintActionWorkItemCreate, "create work items of a type, with the work_item_type_id from the ID column"))
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatGetMarkdown)
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatWorkItemTypeListMarkdown)
}
