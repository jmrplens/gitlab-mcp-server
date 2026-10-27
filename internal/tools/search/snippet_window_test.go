// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// snippet_window_test.go holds the window the Markdown shows of a code search
// snippet to the line that matched. GitLab sends no match line, so where the
// match sits is read from what each backend builds a snippet from: the
// context lines in force, the start line it reports and how many lines it
// sent. The tests rebuild each backend's snippet from GitLab's own source,
// independently of the code under test, and hold the window to showing the
// matched line for every context a caller can ask for.
package search

import (
	"fmt"
	"strings"
	"testing"
)

// TestMatchSpan_EachBackend_LinesThatCanHoldTheMatch verifies, backend by
// backend, the lines of a snippet that can hold its match, counted from 0.
// Advanced search (ee/lib/gitlab/elastic/search_results.rb) starts a snippet n
// lines above the match, or at the match itself when the match is fewer than
// n lines into the file, and ends it n lines below or at the match near the
// end, so the match is the first line or line n, and both when the snippet is
// n+1 lines starting within the first n lines of the file. Exact search on
// GitLab 19.5 (ee/lib/search/zoekt/search_results.rb build_result) starts a
// snippet at the match less the context lines it returned and never above line
// 1, so a start past line 1 puts the match at line n, and a start at line 1
// anywhere it fits. Basic search's git grep hunk holds its first match within
// its first three lines. A backend GitLab did not name is read every way at
// once.
func TestMatchSpan_EachBackend_LinesThatCanHoldTheMatch(t *testing.T) {
	tests := []struct {
		name      string
		backend   string
		context   *int64
		startline int64
		lines     int
		lo, hi    int
	}{
		{name: "basic, one line", backend: backendBasic, startline: 7, lines: 1, lo: 0, hi: 0},
		{name: "basic, two lines", backend: backendBasic, startline: 7, lines: 2, lo: 0, hi: 1},
		{name: "basic, a merged hunk", backend: backendBasic, startline: 7, lines: 40, lo: 0, hi: 2},
		{name: "basic ignores the context asked for", backend: backendBasic, context: new(int64(20)), startline: 7, lines: 41, lo: 0, hi: 2},
		{name: "advanced default, match with both contexts", backend: backendAdvanced, startline: 40, lines: 5, lo: 2, hi: 2},
		{name: "advanced, match with both contexts", backend: backendAdvanced, context: new(int64(5)), startline: 40, lines: 11, lo: 5, hi: 5},
		{name: "advanced, match first, context below", backend: backendAdvanced, context: new(int64(5)), startline: 3, lines: 6, lo: 0, hi: 5},
		{name: "advanced, start line at the context", backend: backendAdvanced, context: new(int64(5)), startline: 5, lines: 6, lo: 0, hi: 5},
		{name: "advanced, start line past the context", backend: backendAdvanced, context: new(int64(5)), startline: 6, lines: 6, lo: 5, hi: 5},
		{name: "advanced, match alone near both ends", backend: backendAdvanced, context: new(int64(5)), startline: 2, lines: 1, lo: 0, hi: 0},
		{name: "advanced, no context", backend: backendAdvanced, context: new(int64(0)), startline: 9, lines: 1, lo: 0, hi: 0},
		{name: "advanced, a shape it does not send", backend: backendAdvanced, context: new(int64(5)), startline: 9, lines: 8, lo: 0, hi: 5},
		{name: "advanced, a shape it does not send, short", backend: backendAdvanced, context: new(int64(5)), startline: 9, lines: 3, lo: 0, hi: 2},
		{name: "exact default, start past line 1", backend: backendExact, startline: 40, lines: 3, lo: 1, hi: 1},
		{name: "exact, start past line 1", backend: backendExact, context: new(int64(20)), startline: 40, lines: 41, lo: 20, hi: 20},
		{name: "exact, start past line 1, end of file", backend: backendExact, context: new(int64(20)), startline: 40, lines: 21, lo: 20, hi: 20},
		{name: "exact, start past line 1, too short", backend: backendExact, context: new(int64(20)), startline: 40, lines: 20, lo: 0, hi: 19},
		{name: "exact, start at line 1, long", backend: backendExact, context: new(int64(20)), startline: 1, lines: 35, lo: 14, hi: 20},
		{name: "exact, start at line 1, as long as the context", backend: backendExact, context: new(int64(20)), startline: 1, lines: 21, lo: 0, hi: 20},
		{name: "exact, start at line 1, both contexts", backend: backendExact, context: new(int64(20)), startline: 1, lines: 41, lo: 20, hi: 20},
		{name: "exact, a shape it does not send", backend: backendExact, context: new(int64(2)), startline: 40, lines: 6, lo: 0, hi: 2},
		{name: "exact on 19.4, the first line", backend: backendExact, context: new(int64(20)), startline: 0, lines: 2, lo: 0, hi: 1},
		{name: "no backend named, defaults", startline: 40, lines: 14, lo: 0, hi: 2},
		{name: "no backend named, a context", context: new(int64(20)), startline: 40, lines: 41, lo: 0, hi: 20},
		{name: "a backend this server does not know", backend: "semantic", startline: 40, lines: 14, lo: 0, hi: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := snippetContext{backend: tt.backend, contextLines: tt.context}
			lo, hi := ctx.matchSpan(tt.startline, tt.lines)
			if lo != tt.lo || hi != tt.hi {
				t.Errorf("matchSpan = %d..%d, want %d..%d", lo, hi, tt.lo, tt.hi)
			}
		})
	}
}

