package mergerequests

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name. A hint names the ID every surface
// resolves — the dynamic surface executes it, and the meta and individual
// surfaces resolve it to their own tool names — so a hint written this way is
// never a name the serving surface does not register, which is what the mix of
// tool names and bare action words in these hints used to be.
const (
	hintActionMRCreate        = "merge_request.create"
	hintActionMRCommits       = "merge_request.commits"
	hintActionMRPipelines     = "merge_request.pipelines"
	hintActionMRRelatedIssues = "merge_request.related_issues"
	hintActionMRDependencies  = "merge_request.dependencies_list"
	hintActionMRParticipants  = "merge_request.participants"
	hintActionChangesGet      = "mr_review.changes_get"
	hintActionDiscussionList  = "mr_review.discussion_list"
	hintActionNoteCreate      = "mr_review.note_create"
	hintActionIssueGet        = "issue.get"
	hintActionCommitGet       = "commit.get"
	hintActionPipelineGet     = "pipeline.get"
	hintActionJobList         = "job.list"
	hintActionTodoMarkDone    = "user.todo_mark_done"
)

type mergeRequestNotFoundOutput struct {
	Identifier string
}

func formatMergeRequestNotFound(out mergeRequestNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Merge Request", out.Identifier,
		"Use gitlab_mr_list with project_id to list available merge requests",
		"Verify the merge request IID is correct for this project",
		"The merge request may have been deleted",
	)
}

// FormatMarkdown renders a single merge request as the card of one object: its
// own fields, the conditions that hold, then the description as quoted prose.
func FormatMarkdown(mr Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, mergeRequestHeading(mr))
	c.Field("Project", mrProjectPath(mr))
	c.Markdown("State", mrStateCell(mr.State))
	c.Flag(toolutil.EmojiDraft, "Draft merge request", mr.Draft)
	// A branch name is not an identifier: git check-ref-format forbids a space
	// and the control bytes and permits '|', '<' and '>', so a pushable branch
	// can end a row or open a tag in it.
	c.Field("Source", mr.SourceBranch)
	c.Field("Target", mr.TargetBranch)
	c.Field("Merge Status", mr.DetailedMergeStatus)
	c.Warn("Has conflicts", mr.HasConflicts)
	c.Markdown("Author", toolutil.MdUserHandle(userName(mr.Author)))
	c.Markdown("Assignees", handleList(userNames(mr.Assignees)))
	c.Markdown("Reviewers", handleList(userNames(mr.Reviewers)))
	if mr.Milestone != nil {
		c.Field("Milestone", mr.Milestone.Title)
	}
	// A label title is free text: GitLab's only rule on one is that it carries
	// no comma.
	c.Field("Labels", strings.Join(mr.Labels, ", "))
	c.Markdown("Pipeline", mrPipelineCell(mr))
	c.Field("Changes", changesText(mr.ChangesCount))
	c.Time("Created", mr.CreatedAt)
	writeTerminalActor(c, mr)
	c.Count("Comments", mr.UserNotesCount)
	// The four conditions that decide whether a merge request can be worked on
	// at all, and none of which the card used to show: a locked discussion
	// refuses every comment, an auto-merge is already scheduled, a rebase is
	// running, and a merge error is why the last attempt failed.
	c.Warn("Discussion locked", mr.DiscussionLocked)
	c.Flag(toolutil.EmojiRefresh, "Auto-merge set (merges when the pipeline succeeds)", mr.MergeWhenPipelineSucceeds)
	c.Flag(toolutil.EmojiRefresh, "Rebase in progress", mr.RebaseInProgress)
	c.Field("Merge Error", mr.MergeError)
	c.URL(mr.WebURL)
	c.Text("Description", mr.Description)
	c.Note(toolutil.RichContentHint(toolutil.DetectRichContent(mr.Description), mr.WebURL))
	c.End(
		toolutil.HintAction(hintActionChangesGet, "see the diff of this merge request"),
		toolutil.HintAction(hintActionDiscussionList, "see its review threads"),
		toolutil.HintAction(hintActionMRPipelines, "check its CI status"),
		toolutil.HintAction(actionMRApprove, "approve it"),
		toolutil.HintAction(actionMRMerge, "merge it"),
	)
	return b.String()
}

