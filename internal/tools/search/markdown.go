package search

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The heading of every search result list is written by
// [toolutil.WriteListHeading], which counts what the response can vouch for:
// the total GitLab sent, or the number shown with "more available" when it
// sent none. These formatters used to print Pagination.TotalItems straight
// into the heading, and a search answered without a total therefore announced
// "(0)" above a table of rows — a count nothing had measured, presented as
// GitLab's own.

// FormatCodeMarkdown renders a paginated list of code search results.
// Includes a Project column so global/group searches show which project
// each blob belongs to.
func FormatCodeMarkdown(out CodeOutput) string {
	if len(out.Blobs) == 0 {
		return toolutil.EmptyMessage("code search results")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Code Search Results", len(out.Blobs), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Project", "File", "Path", "Ref", "Line"))
	for _, bl := range out.Blobs {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(bl.ProjectID, 10),
			toolutil.EscapeMdTableCell(bl.Filename),
			toolutil.EscapeMdTableCell(bl.Path),
			toolutil.EscapeMdTableCell(bl.Ref),
			strconv.FormatInt(bl.Startline, 10),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"Use gitlab_repository action 'file_get' with path to read a found file")
	return b.String()
}

// FormatMRsMarkdown renders a paginated list of merge request search results.
// Shows project path (semantic) instead of numeric project ID, plus state
// emoji, author, and branch flow.
func FormatMRsMarkdown(out MergeRequestsOutput) string {
	if len(out.MergeRequests) == 0 {
		return toolutil.EmptyMessage("merge requests")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "MR Search Results", len(out.MergeRequests), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Author", "Project", "Source -> Target"))
	for _, mr := range out.MergeRequests {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("!%d", mr.IID), mr.WebURL),
			mrTitleCell(mr),
			toolutil.MRStateEmoji(mr.State)+" "+toolutil.EscapeMdTableCell(mr.State),
			toolutil.EscapeMdTableCell(mergerequests.AuthorName(mr)),
			toolutil.EscapeMdTableCell(mergerequests.ProjectPath(mr)),
			toolutil.EscapeMdTableCell(mr.SourceBranch)+" -> "+toolutil.EscapeMdTableCell(mr.TargetBranch),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintPreserveLinks,
		"Use gitlab_merge_request action 'get' with project_id and merge_request_iid to see full details")
	return b.String()
}

// mrTitleCell renders a found merge request's title, marked as a draft when it
// is one. A draft cannot be merged, which is the first thing a reader deciding
// what to do with a search result needs to know, and the column used to read
// exactly like a mergeable one.
func mrTitleCell(mr mergerequests.Output) string {
	title := toolutil.EscapeMdTableCell(mr.Title)
	if mr.Draft {
		title += " " + toolutil.EmojiDraft
	}
	return title
}

// FormatIssuesMarkdown renders a paginated list of issue search results.
func FormatIssuesMarkdown(out IssuesOutput) string {
	if len(out.Issues) == 0 {
		return toolutil.EmptyMessage("issues")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Issue Search Results", len(out.Issues), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Author", "Labels"))
	for _, i := range out.Issues {
		labels := strings.Join(i.Labels, ", ")
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(fmt.Sprintf("#%d", i.IID), i.WebURL),
			toolutil.EscapeMdTableCell(i.Title),
			toolutil.IssueStateEmoji(i.State)+" "+toolutil.EscapeMdTableCell(i.State),
			toolutil.EscapeMdTableCell(issues.AuthorName(i)),
			toolutil.EscapeMdTableCell(labels),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintPreserveLinks,
		"Use gitlab_issue action 'get' with project_id and issue_iid to see full details")
	return b.String()
}

// FormatCommitsMarkdown renders a paginated list of commit search results.
func FormatCommitsMarkdown(out CommitsOutput) string {
	if len(out.Commits) == 0 {
		return toolutil.EmptyMessage("commits")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Commit Search Results", len(out.Commits), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Short ID", "Title", "Author", "Date"))
	for _, c := range out.Commits {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(c.ShortID, c.WebURL),
			toolutil.EscapeMdTableCell(c.Title),
			toolutil.EscapeMdTableCell(c.AuthorName),
			toolutil.FormatTime(c.CommittedDate),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintPreserveLinks,
		"Use gitlab_repository action 'commit_get' with short_id to see full commit details")
	return b.String()
}

