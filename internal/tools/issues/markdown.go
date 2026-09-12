package issues

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name. A hint names the ID every surface
// resolves — the dynamic surface executes it, and the meta and individual
// surfaces resolve it to their own tool names — so a hint written this way is
// never a name the serving surface does not register, which is what the mix of
// tool names and bare action words in these hints used to be.
const (
	hintActionIssueCreate     = "issue.create"
	hintActionIssueNoteList   = "issue.note_list"
	hintActionIssueNoteCreate = "issue.note_create"
	hintActionIssueMRsRelated = "issue.mrs_related"
	hintActionTodoMarkDone    = "user.todo_mark_done"
	hintActionMRGet           = "merge_request.get"
	hintActionMRChangesGet    = "merge_request.changes_get"
)

// FormatTodoMarkdown renders a to-do item as the card of one object.
func FormatTodoMarkdown(t TodoOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Todo #%d", t.ID))
	c.Field("Action", t.ActionName)
	c.Field("Target Type", t.TargetType)
	c.Field("Target", t.TargetTitle)
	c.Field("State", t.State)
	c.Time("Created", t.CreatedAt)
	c.URL(t.TargetURL)
	c.End(
		toolutil.HintAction(hintActionTodoMarkDone, "mark this todo as completed"),
		toolutil.HintAction(actionIssueGet, "view the referenced issue"),
	)
	return b.String()
}

