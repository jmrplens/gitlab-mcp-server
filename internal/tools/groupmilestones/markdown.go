package groupmilestones

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionGet           = "group_milestone.get"
	actionCreate        = "group_milestone.create"
	actionUpdate        = "group_milestone.update"
	actionDelete        = "group_milestone.delete"
	actionIssues        = "group_milestone.issues"
	actionMergeRequests = "group_milestone.merge_requests"
	actionIssueGet      = "issue.get"
	actionMRGet         = "merge_request.get"
)

// FormatMarkdown renders one group milestone as the card of one object.
//
// The hints used to name a milestone_id parameter that no group milestone
// action takes: every one of them is addressed by group_id and milestone_iid,
// which is the IID this card shows.
func FormatMarkdown(v Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Group Milestone #%d: %s", v.IID, v.Title))
	c.Int("ID", v.ID)
	c.Int("IID", v.IID)
	c.Count("Group ID", v.GroupID)
	c.Count("Project ID", v.ProjectID)
	c.Field("State", v.State)
	c.Time("Start Date", v.StartDate)
	c.Time("Due Date", v.DueDate)
	c.Warn("Expired", v.Expired)
	c.URL(v.WebURL)
	c.Time("Created", v.CreatedAt)
	c.Time("Updated", v.UpdatedAt)
	c.Text("Description", v.Description)
	c.End(
		toolutil.HintAction(actionUpdate, "change this milestone, with the same group_id and milestone_iid"),
		toolutil.HintAction(actionIssues, "list its issues, with the same group_id and milestone_iid"),
		toolutil.HintAction(actionMergeRequests, "list its merge requests, with the same group_id and milestone_iid"),
		toolutil.HintAction(actionDelete, "remove it, with the same group_id, milestone_iid and confirm=true"),
	)
	return b.String()
}

// FormatListMarkdownString renders a page of group milestones as a Markdown
// table, with the column set and the date formatting the project scope uses,
// since the two answer the same question about the same entity.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.Milestones) == 0 {
		return toolutil.EmptyMessage("group milestones")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Milestones", len(out.Milestones), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Start Date", "Due Date", "Expired"))
	for _, m := range out.Milestones {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(strconv.FormatInt(m.IID, 10), m.WebURL),
			toolutil.EscapeMdTableCell(m.Title),
			toolutil.EscapeMdTableCell(m.State),
			dateCell(m.StartDate),
			dateCell(m.DueDate),
			toolutil.BoolEmoji(m.Expired),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionGet, "read one milestone by its milestone_iid"),
		toolutil.HintAction(actionCreate, "add a new milestone to the group"),
	)
	return b.String()
}

// dateCell renders a milestone date in the display form, and a dash where
// GitLab sent none, so an empty cell is never mistaken for a date it failed to
// render.
func dateCell(date string) string {
	if date == "" {
		return "-"
	}
	return toolutil.FormatTime(date)
}

// stateCell renders a state word with the glyph its domain gives it, and
// nothing at all when GitLab sent no state.
func stateCell(emoji, state string) string {
	if state == "" {
		return ""
	}
	return emoji + " " + toolutil.EscapeMdTableCell(state)
}

// FormatListMarkdown renders a paginated list of group milestones as an MCP Markdown result.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatIssuesMarkdownString renders a page of a group milestone's issues as a
// Markdown table.
func FormatIssuesMarkdownString(out IssuesOutput) string {
	if len(out.Issues) == 0 {
		return toolutil.EmptyMessage("milestone issues")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Milestone Issues", len(out.Issues), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Created"))
	for _, issue := range out.Issues {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("#%d", issue.IID), issue.WebURL),
			toolutil.EscapeMdTableCell(issue.Title),
			stateCell(toolutil.IssueStateEmoji(issue.State), issue.State),
			toolutil.FormatTime(issue.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionIssueGet, "read one of these issues in full"),
		toolutil.HintAction(actionMergeRequests, "see the merge requests in this milestone instead"),
	)
	return b.String()
}

// FormatIssuesMarkdown renders a paginated list of milestone issues as an MCP Markdown result.
func FormatIssuesMarkdown(out IssuesOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatIssuesMarkdownString(out))
}

// FormatMergeRequestsMarkdownString renders a page of a group milestone's
// merge requests as a Markdown table.
func FormatMergeRequestsMarkdownString(out MergeRequestsOutput) string {
	if len(out.MergeRequests) == 0 {
		return toolutil.EmptyMessage("milestone merge requests")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Milestone Merge Requests", len(out.MergeRequests), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Source", "Target", "Created"))
	for _, mr := range out.MergeRequests {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("!%d", mr.IID), mr.WebURL),
			toolutil.EscapeMdTableCell(mr.Title),
			stateCell(toolutil.MRStateEmoji(mr.State), mr.State),
			// A branch name is not an identifier: git check-ref-format permits
			// '|', '<' and '>'.
			toolutil.EscapeMdTableCell(mr.SourceBranch),
			toolutil.EscapeMdTableCell(mr.TargetBranch),
			toolutil.FormatTime(mr.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionMRGet, "read one of these merge requests in full"),
		toolutil.HintAction(actionIssues, "see the issues in this milestone instead"),
	)
	return b.String()
}

// FormatMergeRequestsMarkdown renders a paginated list of milestone MRs as an MCP Markdown result.
func FormatMergeRequestsMarkdown(out MergeRequestsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMergeRequestsMarkdownString(out))
}

// FormatBurndownChartEventsMarkdownString renders a milestone's burndown chart
// events as a Markdown table.
func FormatBurndownChartEventsMarkdownString(out BurndownChartEventsOutput) string {
	if len(out.Events) == 0 {
		return toolutil.EmptyMessage("burndown chart events")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Burndown Chart Events", len(out.Events), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Created At", "Weight", "Action"))
	for _, e := range out.Events {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.FormatTime(e.CreatedAt),
			weightCell(e.Weight),
			toolutil.EscapeMdTableCell(e.Action),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionIssues, "see the issues whose weight these events moved"),
	)
	return b.String()
}

// weightCell renders a burndown event's weight, and a dash for zero: a weight
// of nothing is GitLab saying the issue carried none, and printing 0 read as a
// weight the team had set.
func weightCell(weight int64) string {
	if weight == 0 {
		return "-"
	}
	return strconv.FormatInt(weight, 10)
}

// FormatBurndownChartEventsMarkdown renders burndown chart events as an MCP Markdown result.
func FormatBurndownChartEventsMarkdown(out BurndownChartEventsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatBurndownChartEventsMarkdownString(out))
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatIssuesMarkdownString)
	toolutil.RegisterMarkdown(FormatMergeRequestsMarkdownString)
	toolutil.RegisterMarkdown(FormatBurndownChartEventsMarkdownString)
}
