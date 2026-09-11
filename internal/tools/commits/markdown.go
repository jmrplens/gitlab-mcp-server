package commits

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves:
// the dynamic surface executes them, and the meta and individual surfaces
// resolve them to their own tool names. Every commit action is a route on the
// gitlab_repository catalog group, so each ID carries that domain rather than
// a "commit." prefix of its own.
const (
	hintCommitGet           = "repository.commit_get"
	hintCommitList          = "repository.commit_list"
	hintCommitDiff          = "repository.commit_diff"
	hintCommitRefs          = "repository.commit_refs"
	hintCommitComments      = "repository.commit_comments"
	hintCommitCommentCreate = "repository.commit_comment_create"
	hintCommitStatuses      = "repository.commit_statuses"
	hintCommitStatusSet     = "repository.commit_status_set"
	hintCommitCherryPick    = "repository.commit_cherry_pick"
	hintFileGet             = "repository.file_get"
	hintBranchGet           = "branch.get"
	hintTagGet              = "tag.get"
	hintMRGet               = "merge_request.get"
	hintMRChangesGet        = "merge_request.changes_get"
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

// userHandle renders a commit comment or status author as the "@handle"
// GitLab shows, and nothing at all when GitLab sent no author, so a card
// carries no row rather than a dash.
func userHandle(u *BasicUserOutput) string {
	if u == nil {
		return ""
	}
	if handle := toolutil.MdUserHandle(u.Username); handle != "" {
		return handle
	}
	return toolutil.EscapeMdTableCell(u.Name)
}

// pipelineSummary renders the pipeline of a commit as its status glyph, the
// status word and a link to the pipeline itself, and nothing when GitLab
// reported neither. A commit whose pipeline the card omits is one a reader
// cannot tell has failed.
func pipelineSummary(status string, p *LastPipelineOutput) string {
	state := status
	if p != nil && p.Status != "" {
		state = p.Status
	}
	summary := ""
	if state != "" {
		summary = toolutil.PipelineStatusEmoji(state) + " " + toolutil.EscapeMdTableCell(state)
	}
	if p == nil || p.ID == 0 {
		return summary
	}
	link := toolutil.MdTitleLink("#"+strconv.FormatInt(p.ID, 10), p.WebURL)
	if summary == "" {
		return link
	}
	return summary + " " + link
}

// commitIdent renders the person a commit names: their name, and the address
// beside it in parentheses rather than in angle brackets, which GFM turns
// into a mailto autolink to an address nobody chose to publish.
func commitIdent(name, email string) string {
	switch {
	case name == "" && email == "":
		return ""
	case email == "":
		return toolutil.EscapeMdTableCell(name)
	case name == "":
		return toolutil.EscapeMdTableCell(email)
	default:
		return toolutil.EscapeMdTableCell(name) + " (" + toolutil.EscapeMdTableCell(email) + ")"
	}
}

// FormatOutputMarkdown renders one commit as a card.
//
// A cherry-pick or revert run with dry_run commits nothing and GitLab answers
// with no commit at all, which this used to render as a commit card whose
// heading and every field were empty: a reader could not tell it from a commit
// that had been made.
func FormatOutputMarkdown(c Output) string {
	if c.ID == "" && c.ShortID == "" {
		return formatNoCommitMarkdown()
	}
	var b strings.Builder
	card := toolutil.NewCard(&b, "Commit "+c.ShortID)
	// A commit's title and ident are what whoever made the commit typed, and
	// this server's own commit.create passes an author name and email through.
	card.Field("Title", c.Title)
	card.Markdown("Author", commitIdent(c.AuthorName, c.AuthorEmail))
	card.Time("Date", c.CommittedDate)
	card.Markdown("Pipeline", pipelineSummary(c.Status, c.LastPipeline))
	card.URL(c.WebURL)
	card.End(
		toolutil.HintAction(hintCommitGet, "see this commit's full details and stats"),
		toolutil.HintAction(hintCommitDiff, "see the file changes for this commit"),
		toolutil.HintAction(hintCommitRefs, "see the branches and tags containing it"),
	)
	return b.String()
}

// formatNoCommitMarkdown is the answer to a call GitLab returned no commit
// for: a dry run reports whether the change applies and commits nothing.
func formatNoCommitMarkdown() string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "No Commit Created")
	card.Note("GitLab returned no commit. A dry run reports whether the change would apply cleanly and commits nothing; run the same action without dry_run to commit it.")
	card.End(
		toolutil.HintAction(hintCommitCherryPick, "apply the commit for real"),
		toolutil.HintAction(hintCommitList, "check the branch for the commit"),
	)
	return b.String()
}