// formatIssueList renders an issue list under the given heading, closing with
// the caller's hints.
//
// The three list surfaces differ only in those two things; the table between
// them is the same. Rendering it once is what keeps them that way: the row
// format carries the issue link, the state emoji, the confidential marker and
// the escaping, and a change applied to one copy and not the others would
// silently leave the project, group and global listings rendering differently.
func formatIssueList(out ListOutput, heading string, hints ...string) string {
	if len(out.Issues) == 0 {
		return toolutil.EmptyMessage("issues")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, heading, len(out.Issues), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Author", "Labels"))
	for _, i := range out.Issues {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(issueReference(i), i.WebURL),
			issueTitleCell(i),
			issueStateCell(i.State),
			toolutil.MdUserHandle(AuthorName(i.BasicOutput)),
			toolutil.EscapeMdTableCell(strings.Join(i.Labels, ", ")),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true, hints...)
	return b.String()
}

// issueReference is the text an issue row is linked by: the full reference
// GitLab renders the issue with when the response carried one, and the #IID
// otherwise.
func issueReference(i Output) string {
	if i.References != nil && i.References.Full != "" {
		return i.References.Full
	}
	return fmt.Sprintf("#%d", i.IID)
}

// issueTitleCell renders an issue's title with the confidential marker a
// restricted issue carries: without it a reader cannot tell a confidential
// issue from an open one, and every row of these tables looked alike.
func issueTitleCell(i Output) string {
	cell := toolutil.EscapeMdTableCell(i.Title)
	if i.Confidential {
		cell += " " + toolutil.EmojiConfidential
	}
	return cell
}

// issueStateCell renders an issue state with its emoji, and nothing when
// GitLab sent no state.
func issueStateCell(state string) string {
	if strings.TrimSpace(state) == "" {
		return ""
	}
	return toolutil.IssueStateEmoji(state) + " " + toolutil.EscapeMdTableCell(state)
}

// FormatListAllMarkdown renders a page of globally-scoped issues as a Markdown
// table. It is registered for [ListAllOutput], the type the global action
// answers with, so this heading and these hints are what a reader of that
// action sees: while both list actions shared one output type, the registry
// had one key for the two of them and this formatter was never reached.
func FormatListAllMarkdown(out ListAllOutput) string {
	return formatIssueList(ListOutput(out), "All Issues",
		toolutil.HintAction(actionIssueGet, "view one issue in full"),
		toolutil.HintAction(actionIssueUpdate, "change state or labels"),
	)
}

// AuthorName returns the issue author's display username for Markdown, read from
// the full author object. It is exported so sibling packages (e.g.
// mergerequests, search) that render issue rows of their own can show the
// author without reaching into the object. It takes [BasicOutput] because the
// author is what every entity that renders an issue carries, the basic one
// included.
func AuthorName(i BasicOutput) string {
	if i.Author != nil {
		return i.Author.Username
	}
	return ""
}

// assigneeUsernames returns the assignee usernames for Markdown rendering,
// read from the full assignee objects.
func assigneeUsernames(i Output) []string {
	names := make([]string, 0, len(i.Assignees))
	for _, a := range i.Assignees {
		if a != nil {
			names = append(names, a.Username)
		}
	}
	return names
}

// handleList renders usernames as the "@handle" list a card row shows, each
// escaped, and nothing at all when there are none, so a card never shows a
// bare "@".
func handleList(names []string) string {
	handles := make([]string, 0, len(names))
	for _, name := range names {
		if handle := toolutil.MdUserHandle(name); handle != "" {
			handles = append(handles, handle)
		}
	}
	return strings.Join(handles, ", ")
}

// closerName returns the username of the user that closed the issue for
// Markdown, read from the full closer object.
func closerName(i Output) string {
	if i.ClosedBy != nil {
		return i.ClosedBy.Username
	}
	return ""
}

// FormatTimeStatsMarkdown renders an issue's time tracking as the card of one
// object: the two durations GitLab renders for a reader and the two counts of
// seconds they are computed from.
func FormatTimeStatsMarkdown(ts TimeStatsOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Time Tracking")
	c.Field("Estimate", ts.HumanTimeEstimate)
	c.Field("Spent", ts.HumanTotalTimeSpent)
	c.Int("Estimate (seconds)", ts.TimeEstimate)
	c.Int("Spent (seconds)", ts.TotalTimeSpent)
	c.End(toolutil.HintAction(actionIssueUpdate, "adjust time tracking"))
	return b.String()
}

// FormatParticipantsMarkdown renders an issue's participants as a Markdown
// table: a collection of objects that share columns.
func FormatParticipantsMarkdown(out ParticipantsOutput) string {
	if len(out.Participants) == 0 {
		return toolutil.EmptyMessage("participants")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Participants", len(out.Participants), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("Username", "Name"))
	for _, p := range out.Participants {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdUserHandle(p.Username),
			toolutil.EscapeMdTableCell(p.Name),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(actionIssueGet, "view the issue details"),
		toolutil.HintAction(hintActionIssueNoteCreate, "notify participants"),
	)
	return b.String()
}

// FormatRelatedMRsMarkdown renders the merge requests tied to an issue as a
// Markdown table.
func FormatRelatedMRsMarkdown(out RelatedMRsOutput, heading string) string {
	if len(out.MergeRequests) == 0 {
		return toolutil.EmptyMessage("merge requests")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, heading, len(out.MergeRequests), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Author", "Source -> Target"))
	for _, mr := range out.MergeRequests {
		author := ""
		if mr.Author != nil {
			author = mr.Author.Username
		}
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("!%d", mr.IID), mr.WebURL),
			toolutil.EscapeMdTableCell(mr.Title),
			mrStateCell(mr.State),
			toolutil.MdUserHandle(author),
			toolutil.EscapeMdTableCell(mr.SourceBranch)+" -> "+toolutil.EscapeMdTableCell(mr.TargetBranch),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(hintActionMRGet, "view one merge request in full"),
		toolutil.HintAction(hintActionMRChangesGet, "see its diff"),
	)
	return b.String()
}

// mrStateCell renders a merge request state with its emoji, the way every
// merge request row in the tree shows it.
func mrStateCell(state string) string {
	if strings.TrimSpace(state) == "" {
		return ""
	}
	return toolutil.MRStateEmoji(state) + " " + toolutil.EscapeMdTableCell(state)
}

// FormatMarkdown renders a single issue as the card of one object: its own
// fields, then the description as quoted prose under its label.
func FormatMarkdown(i Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, issueHeading(i))
	if i.References != nil {
		c.Field("Reference", i.References.Full)
	}
	c.Markdown("State", issueStateCell(i.State))
	if i.IssueType != "" && i.IssueType != "issue" {
		c.Field("Type", i.IssueType)
	}
	c.Flag(toolutil.EmojiConfidential, "Confidential", i.Confidential)
	c.Markdown("Author", toolutil.MdUserHandle(AuthorName(i.BasicOutput)))
	// A label title is free text: GitLab's only rule on one is that it carries
	// no comma.
	c.Field("Labels", strings.Join(i.Labels, ", "))
	c.Markdown("Assignees", handleList(assigneeUsernames(i)))
	if i.Milestone != nil {
		c.Field("Milestone", i.Milestone.Title)
	}
	c.Time("Due Date", i.DueDate)
	c.Time("Created", i.CreatedAt)
	c.Markdown("Closed By", toolutil.MdUserHandle(closerName(i)))
	c.Time("Closed", i.ClosedAt)
	c.Count("Linked MRs", i.MergeRequestCount)
	if i.TaskCompletionStatus != nil && i.TaskCompletionStatus.Count > 0 {
		c.Field("Tasks", fmt.Sprintf("%d/%d completed", i.TaskCompletionStatus.CompletedCount, i.TaskCompletionStatus.Count))
	}
	c.Count("Comments", i.UserNotesCount)
	c.URL(i.WebURL)
	c.Text("Description", i.Description)
	c.Note(toolutil.RichContentHint(toolutil.DetectRichContent(i.Description), i.WebURL))
	c.End(
		toolutil.HintAction(hintActionIssueNoteList, "see comments on this issue"),
		toolutil.HintAction(actionIssueUpdate, "change title, labels, assignees, or milestone"),
		toolutil.HintAction(hintActionIssueMRsRelated, "find linked MRs"),
	)
	return b.String()
}

// issueHeading composes the card's heading: the state as a glyph, the issue's
// reference and its title, with the confidential marker a restricted issue
// carries. The whole composition is escaped by the card.
func issueHeading(i Output) string {
	heading := fmt.Sprintf("%s Issue #%d: %s", toolutil.IssueStateEmoji(i.State), i.IID, i.Title)
	if i.Confidential {
		heading += " " + toolutil.EmojiConfidential
	}
	return heading
}

// formatGetMarkdownResult renders a single issue. The canonical resource is
// not embedded here: the issue.get spec declares it and the dispatchers embed
// it, the same way as for every other get action.
func formatGetMarkdownResult(out getOutput) *mcp.CallToolResult {
	return toolutil.ToolResultAnnotated(FormatMarkdown(out.Output), toolutil.ContentDetail)
}

// FormatListMarkdown renders a page of a project's issues as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	return formatIssueList(out, "Issues",
		toolutil.HintAction(actionIssueGet, "see one issue's full details and description"),
		toolutil.HintAction(hintActionIssueCreate, "create a new issue"),
		toolutil.HintAction(hintActionIssueNoteCreate, "add a comment"),
	)
}

// FormatListGroupMarkdown renders a page of a group's issues as a Markdown
// table, through the same renderer the project and global listings use.
func FormatListGroupMarkdown(out ListGroupOutput) string {
	return formatIssueList(ListOutput{Issues: out.Issues, Pagination: out.Pagination}, "Group Issues",
		toolutil.HintAction(actionIssueGet, "view one issue in full"),
		toolutil.HintAction(hintActionIssueCreate, "open a new issue"),
	)
}

func init() {
	toolutil.RegisterMarkdownResult(formatGetMarkdownResult)
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatListAllMarkdown)
	toolutil.RegisterMarkdown(FormatTodoMarkdown)
	toolutil.RegisterMarkdown(FormatTimeStatsMarkdown)
	toolutil.RegisterMarkdown(FormatParticipantsMarkdown)
	toolutil.RegisterMarkdown(func(v RelatedMRsOutput) string { return FormatRelatedMRsMarkdown(v, "Related MRs") })
	toolutil.RegisterMarkdown(FormatListGroupMarkdown)
}
