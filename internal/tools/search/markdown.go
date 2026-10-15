package search

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

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

// maxSnippetLines bounds the lines of one code search snippet the Markdown
// shows. GitLab sends three to five lines by default and 41 at most on
// advanced search, while a basic-search hunk has no upper bound, since git
// grep merges the context of nearby matches. With the default context every
// backend puts the match within the first three lines it sends, so the head is
// the part worth showing. That holds only because this server sends no
// num_context_lines: passing it through would put the match up to 20 lines
// down, and the cap would then have to keep the lines around the match.
const maxSnippetLines = 10

// maxSnippetLineRunes bounds one line of a snippet. GitLab sends whole lines
// around a content match, so a minified or generated file arrives as one line
// of any length. The cap is per line rather than on the whole snippet so that
// long context lines above the match cannot push the match itself out of
// view.
const maxSnippetLineRunes = 300

// maxCodeMarkdownBytes bounds the table and snippet sections of one code
// search page as the client receives them, fences included. A hundred capped
// snippets would still run to hundreds of kilobytes. The table itself is not
// cut: the budget decides how many snippet sections follow it. Sections are
// written in order and stop at the first that does not fit, so a large early
// snippet leaves later small ones unshown, which keeps the results left
// without a snippet one run a single note can name. Each of them keeps its
// table row.
//
// None of the three is a parameter: the structured result carries every
// snippet whole, and GitLab's own num_context_lines already sizes what it
// sends on the backends that honor it.
const maxCodeMarkdownBytes = 40_000

// snippetLineEndings turns every line ending into a line feed. CommonMark
// reads a lone carriage return as a line ending too, so counting lines by the
// line feed alone would count fewer than a renderer shows.
var snippetLineEndings = strings.NewReplacer("\r\n", "\n", "\r", "\n")

// codeSnippet is what the Markdown shows of one result's data.
type codeSnippet struct {
	// text is the head of the data, one line feed after each line kept.
	text string
	// lines is how many lines GitLab sent.
	lines int
	// longLines reports whether a kept line was cut to maxSnippetLineRunes.
	longLines bool
}

// cut reports whether the snippet shows less than GitLab sent.
func (s codeSnippet) cut() bool {
	return s.lines > maxSnippetLines || s.longLines
}

// shapeSnippet keeps the first maxSnippetLines lines of data, each cut to
// maxSnippetLineRunes, and counts the lines it was sent. Empty data shapes to
// no text and is never cut.
func shapeSnippet(data string) codeSnippet {
	data = strings.ToValidUTF8(snippetLineEndings.Replace(data), string(utf8.RuneError))
	s := codeSnippet{lines: strings.Count(data, "\n")}
	if !strings.HasSuffix(data, "\n") {
		s.lines++
	}
	var b strings.Builder
	rest := data
	for kept := 0; kept < maxSnippetLines && rest != ""; kept++ {
		line, after, _ := strings.Cut(rest, "\n")
		head, cut := cutSnippetLine(line)
		s.longLines = s.longLines || cut
		b.WriteString(head)
		b.WriteString("\n")
		rest = after
	}
	s.text = b.String()
	return s
}

// cutSnippetLine returns the first maxSnippetLineRunes runes of line, and
// whether that left anything out. The cut lands between two runes.
func cutSnippetLine(line string) (string, bool) {
	runes := 0
	for i := range line {
		if runes == maxSnippetLineRunes {
			return line[:i], true
		}
		runes++
	}
	return line, false
}

// writeSnippetSection writes one result's section: a heading naming the
// result, the notes saying what the snippet leaves out, and the snippet in a
// fence sized past its longest backtick run. GitLab sends no match line, so
// the heading names the line the snippet starts on.
func writeSnippetSection(c *toolutil.Card, n int, bl BlobOutput, s codeSnippet) {
	section := c.Section(fmt.Sprintf("Result %d: %s (project %d, start line %d)",
		n, cmp.Or(bl.Path, bl.Filename), bl.ProjectID, bl.Startline))
	if bl.Data == "" {
		section.Note("The snippet GitLab sent for this result is empty.")
		return
	}
	if s.lines > maxSnippetLines {
		section.Note(fmt.Sprintf("Showing the first %d of the %d lines GitLab sent for this result.", maxSnippetLines, s.lines))
	}
	if s.longLines {
		section.Note(fmt.Sprintf("Lines longer than %d characters are cut.", maxSnippetLineRunes))
	}
	section.Fence("", "", s.text)
}