// FormatMilestonesMarkdown renders a paginated list of milestone search results.
func FormatMilestonesMarkdown(out MilestonesOutput) string {
	if len(out.Milestones) == 0 {
		return toolutil.EmptyMessage("milestones")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Milestone Search Results", len(out.Milestones), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("IID", "Title", "State", "Due Date"))
	for _, m := range out.Milestones {
		due := toolutil.FormatTime(m.DueDate)
		if due == "" {
			due = "—"
		}
		//gitlab:allow-unescaped m.State: a milestone state, one of GitLab's fixed set (active, closed).
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(strconv.FormatInt(m.IID, 10), m.WebURL),
			toolutil.EscapeMdTableCell(m.Title),
			m.State,
			due,
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintPreserveLinks,
		"Use gitlab_project action 'milestone_get' with project_id and milestone_id to see full details")
	return b.String()
}

// FormatNotesMarkdown renders a paginated list of note search results.
// Uses notable type and IID for semantic context instead of bare numeric IDs.
func FormatNotesMarkdown(out NotesOutput) string {
	if len(out.Notes) == 0 {
		return toolutil.EmptyMessage("note search results")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Note Search Results", len(out.Notes), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Author", "Type", "Ref", "Body"))
	for _, n := range out.Notes {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(n.Author),
			//gitlab:allow-unescaped n.NoteableType: the GitLab class a note hangs on (Issue, MergeRequest, Snippet, Commit, Epic), never text anybody types.
			n.NoteableType,
			//gitlab:allow-unescaped noteableRef(n.NoteableType, n.NoteableIID): that same class name and an integer IID, so the reference carries nothing a cell reacts to.
			noteableRef(n.NoteableType, n.NoteableIID),
			toolutil.EscapeMdTableCell(truncateBody(n.Body, 80)),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"Use the note's parent tool (gitlab_issue note actions or gitlab_mr_review note actions) to see full note")
	return b.String()
}

// FormatProjectsMarkdown renders a paginated list of project search results.
// Shows the full namespace path instead of numeric IDs.
func FormatProjectsMarkdown(out ProjectsOutput) string {
	rows := make([]searchResultRow, 0, len(out.Projects))
	for _, p := range out.Projects {
		rows = append(rows, searchResultRow{
			Title:    p.Name,
			TitleURL: p.WebURL,
			Cells:    [3]string{p.PathWithNamespace, p.Visibility, p.DefaultBranch},
		})
	}
	return formatSearchResultList("Project", out.Pagination, "projects", [4]string{"Name", "Path", "Visibility", "Default Branch"}, rows,
		toolutil.HintPreserveLinks,
		"Use gitlab_project action 'get' with the project path to see full details")
}

// searchResultRow is one row of a four-column search result table, carried as
// the values GitLab returned rather than as finished cells.
//
// Escaping them is [formatSearchResultList]'s job, so no caller can hand the
// table a value that still has a pipe or an angle bracket in it. The callers
// used to build the first column themselves as "[%s](%s)" around a
// cell-escaped title, and the cell escaper leaves ']' alone: a snippet titled
// "Fix login](http://attacker.invalid/x)" closed the label and pointed the
// link at a host that is not GitLab, on this server's own instruction, since
// the list carries HintPreserveLinks.
type searchResultRow struct {
	// Title and TitleURL are the first column: the result's name, linked to
	// its page when GitLab returned one.
	Title    string
	TitleURL string
	// Cells are the three remaining columns, in the order the header names them.
	Cells [3]string
}

func formatSearchResultList(kind string, pagination toolutil.PaginationOutput, emptyResource string, columns [4]string, rows []searchResultRow, hints ...string) string {
	if len(rows) == 0 {
		return toolutil.EmptyMessage(emptyResource)
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, kind+" Search Results", len(rows), pagination)
	b.WriteString(toolutil.MarkdownTableHeader(columns[0], columns[1], columns[2], columns[3]))
	for _, row := range rows {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(row.Title, row.TitleURL),
			toolutil.EscapeMdTableCell(row.Cells[0]),
			toolutil.EscapeMdTableCell(row.Cells[1]),
			toolutil.EscapeMdTableCell(row.Cells[2]),
		))
	}
	toolutil.WriteListFooter(&b, pagination, true, hints...)
	return b.String()
}

