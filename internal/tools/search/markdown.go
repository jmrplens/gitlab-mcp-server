package search

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	toolutil.WriteHints(&b, "Use gitlab_file action 'get' with path to read a found file")
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
	b.WriteString("| IID | Title | State | Author | Project | Source → Target |\n")
	b.WriteString(toolutil.TblSep6Col)
	for _, mr := range out.MergeRequests {
		fmt.Fprintf(&b, "| [!%d](%s) | %s | %s %s | %s | %s | %s → %s |\n",
			mr.IID, mr.WebURL,
			toolutil.EscapeMdTableCell(mr.Title),
			toolutil.MRStateEmoji(mr.State), mr.State,
			toolutil.EscapeMdTableCell(mr.Author),
			toolutil.EscapeMdTableCell(mr.ProjectPath),
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
		fmt.Fprintf(&b, "| [#%d](%s) | %s | %s %s | %s | %s |\n",
			i.IID, i.WebURL,
			toolutil.EscapeMdTableCell(i.Title),
			toolutil.IssueStateEmoji(i.State), i.State,
			toolutil.EscapeMdTableCell(i.Author),
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
		fmt.Fprintf(&b, "| [%s](%s) | %s | %s | %s |\n",
			toolutil.EscapeMdTableCell(c.ShortID), c.WebURL,
			toolutil.EscapeMdTableCell(c.Title),
			toolutil.EscapeMdTableCell(c.AuthorName),
			toolutil.EscapeMdTableCell(c.CommittedDate))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use gitlab_commit action 'get' with short_id to see full commit details")
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
		fmt.Fprintf(&b, "| [%d](%s) | %s | %s | %s |\n",
			m.IID, m.WebURL,
			toolutil.EscapeMdTableCell(m.Title),
			m.State,
			due)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use gitlab_milestone action 'get' with project_id and milestone_id to see full details")
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
			n.NoteableType,
			noteableRef(n.NoteableType, n.NoteableIID),
			toolutil.EscapeMdTableCell(truncateBody(n.Body, 80)))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, "Use the note's parent tool (gitlab_issue_note or gitlab_mr_note) to see full note")
	return b.String()
}

// FormatProjectsMarkdown renders a paginated list of project search results.
// Shows the full namespace path instead of numeric IDs.
func FormatProjectsMarkdown(out ProjectsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Project Search Results (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Projects), out.Pagination)
	if len(out.Projects) == 0 {
		b.WriteString("No projects found.\n")
		return b.String()
	}
	b.WriteString("| Name | Path | Visibility | Default Branch |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, p := range out.Projects {
		fmt.Fprintf(&b, fmtTableRow4Col,
			fmt.Sprintf("[%s](%s)", toolutil.EscapeMdTableCell(p.Name), p.WebURL),
			toolutil.EscapeMdTableCell(p.PathWithNamespace),
			p.Visibility,
			toolutil.EscapeMdTableCell(p.DefaultBranch))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use gitlab_project action 'get' with the project path to see full details")
	return b.String()
}

// FormatSnippetsMarkdown renders a paginated list of snippet search results.
func FormatSnippetsMarkdown(out SnippetsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Snippet Search Results (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Snippets), out.Pagination)
	if len(out.Snippets) == 0 {
		b.WriteString("No snippets found.\n")
		return b.String()
	}
	b.WriteString("| Title | File | Visibility | Author |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, s := range out.Snippets {
		fmt.Fprintf(&b, fmtTableRow4Col,
			fmt.Sprintf("[%s](%s)", toolutil.EscapeMdTableCell(s.Title), s.WebURL),
			toolutil.EscapeMdTableCell(s.FileName),
			s.Visibility,
			toolutil.EscapeMdTableCell(s.Author))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use gitlab_snippet action 'get' with snippet_id to see full content")
	return b.String()
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
		fmt.Fprintf(&b, "| [@%s](%s) | %s | %s |\n",
			toolutil.EscapeMdTableCell(u.Username), u.WebURL,
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
}
