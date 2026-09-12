package milestones

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionGet           = "milestone.get"
	actionCreate        = "milestone.create"
	actionUpdate        = "milestone.update"
	actionIssues        = "milestone.issues"
	actionMergeRequests = "milestone.merge_requests"
	actionIssueGet      = "issue.get"
	actionMRGet         = "merge_request.get"
)

type milestoneNotFoundOutput struct {
	Identifier string
}

func formatMilestoneNotFound(out milestoneNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Milestone", out.Identifier,
		"Use gitlab_milestone_list with project_id to list milestones",
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
		toolutil.HintAction(actionGet, "read one milestone by its IID"),
		toolutil.HintAction(actionCreate, "add a new milestone to the project"),
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
		toolutil.HintAction(actionIssues, "list the issues in this milestone"),
		toolutil.HintAction(actionMergeRequests, "list the merge requests in this milestone"),
		toolutil.HintAction(actionUpdate, "change this milestone's dates or state"),
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
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Created"))
	for _, issue := range v.Issues {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("#%d", issue.IID), issue.WebURL),
			toolutil.EscapeMdTableCell(issue.Title),
			stateCell(toolutil.IssueStateEmoji(issue.State), issue.State),
			toolutil.FormatTime(issue.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, v.Pagination, true,
		toolutil.HintAction(actionIssueGet, "read one of these issues in full"),
		toolutil.HintAction(actionMergeRequests, "see the merge requests in this milestone instead"),
	)
	return b.String()
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
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Source", "Target", "Created"))
	for _, mr := range v.MergeRequests {
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
	toolutil.WriteListFooter(&b, v.Pagination, true,
		toolutil.HintAction(actionMRGet, "read one of these merge requests in full"),
		toolutil.HintAction(actionIssues, "see the issues in this milestone instead"),
	)
	return b.String()
}

// FormatMergeRequestsMarkdown returns a Markdown MCP tool result for milestone merge requests.
func FormatMergeRequestsMarkdown(v MilestoneMergeRequestsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMergeRequestsMarkdownString(v))
}

func init() {
	toolutil.RegisterMarkdownResult(formatMilestoneNotFound)
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatIssuesMarkdownString)
	toolutil.RegisterMarkdown(FormatMergeRequestsMarkdownString)
}