// FormatListMarkdown renders a page of commits as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Commits) == 0 {
		return toolutil.EmptyMessage("commits")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Commits", len(out.Commits), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Short ID", "Title", "Author", "Date", "Pipeline"))
	for _, c := range out.Commits {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(c.ShortID, c.WebURL),
			toolutil.EscapeMdTableCell(c.Title),
			toolutil.EscapeMdTableCell(c.AuthorName),
			toolutil.FormatTime(c.CommittedDate),
			pipelineSummary(c.Status, c.LastPipeline),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(hintCommitGet, "see one commit in full"),
		toolutil.HintAction(hintCommitDiff, "see the file changes of one commit"),
	)
	return b.String()
}

// FormatDetailMarkdown renders one commit in full as a card: its fields, then
// its message as quoted prose when the message says more than the title.
func FormatDetailMarkdown(c DetailOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "Commit "+c.ShortID)
	// A commit's title and ident are what whoever made the commit typed, and
	// this server's own commit.create passes an author name and email through.
	card.Field("Title", c.Title)
	card.Markdown("Author", commitIdent(c.AuthorName, c.AuthorEmail))
	card.Markdown("Committer", commitIdent(c.CommitterName, c.CommitterEmail))
	card.Time("Date", c.CommittedDate)
	card.Code("Parents", strings.Join(c.ParentIDs, ", "))
	if c.Stats != nil {
		card.Field("Stats", "+"+strconv.FormatInt(c.Stats.Additions, 10)+
			" -"+strconv.FormatInt(c.Stats.Deletions, 10)+
			" ("+strconv.FormatInt(c.Stats.Total, 10)+" total)")
	}
	card.Markdown("Pipeline", pipelineSummary(c.Status, c.LastPipeline))
	card.URL(c.WebURL)
	if c.Message != "" && c.Message != c.Title {
		card.Text("Message", c.Message)
	}
	card.End(
		toolutil.HintAction(hintCommitDiff, "view the file changes"),
		toolutil.HintAction(hintCommitCherryPick, "apply this commit to another branch"),
	)
	return b.String()
}

// FormatDiffMarkdown renders the files one commit changed as a Markdown table.
func FormatDiffMarkdown(out DiffOutput) string {
	if len(out.Diffs) == 0 {
		return toolutil.EmptyMessage("changed files")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Commit Diffs", len(out.Diffs), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Status", "Old Path", "New Path"))
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
		b.WriteString(toolutil.MarkdownTableRow(
			status,
			toolutil.MdCodeSpanCell(d.OldPath),
			toolutil.MdCodeSpanCell(d.NewPath),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(hintFileGet, "view one changed file"),
		toolutil.HintAction(hintCommitCommentCreate, "comment on the changes"),
	)
	return b.String()
}

// FormatRefsMarkdown renders the branches and tags a commit is on as a
// Markdown table.
func FormatRefsMarkdown(out RefsOutput) string {
	if len(out.Refs) == 0 {
		return toolutil.EmptyMessage("branch or tag refs")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Commit Refs", len(out.Refs), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Type", "Name"))
	for _, r := range out.Refs {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(r.Type),
			toolutil.EscapeMdTableCell(r.Name),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(hintBranchGet, "view one branch"),
		toolutil.HintAction(hintTagGet, "view one tag"),
	)
	return b.String()
}

// FormatCommentsMarkdown renders a page of commit comments as a Markdown
// table.
func FormatCommentsMarkdown(out CommentsOutput) string {
	if len(out.Comments) == 0 {
		return toolutil.EmptyMessage("commit comments")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Commit Comments", len(out.Comments), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Author", "Note", "Path", "Line"))
	for _, c := range out.Comments {
		path := c.Path
		if path == "" {
			path = "-"
		}
		line := "-"
		if c.Line > 0 {
			line = strconv.FormatInt(c.Line, 10)
		}
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(userDisplay(c.Author)),
			toolutil.EscapeMdTableCell(c.Note),
			toolutil.EscapeMdTableCell(path),
			line,
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(hintCommitCommentCreate, "add a comment"),
		toolutil.HintAction(hintCommitGet, "view the commit"),
	)
	return b.String()
}

// FormatCommentMarkdown renders one commit comment as a card.
func FormatCommentMarkdown(c CommentOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "Commit Comment")
	card.Markdown("Author", userHandle(c.Author))
	card.Time("Created", c.CreatedAt)
	card.Code("Path", c.Path)
	// A line of zero means the comment is on the commit rather than on a line,
	// which "line 0" read as a line number.
	card.Count("Line", c.Line)
	card.Field("Line Type", c.LineType)
	card.Text("Note", c.Note)
	card.End(
		toolutil.HintAction(hintCommitComments, "list every comment on the commit"),
		toolutil.HintAction(hintFileGet, "view the referenced file"),
	)
	return b.String()
}