// TestSnippetWindow_SpanAndLines_ShowsTheSpanCenteredOrWhole verifies the lines
// the Markdown shows: at least maxSnippetLines of them, more when the lines
// that can hold the match need more, never more than the snippet has, placed
// so the span sits in the middle of the window and moved back inside the
// snippet at either end.
func TestSnippetWindow_SpanAndLines_ShowsTheSpanCenteredOrWhole(t *testing.T) {
	tests := []struct {
		name               string
		lo, hi, lines      int
		wantFirst, wantLen int
	}{
		{name: "no lines", lo: 0, hi: -1, lines: 0, wantFirst: 0, wantLen: 0},
		{name: "fewer lines than the cap", lo: 0, hi: 2, lines: 5, wantFirst: 0, wantLen: 5},
		{name: "exactly the cap", lo: 0, hi: 2, lines: 10, wantFirst: 0, wantLen: 10},
		{name: "head", lo: 0, hi: 2, lines: 14, wantFirst: 0, wantLen: 10},
		{name: "match in the middle", lo: 20, hi: 20, lines: 41, wantFirst: 16, wantLen: 10},
		{name: "even room splits evenly", lo: 20, hi: 21, lines: 41, wantFirst: 16, wantLen: 10},
		{name: "odd room puts the extra line below", lo: 20, hi: 22, lines: 41, wantFirst: 17, wantLen: 10},
		{name: "match near the end", lo: 39, hi: 39, lines: 41, wantFirst: 31, wantLen: 10},
		{name: "match on the last line", lo: 40, hi: 40, lines: 41, wantFirst: 31, wantLen: 10},
		{name: "match near the start", lo: 3, hi: 3, lines: 41, wantFirst: 0, wantLen: 10},
		{name: "span wider than the cap", lo: 9, hi: 20, lines: 30, wantFirst: 9, wantLen: 12},
		{name: "span the whole snippet", lo: 0, hi: 20, lines: 21, wantFirst: 0, wantLen: 21},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first, n := snippetWindow(tt.lo, tt.hi, tt.lines)
			if first != tt.wantFirst || n != tt.wantLen {
				t.Errorf("snippetWindow(%d, %d, %d) = %d, %d, want %d, %d", tt.lo, tt.hi, tt.lines, first, n, tt.wantFirst, tt.wantLen)
			}
		})
	}
}