// mergeRequestHeading composes the card's heading: the state as a glyph, the
// draft marker a draft carries, the reference and the title. The whole
// composition is escaped by the card.
func mergeRequestHeading(mr Output) string {
	prefix := toolutil.MRStateEmoji(mr.State)
	if mr.Draft {
		prefix += " " + toolutil.EmojiDraft
	}
	heading := fmt.Sprintf("%s MR !%d", prefix, mr.IID)
	if strings.TrimSpace(mr.Title) == "" {
		return heading
	}
	return heading + ": " + mr.Title
}

// mrStateCell renders a merge request state with its emoji, and nothing when
// GitLab sent no state.
func mrStateCell(state string) string {
	if strings.TrimSpace(state) == "" {
		return ""
	}
	return toolutil.MRStateEmoji(state) + " " + toolutil.EscapeMdTableCell(state)
}

// changesText renders GitLab's own count of changed files, which it sends as a
// string because it caps it ("1000+").
func changesText(count string) string {
	if strings.TrimSpace(count) == "" {
		return ""
	}
	return count + " files"
}

// mrPipelineCell renders the merge request's pipeline as its number linked to
// the pipeline, with the status glyph and word when GitLab sent one.
func mrPipelineCell(mr Output) string {
	if mr.Pipeline == nil || mr.Pipeline.ID <= 0 {
		return ""
	}
	cell := toolutil.MdTitleLink(fmt.Sprintf("#%d", mr.Pipeline.ID), mr.Pipeline.WebURL)
	if mr.Pipeline.Status != "" {
		cell += " " + toolutil.PipelineStatusEmoji(mr.Pipeline.Status) + " " + toolutil.EscapeMdTableCell(mr.Pipeline.Status)
	}
	return cell
}

// writeTerminalActor writes who ended the merge request and when, for the two
// states that have an ending.
func writeTerminalActor(c *toolutil.Card, mr Output) {
	switch mr.State {
	case "merged":
		c.Markdown("Merged By", toolutil.MdUserHandle(userName(mr.MergeUser)))
		c.Time("Merged", mr.MergedAt)
	case "closed":
		c.Markdown("Closed By", toolutil.MdUserHandle(userName(mr.ClosedBy)))
		c.Time("Closed", mr.ClosedAt)
	}
}

// AuthorName returns the username of the merge request author, or "" when the
// author object is absent. Exported for downstream consumers (e.g. search)
// that render the author after the 1:1 object migration.
func AuthorName(mr Output) string {
	return userName(mr.Author)
}

// ProjectPath returns the "group/project" path derived from the MR references
// object, or "" when unavailable. Exported for downstream consumers (e.g.
// search) that previously read the flattened project_path scalar.
func ProjectPath(mr Output) string {
	return mrProjectPath(mr)
}

// mrProjectPath derives the "group/project" path from the MR references object
// (the part before the "!IID" suffix in the full reference), or "" when the
// references object is absent or malformed.
func mrProjectPath(mr Output) string {
	if mr.References == nil {
		return ""
	}
	full := mr.References.Full
	if idx := strings.LastIndex(full, "!"); idx > 0 {
		return full[:idx]
	}
	return ""
}

// userName returns the username of a basic-user object, or "" when nil.
func userName(u *toolutil.BasicUserOutput) string {
	if u == nil {
		return ""
	}
	return u.Username
}

// userNames maps a slice of basic-user objects to their usernames, skipping nil.
func userNames(users []*toolutil.BasicUserOutput) []string {
	out := make([]string, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		out = append(out, u.Username)
	}
	return out
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

// FormatListMarkdown renders a page of merge requests as a Markdown table: a
// collection of objects that share columns.
func FormatListMarkdown(out ListOutput) string {
	if len(out.MergeRequests) == 0 {
		return toolutil.EmptyMessage("merge requests")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Merge Requests", len(out.MergeRequests), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Author", "Project", "Source -> Target"))
	for _, mr := range out.MergeRequests {
		title := toolutil.EscapeMdTableCell(mr.Title)
		if mr.Draft {
			title += " " + toolutil.EmojiDraft
		}
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("!%d", mr.IID), mr.WebURL),
			title,
			mrStateCell(mr.State),
			toolutil.MdUserHandle(userName(mr.Author)),
			toolutil.EscapeMdTableCell(mrProjectPath(mr)),
			toolutil.EscapeMdTableCell(mr.SourceBranch)+" -> "+toolutil.EscapeMdTableCell(mr.TargetBranch),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionMRGet, "see one merge request in full"),
		toolutil.HintAction(hintActionMRCreate, "open a new merge request"),
		toolutil.HintAction(hintActionChangesGet, "review a merge request's diff"),
	)
	return b.String()
}

