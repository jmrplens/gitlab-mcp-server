package resourceevents

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// The values below reach a cell in this file with no escaper on them, because
// each is a word GitLab picked from a fixed set rather than text anybody typed.
// The five event kinds share them, so one declaration covers every table here.
//
//gitlab:allow-unescaped e.Action: a resource event action GitLab writes, either add or remove.
//gitlab:allow-unescaped out.Action: a resource event action GitLab writes, either add or remove.
//gitlab:allow-unescaped e.State: the state a state event recorded, one of GitLab's own set (closed, opened, merged).
//gitlab:allow-unescaped out.State: the state a state event recorded, one of GitLab's own set (closed, opened, merged).
//gitlab:allow-unescaped e.ResourceType: the kind of thing the event hangs on, which GitLab names Issue, MergeRequest or Epic.
//gitlab:allow-unescaped out.ResourceType: the kind of thing the event hangs on, which GitLab names Issue, MergeRequest or Epic.

// FormatLabelEventsMarkdown formats a list of label events.
func FormatLabelEventsMarkdown(out ListLabelEventsOutput) string {
	if len(out.Events) == 0 {
		return "No label events found.\n"
	}
	var sb strings.Builder
	sb.WriteString("## Label Events\n\n| ID | Action | Label | User | Date |\n|---|---|---|---|---|\n")
	for _, e := range out.Events {
		fmt.Fprintf(&sb, fmtEventTableRow, e.ID, e.Action, labelName(e.Label), eventUsername(e.User), toolutil.FormatTime(e.CreatedAt))
	}
	toolutil.WriteHints(&sb, "Use filters to narrow down label events by date or action")
	return sb.String()
}

// FormatLabelEventMarkdown formats a single label event.
func FormatLabelEventMarkdown(out LabelEventOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Label Event #%d\n\n", out.ID)
	sb.WriteString(fmtPropertyValueTableHeader)
	fmt.Fprintf(&sb, fmtActionRow, out.Action)
	fmt.Fprintf(&sb, "| Label | %s |\n", labelName(out.Label))
	fmt.Fprintf(&sb, fmtUserRow, eventUsername(out.User))
	fmt.Fprintf(&sb, fmtResourceRow, out.ResourceType, out.ResourceID)
	fmt.Fprintf(&sb, fmtCreatedRow, toolutil.FormatTime(out.CreatedAt))
	toolutil.WriteHints(&sb, "Use `gitlab_issue_label_event_list` or `gitlab_mr_label_event_list` to see all label changes")
	return sb.String()
}

// FormatMilestoneEventsMarkdown formats a list of milestone events.
func FormatMilestoneEventsMarkdown(out ListMilestoneEventsOutput) string {
	if len(out.Events) == 0 {
		return "No milestone events found.\n"
	}
	var sb strings.Builder
	sb.WriteString("## Milestone Events\n\n| ID | Action | Milestone | User | Date |\n|---|---|---|---|---|\n")
	for _, e := range out.Events {
		fmt.Fprintf(&sb, fmtEventTableRow, e.ID, e.Action, milestoneTitle(e.Milestone), eventUsername(e.User), toolutil.FormatTime(e.CreatedAt))
	}
	toolutil.WriteHints(&sb, "Use filters to narrow down milestone events by date or action")
	return sb.String()
}

// FormatMilestoneEventMarkdown formats a single milestone event.
func FormatMilestoneEventMarkdown(out MilestoneEventOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Milestone Event #%d\n\n", out.ID)
	sb.WriteString(fmtPropertyValueTableHeader)
	fmt.Fprintf(&sb, fmtActionRow, out.Action)
	fmt.Fprintf(&sb, "| Milestone | %s (ID: %d) |\n", milestoneTitle(out.Milestone), milestoneID(out.Milestone))
	fmt.Fprintf(&sb, fmtUserRow, eventUsername(out.User))
	fmt.Fprintf(&sb, fmtResourceRow, out.ResourceType, out.ResourceID)
	fmt.Fprintf(&sb, fmtCreatedRow, toolutil.FormatTime(out.CreatedAt))
	toolutil.WriteHints(&sb, "Use `gitlab_issue_milestone_event_list` or `gitlab_mr_milestone_event_list` to see all milestone changes")
	return sb.String()
}