// gitlabSnippet is a snippet as one backend builds it, read from GitLab's
// source rather than from the code under test: the 0-based range of a file's
// lines it sends and the start line it reports, for a file of total lines
// whose line m (from 1) matched and the context n the backend applies.
type gitlabSnippet func(total, m, n int) (from, to int, startline int64)

// advancedSnippet is ee/lib/gitlab/elastic/search_results.rb
// parse_search_result: from and to as it writes them from GitLab 19.0 on.
func advancedSnippet(total, m, n int) (from, to int, startline int64) {
	found := m - 1
	from, to = found, found
	if found >= n {
		from = found - n
	}
	if total-found >= n+1 {
		to = found + n
	}
	return from, to, int64(from + 1)
}

// advanced1811Snippet is parse_search_result as GitLab 18.11, the release
// that introduced num_context_lines, wrote it: the context after the match
// was sent only when strictly more than n+1 lines followed it, so a match
// exactly n lines from the end of the file is sent without it.
func advanced1811Snippet(total, m, n int) (from, to int, startline int64) {
	found := m - 1
	from, to = found, found
	if found >= n {
		from = found - n
	}
	if total-found > n+1 {
		to = found + n
	}
	return from, to, int64(from + 1)
}

// exactSnippet is the zoekt webserver's line match on GitLab 19.5 and later:
// up to n lines before and after the match, and build_result's start line,
// the match less the lines before it and never above line 1.
func exactSnippet(total, m, n int) (from, to int, startline int64) {
	before, after := min(n, m-1), min(n, total-m)
	return m - 1 - before, m - 1 + after, int64(max(m-before, 1))
}

// exact194Snippet is exact search on GitLab 19.4, which applies one line of
// context whatever was asked for and reports the match less one, never below 0.
func exact194Snippet(total, m, _ int) (from, to int, startline int64) {
	from, to, _ = exactSnippet(total, m, 1)
	return from, to, int64(max(m-1, 0))
}

// basicSnippet is Gitaly's git grep hunk around one match: two lines of
// context on either side.
func basicSnippet(total, m, _ int) (from, to int, startline int64) {
	from, to, _ = exactSnippet(total, m, 2)
	return from, to, int64(from + 1)
}

// lineStyle is how the lines of a matched file are written: the ending each
// line carries and what each line holds after its text. GitLab counts a
// file's lines by the line feed alone (Ruby's String#lines on advanced
// search, the line count of the context zoekt returns on exact search, git
// grep on basic search), so a carriage return before the line feed or inside
// a line is part of a line and never a line of its own.
type lineStyle struct {
	name, ending, inner string
}

// lineStyles are the ways a file's lines are written that the window is held
// to: line feeds, CRLF, the CRCRLF a file converted twice ends up with, and a
// lone carriage return inside every line.
var lineStyles = []lineStyle{
	{name: "LF", ending: "\n"},
	{name: "CRLF", ending: "\r\n"},
	{name: "CRCRLF", ending: "\r\r\n"},
	{name: "lone CR inside", ending: "\n", inner: "\rx"},
}

// lf is the line style of a file written with line feeds alone.
var lf = lineStyles[0]

// matchFile is a file of total lines written in style whose line m holds the
// match.
func matchFile(total, m int, style lineStyle) []string {
	lines := make([]string, total)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d%s%s", i+1, style.inner, style.ending)
	}
	lines[m-1] = fmt.Sprintf("MATCH %d%s%s", m, style.inner, style.ending)
	return lines
}

// matchShown is what the Markdown shows of the matched line m: its text
// followed by a line feed, which every line ending becomes.
func matchShown(m int) string {
	return fmt.Sprintf("MATCH %d\n", m)
}

