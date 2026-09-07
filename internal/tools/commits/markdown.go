package commits

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// userDisplay returns a human-readable name for a commit comment/status author,
// preferring the username and falling back to the display name. Returns a
// dash when the author is absent.
func userDisplay(u *BasicUserOutput) string {
	if u == nil {
		return "-"
	}
	if u.Username != "" {
		return u.Username
	}
	if u.Name != "" {
		return u.Name
	}
	return "-"
}

// FormatOutputMarkdown renders a single commit as a Markdown summary.
func FormatOutputMarkdown(c Output) string {
	var b strings.Builder
	//gitlab:allow-unescaped c.ShortID: an abbreviated git object id, which is hexadecimal.
	fmt.Fprintf(&b, "## Commit %s\n\n", c.ShortID)
	// A commit's title and ident are what whoever made the commit typed, and
	// this server's own commit.create passes an author name and email through.
	fmt.Fprintf(&b, toolutil.FmtMdTitle, toolutil.EscapeMdTableCell(c.Title))
	fmt.Fprintf(&b, "- **Author**: %s <%s>\n",
		toolutil.EscapeMdTableCell(c.AuthorName), toolutil.EscapeMdTableCell(c.AuthorEmail))
	fmt.Fprintf(&b, "- **Date**: %s\n", toolutil.FormatTime(c.CommittedDate))
	toolutil.WriteMdURL(&b, c.WebURL)
	toolutil.WriteHints(
		&b,
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
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", toolutil.MdTitleLink(c.ShortID, c.WebURL), toolutil.EscapeMdTableCell(c.Title), toolutil.EscapeMdTableCell(c.AuthorName), toolutil.FormatTime(c.CommittedDate))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use action 'commit_get' with a SHA to see commit summary",
		"Use action 'commit_diff' to see file changes for a specific commit",
	)
	return b.String()
}

// FormatDetailMarkdown renders a single commit detail as a Markdown summary.
func FormatDetailMarkdown(c DetailOutput) string {
	var b strings.Builder
	//gitlab:allow-unescaped c.ShortID: an abbreviated git object id, which is hexadecimal.
	fmt.Fprintf(&b, "## Commit %s\n\n", c.ShortID)
	// A commit's title and ident are what whoever made the commit typed, and
	// this server's own commit.create passes an author name and email through.
	fmt.Fprintf(&b, toolutil.FmtMdTitle, toolutil.EscapeMdTableCell(c.Title))
	fmt.Fprintf(&b, "- **Author**: %s <%s>\n",
		toolutil.EscapeMdTableCell(c.AuthorName), toolutil.EscapeMdTableCell(c.AuthorEmail))
	fmt.Fprintf(&b, "- **Date**: %s\n", toolutil.FormatTime(c.CommittedDate))
	if len(c.ParentIDs) > 0 {
		//gitlab:allow-unescaped strings.Join(c.ParentIDs, ", "): full git object ids, which are hexadecimal.
		fmt.Fprintf(&b, "- **Parents**: %s\n", strings.Join(c.ParentIDs, ", "))
	}
	if c.Stats != nil {
		fmt.Fprintf(&b, "- **Stats**: +%d -%d (%d total)\n", c.Stats.Additions, c.Stats.Deletions, c.Stats.Total)
	}
	if c.Message != "" && c.Message != c.Title {
		fmt.Fprintf(&b, "\n### Message\n\n%s\n", toolutil.WrapGFMBody(c.Message))
	}
	toolutil.WriteMdURLNewline(&b, c.WebURL)
	toolutil.WriteHints(
		&b,
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
		switch {
		case d.NewFile:
			status = "added"
		case d.DeletedFile:
			status = "deleted"
		case d.RenamedFile:
			status = "renamed"
		}
		fmt.Fprintf(&b, toolutil.FmtRow3Str, status, toolutil.EscapeMdTableCell(d.OldPath), toolutil.EscapeMdTableCell(d.NewPath))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
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
		//gitlab:allow-unescaped r.Type: the ref kind GitLab answers with, either branch or tag.
		fmt.Fprintf(&b, "| %s | %s |\n", r.Type, toolutil.EscapeMdTableCell(r.Name))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
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
			path = "-"
		}
		line := "-"
		if c.Line > 0 {
			line = strconv.FormatInt(c.Line, 10)
		}
		fmt.Fprintf(&b, toolutil.FmtRow4Str, toolutil.EscapeMdTableCell(userDisplay(c.Author)), toolutil.EscapeMdTableCell(c.Note), toolutil.EscapeMdTableCell(path), line)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		"Use `gitlab_commit_comment_create` to add a comment",
		"Use `gitlab_commit_get` to view the commit details",
	)
	return b.String()
}

