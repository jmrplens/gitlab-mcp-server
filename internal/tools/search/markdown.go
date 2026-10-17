package search

import (
	"cmp"
	"fmt"
	pathpkg "path"
	"slices"
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
// the part shown. A num_context_lines the caller sets can put it up to 20
// lines down, so the lines shown are a window around the lines that can hold
// the match ([snippetWindow]), which is the head whenever those lines are
// within it and never fewer lines than they span.
const maxSnippetLines = 10

// maxSnippetLineRunes bounds one line of a snippet. GitLab sends whole lines
// around a content match, so a minified or generated file arrives as one line
// of any length. The cap is per line rather than on the whole snippet so that
// long context lines above the match cannot push the match itself out of
// view.
const maxSnippetLineRunes = 300

// maxCodeMarkdownBytes bounds the heading, table and snippet sections of one
// code search page as the client receives them, fences included: with every
// guidance heading defused, as WriteHints leaves the response, and normalized,
// as every response is on its way into a content block, which drops trailing
// spaces and control bytes. A hundred capped snippets would still run to
// hundreds of kilobytes, and a hundred rows of long paths pass the budget on
// their own, so the table is held to it first. Its rows are written in order
// and stop before the first that does not fit under the heading that counts
// them, which keeps the results left out of the table one run at the end that
// a single note can name; a result with no row gets no section either, and
// when not even the first row fits no table is written and the heading counts
// none. The sections of the rows shown share what is left: they too are
// written in order and stop at the first that does not fit, so a large early
// snippet leaves later small ones unshown and the rows left without a snippet
// are one run a second note names. Each of them keeps its table row. A note
// offers fewer results per page only for the results a page of their own
// would show, and names those whose row, or row and snippet, would not fit
// even there.
//
// None of the three is a parameter: the structured result carries every
// result and snippet whole, and GitLab's own num_context_lines already sizes
// what it sends on the backends that honor it.
const maxCodeMarkdownBytes = 40_000

// codeSearchHeader is the header of the code search table.
var codeSearchHeader = toolutil.MarkdownTableHeader("Project", "File", "Path", "Ref", "Start line")

// blobListing is one search scope GitLab answers as API::Entities::Blob, the
// code scope and the wiki scope (lib/api/search.rb, SCOPE_ENTITY). Both carry
// a snippet of unbounded size, so both are rendered the same way and held to
// the same budget, and they differ in what the heading calls the list, how
// the table names a result, and which action reads a whole file.
type blobListing struct {
	// title is the subject of the list heading, and empty what the answer
	// with no result says it found none of.
	title, empty string
	// header is the table header, and row writes one result's row under it.
	header string
	row    func(foundBlob) string
	// owner names where a result lives, as its section heading says it.
	owner func(foundBlob) string
	// readHints are the hints every page closes with, and cutHint the one a
	// page whose snippets were cut or left unshown adds before them.
	readHints []string
	cutHint   string
}

// foundBlob is one result as a listing renders it: the blob, and for a wiki
// page the group GitLab names, which a code search result never carries.
type foundBlob struct {
	BlobOutput
	groupID int64
	// context is how GitLab cut the snippet, which a wiki result does not
	// say.
	context snippetContext
}

// snippet is what the Markdown shows of the result's data.
func (bl foundBlob) snippet() codeSnippet {
	return shapeSnippet(bl.Data, bl.context, bl.Startline)
}

// codeListing renders the code scope.
var codeListing = blobListing{
	title:     "Code Search Results",
	empty:     "code search results",
	header:    codeSearchHeader,
	row:       func(bl foundBlob) string { return codeTableRow(bl.BlobOutput) },
	owner:     func(bl foundBlob) string { return fmt.Sprintf("project %d", bl.ProjectID) },
	readHints: []string{toolutil.HintAction("repository.file_get", "read a found file by its path")},
	cutHint:   toolutil.HintAction("repository.file_get", "read the whole file of a cut or unshown snippet, with its path and ref from the table or the structured result"),
}

// wikiListing renders the wiki scope. A wiki page is a file of the wiki's
// repository, named by its slug with the format's extension, so the table's
// Page column is the path without that extension, which is the slug the wiki
// actions take. GitLab's basename is not used for it, because advanced search
// writes a top-level page's as "./slug".
var wikiListing = blobListing{
	title:  "Wiki Search Results",
	empty:  "wiki pages",
	header: toolutil.MarkdownTableHeader("Wiki", "Page", "Path", "Ref", "Start line"),
	row: func(bl foundBlob) string {
		return toolutil.MarkdownTableRow(
			wikiOwner(bl),
			toolutil.EscapeMdTableCell(wikiSlug(bl.Path)),
			toolutil.EscapeMdTableCell(bl.Path),
			toolutil.EscapeMdTableCell(bl.Ref),
			startCell(bl.BlobOutput),
		)
	},
	owner: wikiOwner,
	readHints: []string{
		toolutil.HintAction("wiki.get", "read a found page of a project wiki, with its project as project_id and its Page as slug"),
		toolutil.HintAction("group.wiki_get", "read a found page of a group wiki, with its group as group_id and its Page as slug"),
	},
	cutHint: toolutil.HintAction("wiki.get", "read the whole page of a cut or unshown snippet, or group.wiki_get for a group wiki, with its Page from the table, or its path without the extension from the structured result, as slug"),
}

// wikiOwner names the wiki a result belongs to: its project, or the group of
// a group wiki page, which advanced search answers with no project.
func wikiOwner(bl foundBlob) string {
	if bl.ProjectID == 0 && bl.groupID != 0 {
		return fmt.Sprintf("group %d", bl.groupID)
	}
	return fmt.Sprintf("project %d", bl.ProjectID)
}

// wikiSlug is the slug of the wiki page whose file is at path: the path
// without the format's extension.
func wikiSlug(path string) string {
	return strings.TrimSuffix(path, pathpkg.Ext(path))
}

// foundCode is a page of code search results as the listing renders them,
// each carrying how GitLab cut the page's snippets.
func foundCode(blobs []BlobOutput, ctx snippetContext) []foundBlob {
	out := make([]foundBlob, len(blobs))
	for i, bl := range blobs {
		out[i] = foundBlob{BlobOutput: bl, context: ctx}
	}
	return out
}

// foundWiki is a page of wiki search results as the listing renders them.
func foundWiki(pages []WikiBlobOutput) []foundBlob {
	out := make([]foundBlob, len(pages))
	for i, page := range pages {
		out[i] = foundBlob{
			Basename: page.Basename, Data: page.Data, Path: page.Path, Filename: page.Filename,
			ID: page.ID, Ref: page.Ref, Startline: page.Startline, ProjectID: page.ProjectID,
			groupID: page.GroupID,
		}
	}
	return out
}

// snippetLine is the text of one line of a snippet, the line ending GitLab
// sent dropped, a CRLF's carriage return included. GitLab counts a snippet's
// lines by the line feed alone (Ruby's String#lines on advanced search, the
// line count of zoekt's context on exact search, git grep on basic search), so
// the window is placed on those lines and a carriage return left inside one
// moves none of them. shapeSnippet then shows such a carriage return as a line
// feed, which is how CommonMark reads a lone one and no response may carry.
func snippetLine(line string) string {
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
}

// codeSnippet is what the Markdown shows of one result's data.
type codeSnippet struct {
	// text is the lines of the data the window keeps, one line feed after
	// each.
	text string
	// lines is how many lines GitLab sent, counted as GitLab counts them.
	lines int
	// first is the first line kept, counted from 0, and shown how many of
	// GitLab's lines text holds.
	first, shown int
	// startline is the line of the file the snippet starts on, as GitLab
	// reports it.
	startline int64
	// longLines reports whether a kept line was cut to maxSnippetLineRunes.
	longLines bool
}

// cut reports whether the snippet shows less than GitLab sent.
func (s codeSnippet) cut() bool {
	return s.shown < s.lines || s.longLines
}

// windowNote says which of the lines GitLab sent the snippet shows, or is
// empty when it shows them all: the first ones, or a run around the match
// further down, which it names by the file's own line numbers, those of the
// start line the section's heading gives, since the fence no longer starts on
// that line.
func windowNote(s codeSnippet) string {
	if s.shown == s.lines {
		return ""
	}
	if s.first == 0 {
		return fmt.Sprintf("Showing the first %d of the %d lines GitLab sent for this result.", s.shown, s.lines)
	}
	from := s.startline + int64(s.first)
	return fmt.Sprintf("Showing lines %d to %d of the file, around the match, of lines %d to %d that GitLab sent for this result.",
		from, from+int64(s.shown)-1, s.startline, s.startline+int64(s.lines)-1)
}

// shapeSnippet keeps the lines of data that snippetWindow places around the
// lines that can hold the match, as ctx reads them for a snippet starting on
// startline, each cut to maxSnippetLineRunes, and counts the lines it was
// sent as GitLab counts them. With the context GitLab applies by default that
// is the first maxSnippetLines lines. Empty data shapes to no text and is never
// cut.
func shapeSnippet(data string, ctx snippetContext, startline int64) codeSnippet {
	all := slices.Collect(strings.Lines(strings.ToValidUTF8(data, string(utf8.RuneError))))
	s := codeSnippet{lines: len(all), startline: startline}
	lo, hi := ctx.matchSpan(startline, len(all))
	s.first, s.shown = snippetWindow(lo, hi, len(all))
	var b strings.Builder
	for _, line := range all[s.first : s.first+s.shown] {
		head, cut := cutSnippetLine(snippetLine(line))
		s.longLines = s.longLines || cut
		b.WriteString(strings.ReplaceAll(head, "\r", "\n"))
		b.WriteString("\n")
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

// pathMatchNote is what the section of a file that matched by its path says
// before anything else when GitLab gave it a blob id. Basic search answers
// such a result with the start of the file (the first 1024 bytes,
// Gitlab::Search::FoundBlob fetch_blob) and start line 1, so its snippet need
// not hold the query at all, and read as the context of a match it sends a
// reader looking for a line that is not there.
const pathMatchNote = "This file matched the query by its path, so the snippet is the start of the file rather than the lines around a match."

// nameMatchNote is what the section of a file that matched by its path says
// when GitLab sent the path itself in place of a snippet, which is how exact
// code search answers a match on a file's name: zoekt marks such a line match
// FileName and gives the file name as its line, and the API's build_result
// (ee/lib/search/zoekt/search_results.rb) reads the line and not the mark, so
// the answer's data is the path and its start line 1 (0 on GitLab 19.4). There
// is no snippet to show, and fencing the path would present it as line 1 of
// the file.
const nameMatchNote = "This file matched the query by its path, and GitLab sent the path in place of a snippet."

// nameMatched reports whether bl is exact search's answer to a file that
// matched by its name: no blob id, and the file's path, or its file name when
// GitLab sent no path, as the whole snippet. A content match reads the same
// only when its snippet is a single line holding exactly the path with no line
// feed after it, the last line of a one-line file that does not end with one,
// since every other line a snippet carries ends with a line feed.
func nameMatched(bl BlobOutput) bool {
	return bl.ID == "" && bl.Data != "" && bl.Data == cmp.Or(bl.Path, bl.Filename)
}

// pathMatched reports whether bl matched the query by its path, which GitLab
// marks with a blob id on basic search and with the path as the snippet on
// exact search. Advanced search marks neither, so its path matches read as
// content matches from line 1.
func pathMatched(bl BlobOutput) bool {
	return bl.ID != "" || nameMatched(bl)
}

// startCell is the Start line cell of a result's row: the line its snippet
// starts on, or "path match" for a file that matched by its path, whose
// snippet is the start of the file or no line of it at all, so that a row
// whose section the budget leaves out still says which kind of match it is.
func startCell(bl BlobOutput) string {
	if pathMatched(bl) {
		return "path match"
	}
	return strconv.FormatInt(bl.Startline, 10)
}

// writeSnippetSection writes one result's section: a heading naming the
// result, the notes saying what the snippet is and what it leaves out, and the
// snippet in a fence sized past its longest backtick run. GitLab sends no
// match line, so the heading of a content match names the line the snippet
// starts on. A file that matched by its path is headed as a path match, and
// the first thing its section says is what its snippet is: the start of the
// file when basic search gave it a blob id, or nothing of the file when exact
// search sent its path in place of a snippet, which leaves nothing to fence.
// Advanced search marks neither kind, so its results read as content matches
// whichever they are. owner names where the result lives, as its listing
// names it.
func writeSnippetSection(c *toolutil.Card, n int, owner string, bl BlobOutput, s codeSnippet) {
	where := fmt.Sprintf("start line %d", bl.Startline)
	if pathMatched(bl) {
		where = "path match"
	}
	section := c.Section(fmt.Sprintf("Result %d: %s (%s, %s)", n, cmp.Or(bl.Path, bl.Filename), owner, where))
	if bl.ID != "" {
		section.Note(pathMatchNote)
	}
	if nameMatched(bl) {
		section.Note(nameMatchNote)
		return
	}
	if bl.Data == "" {
		section.Note("The snippet GitLab sent for this result is empty.")
		return
	}
	if note := windowNote(s); note != "" {
		section.Note(note)
	}
	if s.longLines {
		section.Note(fmt.Sprintf("Lines longer than %d characters are cut.", maxSnippetLineRunes))
	}
	section.Fence("", "", s.text)
}

// deliveredBytes is the size of s as the client receives it. WriteHints
// defuses every guidance heading in the response, fences included, and a
// defused heading is longer; every response is then normalized on its way
// into a content block ([toolutil.NormalizeResultMarkdown]), which drops the
// trailing spaces and tabs of each line and every control byte. Both work
// within a line, so s measures its own share of the response when it ends
// with a line break and follows one.
func deliveredBytes(s string) int {
	return len(toolutil.NormalizeResultMarkdown(toolutil.DefuseHintsHeading(s)))
}

// snippetSectionBytes is how many bytes writeSnippetSection adds to b, as the
// client receives them. The section is rendered on its own after the last two
// bytes b holds, so the card writes the blank line that separates it from
// what b ends with, as it will on b; b always ends with a line break, so the
// size is what the seed and the section measure together less the seed.
func snippetSectionBytes(b *strings.Builder, n int, owner string, bl BlobOutput, s codeSnippet) int {
	written := b.String()
	seed := written[max(0, len(written)-2):]
	var probe strings.Builder
	probe.WriteString(seed)
	writeSnippetSection(toolutil.NewCard(&probe, ""), n, owner, bl, s)
	return deliveredBytes(probe.String()) - deliveredBytes(seed)
}

// fitsCodeMarkdown reports whether a section of the given size, written after
// used bytes, keeps the response within maxCodeMarkdownBytes.
func fitsCodeMarkdown(used, section int) bool {
	return used+section <= maxCodeMarkdownBytes
}

// leftOut words the note naming a run of results the budget left out of one
// part of the response: the table, or the snippet sections of the rows the
// table shows.
type leftOut struct {
	// one and many say what was left out, after one result and after a run.
	one, many string
	// part and parts name what of one result, and of several, would have to
	// fit for a page of its own to show it.
	part, parts string
	// every and them name what a smaller page would show: all of the run,
	// and the ones of it that fit.
	every, them string
}

var (
	// rowsLeftOut names the results left out of the table.
	rowsLeftOut = leftOut{
		one: "is not shown here", many: "are not shown here",
		part: "row", parts: "rows",
		every: "every result", them: "them",
	}
	// snippetsLeftOut names the table rows left without a snippet section.
	snippetsLeftOut = leftOut{
		one: "has no snippet shown here", many: "have no snippet shown here",
		part: "row and snippet", parts: "rows and snippets",
		every: "every snippet", them: "their snippets",
	}
)

// note names the results first to last, counted from one, that the budget
// left out, and says why. It asks for fewer results per page only for what a
// page of its own would show: tooLarge numbers, in order, the results of the
// run whose part would not fit the budget even there. When that is all of
// them no smaller page is offered; otherwise it is offered for the rest, and
// the one that would not fit is named or, when there are several, how many
// and the first, which keeps the note one line of bounded size whatever the
// page holds.
func (k leftOut) note(first, last int, tooLarge []int) string {
	subject := fmt.Sprintf("Result %d %s", first, k.one)
	if first < last {
		subject = fmt.Sprintf("Results %d to %d %s", first, last, k.many)
	}
	const alone = " alone would make the response too large."
	run := last - first + 1
	if len(tooLarge) == 0 {
		return subject + ", as the response would be too large. Ask for fewer results per page to see " + k.every + "."
	}
	if len(tooLarge) == run {
		if run == 1 {
			return subject + ", as its " + k.part + alone
		}
		return subject + ", as the " + k.part + " of each" + alone
	}
	smaller := subject + ", as the response would be too large. Ask for fewer results per page to see " + k.them + ", except "
	if len(tooLarge) == 1 {
		return smaller + fmt.Sprintf("result %d, whose %s", tooLarge[0], k.part) + alone
	}
	return smaller + fmt.Sprintf("the %d whose %s alone would make the response too large, the first of them result %d.", len(tooLarge), k.parts, tooLarge[0])
}

// heading is the heading of a page of the listing whose table shows shown
// rows. It counts the rows shown, which is what [toolutil.WriteListHeading]
// asks for, so a page whose table the budget cut does not announce the
// results it left out as shown. Fewer rows never make it longer.
func (l blobListing) heading(shown int, p toolutil.PaginationOutput) string {
	var b strings.Builder
	toolutil.WriteListHeading(&b, l.title, shown, p)
	return b.String()
}

// codeTableRow is one result's row of the code search table.
func codeTableRow(bl BlobOutput) string {
	return toolutil.MarkdownTableRow(
		strconv.FormatInt(bl.ProjectID, 10),
		toolutil.EscapeMdTableCell(bl.Filename),
		toolutil.EscapeMdTableCell(bl.Path),
		toolutil.EscapeMdTableCell(bl.Ref),
		startCell(bl),
	)
}

// table returns the table of the rows of blobs that fit maxCodeMarkdownBytes
// under the heading that counts them, and how many it holds. Rows are taken
// in order and stop before the first that does not fit, each measured as the
// client receives it, a path that spells the guidance heading included. The
// heading is measured counting the rows with the one being tried, and a
// heading is never shorter for more rows, so a row that fails fails under
// every larger count too and the rows shown are the longest run from the
// first that fits. When not even the first row fits the table is empty, since
// a header with no row under it names nothing.
func (l blobListing) table(blobs []foundBlob, p toolutil.PaginationOutput) (table string, shown int) {
	var rows strings.Builder
	used := len(l.header)
	for _, bl := range blobs {
		row := l.row(bl)
		size := deliveredBytes(row)
		if !fitsCodeMarkdown(deliveredBytes(l.heading(shown+1, p))+used, size) {
			break
		}
		rows.WriteString(row)
		used += size
		shown++
	}
	if shown == 0 {
		return "", 0
	}
	return l.header + rows.String(), shown
}

// rowFitsAlone reports whether bl's row fits maxCodeMarkdownBytes on a table
// of its own, under the heading this page would give one row: what a page of
// that one result would show, but for its page numbers.
func (l blobListing) rowFitsAlone(bl foundBlob, p toolutil.PaginationOutput) bool {
	return fitsCodeMarkdown(deliveredBytes(l.heading(1, p))+len(l.header), deliveredBytes(l.row(bl)))
}

// sectionFitsAlone reports whether the snippet section of result n fits
// maxCodeMarkdownBytes after its row on a table of its own, under the heading
// this page would give one row: whether a page of that one result would show
// its snippet, but for its page numbers.
func (l blobListing) sectionFitsAlone(n int, bl foundBlob, p toolutil.PaginationOutput) bool {
	var b strings.Builder
	b.WriteString(l.row(bl))
	used := deliveredBytes(l.heading(1, p)) + len(l.header) + deliveredBytes(b.String())
	return fitsCodeMarkdown(used, snippetSectionBytes(&b, n, l.owner(bl), bl.BlobOutput, bl.snippet()))
}

// writeSnippetSections writes the section of each of blobs, the results the
// table shows, in order, until the first that does not fit what is left of
// maxCodeMarkdownBytes, and names the rows left without a section in one
// note, apart from those whose section would not fit a page of its own
// either. It reports whether any snippet was cut or left unshown.
func (l blobListing) writeSnippetSections(b *strings.Builder, c *toolutil.Card, blobs []foundBlob, p toolutil.PaginationOutput) bool {
	used := deliveredBytes(b.String())
	cut := false
	for i, bl := range blobs {
		n := i + 1
		s := bl.snippet()
		owner := l.owner(bl)
		section := snippetSectionBytes(b, n, owner, bl.BlobOutput, s)
		if !fitsCodeMarkdown(used, section) {
			var tooLarge []int
			for j := i; j < len(blobs); j++ {
				if !l.sectionFitsAlone(j+1, blobs[j], p) {
					tooLarge = append(tooLarge, j+1)
				}
			}
			c.Note(snippetsLeftOut.note(n, len(blobs), tooLarge))
			return true
		}
		writeSnippetSection(c, n, owner, bl.BlobOutput, s)
		used += section
		cut = cut || s.cut()
	}
	return cut
}

// format renders a page of the listing: a table naming each result, where it
// lives and the line its snippet starts on, then each snippet under a heading
// of its own, cut to a bounded head. The heading, the table and the sections
// share maxCodeMarkdownBytes, so a page of long paths shows the rows that
// fit, its heading counts those, and a note names the results it leaves out;
// the hint to read a whole file then says where their paths are. The
// structured result carries every result and every snippet whole.
func (l blobListing) format(blobs []foundBlob, p toolutil.PaginationOutput) string {
	if len(blobs) == 0 {
		return toolutil.EmptyMessage(l.empty)
	}
	table, rows := l.table(blobs, p)
	var b strings.Builder
	b.WriteString(l.heading(rows, p))
	b.WriteString(table)
	c := toolutil.NewCard(&b, "")
	cut := l.writeSnippetSections(&b, c, blobs[:rows], p)
	if rows < len(blobs) {
		var tooLarge []int
		for i := rows; i < len(blobs); i++ {
			if !l.rowFitsAlone(blobs[i], p) {
				tooLarge = append(tooLarge, i+1)
			}
		}
		c.Note(rowsLeftOut.note(rows+1, len(blobs), tooLarge))
		cut = true
	}
	var hints []string
	if cut {
		hints = append(hints, l.cutHint)
	}
	hints = append(hints, l.readHints...)
	toolutil.WriteListFooter(&b, p, false, hints...)
	return b.String()
}

// FormatCodeMarkdown renders a paginated list of code search results through
// codeListing: a table naming each result, its project and the line its
// snippet starts on, then each snippet under a heading of its own, cut to a
// bounded window around its match and held with the table to
// maxCodeMarkdownBytes.
func FormatCodeMarkdown(out CodeOutput) string {
	return codeListing.format(foundCode(out.Blobs, out.snippets), out.Pagination)
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
			toolutil.EscapeMdTableCell(noteKind(n)),
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

// FormatWikiMarkdown renders a paginated list of wiki search results through
// wikiListing. GitLab answers the wiki_blobs scope with the blob entity code
// search answers with, matched lines and all, so a wiki page is rendered the
// way a code result is: a table naming the wiki, the page and the line its
// snippet starts on, then each snippet in a fence, under the same caps and
// budget.
func FormatWikiMarkdown(out WikiOutput) string {
	return wikiListing.format(foundWiki(out.WikiBlobs), out.Pagination)
}

// noteKind is the Type cell of a note search row: the class of the object the
// note is on, marked when the note is internal. The row quotes the note's
// body, so the marker is what keeps a reader of the table alone from
// repeating an internal note as one anyone can read.
func noteKind(n NoteOutput) string {
	if n.Internal {
		return n.NoteableType + " (internal)"
	}
	return n.NoteableType
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