// TestShapeSnippet_EveryContextAndBackend_ShowsTheMatchedLine verifies the
// acceptance of issue 1392 exhaustively: for every value of num_context_lines
// the input accepts and for none, every backend that can answer, files of 1 to
// 45 lines and a match on each of their lines, written with each line style,
// the lines the Markdown shows of the snippet GitLab builds hold the matched
// line, and never more than maxContextLines+1 of the lines GitLab sent are
// shown. Each backend is read both as GitLab names it in X-Search-Type and
// with no name at all. The forty combinations run in parallel, since each
// shapes some twenty thousand snippets and none shares state with another.
func TestShapeSnippet_EveryContextAndBackend_ShowsTheMatchedLine(t *testing.T) {
	t.Parallel()
	backends := []struct {
		name, header string
		snippet      gitlabSnippet
		defaultN     int
	}{
		{name: "basic", header: backendBasic, snippet: basicSnippet, defaultN: 2},
		{name: "advanced", header: backendAdvanced, snippet: advancedSnippet, defaultN: 2},
		{name: "advanced on 18.11", header: backendAdvanced, snippet: advanced1811Snippet, defaultN: 2},
		{name: "exact", header: backendExact, snippet: exactSnippet, defaultN: 1},
		{name: "exact on 19.4", header: backendExact, snippet: exact194Snippet, defaultN: 1},
	}
	contexts := []*int64{nil}
	for n := range int64(maxContextLines + 1) {
		contexts = append(contexts, new(n))
	}
	for _, backend := range backends {
		for _, named := range []bool{true, false} {
			for _, style := range lineStyles {
				t.Run(fmt.Sprintf("%s/named %v/%s", backend.name, named, style.name), func(t *testing.T) {
					t.Parallel()
					header := ""
					if named {
						header = backend.header
					}
					for _, contextLines := range contexts {
						n := backend.defaultN
						if contextLines != nil {
							n = int(*contextLines)
						}
						assertEveryMatchShown(t, backend.snippet, snippetContext{backend: header, contextLines: contextLines}, n, style)
					}
				})
			}
		}
	}
}

// assertEveryMatchShown builds, for files of 1 to 45 lines written in style
// and a match on each of their lines, the snippet build cuts with n lines of
// context, and checks that the lines shapeSnippet keeps of it, read as ctx,
// hold the matched line, that it counts the lines GitLab sent as GitLab
// counts them, and that it keeps never more than maxContextLines+1 of them.
func assertEveryMatchShown(t *testing.T, build gitlabSnippet, ctx snippetContext, n int, style lineStyle) {
	t.Helper()
	for total := 1; total <= 45; total++ {
		for m := 1; m <= total; m++ {
			file := matchFile(total, m, style)
			from, to, startline := build(total, m, n)
			s := shapeSnippet(strings.Join(file[from:to+1], ""), ctx, startline)
			if !strings.Contains(s.text, matchShown(m)) {
				t.Fatalf("context %d, %d lines, match on line %d (snippet lines %d to %d, start line %d): shown %q holds no %q",
					n, total, m, from+1, to+1, startline, s.text, matchShown(m))
			}
			if s.lines != to-from+1 {
				t.Fatalf("context %d, %d lines, match on line %d: counted %d lines where GitLab sent %d", n, total, m, s.lines, to-from+1)
			}
			if s.shown > maxContextLines+1 {
				t.Fatalf("context %d, %d lines, match on line %d: %d lines shown", n, total, m, s.shown)
			}
		}
	}
}

