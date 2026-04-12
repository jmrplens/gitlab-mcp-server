// markdown.go provides Markdown formatting functions for merge request MCP tool output.

package mergerequests

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatMarkdown renders a single merge request as a Markdown summary.
func FormatMarkdown(mr Output) string {
	var b strings.Builder
	titlePrefix := toolutil.MRStateEmoji(mr.State)
	if mr.Draft {
		titlePrefix += " " + toolutil.EmojiDraft
	}
	fmt.Fprintf(&b, "## %s MR !%d: %s\n\n", titlePrefix, mr.IID, toolutil.EscapeMdHeading(mr.Title))
	if mr.ProjectPath != "" {
		fmt.Fprintf(&b, "- **Project**: %s\n", mr.ProjectPath)
	}
	fmt.Fprintf(&b, "- **State**: %s %s\n", toolutil.MRStateEmoji(mr.State), mr.State)
	if mr.Draft {
		fmt.Fprintf(&b, "- %s **Draft** merge request\n", toolutil.EmojiDraft)
	}
	fmt.Fprintf(&b, "- **Source**: %s → **Target**: %s\n", mr.SourceBranch, mr.TargetBranch)
	fmt.Fprintf(&b, "- **Merge Status**: %s\n", mr.MergeStatus)
	if mr.HasConflicts {
		fmt.Fprintf(&b, "- %s **Has Conflicts**\n", toolutil.EmojiWarning)
	}
	if mr.Author != "" {
		fmt.Fprintf(&b, toolutil.FmtMdAuthorAt, mr.Author)
	}
	if len(mr.Assignees) > 0 {
		fmt.Fprintf(&b, "- **Assignees**: %s\n", strings.Join(prefixAt(mr.Assignees), ", "))
	}
	if len(mr.Reviewers) > 0 {
		fmt.Fprintf(&b, "- **Reviewers**: %s\n", strings.Join(prefixAt(mr.Reviewers), ", "))
	}
	if mr.Milestone != "" {
		fmt.Fprintf(&b, "- **Milestone**: %s\n", mr.Milestone)
	}
	if len(mr.Labels) > 0 {
		fmt.Fprintf(&b, "- **Labels**: %s\n", strings.Join(mr.Labels, ", "))
	}
	if mr.PipelineID > 0 {
		if mr.PipelineWebURL != "" {
			fmt.Fprintf(&b, "- **Pipeline**: [#%d](%s)\n", mr.PipelineID, mr.PipelineWebURL)
		} else {
			fmt.Fprintf(&b, "- **Pipeline**: #%d\n", mr.PipelineID)
		}
	}
	if mr.ChangesCount != "" {
		fmt.Fprintf(&b, "- **Changes**: %s files\n", mr.ChangesCount)
	}
	if mr.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(mr.CreatedAt))
	}
	if mr.State == "merged" && mr.MergedBy != "" {
		fmt.Fprintf(&b, "- **Merged By**: @%s", mr.MergedBy)
		if mr.MergedAt != "" {
			fmt.Fprintf(&b, " on %s", toolutil.FormatTime(mr.MergedAt))
		}
		b.WriteByte('\n')
	}
	if mr.State == "closed" && mr.ClosedBy != "" {
		fmt.Fprintf(&b, "- **Closed By**: @%s", mr.ClosedBy)
		if mr.ClosedAt != "" {
			fmt.Fprintf(&b, " on %s", toolutil.FormatTime(mr.ClosedAt))
		}
		b.WriteByte('\n')
	}
	if mr.UserNotesCount > 0 {
		fmt.Fprintf(&b, "- **Comments**: %d\n", mr.UserNotesCount)
	}
	if mr.Description != "" {
		fmt.Fprintf(&b, "\n### Description\n\n%s%s\n", toolutil.WrapGFMBody(mr.Description), toolutil.RichContentHint(toolutil.DetectRichContent(mr.Description), mr.WebURL))
	}
	fmt.Fprintf(&b, "\n- **URL**: [%[1]s](%[1]s)\n", mr.WebURL)
	toolutil.WriteHints(&b,
		"Use gitlab_mr_review action 'changes_get' to see the diff of this MR",
		"Use gitlab_mr_review action 'discussion_list' to see review threads",
		"Use gitlab_merge_request action 'pipelines' to check CI/CD status",
		"Use gitlab_merge_request action 'approve' or 'merge' to progress the MR",
	)
	return b.String()
}