// snippetSectionBytes is how many bytes writeSnippetSection adds to b, as the
// client receives them. The section is rendered on its own after the last two
// bytes b holds, so the card writes the blank line that separates it from
// what b ends with, as it will on b. WriteHints later defuses every guidance
// heading in the response, fences included, and a defused heading is longer,
// so the size is that of the defused text. No heading can span the seed and
// the section, since the card puts a line break between them.
func snippetSectionBytes(b *strings.Builder, n int, bl BlobOutput, s codeSnippet) int {
	written := b.String()
	seed := written[max(0, len(written)-2):]
	var probe strings.Builder
	probe.WriteString(seed)
	writeSnippetSection(toolutil.NewCard(&probe, ""), n, bl, s)
	return len(toolutil.DefuseHintsHeading(probe.String())) - len(seed)
}

// fitsCodeMarkdown reports whether a section of the given size, written after
// used bytes, keeps the response within maxCodeMarkdownBytes.
func fitsCodeMarkdown(used, section int) bool {
	return used+section <= maxCodeMarkdownBytes
}

// unshownSnippetsNote names the results from first to last, counted from one,
// that the budget left without a snippet section.
func unshownSnippetsNote(first, last int) string {
	const why = " no snippet shown here, as the response would be too large. Ask for fewer results per page to see every snippet."
	if first == last {
		return fmt.Sprintf("Result %d has", first) + why
	}
	return fmt.Sprintf("Results %d to %d have", first, last) + why
}

// FormatCodeMarkdown renders a paginated list of code search results: a
// table naming each result, its project and the line its snippet starts on,
// then each snippet under a heading of its own, cut to a bounded head. The
// structured result carries every snippet whole.
func FormatCodeMarkdown(out CodeOutput) string {
	if len(out.Blobs) == 0 {
		return toolutil.EmptyMessage("code search results")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Code Search Results", len(out.Blobs), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Project", "File", "Path", "Ref", "Start line"))
	for _, bl := range out.Blobs {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(bl.ProjectID, 10),
			toolutil.EscapeMdTableCell(bl.Filename),
			toolutil.EscapeMdTableCell(bl.Path),
			toolutil.EscapeMdTableCell(bl.Ref),
			strconv.FormatInt(bl.Startline, 10),
		))
	}
	c := toolutil.NewCard(&b, "")
	// used counts the response as the client receives it, a path that spells
	// the guidance heading in the table included.
	used := len(toolutil.DefuseHintsHeading(b.String()))
	cut := false
	for i, bl := range out.Blobs {
		n := i + 1
		s := shapeSnippet(bl.Data)
		section := snippetSectionBytes(&b, n, bl, s)
		if !fitsCodeMarkdown(used, section) {
			c.Note(unshownSnippetsNote(n, len(out.Blobs)))
			cut = true
			break
		}
		writeSnippetSection(c, n, bl, s)
		used += section
		cut = cut || s.cut()
	}
	var hints []string
	if cut {
		hints = append(hints, toolutil.HintAction("repository.file_get", "read the whole file of a cut or unshown snippet, with its path and ref"))
	}
	hints = append(hints, toolutil.HintAction("repository.file_get", "read a found file by its path"))
	toolutil.WriteListFooter(&b, out.Pagination, false, hints...)
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
		toolutil.HintAction("merge_request.get", "see full details, with project_id and merge_request_iid"))
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
		toolutil.HintAction("issue.get", "see full details, with project_id and issue_iid"))
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
		toolutil.HintAction("repository.commit_get", "see full commit details, with short_id"))
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
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(strconv.FormatInt(m.IID, 10), m.WebURL),
			toolutil.EscapeMdTableCell(m.Title),
			toolutil.EscapeMdTableCell(m.State),
			due,
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintPreserveLinks,
		toolutil.HintAction("project.milestone_get", "see full details, with project_id and milestone_iid (the IID column)"))
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
			toolutil.EscapeMdTableCell(n.NoteableType),
			//gitlab:allow-unescaped noteableRef(n.NoteableType, n.NoteableIID): a reference this package composes itself from the class name and an integer IID, kept unescaped so the cell renders it as the reference it is.
			noteableRef(n.NoteableType, n.NoteableIID),
			toolutil.EscapeMdTableCell(truncateBody(n.Body, 80)),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"Use `issue.note_get` or `mr_review.note_get`, whichever the note's parent is, to see the full note")
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
		toolutil.HintAction("project.get", "see full details, with the project path"))
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
		toolutil.HintAction("snippet.get", "see full content, with snippet_id"))
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
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdUserLink(u.Username, u.WebURL),
			toolutil.EscapeMdTableCell(u.Name),
			toolutil.EscapeMdTableCell(u.State),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintPreserveLinks,
		toolutil.HintAction("user.get", "see full profile, with user_id"))
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
			toolutil.EscapeMdTableCell(w.Format),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction("wiki.get", "read the full wiki page, with slug"))
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