// FormatApproveMarkdown renders the approval state after an approve or
// unapprove as the card of one object.
func FormatApproveMarkdown(a ApproveOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "MR Approval Status")
	c.Bool("Approved", a.Approved)
	c.Int("Approvals Required", int64(a.ApprovalsRequired))
	c.Int("Approvals Given", int64(a.ApprovedBy))
	c.End(
		toolutil.HintAction(actionMRMerge, "merge this merge request"),
		toolutil.HintAction(actionMRGet, "see its full details"),
	)
	return b.String()
}

// FormatCommitsMarkdown renders the commits of a merge request as a Markdown
// table.
func FormatCommitsMarkdown(out CommitsOutput) string {
	if len(out.Commits) == 0 {
		return toolutil.EmptyMessage("commits")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "MR Commits", len(out.Commits), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Short ID", "Title", "Author", "Date"))
	for _, commit := range out.Commits {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(commit.ShortID, commit.WebURL),
			toolutil.EscapeMdTableCell(commit.Title),
			toolutil.EscapeMdTableCell(commit.AuthorName),
			toolutil.FormatTime(commit.CommittedDate),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(hintActionCommitGet, "view one of these commits"),
		toolutil.HintAction(hintActionChangesGet, "review the combined diff"),
	)
	return b.String()
}

// FormatPipelinesMarkdown renders the pipelines of a merge request as a
// Markdown table.
func FormatPipelinesMarkdown(out PipelinesOutput) string {
	if len(out.Pipelines) == 0 {
		return toolutil.EmptyMessage("pipelines")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "MR Pipelines", len(out.Pipelines), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Status", "Source", "Ref"))
	for _, p := range out.Pipelines {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("#%d", p.ID), p.WebURL),
			pipelineStatusCell(p.Status),
			toolutil.EscapeMdTableCell(p.Source),
			toolutil.EscapeMdTableCell(p.Ref),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, true,
		toolutil.HintAction(hintActionPipelineGet, "view one pipeline's details"),
		toolutil.HintAction(hintActionJobList, "see its job statuses"),
	)
	return b.String()
}

// pipelineStatusCell renders a pipeline status with its glyph, and nothing when
// GitLab sent no status.
func pipelineStatusCell(status string) string {
	if strings.TrimSpace(status) == "" {
		return ""
	}
	return toolutil.PipelineStatusEmoji(status) + " " + toolutil.EscapeMdTableCell(status)
}

// FormatRebaseMarkdown renders the outcome of a rebase.
func FormatRebaseMarkdown(r RebaseOutput) string {
	var b strings.Builder
	heading := toolutil.EmojiSuccess + " Rebase completed"
	note := "The rebase has finished successfully."
	if r.RebaseInProgress {
		heading = toolutil.EmojiRefresh + " Rebase in progress"
		note = "The rebase has been initiated and is currently running."
	}
	c := toolutil.NewCard(&b, heading)
	c.Bool("Rebase in progress", r.RebaseInProgress)
	c.Note(note)
	c.End(
		toolutil.HintAction(actionMRGet, "check whether the rebase has finished"),
		toolutil.HintAction(actionMRMerge, "merge once it has"),
	)
	return b.String()
}

// FormatParticipantsMarkdown renders the participants of a merge request as a
// Markdown table.
func FormatParticipantsMarkdown(out ParticipantsOutput) string {
	if len(out.Participants) == 0 {
		return toolutil.EmptyMessage("participants")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "MR Participants", len(out.Participants), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Username", "Name", "State"))
	for _, p := range out.Participants {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(p.ID, 10),
			toolutil.MdUserLink(p.Username, p.WebURL),
			toolutil.EscapeMdTableCell(p.Name),
			toolutil.EscapeMdTableCell(p.State),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, true,
		toolutil.HintAction(actionMRGet, "view the merge request"),
		toolutil.HintAction(hintActionNoteCreate, "notify these participants"),
	)
	return b.String()
}

// FormatReviewersMarkdown renders the reviewers of a merge request as a
// Markdown table.
func FormatReviewersMarkdown(out ReviewersOutput) string {
	if len(out.Reviewers) == 0 {
		return toolutil.EmptyMessage("reviewers")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "MR Reviewers", len(out.Reviewers), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Username", "Name", "Review State", "Assigned At"))
	for _, r := range out.Reviewers {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(r.ID, 10),
			toolutil.MdUserLink(r.Username, r.WebURL),
			toolutil.EscapeMdTableCell(r.Name),
			toolutil.EscapeMdTableCell(r.Review),
			toolutil.FormatTime(r.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, true,
		toolutil.HintAction(actionMRUpdate, "add or change reviewers"),
		toolutil.HintAction(actionMRApprove, "approve the merge request"),
	)
	return b.String()
}

// FormatIssuesClosedMarkdown renders the issues a merge would close as a
// Markdown table.
func FormatIssuesClosedMarkdown(out IssuesClosedOutput) string {
	if len(out.Issues) == 0 {
		return toolutil.EmptyMessage("issues that would be closed on merge")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Issues Closed on Merge", len(out.Issues), out.Pagination)
	writeIssueRows(&b, out.Issues)
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(hintActionIssueGet, "view one of these issues"),
		toolutil.HintAction(actionMRMerge, "merge and close them"),
	)
	return b.String()
}

// FormatRelatedIssuesMarkdown renders the issues a merge request references as
// a Markdown table.
func FormatRelatedIssuesMarkdown(out RelatedIssuesOutput) string {
	if len(out.Issues) == 0 {
		return toolutil.EmptyMessage("related issues")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Related Issues", len(out.Issues), out.Pagination)
	writeIssueRows(&b, out.Issues)
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(hintActionIssueGet, "view one issue's details"),
		toolutil.HintAction(hintActionMRRelatedIssues, "list them again after the merge request changes"),
	)
	return b.String()
}

// writeIssueRows writes the issue table the two issue listings share: the same
// columns, the same escaping and the same state glyph, so the two cannot drift.
func writeIssueRows(b *strings.Builder, list []issues.BasicOutput) {
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Author", "Labels"))
	for _, issue := range list {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("#%d", issue.IID), issue.WebURL),
			toolutil.EscapeMdTableCell(issue.Title),
			issueStateCell(issue.State),
			toolutil.MdUserHandle(issues.AuthorName(issue)),
			toolutil.EscapeMdTableCell(strings.Join(issue.Labels, ", ")),
		))
	}
}

// issueStateCell renders an issue state with its emoji, and nothing when GitLab
// sent no state.
func issueStateCell(state string) string {
	if strings.TrimSpace(state) == "" {
		return ""
	}
	return toolutil.IssueStateEmoji(state) + " " + toolutil.EscapeMdTableCell(state)
}

// FormatCreatePipelineMarkdown renders the pipeline a merge request just
// created as the card of one object.
func FormatCreatePipelineMarkdown(p pipelines.Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("%s Pipeline #%d Created", toolutil.PipelineStatusEmoji(p.Status), p.ID))
	c.Int("ID", p.ID)
	c.Markdown("Status", pipelineStatusCell(p.Status))
	c.Field("Source", p.Source)
	c.Field("Ref", p.Ref)
	c.Code("SHA", p.SHA)
	c.URL(p.WebURL)
	c.End(
		toolutil.HintAction(hintActionPipelineGet, "check the pipeline's progress"),
		toolutil.HintAction(hintActionJobList, "monitor its job statuses"),
	)
	return b.String()
}