// prefixAt adds '@' before each username for Markdown @mention formatting.
func prefixAt(usernames []string) []string {
	result := make([]string, len(usernames))
	for i, u := range usernames {
		result[i] = "@" + u
	}
	return result
}

// FormatListMarkdown renders a list of merge requests as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Merge Requests (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.MergeRequests), out.Pagination)
	if len(out.MergeRequests) == 0 {
		b.WriteString("No merge requests found.\n")
		return b.String()
	}
	b.WriteString("| IID | Title | State | Author | Project | Source → Target |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, mr := range out.MergeRequests {
		draftTag := ""
		if mr.Draft {
			draftTag = " " + toolutil.EmojiDraft
		}
		fmt.Fprintf(&b, "| [!%d](%s) | %s%s | %s %s | %s | %s | %s → %s |\n",
			mr.IID, mr.WebURL, toolutil.EscapeMdTableCell(mr.Title), draftTag, toolutil.MRStateEmoji(mr.State), mr.State, toolutil.EscapeMdTableCell(mr.Author), toolutil.EscapeMdTableCell(mr.ProjectPath), toolutil.EscapeMdTableCell(mr.SourceBranch), toolutil.EscapeMdTableCell(mr.TargetBranch))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'get' with a merge_request_iid to see full details",
		"Use action 'create' to open a new merge request",
		"Use gitlab_mr_review action 'changes_get' to review MR diffs",
	)
	return b.String()
}

// FormatApproveMarkdown renders the MR approval status as Markdown.
func FormatApproveMarkdown(a ApproveOutput) string {
	var b strings.Builder
	b.WriteString("## MR Approval Status\n\n")
	fmt.Fprintf(&b, "- **Approved**: %v\n", a.Approved)
	fmt.Fprintf(&b, "- **Approvals Required**: %d\n", a.ApprovalsRequired)
	fmt.Fprintf(&b, "- **Approved By**: %d\n", a.ApprovedBy)
	toolutil.WriteHints(&b,
		"Use `gitlab_mr_merge` to merge this MR",
		"Use `gitlab_mr_get` to see full MR details",
	)
	return b.String()
}

// FormatCommitsMarkdown renders a paginated list of MR commits as a Markdown table.
func FormatCommitsMarkdown(out CommitsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Commits (%d)\n\n", out.Pagination.TotalItems)
	if len(out.Commits) == 0 {
		b.WriteString("No commits found.\n")
		return b.String()
	}
	b.WriteString("| Short ID | Title | Author | Date |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, c := range out.Commits {
		fmt.Fprintf(&b, "| [%s](%s) | %s | %s | %s |\n", c.ShortID, c.WebURL, toolutil.EscapeMdTableCell(c.Title), toolutil.EscapeMdTableCell(c.AuthorName), toolutil.FormatTime(c.CommittedDate))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_commit_get` to view a specific commit",
		"Use `gitlab_mr_changes_get` to review the combined diff",
	)
	return b.String()
}

// FormatPipelinesMarkdown renders a list of MR pipelines as a Markdown table.
func FormatPipelinesMarkdown(out PipelinesOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Pipelines (%d)\n\n", len(out.Pipelines))
	if len(out.Pipelines) == 0 {
		b.WriteString("No pipelines found.\n")
		return b.String()
	}
	b.WriteString("| ID | Status | Source | Ref |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, p := range out.Pipelines {
		fmt.Fprintf(&b, "| [#%d](%s) | %s %s | %s | %s |\n",
			p.ID, p.WebURL, toolutil.PipelineStatusEmoji(p.Status), p.Status, toolutil.EscapeMdTableCell(p.Source), toolutil.EscapeMdTableCell(p.Ref))
	}
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_pipeline_get` to view pipeline details",
		"Use `gitlab_job_list` to see job statuses",
	)
	return b.String()
}