// FormatSnippetsMarkdown renders a paginated list of snippet search results.
func FormatSnippetsMarkdown(out SnippetsOutput) string {
	rows := make([]searchResultRow, 0, len(out.Snippets))
	for _, s := range out.Snippets {
		rows = append(rows, searchResultRow{
			Title:    s.Title,
			TitleURL: s.WebURL,
			Cells:    [3]string{s.FileName, s.Visibility, s.Author},
		})
	}
	return formatSearchResultList("Snippet", out.Pagination, "snippets", [4]string{"Title", "File", "Visibility", "Author"}, rows,
		toolutil.HintPreserveLinks,
		"Use gitlab_snippet action 'get' with snippet_id to see full content")
}

// FormatUsersMarkdown renders a paginated list of user search results.
func FormatUsersMarkdown(out UsersOutput) string {
	if len(out.Users) == 0 {
		return toolutil.EmptyMessage("users")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "User Search Results", len(out.Users), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Username", "Name", "State"))
	for _, u := range out.Users {
		//gitlab:allow-unescaped u.State: a user account state, one of GitLab's fixed set (active, blocked, deactivated, banned).
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdUserLink(u.Username, u.WebURL),
			toolutil.EscapeMdTableCell(u.Name),
			u.State,
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintPreserveLinks,
		"Use gitlab_user action 'get' with user_id to see full profile")
	return b.String()
}

// FormatWikiMarkdown renders a paginated list of wiki search results.
//
// The three columns are what [WikiBlobOutput] carries. GitLab's wiki_blobs
// scope answers with a blob (basename, path, ref, startline and the matched
// data) rather than with a wiki page, so a real search fills none of them;
// closing that means widening the output type from the captured response
// (ADR-0021), which is a schema change and not this formatter's to make.
func FormatWikiMarkdown(out WikiOutput) string {
	if len(out.WikiBlobs) == 0 {
		return toolutil.EmptyMessage("wiki pages")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Wiki Search Results", len(out.WikiBlobs), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Title", "Slug", "Format"))
	for _, w := range out.WikiBlobs {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(w.Title),
			toolutil.EscapeMdTableCell(w.Slug),
			//gitlab:allow-unescaped w.Format: a wiki format, a gl.WikiFormatValue GitLab picks from a fixed set (markdown, rdoc, asciidoc, org).
			w.Format,
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"Use gitlab_wiki action 'get' with slug to read the full wiki page")
	return b.String()
}

// noteableRef formats a notable type and IID as a human-readable reference
// (e.g. "#5" for issues, "!10" for merge requests).
func noteableRef(noteableType string, noteableIID int64) string {
	switch noteableType {
	case "MergeRequest":
		return fmt.Sprintf("!%d", noteableIID)
	case "Issue":
		return fmt.Sprintf("#%d", noteableIID)
	default:
		if noteableIID > 0 {
			return fmt.Sprintf("%s #%d", noteableType, noteableIID)
		}
		return noteableType
	}
}

// truncateBody shortens a text body to max runes, collapsing newlines.
func truncateBody(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "…"
	}
	return s
}

func init() {
	toolutil.RegisterMarkdown(FormatCodeMarkdown)
	toolutil.RegisterMarkdown(FormatMRsMarkdown)
	toolutil.RegisterMarkdown(FormatIssuesMarkdown)
	toolutil.RegisterMarkdown(FormatCommitsMarkdown)
	toolutil.RegisterMarkdown(FormatMilestonesMarkdown)
	toolutil.RegisterMarkdown(FormatNotesMarkdown)
	toolutil.RegisterMarkdown(FormatProjectsMarkdown)
	toolutil.RegisterMarkdown(FormatSnippetsMarkdown)
	toolutil.RegisterMarkdown(FormatUsersMarkdown)
	toolutil.RegisterMarkdown(FormatWikiMarkdown)
}