// FormatTimeStatsMarkdown renders a merge request's time tracking as the card
// of one object: the two durations GitLab renders for a reader and the two
// counts of seconds they are computed from.
func FormatTimeStatsMarkdown(ts TimeStatsOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Time Tracking Stats")
	c.FieldOr("Estimate", ts.HumanTimeEstimate, "not set")
	c.FieldOr("Spent", ts.HumanTotalTimeSpent, "none")
	c.Int("Estimate (seconds)", ts.TimeEstimate)
	c.Int("Spent (seconds)", ts.TotalTimeSpent)
	c.End(
		toolutil.HintAction(actionMRTimeEstimateSet, "set the estimate"),
		toolutil.HintAction(actionMRSpentTimeAdd, "log time spent"),
	)
	return b.String()
}

// FormatCreateTodoMarkdown renders the to-do item a merge request just created
// as the card of one object.
func FormatCreateTodoMarkdown(t CreateTodoOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Todo #%d", t.ID))
	c.Int("ID", t.ID)
	c.Field("Action", t.ActionName)
	c.Field("Target Type", t.TargetType)
	c.Field("Target", t.TargetTitle)
	c.Field("Project", t.ProjectName)
	c.Field("State", t.State)
	c.Time("Created", t.CreatedAt)
	c.URL(t.TargetURL)
	c.End(
		toolutil.HintAction(actionMRGet, "view the merge request this is about"),
		toolutil.HintAction(hintActionTodoMarkDone, "mark this todo as completed"),
	)
	return b.String()
}

