package resourceevents

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Every value GitLab sends here reaches the page through an escaper: the card
// rows through [toolutil.Card], which escapes what it writes, and the table
// cells through [toolutil.EscapeMdTableCell] at the row. The five event kinds
// used to interpolate the action, the state and the resource type raw, under
// declarations saying each was a word from a fixed set, and a word from a fixed
// set survives the escaper unchanged, so the declarations bought nothing and
// are gone.

// FormatLabelEventsMarkdown formats a list of label events.
func FormatLabelEventsMarkdown(out ListLabelEventsOutput) string {
	if len(out.Events) == 0 {
		return toolutil.EmptyMessage("label events")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Label Events", len(out.Events), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Action", "Label", "User", "Date"))
	for _, e := range out.Events {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(e.ID, 10),
			toolutil.EscapeMdTableCell(e.Action),
			labelName(e.Label),
			eventUsername(e.User),
			toolutil.FormatTime(e.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false, "Use filters to narrow down label events by date or action")
	return b.String()
}

// FormatLabelEventMarkdown formats a single label event as a card.
func FormatLabelEventMarkdown(out LabelEventOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Label Event #%d", out.ID))
	c.Field("Action", out.Action)
	c.Field("Label", labelName(out.Label))
	c.Field("User", eventUsername(out.User))
	writeEventResource(c, out.ResourceType, out.ResourceID)
	c.Time("Created", out.CreatedAt)
	c.End("Use `gitlab_issue_label_event_list` or `gitlab_mr_label_event_list` to see all label changes")
	return b.String()
}

// FormatMilestoneEventsMarkdown formats a list of milestone events.
func FormatMilestoneEventsMarkdown(out ListMilestoneEventsOutput) string {
	if len(out.Events) == 0 {
		return toolutil.EmptyMessage("milestone events")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Milestone Events", len(out.Events), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Action", "Milestone", "User", "Date"))
	for _, e := range out.Events {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(e.ID, 10),
			toolutil.EscapeMdTableCell(e.Action),
			milestoneTitle(e.Milestone),
			eventUsername(e.User),
			toolutil.FormatTime(e.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false, "Use filters to narrow down milestone events by date or action")
	return b.String()
}

// FormatMilestoneEventMarkdown formats a single milestone event as a card. The
// milestone's own ID is a row of its own and is written only when GitLab sent
// one: an event whose milestone has been deleted used to read "(ID: 0)".
func FormatMilestoneEventMarkdown(out MilestoneEventOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Milestone Event #%d", out.ID))
	c.Field("Action", out.Action)
	c.Field("Milestone", milestoneTitle(out.Milestone))
	c.Count("Milestone ID", milestoneID(out.Milestone))
	c.Field("User", eventUsername(out.User))
	writeEventResource(c, out.ResourceType, out.ResourceID)
	c.Time("Created", out.CreatedAt)
	c.End("Use `gitlab_issue_milestone_event_list` or `gitlab_mr_milestone_event_list` to see all milestone changes")
	return b.String()
}

// FormatStateEventsMarkdown formats a list of state events.
func FormatStateEventsMarkdown(out ListStateEventsOutput) string {
	if len(out.Events) == 0 {
		return toolutil.EmptyMessage("state events")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "State Events", len(out.Events), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "State", "User", "Resource", "Date"))
	for _, e := range out.Events {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(e.ID, 10),
			toolutil.EscapeMdTableCell(e.State),
			eventUsername(e.User),
			resourceReference(e.ResourceType, e.ResourceID),
			toolutil.FormatTime(e.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false, "Use filters to narrow down state events by date or action")
	return b.String()
}

// FormatStateEventMarkdown formats a single state event as a card.
func FormatStateEventMarkdown(out StateEventOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("State Event #%d", out.ID))
	c.Field("State", out.State)
	c.Field("User", eventUsername(out.User))
	writeEventResource(c, out.ResourceType, out.ResourceID)
	c.Time("Created", out.CreatedAt)
	c.End("Use `gitlab_issue_state_event_list` or `gitlab_mr_state_event_list` to see all state changes")
	return b.String()
}

// FormatIterationEventsMarkdown formats a list of iteration events.
func FormatIterationEventsMarkdown(out ListIterationEventsOutput) string {
	if len(out.Events) == 0 {
		return toolutil.EmptyMessage("iteration events")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Iteration Events", len(out.Events), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Action", "Iteration", "User", "Date"))
	for _, e := range out.Events {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(e.ID, 10),
			toolutil.EscapeMdTableCell(e.Action),
			iterationTitle(e.Iteration),
			eventUsername(e.User),
			toolutil.FormatTime(e.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false, "Use filters to narrow down iteration events by date or action")
	return b.String()
}

