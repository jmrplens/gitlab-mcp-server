package search

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const fmtTableRow4Col = "| %s | %s | %s | %s |\n"

// FormatCodeMarkdown renders a paginated list of code search results.
// Includes a Project column so global/group searches show which project
// each blob belongs to.
func FormatCodeMarkdown(out CodeOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Code Search Results (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Blobs), out.Pagination)
	if len(out.Blobs) == 0 {
		b.WriteString("No code search results found.\n")
		return b.String()
	}
	b.WriteString("| Project | File | Path | Ref | Line |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, bl := range out.Blobs {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %d |\n",
			bl.ProjectID,
			toolutil.EscapeMdTableCell(bl.Filename),
			toolutil.EscapeMdTableCell(bl.Path),
			toolutil.EscapeMdTableCell(bl.Ref),
			bl.Startline)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, "Use gitlab_repository action 'file_get' with path to read a found file")
	return b.String()
}

// FormatMRsMarkdown renders a paginated list of merge request search results.
// Shows project path (semantic) instead of numeric project ID, plus state
// emoji, author, and branch flow.
func FormatMRsMarkdown(out MergeRequestsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Search Results (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.MergeRequests), out.Pagination)
	if len(out.MergeRequests) == 0 {
		b.WriteString("No merge requests found.\n")
		return b.String()
	}
	b.WriteString("| IID | Title | State | Author | Project | Source -> Target |\n")
	b.WriteString(toolutil.TblSep6Col)
	for _, mr := range out.MergeRequests {
		//gitlab:allow-unescaped mr.State: a merge request state, one of GitLab's fixed set (opened, closed, locked, merged).
		fmt.Fprintf(&b, "| %s | %s | %s %s | %s | %s | %s -> %s |\n",
			toolutil.MdTitleLink(fmt.Sprintf("!%d", mr.IID), mr.WebURL),
			toolutil.EscapeMdTableCell(mr.Title),
			toolutil.MRStateEmoji(mr.State), mr.State,
			toolutil.EscapeMdTableCell(mergerequests.AuthorName(mr)),
			toolutil.EscapeMdTableCell(mergerequests.ProjectPath(mr)),
			toolutil.EscapeMdTableCell(mr.SourceBranch),
			toolutil.EscapeMdTableCell(mr.TargetBranch))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use gitlab_merge_request action 'get' with project_id and merge_request_iid to see full details")
	return b.String()
}

// FormatIssuesMarkdown renders a paginated list of issue search results.
func FormatIssuesMarkdown(out IssuesOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Issue Search Results (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Issues), out.Pagination)
	if len(out.Issues) == 0 {
		b.WriteString("No issues found.\n")
		return b.String()
	}
	b.WriteString("| IID | Title | State | Author | Labels |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, i := range out.Issues {
		labels := strings.Join(i.Labels, ", ")
		//gitlab:allow-unescaped i.State: an issue state, one of GitLab's fixed set (opened, closed).
		fmt.Fprintf(&b, "| %s | %s | %s %s | %s | %s |\n",
			toolutil.MdTitleLink(fmt.Sprintf("#%d", i.IID), i.WebURL),
			toolutil.EscapeMdTableCell(i.Title),
			toolutil.IssueStateEmoji(i.State), i.State,
			toolutil.EscapeMdTableCell(issues.AuthorName(i)),
			toolutil.EscapeMdTableCell(labels))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use gitlab_issue action 'get' with project_id and issue_iid to see full details")
	return b.String()
}

// FormatCommitsMarkdown renders a paginated list of commit search results.
func FormatCommitsMarkdown(out CommitsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Commit Search Results (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Commits), out.Pagination)
	if len(out.Commits) == 0 {
		b.WriteString("No commits found.\n")
		return b.String()
	}
	b.WriteString("| Short ID | Title | Author | Date |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, c := range out.Commits {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
			toolutil.MdTitleLink(c.ShortID, c.WebURL),
			toolutil.EscapeMdTableCell(c.Title),
			toolutil.EscapeMdTableCell(c.AuthorName),
			toolutil.EscapeMdTableCell(c.CommittedDate))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use gitlab_repository action 'commit_get' with short_id to see full commit details")
	return b.String()
}

// FormatMilestonesMarkdown renders a paginated list of milestone search results.
func FormatMilestonesMarkdown(out MilestonesOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Milestone Search Results (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Milestones), out.Pagination)
	if len(out.Milestones) == 0 {
		b.WriteString("No milestones found.\n")
		return b.String()
	}
	b.WriteString("| IID | Title | State | Due Date |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, m := range out.Milestones {
		due := m.DueDate
		if due == "" {
			due = "\u2014"
		}
		//gitlab:allow-unescaped m.State: a milestone state, one of GitLab's fixed set (active, closed).
		//gitlab:allow-unescaped due: a due date the SDK's ISOTime rendered as digits and dashes, or the em dash substituted just above.
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
			toolutil.MdTitleLink(strconv.FormatInt(m.IID, 10), m.WebURL),
			toolutil.EscapeMdTableCell(m.Title),
			m.State,
			due)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use gitlab_project action 'milestone_get' with project_id and milestone_id to see full details")
	return b.String()
}