// FormatRebaseMarkdown renders a rebase result as a short Markdown message.
func FormatRebaseMarkdown(r RebaseOutput) string {
	var b strings.Builder
	if r.RebaseInProgress {
		b.WriteString("## " + toolutil.EmojiRefresh + " Rebase in progress\n\nThe rebase has been initiated and is currently running.\n")
	} else {
		b.WriteString("## " + toolutil.EmojiSuccess + " Rebase completed\n\nThe rebase has finished successfully.\n")
	}
	toolutil.WriteHints(&b,
		"Use gitlab_mr_get to check rebase status",
		"Use action 'merge' once the rebase is complete",
	)
	return b.String()
}

// FormatParticipantsMarkdown renders MR participants as a Markdown table.
func FormatParticipantsMarkdown(out ParticipantsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Participants (%d)\n\n", len(out.Participants))
	if len(out.Participants) == 0 {
		b.WriteString("No participants found.\n")
		return b.String()
	}
	b.WriteString("| ID | Username | Name | State |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, p := range out.Participants {
		fmt.Fprintf(&b, "| %d | [@%s](%s) | %s | %s |\n",
			p.ID, toolutil.EscapeMdTableCell(p.Username), p.WebURL, toolutil.EscapeMdTableCell(p.Name), p.State)
	}
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_mr_get` to view MR details",
		"Use `gitlab_mr_note_create` to notify participants",
	)
	return b.String()
}

// FormatReviewersMarkdown renders MR reviewers as a Markdown table.
func FormatReviewersMarkdown(out ReviewersOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Reviewers (%d)\n\n", len(out.Reviewers))
	if len(out.Reviewers) == 0 {
		b.WriteString("No reviewers found.\n")
		return b.String()
	}
	b.WriteString("| ID | Username | Name | Review State | Assigned At |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, r := range out.Reviewers {
		fmt.Fprintf(&b, "| %d | [@%s](%s) | %s | %s | %s |\n",
			r.ID, toolutil.EscapeMdTableCell(r.Username), r.WebURL, toolutil.EscapeMdTableCell(r.Name), r.Review, toolutil.FormatTime(r.CreatedAt))
	}
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_mr_update` to add or change reviewers",
		"Use `gitlab_mr_approve` to approve the MR",
	)
	return b.String()
}

// FormatIssuesClosedMarkdown renders the issues-closed-on-merge list as a Markdown table.
func FormatIssuesClosedMarkdown(out IssuesClosedOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Issues Closed on Merge (%d)\n\n", out.Pagination.TotalItems)
	if len(out.Issues) == 0 {
		b.WriteString("No issues will be closed on merge.\n")
		return b.String()
	}
	b.WriteString("| IID | Title | State | Author | Labels |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, issue := range out.Issues {
		fmt.Fprintf(&b, "| [#%d](%s) | %s | %s | %s | %s |\n",
			issue.IID, issue.WebURL, toolutil.EscapeMdTableCell(issue.Title), issue.State, toolutil.EscapeMdTableCell(issue.Author), strings.Join(issue.Labels, ", "))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_issue_get` to view details of an issue",
		"Use `gitlab_mr_merge` to merge and close these issues",
	)
	return b.String()
}

// FormatCreatePipelineMarkdown renders a single pipeline (from create-pipeline) as Markdown.
func FormatCreatePipelineMarkdown(p pipelines.Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s Pipeline #%d Created\n\n", toolutil.PipelineStatusEmoji(p.Status), p.ID)
	fmt.Fprintf(&b, "- **Status**: %s %s\n", toolutil.PipelineStatusEmoji(p.Status), p.Status)
	if p.Source != "" {
		fmt.Fprintf(&b, "- **Source**: %s\n", p.Source)
	}
	if p.Ref != "" {
		fmt.Fprintf(&b, "- **Ref**: %s\n", p.Ref)
	}
	if p.SHA != "" {
		fmt.Fprintf(&b, "- **SHA**: %s\n", p.SHA)
	}
	if p.WebURL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURL, p.WebURL)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_pipeline_get` to check pipeline progress",
		"Use `gitlab_job_list` to monitor job statuses",
	)
	return b.String()
}