// FormatStateEventsMarkdown formats a list of state events.
func FormatStateEventsMarkdown(out ListStateEventsOutput) string {
	if len(out.Events) == 0 {
		return "No state events found.\n"
	}
	var sb strings.Builder
	sb.WriteString("## State Events\n\n| ID | State | User | Resource | Date |\n|---|---|---|---|---|\n")
	for _, e := range out.Events {
		fmt.Fprintf(&sb, "| %d | %s | %s | %s #%d | %s |\n", e.ID, e.State, eventUsername(e.User), e.ResourceType, e.ResourceID, toolutil.FormatTime(e.CreatedAt))
	}
	toolutil.WriteHints(&sb, "Use filters to narrow down state events by date or action")
	return sb.String()
}

// FormatStateEventMarkdown formats a single state event.
func FormatStateEventMarkdown(out StateEventOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## State Event #%d\n\n", out.ID)
	sb.WriteString(fmtPropertyValueTableHeader)
	fmt.Fprintf(&sb, "| State | %s |\n", out.State)
	fmt.Fprintf(&sb, fmtUserRow, eventUsername(out.User))
	fmt.Fprintf(&sb, fmtResourceRow, out.ResourceType, out.ResourceID)
	fmt.Fprintf(&sb, fmtCreatedRow, toolutil.FormatTime(out.CreatedAt))
	toolutil.WriteHints(&sb, "Use `gitlab_issue_state_event_list` or `gitlab_mr_state_event_list` to see all state changes")
	return sb.String()
}

// FormatIterationEventsMarkdown formats a list of iteration events.
func FormatIterationEventsMarkdown(out ListIterationEventsOutput) string {
	if len(out.Events) == 0 {
		return "No iteration events found.\n"
	}
	var sb strings.Builder
	sb.WriteString("## Iteration Events\n\n| ID | Action | Iteration | User | Date |\n|---|---|---|---|---|\n")
	for _, e := range out.Events {
		fmt.Fprintf(&sb, fmtEventTableRow, e.ID, e.Action, iterationTitle(e.Iteration), eventUsername(e.User), toolutil.FormatTime(e.CreatedAt))
	}
	toolutil.WriteHints(&sb, "Use filters to narrow down iteration events by date or action")
	return sb.String()
}

// FormatIterationEventMarkdown formats a single iteration event.
func FormatIterationEventMarkdown(out IterationEventOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Iteration Event #%d\n\n", out.ID)
	sb.WriteString(fmtPropertyValueTableHeader)
	fmt.Fprintf(&sb, fmtActionRow, out.Action)
	fmt.Fprintf(&sb, "| Iteration | %s (ID: %d) |\n", iterationTitle(out.Iteration), iterationID(out.Iteration))
	fmt.Fprintf(&sb, fmtUserRow, eventUsername(out.User))
	fmt.Fprintf(&sb, fmtResourceRow, out.ResourceType, out.ResourceID)
	fmt.Fprintf(&sb, fmtCreatedRow, toolutil.FormatTime(out.CreatedAt))
	toolutil.WriteHints(&sb, "Use `gitlab_issue_iteration_event_list` to see all iteration changes")
	return sb.String()
}

// FormatWeightEventsMarkdown formats a list of weight events.
func FormatWeightEventsMarkdown(out ListWeightEventsOutput) string {
	if len(out.Events) == 0 {
		return "No weight events found.\n"
	}
	var sb strings.Builder
	sb.WriteString("## Weight Events\n\n| ID | Weight | User | Resource | Date |\n|---|---|---|---|---|\n")
	for _, e := range out.Events {
		fmt.Fprintf(&sb, "| %d | %d | %s | %s #%d | %s |\n", e.ID, e.Weight, eventUsername(e.User), e.ResourceType, e.ResourceID, toolutil.FormatTime(e.CreatedAt))
	}
	toolutil.WriteHints(&sb, "Use filters to narrow down weight events by date")
	return sb.String()
}

// eventUsername returns the username of an event's user as a table cell, or ""
// when absent.
//
// This accessor and the three below exist only to fill the cells of this
// file's tables, so each escapes what it returns rather than leaving the
// question to a dozen call sites. A label name and the two titles are text a
// person typed, and GitLab's only rule on a label name is that it holds no
// comma.
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
