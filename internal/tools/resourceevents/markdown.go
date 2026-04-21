// markdown.go provides Markdown formatting functions for resource event MCP tool output.

package resourceevents

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatLabelEventsMarkdown formats a list of label events.
func FormatLabelEventsMarkdown(out ListLabelEventsOutput) string {
	if len(out.Events) == 0 {
		return "No label events found.\n"
	}
	var sb strings.Builder
	sb.WriteString("## Label Events\n\n| ID | Action | Label | User | Date |\n|---|---|---|---|---|\n")
	for _, e := range out.Events {
		fmt.Fprintf(&sb, fmtEventTableRow, e.ID, e.Action, e.Label.Name, e.Username, toolutil.FormatTime(e.CreatedAt))
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
	fmt.Fprintf(&sb, "| Label | %s |\n", out.Label.Name)
	fmt.Fprintf(&sb, fmtUserRow, out.Username)
	fmt.Fprintf(&sb, fmtResourceRow, out.ResourceType, out.ResourceID)
	fmt.Fprintf(&sb, fmtCreatedRow, toolutil.FormatTime(out.CreatedAt))
	toolutil.WriteHints(&sb, "Use `gitlab_list_label_events` to see all label changes")
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
		fmt.Fprintf(&sb, fmtEventTableRow, e.ID, e.Action, e.MilestoneTitle, e.Username, toolutil.FormatTime(e.CreatedAt))
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
	fmt.Fprintf(&sb, "| Milestone | %s (ID: %d) |\n", out.MilestoneTitle, out.MilestoneID)
	fmt.Fprintf(&sb, fmtUserRow, out.Username)
	fmt.Fprintf(&sb, fmtResourceRow, out.ResourceType, out.ResourceID)
	fmt.Fprintf(&sb, fmtCreatedRow, toolutil.FormatTime(out.CreatedAt))
	toolutil.WriteHints(&sb, "Use `gitlab_list_milestone_events` to see all milestone changes")
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
		fmt.Fprintf(&sb, "| %d | %s | %s | %s #%d | %s |\n", e.ID, e.State, e.Username, e.ResourceType, e.ResourceID, toolutil.FormatTime(e.CreatedAt))
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
	fmt.Fprintf(&sb, fmtUserRow, out.Username)
	fmt.Fprintf(&sb, fmtResourceRow, out.ResourceType, out.ResourceID)
	fmt.Fprintf(&sb, fmtCreatedRow, toolutil.FormatTime(out.CreatedAt))
	toolutil.WriteHints(&sb, "Use `gitlab_list_state_events` to see all state changes")
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
		fmt.Fprintf(&sb, fmtEventTableRow, e.ID, e.Action, e.Iteration.Title, e.Username, toolutil.FormatTime(e.CreatedAt))
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
	fmt.Fprintf(&sb, "| Iteration | %s (ID: %d) |\n", out.Iteration.Title, out.Iteration.ID)
	fmt.Fprintf(&sb, fmtUserRow, out.Username)
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
		fmt.Fprintf(&sb, "| %d | %d | %s | %s #%d | %s |\n", e.ID, e.Weight, e.Username, e.ResourceType, e.ResourceID, toolutil.FormatTime(e.CreatedAt))
	}
	toolutil.WriteHints(&sb, "Use filters to narrow down weight events by date")
	return sb.String()
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