// FormatNotesMarkdown renders a paginated list of note search results.
// Uses notable type and IID for semantic context instead of bare numeric IDs.
func FormatNotesMarkdown(out NotesOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Note Search Results (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Notes), out.Pagination)
	if len(out.Notes) == 0 {
		b.WriteString("No note search results found.\n")
		return b.String()
	}
	b.WriteString("| Author | Type | Ref | Body |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, n := range out.Notes {
		fmt.Fprintf(&b, fmtTableRow4Col,
			toolutil.EscapeMdTableCell(n.Author),
			//gitlab:allow-unescaped n.NoteableType: the GitLab class a note hangs on (Issue, MergeRequest, Snippet, Commit, Epic), never text anybody types.
			n.NoteableType,
			//gitlab:allow-unescaped noteableRef(n.NoteableType, n.NoteableIID): that same class name and an integer IID, so the reference carries nothing a cell reacts to.
			noteableRef(n.NoteableType, n.NoteableIID),
			toolutil.EscapeMdTableCell(truncateBody(n.Body, 80)))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, "Use the note's parent tool (gitlab_issue note actions or gitlab_mr_review note actions) to see full note")
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
	return formatSearchResultList("Project", len(out.Projects), out.Pagination, "No projects found.", "| Name | Path | Visibility | Default Branch |", rows,
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

func formatSearchResultList(kind string, count int, pagination toolutil.PaginationOutput, emptyMessage, header string, rows []searchResultRow, hints ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s Search Results (%d)\n\n", kind, pagination.TotalItems)
	toolutil.WriteListSummary(&b, count, pagination)
	if count == 0 {
		b.WriteString(emptyMessage)
		b.WriteByte('\n')
		return b.String()
	}
	b.WriteString(header)
	b.WriteByte('\n')
	b.WriteString(toolutil.TblSep4Col)
	for _, row := range rows {
		fmt.Fprintf(&b, fmtTableRow4Col,
			toolutil.MdTitleLink(row.Title, row.TitleURL),
			toolutil.EscapeMdTableCell(row.Cells[0]),
			toolutil.EscapeMdTableCell(row.Cells[1]),
			toolutil.EscapeMdTableCell(row.Cells[2]))
	}
	toolutil.WritePagination(&b, pagination)
	toolutil.WriteHints(&b, hints...)
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
	return formatSearchResultList("Snippet", len(out.Snippets), out.Pagination, "No snippets found.", "| Title | File | Visibility | Author |", rows,
		toolutil.HintPreserveLinks,
		"Use gitlab_snippet action 'get' with snippet_id to see full content")
}

// FormatUsersMarkdown renders a paginated list of user search results.
func FormatUsersMarkdown(out UsersOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## User Search Results (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Users), out.Pagination)
	if len(out.Users) == 0 {
		b.WriteString("No users found.\n")
		return b.String()
	}
	b.WriteString("| Username | Name | State |\n")
	b.WriteString(toolutil.TblSep3Col)
	for _, u := range out.Users {
		//gitlab:allow-unescaped u.State: a user account state, one of GitLab's fixed set (active, blocked, deactivated, banned).
		fmt.Fprintf(&b, "| %s | %s | %s |\n",
			toolutil.MdTitleLink("@"+u.Username, u.WebURL),
			toolutil.EscapeMdTableCell(u.Name),
			u.State)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use gitlab_user action 'get' with user_id to see full profile")
	return b.String()
}

// FormatWikiMarkdown renders a paginated list of wiki search results.
func FormatWikiMarkdown(out WikiOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Wiki Search Results (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.WikiBlobs), out.Pagination)
	if len(out.WikiBlobs) == 0 {
		b.WriteString("No wiki pages found.\n")
		return b.String()
	}
	b.WriteString("| Title | Slug | Format |\n")
	b.WriteString(toolutil.TblSep3Col)
	for _, w := range out.WikiBlobs {
		fmt.Fprintf(&b, "| %s | %s | %s |\n",
			toolutil.EscapeMdTableCell(w.Title),
			toolutil.EscapeMdTableCell(w.Slug),
			//gitlab:allow-unescaped w.Format: a wiki format, a gl.WikiFormatValue GitLab picks from a fixed set (markdown, rdoc, asciidoc, org).
			w.Format)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, "Use gitlab_wiki action 'get' with slug to read the full wiki page")
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
		return string(runes[:maxLen]) + "\u2026"
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
