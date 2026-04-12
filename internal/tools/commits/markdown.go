// markdown.go provides Markdown formatting functions for commit MCP tool output.

package commits

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single commit as a Markdown summary.
func FormatOutputMarkdown(c Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Commit %s\n\n", c.ShortID)
	fmt.Fprintf(&b, toolutil.FmtMdTitle, c.Title)
	fmt.Fprintf(&b, "- **Author**: %s <%s>\n", c.AuthorName, c.AuthorEmail)
	fmt.Fprintf(&b, "- **Date**: %s\n", toolutil.FormatTime(c.CommittedDate))
	fmt.Fprintf(&b, toolutil.FmtMdURL, c.WebURL)
	toolutil.WriteHints(&b,
		"Use action 'commit_get' with this SHA to see full commit details and stats",
		"Use action 'commit_diff' to see file changes for this commit",
		"Use action 'commit_refs' to see branches/tags containing this commit",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of commits as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Commits (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Commits), out.Pagination)
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
		"Use action 'commit_get' with a SHA to see commit summary",
		"Use action 'commit_diff' to see file changes for a specific commit",
	)
	return b.String()
}

// FormatDetailMarkdown renders a single commit detail as a Markdown summary.
func FormatDetailMarkdown(c DetailOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Commit %s\n\n", c.ShortID)
	fmt.Fprintf(&b, toolutil.FmtMdTitle, c.Title)
	fmt.Fprintf(&b, "- **Author**: %s <%s>\n", c.AuthorName, c.AuthorEmail)
	fmt.Fprintf(&b, "- **Date**: %s\n", toolutil.FormatTime(c.CommittedDate))
	if len(c.ParentIDs) > 0 {
		fmt.Fprintf(&b, "- **Parents**: %s\n", strings.Join(c.ParentIDs, ", "))
	}
	if c.Stats != nil {
		fmt.Fprintf(&b, "- **Stats**: +%d -%d (%d total)\n", c.Stats.Additions, c.Stats.Deletions, c.Stats.Total)
	}
	if c.Message != "" && c.Message != c.Title {
		fmt.Fprintf(&b, "\n### Message\n\n%s\n", c.Message)
	}
	fmt.Fprintf(&b, toolutil.FmtMdURLNewline, c.WebURL)
	toolutil.WriteHints(&b,
		"Use `gitlab_commit_diff` to view file changes",
		"Use `gitlab_commit_cherry_pick` to apply this commit to another branch",
	)
	return b.String()
}

// FormatDiffMarkdown renders a paginated list of commit diffs as a Markdown table.
func FormatDiffMarkdown(out DiffOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Commit Diffs (%d files)\n\n", len(out.Diffs))
	if len(out.Diffs) == 0 {
		b.WriteString("No diffs found.\n")
		return b.String()
	}
	b.WriteString("| Status | Old Path | New Path |\n")
	b.WriteString(toolutil.TblSep3Col)
	for _, d := range out.Diffs {
		status := "modified"
		if d.NewFile {
			status = "added"
		} else if d.DeletedFile {
			status = "deleted"
		} else if d.RenamedFile {
			status = "renamed"
		}
		fmt.Fprintf(&b, toolutil.FmtRow3Str, status, toolutil.EscapeMdTableCell(d.OldPath), toolutil.EscapeMdTableCell(d.NewPath))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use `gitlab_file_get` to view a specific changed file",
		"Use `gitlab_commit_comment_create` to comment on the changes",
	)
	return b.String()
}

// FormatRefsMarkdown renders a paginated list of commit refs as Markdown.
func FormatRefsMarkdown(out RefsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Commit Refs (%d)\n\n", len(out.Refs))
	if len(out.Refs) == 0 {
		b.WriteString("No branch or tag refs found.\n")
		return b.String()
	}
	b.WriteString("| Type | Name |\n")
	b.WriteString(toolutil.TblSep2Col)
	for _, r := range out.Refs {
		fmt.Fprintf(&b, "| %s | %s |\n", r.Type, toolutil.EscapeMdTableCell(r.Name))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use `gitlab_branch_get` to view branch details",
		"Use `gitlab_tag_get` to view tag details",
	)
	return b.String()
}

// FormatCommentsMarkdown renders a paginated list of commit comments.
func FormatCommentsMarkdown(out CommentsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Commit Comments (%d)\n\n", len(out.Comments))
	if len(out.Comments) == 0 {
		b.WriteString("No commit comments found.\n")
		return b.String()
	}
	b.WriteString("| Author | Note | Path | Line |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, c := range out.Comments {
		path := c.Path
		if path == "" {
			path = "—"
		}
		line := "—"
		if c.Line > 0 {
			line = strconv.FormatInt(c.Line, 10)
		}
		fmt.Fprintf(&b, toolutil.FmtRow4Str, toolutil.EscapeMdTableCell(c.Author), toolutil.EscapeMdTableCell(c.Note), toolutil.EscapeMdTableCell(path), line)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use `gitlab_commit_comment_create` to add a comment",
		"Use `gitlab_commit_get` to view the commit details",
	)
	return b.String()
}