// TestFormatCodeMarkdown_ContextLines_WindowFollowsTheMatch verifies the whole
// render of a snippet a large context made, for each shape that moves or
// widens the window: the section names the lines of the file it shows when
// they do not start at the top, together with the lines of the file GitLab
// sent, so they read in the numbering the heading's start line uses; it shows
// every line that can hold the match when GitLab's answer cannot say which it
// is, leaves the head of a snippet that fits the cap untouched, and counts a
// snippet's lines as GitLab does whatever carriage returns its lines carry.
func TestFormatCodeMarkdown_ContextLines_WindowFollowsTheMatch(t *testing.T) {
	type window struct{ from, to int }
	tests := []struct {
		name     string
		backend  string
		context  int64
		total, m int
		snippet  gitlabSnippet
		style    lineStyle
		shown    window
		note     string
	}{
		{
			name: "advanced, twenty lines either side", backend: backendAdvanced, context: 20, total: 400, m: 300,
			snippet: advancedSnippet, style: lf, shown: window{16, 25},
			note: "Showing lines 296 to 305 of the file, around the match, of lines 280 to 320 that GitLab sent for this result.",
		},
		{
			name: "advanced, twenty lines either side, a lone carriage return in every line", backend: backendAdvanced, context: 20, total: 400, m: 300,
			snippet: advancedSnippet, style: lineStyles[3], shown: window{16, 25},
			note: "Showing lines 296 to 305 of the file, around the match, of lines 280 to 320 that GitLab sent for this result.",
		},
		{
			name: "advanced, match first or last", backend: backendAdvanced, context: 12, total: 400, m: 3,
			snippet: advancedSnippet, style: lf, shown: window{0, 12},
		},
		{
			name: "exact, match fourteen lines into the file", backend: backendExact, context: 20, total: 40, m: 15,
			snippet: exactSnippet, style: lf, shown: window{13, 22},
			note: "Showing lines 14 to 23 of the file, around the match, of lines 1 to 35 that GitLab sent for this result.",
		},
		{
			name: "exact, match ten lines into a short file", backend: backendExact, context: 20, total: 20, m: 10,
			snippet: exactSnippet, style: lf, shown: window{0, 19},
		},
		{
			name: "exact, match nine lines into the file, wider than the cap", backend: backendExact, context: 20, total: 30, m: 10,
			snippet: exactSnippet, style: lf, shown: window{9, 20},
			note: "Showing lines 10 to 21 of the file, around the match, of lines 1 to 30 that GitLab sent for this result.",
		},
		{
			name: "exact, five lines either side, CRCRLF line endings", backend: backendExact, context: 5, total: 400, m: 300,
			snippet: exactSnippet, style: lineStyles[2], shown: window{1, 10},
			note: "Showing lines 296 to 305 of the file, around the match, of lines 295 to 305 that GitLab sent for this result.",
		},
		{
			name: "exact, default context", backend: backendExact, total: 400, m: 300, context: -1,
			snippet: exactSnippet, style: lf, shown: window{0, 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := snippetContext{backend: tt.backend}
			n := 1
			if tt.context >= 0 {
				ctx.contextLines = new(tt.context)
				n = int(tt.context)
			}
			file := matchFile(tt.total, tt.m, tt.style)
			from, to, startline := tt.snippet(tt.total, tt.m, n)
			sent := file[from : to+1]
			got := FormatCodeMarkdown(CodeOutput{
				Blobs:      []BlobOutput{{Filename: "a.go", Path: "a.go", Ref: "main", Startline: startline, ProjectID: 7, Data: strings.Join(sent, "")}},
				Pagination: onePage,
				snippets:   ctx,
			})
			section := fmt.Sprintf("\n### Result 1: a.go (project 7, start line %d)\n\n", startline)
			hints := []string{hintFileGet}
			if tt.note != "" {
				section += tt.note + "\n\n"
				hints = []string{hintCutSnippet, hintFileGet}
			}
			section += "```\n" + renderedLines(sent[tt.shown.from:tt.shown.to+1]) + "```\n"
			want := "## Code Search Results (1)\n\n" + codeTableHeader +
				fmt.Sprintf("| 7 | a.go | a.go | main | %d |\n", startline) +
				section + onePageFooter(hints...)
			if got != want {
				t.Errorf("render:\n got %q\nwant %q", got, want)
			}
		})
	}
}

// renderedLines is how the fence shows lines GitLab sent: each one's text and
// a line feed, the carriage return of a CRLF or CRCRLF ending dropped as part
// of the ending, and any other carriage return turned into a line feed,
// since CommonMark reads it as a line break.
func renderedLines(lines []string) string {
	var b strings.Builder
	for _, line := range lines {
		text := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		b.WriteString(strings.ReplaceAll(text, "\r", "\n"))
		b.WriteString("\n")
	}
	return b.String()
}