// FormatCommentMarkdown renders a single commit comment.
func FormatCommentMarkdown(c CommentOutput) string {
	var b strings.Builder
	b.WriteString("## Commit Comment\n\n")
	fmt.Fprintf(&b, toolutil.FmtMdAuthor, toolutil.EscapeMdTableCell(userDisplay(c.Author)))
	fmt.Fprintf(&b, "- **Note**: %s\n", toolutil.EscapeMdTableCell(c.Note))
	if c.Path != "" {
		fmt.Fprintf(&b, "- **Path**: %s (line %d)\n", toolutil.EscapeMdTableCell(c.Path), c.Line)
	}
	toolutil.WriteHints(
		&b,
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
		//gitlab:allow-unescaped s.Status: a build state GitLab rejects with a 400 unless it is one of the six its own schema enumerates.
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n", s.ID, s.Status, toolutil.EscapeMdTableCell(s.Name), toolutil.EscapeMdTableCell(s.Ref), toolutil.EscapeMdTableCell(s.Description))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
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
	// The name and the ref of a commit status are both supplied by whatever CI
	// system posted it, and this server's own commit_status_set writes them.
	fmt.Fprintf(&b, toolutil.FmtMdName, toolutil.EscapeMdTableCell(s.Name))
	fmt.Fprintf(&b, "- **Ref**: %s\n", toolutil.EscapeMdTableCell(s.Ref))
	if s.Description != "" {
		toolutil.WriteDescription(&b, s.Description)
	}
	if s.TargetURL != "" {
		toolutil.WriteMdURL(&b, s.TargetURL)
	}
	toolutil.WriteHints(
		&b,
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
	b.WriteString("| IID | Title | State | Source -> Target | Author |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, mr := range out.MergeRequests {
		fmt.Fprintf(&b, "| !%d | %s | %s | %s -> %s | %s |\n",
			//gitlab:allow-unescaped mr.State: a merge request state, one of GitLab's fixed set (opened, closed, locked, merged).
			mr.IID, toolutil.EscapeMdTableCell(mr.Title), mr.State,
			toolutil.EscapeMdTableCell(mr.SourceBranch), toolutil.EscapeMdTableCell(mr.TargetBranch),
			toolutil.EscapeMdTableCell(mr.Author))
	}
	toolutil.WriteHints(
		&b,
		"Use `gitlab_mr_get` to view MR details",
		"Use `gitlab_mr_changes_get` to see MR diff",
	)
	return b.String()
}

// FormatGPGSignatureMarkdown renders a GPG signature as Markdown.
func FormatGPGSignatureMarkdown(sig GPGSignatureOutput) string {
	var b strings.Builder
	b.WriteString("## Commit Signature\n\n")
	if sig.SignatureType != "" {
		//gitlab:allow-unescaped sig.SignatureType: the signing scheme GitLab names, one of PGP, SSH or X509.
		fmt.Fprintf(&b, "- **Type**: %s\n", sig.SignatureType)
	}
	//gitlab:allow-unescaped sig.VerificationStatus: a verification verdict GitLab computes, not text it stored.
	fmt.Fprintf(&b, "- **Verification**: %s\n", sig.VerificationStatus)
	switch {
	case sig.X509Certificate != nil:
		fmt.Fprintf(&b, "- **X.509 Subject**: %s\n", toolutil.EscapeMdTableCell(sig.X509Certificate.Subject))
		if sig.X509Certificate.Email != "" {
			// Read out of the signer's own certificate, like the subject above.
			fmt.Fprintf(&b, "- **X.509 Email**: %s\n", toolutil.EscapeMdTableCell(sig.X509Certificate.Email))
		}
	case sig.Key != nil:
		if sig.Key.Title != "" {
			fmt.Fprintf(&b, "- **SSH Key**: %s\n", toolutil.EscapeMdTableCell(sig.Key.Title))
		}
		if sig.Key.UsageType != "" {
			//gitlab:allow-unescaped sig.Key.UsageType: the key usage GitLab records, one of auth, signing or auth_and_signing.
			fmt.Fprintf(&b, "- **Usage**: %s\n", sig.Key.UsageType)
		}
	default:
		// Both come out of the GPG key's user ID packet, which is whatever the
		// key's owner typed when they generated it.
		fmt.Fprintf(&b, "- **Key User**: %s <%s>\n",
			toolutil.EscapeMdTableCell(sig.KeyUserName), toolutil.EscapeMdTableCell(sig.KeyUserEmail))
		fmt.Fprintf(&b, "- **Key ID**: %d\n", sig.KeyID)
		//gitlab:allow-unescaped sig.KeyPrimaryKeyID: a GPG key id, which is hexadecimal.
		fmt.Fprintf(&b, "- **Primary Key ID**: %s\n", sig.KeyPrimaryKeyID)
	}
	if sig.CommitSource != "" {
		//gitlab:allow-unescaped sig.CommitSource: where GitLab read the commit from, either gitaly or rugged.
		fmt.Fprintf(&b, "- **Commit Source**: %s\n", sig.CommitSource)
	}
	toolutil.WriteHints(
		&b,
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