// FormatStatusesMarkdown renders a page of commit statuses as a Markdown
// table.
func FormatStatusesMarkdown(out StatusesOutput) string {
	if len(out.Statuses) == 0 {
		return toolutil.EmptyMessage("commit statuses")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Commit Statuses", len(out.Statuses), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Status", "Name", "Ref", "Description"))
	for _, s := range out.Statuses {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(s.ID, 10),
			statusCell(s.Status),
			toolutil.EscapeMdTableCell(s.Name),
			toolutil.EscapeMdTableCell(s.Ref),
			toolutil.EscapeMdTableCell(s.Description),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(hintCommitStatusSet, "update a status"),
		toolutil.HintAction(hintCommitGet, "view the commit"),
	)
	return b.String()
}

// statusCell renders a build state with the glyph every pipeline status in
// this tree carries, and nothing when GitLab sent no state.
func statusCell(status string) string {
	if status == "" {
		return ""
	}
	return toolutil.PipelineStatusEmoji(status) + " " + toolutil.EscapeMdTableCell(status)
}

// FormatStatusMarkdown renders one commit status as a card.
func FormatStatusMarkdown(s StatusOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "Commit Status #"+strconv.FormatInt(s.ID, 10))
	card.Markdown("Status", statusCell(s.Status))
	// The name and the ref of a commit status are both supplied by whatever CI
	// system posted it, and this server's own commit_status_set writes them.
	card.Field("Name", s.Name)
	card.Field("Ref", s.Ref)
	card.Code("SHA", s.SHA)
	card.Markdown("Author", userHandle(s.Author))
	card.Bool("Allow Failure", s.AllowFailure)
	card.Count("Pipeline ID", s.PipelineID)
	card.Time("Created", s.CreatedAt)
	card.Time("Started", s.StartedAt)
	card.Time("Finished", s.FinishedAt)
	card.Link("Target", s.TargetURL, s.TargetURL)
	card.Text("Description", s.Description)
	card.End(
		toolutil.HintAction(hintCommitStatusSet, "update this status"),
		toolutil.HintAction(hintCommitStatuses, "see all statuses on the commit"),
	)
	return b.String()
}

// FormatMRsByCommitMarkdown renders the merge requests a commit belongs to as
// a Markdown table.
func FormatMRsByCommitMarkdown(out MRsByCommitOutput) string {
	if len(out.MergeRequests) == 0 {
		return toolutil.EmptyMessage("merge requests")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Merge Requests for Commit", len(out.MergeRequests), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Source -> Target", "Author"))
	for _, mr := range out.MergeRequests {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink("!"+strconv.FormatInt(mr.IID, 10), mr.WebURL),
			toolutil.EscapeMdTableCell(mr.Title),
			mrStateCell(mr.State),
			toolutil.EscapeMdTableCell(mr.SourceBranch)+" -> "+toolutil.EscapeMdTableCell(mr.TargetBranch),
			toolutil.EscapeMdTableCell(mr.Author),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, true,
		toolutil.HintAction(hintMRGet, "view one merge request"),
		toolutil.HintAction(hintMRChangesGet, "see its diff"),
	)
	return b.String()
}

// mrStateCell renders a merge request state with its emoji, the way every
// merge request row in the tree shows it.
func mrStateCell(state string) string {
	if state == "" {
		return ""
	}
	return toolutil.MRStateEmoji(state) + " " + toolutil.EscapeMdTableCell(state)
}

// FormatGPGSignatureMarkdown renders a commit's signature as a card: the
// verdict, then whichever signer object the signing scheme carries.
func FormatGPGSignatureMarkdown(sig GPGSignatureOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Commit Signature")
	c.Field("Type", sig.SignatureType)
	c.Field("Verification", sig.VerificationStatus)
	switch {
	case sig.X509Certificate != nil:
		// Both are read out of the signer's own certificate, which GitLab
		// stores as parsed rather than validating.
		c.Field("X.509 Subject", sig.X509Certificate.Subject)
		c.Field("X.509 Email", sig.X509Certificate.Email)
	case sig.Key != nil:
		c.Field("SSH Key", sig.Key.Title)
		c.Field("Usage", sig.Key.UsageType)
	default:
		// Both come out of the GPG key's user ID packet, which is whatever the
		// key's owner typed when they generated it.
		c.Markdown("Key User", commitIdent(sig.KeyUserName, sig.KeyUserEmail))
		c.Int("Key ID", sig.KeyID)
		c.Code("Primary Key ID", sig.KeyPrimaryKeyID)
	}
	c.Field("Commit Source", sig.CommitSource)
	c.End(toolutil.HintAction(hintCommitGet, "view the full commit details"))
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