// FormatIterationEventMarkdown formats a single iteration event as a card. The
// iteration's own ID is written only when GitLab sent one, on the same terms as
// the milestone's.
func FormatIterationEventMarkdown(out IterationEventOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Iteration Event #%d", out.ID))
	c.Field("Action", out.Action)
	c.Field("Iteration", iterationTitle(out.Iteration))
	c.Count("Iteration ID", iterationID(out.Iteration))
	c.Field("User", eventUsername(out.User))
	writeEventResource(c, out.ResourceType, out.ResourceID)
	c.Time("Created", out.CreatedAt)
	c.End("Use `gitlab_issue_iteration_event_list` to see all iteration changes")
	return b.String()
}

// FormatWeightEventsMarkdown formats a list of weight events.
func FormatWeightEventsMarkdown(out ListWeightEventsOutput) string {
	if len(out.Events) == 0 {
		return toolutil.EmptyMessage("weight events")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Weight Events", len(out.Events), out.Pagination)
	// Issue ID rather than the Resource column its siblings render: the weight
	// entity exposes issue_id and no resource_type or resource_id, so that
	// column read " #0" on every row. The number is written plainly, since the
	// issue_id of a weight event is the issue's database ID and a "#42" reads
	// as the per-project number a reader could look up.
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Weight", "User", "Issue ID", "Date"))
	for _, e := range out.Events {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(e.ID, 10),
			strconv.FormatInt(e.Weight, 10),
			eventUsername(e.User),
			strconv.FormatInt(e.IssueID, 10),
			toolutil.FormatTime(e.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false, "Use filters to narrow down weight events by date")
	return b.String()
}

// writeEventResource writes the object an event hangs on as two rows: the kind
// GitLab names it with, and its database ID when GitLab sent one. The pair used
// to be one "Issue #42" line, whose sigil reads as the per-project number a
// reader could look up while the value is the database ID, and whose zero read
// as a resource that does not exist.
func writeEventResource(c *toolutil.Card, resourceType string, resourceID int64) {
	c.Field("Resource Type", resourceType)
	c.Count("Resource ID", resourceID)
}

// resourceReference renders the same pair as one table cell, "Issue (ID 42)",
// and nothing at all when GitLab named neither.
func resourceReference(resourceType string, resourceID int64) string {
	kind := toolutil.EscapeMdTableCell(resourceType)
	switch {
	case kind == "" && resourceID == 0:
		return ""
	case kind == "":
		return "ID " + strconv.FormatInt(resourceID, 10)
	case resourceID == 0:
		return kind
	default:
		return fmt.Sprintf("%s (ID %d)", kind, resourceID)
	}
}

// eventUsername returns the username of an event's user as a table cell, or ""
// when absent.
//
// This accessor and the three below exist only to fill the cells of this
// file's tables and the rows of its cards, so each escapes what it returns
// rather than leaving the question to a dozen call sites; the card escaper is
// idempotent, so a value reaching a row through one of them renders unchanged.
// A label name and the two titles are text a person typed, and GitLab's only
// rule on a label name is that it holds no comma.
func eventUsername(u *EventUserOutput) string {
	if u == nil {
		return ""
	}
	return toolutil.EscapeMdTableCell(u.Username)
}

// labelName returns a label event label's name as a table cell, or "" when absent.
func labelName(l *LabelEventLabelOutput) string {
	if l == nil {
		return ""
	}
	return toolutil.EscapeMdTableCell(l.Name)
}

// milestoneTitle returns a milestone's title as a table cell, or "" when absent.
func milestoneTitle(m *MilestoneOutput) string {
	if m == nil {
		return ""
	}
	return toolutil.EscapeMdTableCell(m.Title)
}

// milestoneID returns a milestone's id, or 0 when absent.
func milestoneID(m *MilestoneOutput) int64 {
	if m == nil {
		return 0
	}
	return m.ID
}

// iterationTitle returns an iteration's title as a table cell, or "" when absent.
func iterationTitle(it *IterationOutput) string {
	if it == nil {
		return ""
	}
	return toolutil.EscapeMdTableCell(it.Title)
}

// iterationID returns an iteration's id, or 0 when absent.
func iterationID(it *IterationOutput) int64 {
	if it == nil {
		return 0
	}
	return it.ID
}

func init() {
	toolutil.RegisterMarkdown(FormatLabelEventsMarkdown)
	toolutil.RegisterMarkdown(FormatLabelEventMarkdown)
	toolutil.RegisterMarkdown(FormatMilestoneEventsMarkdown)
	toolutil.RegisterMarkdown(FormatMilestoneEventMarkdown)
	toolutil.RegisterMarkdown(FormatStateEventsMarkdown)
	toolutil.RegisterMarkdown(FormatStateEventMarkdown)
	toolutil.RegisterMarkdown(FormatIterationEventsMarkdown)
	toolutil.RegisterMarkdown(FormatIterationEventMarkdown)
	toolutil.RegisterMarkdown(FormatWeightEventsMarkdown)
}
