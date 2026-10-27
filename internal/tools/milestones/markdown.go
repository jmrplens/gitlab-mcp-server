// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package milestones

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

type milestoneNotFoundOutput struct {
	Identifier string
}

func formatMilestoneNotFound(out milestoneNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Milestone", out.Identifier,
		"Use project.milestone_list with project_id to list milestones",
		"Verify the milestone IID is correct for this project",
	)
}

// FormatListMarkdownString renders a page of milestones as a Markdown table: a
// collection of objects that share columns.
func FormatListMarkdownString(v ListOutput) string {
	if len(v.Milestones) == 0 {
		return toolutil.EmptyMessage("milestones")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Milestones", len(v.Milestones), v.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Due Date", "Expired"))
	for _, m := range v.Milestones {
		due := "-"
		if m.DueDate != "" {
			due = toolutil.FormatTime(m.DueDate)
		}
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(strconv.FormatInt(m.IID, 10), m.WebURL),
			toolutil.EscapeMdTableCell(m.Title),
			toolutil.EscapeMdTableCell(m.State),
			due,
			toolutil.BoolEmoji(m.Expired),
		))
	}
	toolutil.WriteListFooter(&b, v.Pagination, true,
		toolutil.HintAction(actionMilestoneGet, "read one milestone by its IID"),
		toolutil.HintAction(actionMilestoneCreate, "add a new milestone to the project"),
	)
	return b.String()
}

// FormatListMarkdown returns a Markdown MCP tool result for a ListOutput.
func FormatListMarkdown(v ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(v))
}

// FormatMarkdown renders one milestone as the card of one object.
//
// The expiry used to print as the word "true", which reads as a value GitLab
// stored rather than as a warning; an expired milestone is now marked with the
// warning sign and an unexpired one says nothing at all, since a tick on
// "Expired" reads as success.
func FormatMarkdown(v Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Milestone #%d: %s", v.IID, v.Title))
	c.Int("ID", v.ID)
	c.Int("IID", v.IID)
	c.Count("Project ID", v.ProjectID)
	c.Count("Group ID", v.GroupID)
	c.Field("State", v.State)
	c.Time("Start Date", v.StartDate)
	c.Time("Due Date", v.DueDate)
	c.Warn("Expired", v.Expired)
	c.URL(v.WebURL)
	c.Time("Created", v.CreatedAt)
	c.Time("Updated", v.UpdatedAt)
	c.Text("Description", v.Description)
	c.End(
		toolutil.HintAction(actionMilestoneIssues, "list the issues in this milestone"),
		toolutil.HintAction(actionMilestoneMergeRequests, "list the merge requests in this milestone"),
		toolutil.HintAction(actionMilestoneUpdate, "change this milestone's dates or state"),
	)
	return b.String()
}

// FormatIssuesMarkdownString renders a page of a milestone's issues as a
// Markdown table.
func FormatIssuesMarkdownString(v MilestoneIssuesOutput) string {
	if len(v.Issues) == 0 {
		return toolutil.EmptyMessage("milestone issues")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Milestone Issues", len(v.Issues), v.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Assignees", "Labels", "Due", "Created"))
	for _, issue := range v.Issues {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("#%d", issue.IID), issue.WebURL),
			toolutil.EscapeMdTableCell(issue.Title),
			stateCell(toolutil.IssueStateEmoji(issue.State), issue.State),
			assigneeHandles(issue.Assignees),
			toolutil.EscapeMdTableCell(strings.Join(issue.Labels, ", ")),
			toolutil.FormatTime(issue.DueDate),
			toolutil.FormatTime(issue.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, v.Pagination, true,
		toolutil.HintAction(actionIssueGet, "read one of these issues in full"),
		toolutil.HintAction(actionMilestoneMergeRequests, "see the merge requests in this milestone instead"),
	)
	return b.String()
}

// assigneeHandles renders an issue row's assignees as the handles GitLab
// shows, for a table cell.
func assigneeHandles(assignees []*toolutil.IssueUserOutput) string {
	handles := make([]string, 0, len(assignees))
	for _, assignee := range assignees {
		if handle := toolutil.MdUserHandle(assignee.Username); handle != "" {
			handles = append(handles, handle)
		}
	}
	return strings.Join(handles, ", ")
}