// FormatDependencyMarkdown renders one merge request dependency as the card of
// one object, with the merge requests at both ends as nested objects.
func FormatDependencyMarkdown(d DependencyOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("MR Dependency #%d", d.ID))
	c.Int("ID", d.ID)
	c.Count("Project ID", d.ProjectID)
	writeBlockingMR(c, "Blocking MR", d.BlockingMergeRequest)
	writeBlockingMR(c, "Blocked MR", d.BlockedMergeRequest)
	c.End(
		toolutil.HintAction(actionMRGet, "view either merge request in full"),
		toolutil.HintAction(hintActionMRDependencies, "list every dependency of this merge request"),
	)
	return b.String()
}

// writeBlockingMR writes one end of a dependency as a nested object under its
// label, or nothing when GitLab did not send it.
func writeBlockingMR(c *toolutil.Card, label string, mr *BlockingMergeRequestOutput) {
	if mr == nil {
		return
	}
	sub := c.Sub(label)
	sub.Markdown("Reference", toolutil.MdTitleLink(fmt.Sprintf("!%d", mr.IID), mr.WebURL))
	sub.Int("ID", mr.ID)
	sub.Field("Title", mr.Title)
	sub.Markdown("State", mrStateCell(mr.State))
	sub.Field("Source", mr.SourceBranch)
	sub.Field("Target", mr.TargetBranch)
}

// FormatDependenciesMarkdown renders the dependencies of a merge request as a
// Markdown table.
func FormatDependenciesMarkdown(out DependenciesOutput) string {
	if len(out.Dependencies) == 0 {
		return toolutil.EmptyMessage("dependencies")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "MR Dependencies", len(out.Dependencies), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Blocking MR", "Title", "State"))
	for _, d := range out.Dependencies {
		reference, title, state := "", "", ""
		if bmr := d.BlockingMergeRequest; bmr != nil {
			reference = toolutil.MdTitleLink(fmt.Sprintf("!%d", bmr.IID), bmr.WebURL)
			title, state = toolutil.EscapeMdTableCell(bmr.Title), mrStateCell(bmr.State)
		}
		b.WriteString(toolutil.MarkdownTableRow(strconv.FormatInt(d.ID, 10), reference, title, state))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, true,
		toolutil.HintAction(actionMRGet, "view a blocking merge request"),
		toolutil.HintAction(actionMRMerge, "merge a blocker to clear the dependency"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdownResult(formatMergeRequestNotFound)
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatApproveMarkdown)
	toolutil.RegisterMarkdown(FormatCommitsMarkdown)
	toolutil.RegisterMarkdown(FormatPipelinesMarkdown)
	toolutil.RegisterMarkdown(FormatRebaseMarkdown)
	toolutil.RegisterMarkdown(FormatParticipantsMarkdown)
	toolutil.RegisterMarkdown(FormatReviewersMarkdown)
	toolutil.RegisterMarkdown(FormatIssuesClosedMarkdown)
	toolutil.RegisterMarkdown(FormatTimeStatsMarkdown)
	toolutil.RegisterMarkdown(FormatRelatedIssuesMarkdown)
	toolutil.RegisterMarkdown(FormatCreateTodoMarkdown)
	toolutil.RegisterMarkdown(FormatDependencyMarkdown)
	toolutil.RegisterMarkdown(FormatDependenciesMarkdown)
	toolutil.RegisterMarkdown(FormatCreatePipelineMarkdown)
}