// FormatTimeStatsMarkdown renders time tracking statistics as markdown.
func FormatTimeStatsMarkdown(ts TimeStatsOutput) string {
	var b strings.Builder
	b.WriteString("## Time Tracking Stats\n\n")
	if ts.HumanTimeEstimate != "" {
		fmt.Fprintf(&b, "- **Estimate**: %s (%d seconds)\n", ts.HumanTimeEstimate, ts.TimeEstimate)
	} else {
		b.WriteString("- **Estimate**: not set\n")
	}
	if ts.HumanTotalTimeSpent != "" {
		fmt.Fprintf(&b, "- **Spent**: %s (%d seconds)\n", ts.HumanTotalTimeSpent, ts.TotalTimeSpent)
	} else {
		b.WriteString("- **Spent**: none\n")
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_mr_update` to add time tracking notes",
	)
	return b.String()
}

// FormatRelatedIssuesMarkdown renders related issues as markdown.
func FormatRelatedIssuesMarkdown(out RelatedIssuesOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Related Issues (%d)\n\n", len(out.Issues))
	if len(out.Issues) == 0 {
		b.WriteString("No related issues found.\n")
		return b.String()
	}
	b.WriteString("| IID | Title | State | Author | Labels |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, iss := range out.Issues {
		fmt.Fprintf(&b, "| [#%d](%s) | %s | %s | %s | %s |\n",
			iss.IID,
			iss.WebURL,
			toolutil.EscapeMdTableCell(iss.Title),
			iss.State,
			iss.Author,
			strings.Join(iss.Labels, ", "))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_issue_get` to view issue details",
		"Use `gitlab_issue_note_create` to comment on an issue",
	)
	return b.String()
}

// FormatCreateTodoMarkdown renders a created to-do item as markdown.
func FormatCreateTodoMarkdown(t CreateTodoOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Todo Created (#%d)\n\n", t.ID)
	fmt.Fprintf(&b, "- **Action**: %s\n", t.ActionName)
	fmt.Fprintf(&b, "- **Type**: %s\n", t.TargetType)
	if t.TargetTitle != "" {
		fmt.Fprintf(&b, toolutil.FmtMdTarget, t.TargetTitle)
	}
	if t.TargetURL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURL, t.TargetURL)
	}
	fmt.Fprintf(&b, toolutil.FmtMdState, t.State)
	toolutil.WriteHints(&b,
		"Use `gitlab_mr_get` to view the MR",
		"Use `gitlab_todo_mark_done` to mark completed",
	)
	return b.String()
}

// FormatDependencyMarkdown renders a single merge request dependency as markdown.
func FormatDependencyMarkdown(d DependencyOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Dependency (#%d)\n\n", d.ID)
	fmt.Fprintf(&b, "- **Blocking MR**: !%d (ID: %d)\n", d.BlockingMRIID, d.BlockingMRID)
	fmt.Fprintf(&b, toolutil.FmtMdTitle, d.BlockingMRTitle)
	fmt.Fprintf(&b, toolutil.FmtMdState, d.BlockingMRState)
	fmt.Fprintf(&b, "- **Source**: %s → **Target**: %s\n", d.BlockingSourceBranch, d.BlockingTargetBranch)
	toolutil.WriteHints(&b,
		"Use `gitlab_mr_get` to view the blocking MR",
	)
	return b.String()
}

// FormatDependenciesMarkdown renders a list of merge request dependencies as markdown.
func FormatDependenciesMarkdown(out DependenciesOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Dependencies (%d)\n\n", len(out.Dependencies))
	if len(out.Dependencies) == 0 {
		b.WriteString("No dependencies found.\n")
		return b.String()
	}
	b.WriteString("| ID | Blocking MR | Title | State |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, d := range out.Dependencies {
		fmt.Fprintf(&b, "| %d | !%d | %s | %s |\n",
			d.ID,
			d.BlockingMRIID,
			toolutil.EscapeMdTableCell(d.BlockingMRTitle),
			d.BlockingMRState)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_mr_get` to view a blocking MR",
		"Use `gitlab_mr_merge` to resolve blocking dependencies",
	)
	return b.String()
}

func init() {
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
}