// authorHandle renders a merge request row's author as a handle, or nothing
// when GitLab named none.
func authorHandle(author *toolutil.BasicUserOutput) string {
	if author == nil {
		return ""
	}
	return toolutil.MdUserHandle(author.Username)
}

// stateCell renders a state word with the glyph its domain gives it, and
// nothing at all when GitLab sent no state.
func stateCell(emoji, state string) string {
	if state == "" {
		return ""
	}
	return emoji + " " + toolutil.EscapeMdTableCell(state)
}

// FormatIssuesMarkdown returns a Markdown MCP tool result for milestone issues.
func FormatIssuesMarkdown(v MilestoneIssuesOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatIssuesMarkdownString(v))
}

// FormatMergeRequestsMarkdownString renders a page of a milestone's merge
// requests as a Markdown table.
func FormatMergeRequestsMarkdownString(v MilestoneMergeRequestsOutput) string {
	if len(v.MergeRequests) == 0 {
		return toolutil.EmptyMessage("milestone merge requests")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Milestone Merge Requests", len(v.MergeRequests), v.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Author", "Source", "Target", "Created"))
	for _, mr := range v.MergeRequests {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("!%d", mr.IID), mr.WebURL),
			toolutil.EscapeMdTableCell(mr.Title),
			stateCell(toolutil.MRStateEmoji(mr.State), mr.State),
			authorHandle(mr.Author),
			// A branch name is not an identifier: git check-ref-format permits
			// '|', '<' and '>'.
			toolutil.EscapeMdTableCell(mr.SourceBranch),
			toolutil.EscapeMdTableCell(mr.TargetBranch),
			toolutil.FormatTime(mr.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, v.Pagination, true,
		toolutil.HintAction(actionMergeRequestGet, "read one of these merge requests in full"),
		toolutil.HintAction(actionMilestoneIssues, "see the issues in this milestone instead"),
	)
	return b.String()
}

// FormatMergeRequestsMarkdown returns a Markdown MCP tool result for milestone merge requests.
func FormatMergeRequestsMarkdown(v MilestoneMergeRequestsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMergeRequestsMarkdownString(v))
}

// FormatPromoteMarkdown renders a promotion as a confirmation card. GitLab
// answers a promotion with no milestone, so the next step is the group
// milestone list, where the new milestone is found by the title the card
// names.
func FormatPromoteMarkdown(v PromoteOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Milestone promoted: "+v.Title)
	c.Field("Title", v.Title)
	c.Note(v.Message)
	c.End(toolutil.HintAction(actionGroupMilestoneList, "find the group milestone by its title"))
	return b.String()
}

// FormatBurndownChartEventsMarkdownString renders a milestone's burndown
// chart events as a Markdown table.
func FormatBurndownChartEventsMarkdownString(v BurndownChartEventsOutput) string {
	if len(v.Events) == 0 {
		return toolutil.EmptyMessage("burndown chart events")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Burndown Chart Events", len(v.Events), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("Created At", "Weight", "Action"))
	for _, e := range v.Events {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.FormatTime(e.CreatedAt),
			weightCell(e.Weight),
			toolutil.EscapeMdTableCell(e.Action),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(actionMilestoneIssues, "see the issues whose weight these events moved"),
	)
	return b.String()
}

// weightCell renders a burndown event's weight, and a dash for an event whose
// issue carries no weight, which GitLab sends as null. A weight of 0 is a
// weight the team set, and is printed as 0.
func weightCell(weight *int64) string {
	if weight == nil {
		return "-"
	}
	return strconv.FormatInt(*weight, 10)
}

func init() {
	toolutil.RegisterMarkdownResult(formatMilestoneNotFound)
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatIssuesMarkdownString)
	toolutil.RegisterMarkdown(FormatMergeRequestsMarkdownString)
	toolutil.RegisterMarkdown(FormatPromoteMarkdown)
	toolutil.RegisterMarkdown(FormatBurndownChartEventsMarkdownString)
}
