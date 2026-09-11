package epics

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name: the one form every surface resolves,
// where an individual tool name is a name two of the three surfaces do not
// register. Epics are routes on the group catalog group, so every ID is
// namespaced under the group domain, which is what a caller passes to
// gitlab_execute_action and what the meta and individual surfaces resolve to
// their own names.
const (
	hintActionEpicGet      = "group.epic_get"
	hintActionEpicList     = "group.epic_list"
	hintActionEpicCreate   = "group.epic_create"
	hintActionEpicUpdate   = "group.epic_update"
	hintActionEpicGetLinks = "group.epic_get_links"
	hintActionEpicNoteList = "group.epic_note_list"
)

// userName returns the username of a nested user object, or "" when nil.
func userName(u *BasicUserOutput) string {
	if u == nil {
		return ""
	}
	return u.Username
}

// userNames maps a slice of nested user objects to their usernames, skipping
// nil entries.
func userNames(users []*BasicUserOutput) []string {
	if len(users) == 0 {
		return nil
	}
	names := make([]string, 0, len(users))
	for _, u := range users {
		if u != nil {
			names = append(names, u.Username)
		}
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

// stateCell renders an epic state with the emoji every issue row in the tree
// shows.
//
// One epic reaches this formatter under two spellings: the REST epics endpoint
// sends opened and closed, the Work Items query OPEN and CLOSED, and the shared
// emoji table is keyed by the REST one. The lookup is made on the normalized
// spelling and the value is shown as GitLab sent it, so nothing here rewrites
// what the output struct carries.
func stateCell(state string) string {
	if strings.TrimSpace(state) == "" {
		return ""
	}
	return toolutil.IssueStateEmoji(normalizedState(state)) + " " + toolutil.EscapeMdTableCell(state)
}

// normalizedState maps either spelling of an epic state onto the REST one the
// emoji table is keyed by, and leaves anything else alone.
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

// FormatOutputMarkdown renders a single epic as a card: its own fields, then
// the description as quoted prose, then the hierarchy it carries as nested
// collections.
func FormatOutputMarkdown(e Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Epic &%d: %s", e.IID, e.Title))
	c.Markdown("State", stateCell(e.State))
	c.Markdown("Author", toolutil.MdUserHandle(userName(e.Author)))
	c.Markdown("Assignees", handleList(userNames(e.Assignees)))
	c.Flag(toolutil.EmojiConfidential, "Confidential", e.Confidential)
	// A label title is free text: GitLab's only rule on one is that it carries
	// no comma.
	c.Field("Labels", strings.Join(e.Labels, ", "))
	c.Field("Health", e.HealthStatus)
	if e.Weight != nil {
		c.Int("Weight", *e.Weight)
	}
	if e.MilestoneID != nil {
		c.Int("Milestone ID", *e.MilestoneID)
	}
	c.Time("Start date", e.StartDate)
	c.Time("Due date", e.DueDate)
	c.Field("Color", e.Color)
	if e.ParentIID > 0 {
		parent := c.Sub("Parent")
		parent.Int("IID", e.ParentIID)
		parent.Field("Path", e.ParentPath)
	}
	c.Time("Created", e.CreatedAt)
	c.Time("Closed", e.ClosedAt)
	c.URL(e.WebURL)
	c.Text("Description", e.Description)
	writeLinkedItems(c, e.LinkedItems)
	writeChildren(c, e.Children)
	c.End(
		toolutil.HintAction(hintActionEpicUpdate, "modify this epic"),
		toolutil.HintAction(hintActionEpicGetLinks, "see child epics"),
		toolutil.HintAction(hintActionEpicNoteList, "see comments on this epic"),
	)
	return b.String()
}

// writeLinkedItems writes the work items linked to an epic as a nested
// collection.
func writeLinkedItems(c *toolutil.Card, items []LinkedItem) {
	if len(items) == 0 {
		return
	}
	t := c.Table("Linked Items", "IID", "Link Type", "Path")
	for _, li := range items {
		t.Row(
			strconv.FormatInt(li.IID, 10),
			toolutil.EscapeMdTableCell(li.LinkType),
			toolutil.EscapeMdTableCell(li.Path),
		)
	}
}

// writeChildren writes the epics under one epic as a nested collection.
func writeChildren(c *toolutil.Card, children []ChildItem) {
	if len(children) == 0 {
		return
	}
	t := c.Table("Child Epics", "IID", "Path")
	for _, child := range children {
		t.Row(fmt.Sprintf("&%d", child.IID), toolutil.EscapeMdTableCell(child.Path))
	}
}

// FormatListMarkdown renders a group's epics as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Epics) == 0 {
		return toolutil.EmptyMessage("epics")
	}
	var b strings.Builder
	offset := toolutil.PaginationOutput{}
	if out.OffsetPagination != nil {
		offset = *out.OffsetPagination
	}
	toolutil.WriteListHeading(&b, "Group Epics", len(out.Epics), offset)
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Author", "Labels", "Created"))
	for _, e := range out.Epics {
		b.WriteString(toolutil.MarkdownTableRow(
			epicReferenceCell(e.IID, e.WebURL, e.Confidential),
			toolutil.MdTitleLink(e.Title, e.WebURL),
			stateCell(e.State),
			toolutil.MdUserHandle(userName(e.Author)),
			toolutil.EscapeMdTableCell(strings.Join(e.Labels, ", ")),
			toolutil.FormatTime(e.CreatedAt),
		))
	}
	// Exactly one block is ever set, and which one says which API answered:
	// the cursor pair belongs to the Work Items query, the page numbers to the
	// REST epics endpoint.
	if out.Pagination != nil {
		toolutil.WriteGraphQLPagination(&b, *out.Pagination, len(out.Epics))
	}
	toolutil.WriteListFooter(&b, offset, true,
		toolutil.HintAction(hintActionEpicGet, "see full details of one epic"),
		toolutil.HintAction(hintActionEpicCreate, "add a new epic"),
	)
	return b.String()
}

// FormatLinksMarkdown renders the child epics of one epic as a Markdown table.
func FormatLinksMarkdown(out LinksOutput) string {
	if len(out.ChildEpics) == 0 {
		return toolutil.EmptyMessage("child epics")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Child Epics", len(out.ChildEpics), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Author", "Created"))
	for _, e := range out.ChildEpics {
		b.WriteString(toolutil.MarkdownTableRow(
			epicReferenceCell(e.IID, e.WebURL, e.Confidential),
			toolutil.MdTitleLink(e.Title, e.WebURL),
			stateCell(e.State),
			toolutil.MdUserHandle(userName(e.Author)),
			toolutil.FormatTime(e.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, true,
		toolutil.HintAction(hintActionEpicGet, "see one child epic in full"),
		toolutil.HintAction(hintActionEpicList, "list the group's epics"),
	)
	return b.String()
}

// epicReferenceCell renders an epic's reference in GitLab's own &N spelling,
// linked to the epic, with the confidential marker a restricted epic carries:
// without it a reader cannot tell a restricted epic from an open one.
func epicReferenceCell(iid int64, webURL string, confidential bool) string {
	cell := toolutil.MdTitleLink(fmt.Sprintf("&%d", iid), webURL)
	if confidential {
		cell += " " + toolutil.EmojiConfidential
	}
	return cell
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatLinksMarkdown)
}