// FormatCommentMarkdown renders a single commit comment.
func FormatCommentMarkdown(c CommentOutput) string {
	var b strings.Builder
	b.WriteString("## Commit Comment\n\n")
	fmt.Fprintf(&b, toolutil.FmtMdAuthor, c.Author)
	fmt.Fprintf(&b, "- **Note**: %s\n", c.Note)
	if c.Path != "" {
		fmt.Fprintf(&b, "- **Path**: %s (line %d)\n", c.Path, c.Line)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_commit_comments` to list all comments",
		"Use `gitlab_file_get` to view the referenced file",
	)
	return b.String()
}

// FormatStatusesMarkdown renders a paginated list of commit statuses.
func FormatStatusesMarkdown(out StatusesOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Commit Statuses (%d)\n\n", len(out.Statuses))
	if len(out.Statuses) == 0 {
		b.WriteString("No commit statuses found.\n")
		return b.String()
	}
	b.WriteString("| ID | Status | Name | Ref | Description |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, s := range out.Statuses {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n", s.ID, s.Status, toolutil.EscapeMdTableCell(s.Name), toolutil.EscapeMdTableCell(s.Ref), toolutil.EscapeMdTableCell(s.Description))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use `gitlab_commit_status_set` to update a status",
		"Use `gitlab_commit_get` to view commit details",
	)
	return b.String()
}

// FormatStatusMarkdown renders a single commit status.
func FormatStatusMarkdown(s StatusOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Commit Status #%d\n\n", s.ID)
	fmt.Fprintf(&b, toolutil.FmtMdStatus, s.Status)
	fmt.Fprintf(&b, toolutil.FmtMdName, s.Name)
	fmt.Fprintf(&b, "- **Ref**: %s\n", s.Ref)
	if s.Description != "" {
		fmt.Fprintf(&b, toolutil.FmtMdDescription, s.Description)
	}
	if s.TargetURL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURL, s.TargetURL)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_commit_status_set` to update this status",
		"Use `gitlab_commit_statuses` to see all statuses",
	)
	return b.String()
}

// FormatMRsByCommitMarkdown renders a list of merge requests for a commit.
func FormatMRsByCommitMarkdown(out MRsByCommitOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Merge Requests for Commit (%d)\n\n", len(out.MergeRequests))
	if len(out.MergeRequests) == 0 {
		b.WriteString("No merge requests found.\n")
		return b.String()
	}
	b.WriteString("| IID | Title | State | Source → Target | Author |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, mr := range out.MergeRequests {
		fmt.Fprintf(&b, "| !%d | %s | %s | %s → %s | %s |\n",
			mr.IID, toolutil.EscapeMdTableCell(mr.Title), mr.State,
			toolutil.EscapeMdTableCell(mr.SourceBranch), toolutil.EscapeMdTableCell(mr.TargetBranch),
			toolutil.EscapeMdTableCell(mr.Author))
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_mr_get` to view MR details",
		"Use `gitlab_mr_changes_get` to see MR diff",
	)
	return b.String()
}

// FormatGPGSignatureMarkdown renders a GPG signature as Markdown.
func FormatGPGSignatureMarkdown(sig GPGSignatureOutput) string {
	var b strings.Builder
	b.WriteString("## GPG Signature\n\n")
	fmt.Fprintf(&b, "- **Verification**: %s\n", sig.VerificationStatus)
	fmt.Fprintf(&b, "- **Key User**: %s <%s>\n", sig.KeyUserName, sig.KeyUserEmail)
	fmt.Fprintf(&b, "- **Key ID**: %d\n", sig.KeyID)
	fmt.Fprintf(&b, "- **Primary Key ID**: %s\n", sig.KeyPrimaryKeyID)
	toolutil.WriteHints(&b,
		"Use `gitlab_commit_get` to view the full commit details",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatDetailMarkdown)
	toolutil.RegisterMarkdown(FormatDiffMarkdown)
	toolutil.RegisterMarkdown(FormatRefsMarkdown)
	toolutil.RegisterMarkdown(FormatCommentsMarkdown)
	toolutil.RegisterMarkdown(FormatCommentMarkdown)
	toolutil.RegisterMarkdown(FormatStatusesMarkdown)
	toolutil.RegisterMarkdown(FormatStatusMarkdown)
	toolutil.RegisterMarkdown(FormatMRsByCommitMarkdown)
	toolutil.RegisterMarkdown(FormatGPGSignatureMarkdown)
}
