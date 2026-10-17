package search

// GitLab sends no match line with a code search result (API::Entities::Blob
// exposes none), only the lines around the match and the line the first of
// them starts on. Where the match sits among those lines is decided by how
// the backend that answered builds a snippet, which is read here from its
// source rather than guessed from the text, so the Markdown can show the
// lines around the match whatever context the caller asked for.

// The context lines each backend applies when the request sends no
// num_context_lines: advanced search's DEFAULT_NUM_CONTEXT_LINES
// (ee/lib/gitlab/elastic/search_results.rb), exact search's
// DEFAULT_NUM_CONTEXT_LINES (ee/lib/search/zoekt/params.rb, the constant
// CONTEXT_LINES_COUNT of the client on 19.4 and earlier), and the context
// basic search's git grep is always run with, which no parameter changes
// (Gitaly's SearchFilesByContent, --before-context and --after-context 2).
const (
	advancedDefaultContext = 2
	exactDefaultContext    = 1
	basicContext           = 2
)

// snippetContext is how GitLab cut the snippets of one page of code search
// results: the backend that answered, as its X-Search-Type header names it,
// and the num_context_lines the request sent, nil when it sent none. A wiki
// search page carries the zero value, which reads its snippets the way every
// backend could have cut them with its default context.
type snippetContext struct {
	backend      string
	contextLines *int64
}

// context is the context lines in force: the ones the request sent, or the
// backend's default.
func (c snippetContext) context(byDefault int) int {
	if c.contextLines != nil {
		return int(*c.contextLines)
	}
	return byDefault
}

// matchSpan returns the first and the last of a snippet's lines, counted from
// 0, that can hold the line the result matched on, for a snippet of lines
// lines whose first line is startline. A backend GitLab did not name, or one
// this server does not know, is read every way a backend could have cut the
// snippet, which spans them all.
func (c snippetContext) matchSpan(startline int64, lines int) (lo, hi int) {
	switch c.backend {
	case backendBasic:
		return basicSpan(lines)
	case backendAdvanced:
		return advancedSpan(c.context(advancedDefaultContext), startline, lines)
	case backendExact:
		return exactSpan(c.context(exactDefaultContext), startline, lines)
	}
	basicLo, basicHi := basicSpan(lines)
	advancedLo, advancedHi := advancedSpan(c.context(advancedDefaultContext), startline, lines)
	exactLo, exactHi := exactSpan(c.context(exactDefaultContext), startline, lines)
	return min(basicLo, advancedLo, exactLo), max(basicHi, advancedHi, exactHi)
}

// basicSpan reads a basic search snippet, a git grep hunk: it starts two
// lines above its first match, or at the top of the file, so that match is
// within its first three lines. The parameter is refused on GitLab Enterprise
// and dropped on Community Edition, so the context the request asked for has
// no part in it.
func basicSpan(lines int) (lo, hi int) {
	return 0, min(basicContext, lines-1)
}

// advancedSpan reads an advanced search snippet. parse_search_result
// (ee/lib/gitlab/elastic/search_results.rb) starts it n lines above the match
// when the match is at least n lines into the file and at the match itself
// otherwise, and ends it n lines below the match when the file has them and
// at the match otherwise. So the match is the first line, of a snippet of
// n+1 lines or 1 starting within the file's first n lines, or line n, of a
// snippet of 2n+1 lines or n+1. So a snippet of 2n+1 lines, or of n+1 lines
// starting past line n, holds it on line n, and a snippet of one line on that
// line. Where both readings fit, which is a snippet of n+1 lines starting
// within the first n, the answer does not say which, and a shape advanced
// search does not send is read as the first n+1 lines.
func advancedSpan(n int, startline int64, lines int) (lo, hi int) {
	if lines == 2*n+1 || (lines == n+1 && startline > int64(n)) {
		return n, n
	}
	return 0, min(n, lines-1)
}

// exactSpan reads an exact search snippet. Exact search on GitLab 19.5 and
// later sends up to n lines either side of the match and reports the match
// less the lines before it, never above line 1 (build_result,
// ee/lib/search/zoekt/search_results.rb). So a start past line 1 puts the
// match at line n, and a start at line 1 leaves it wherever the lines after it
// allow: at least lines-1-n lines down and at most n. On 19.4 and earlier one
// line of context is sent whatever was asked for, which reads either way
// within a snippet of three lines; a shape exact search does not send is read
// as the first n+1 lines.
func exactSpan(n int, startline int64, lines int) (lo, hi int) {
	if lines > 2*n+1 {
		return 0, n
	}
	if startline > 1 && lines > n {
		return n, n
	}
	return max(0, lines-1-n), min(n, lines-1)
}

// snippetWindow returns the first line, counted from 0, and the number of the
// lines the Markdown shows of a snippet of lines lines whose match is within
// lo to hi: maxSnippetLines of them, or the whole span when it is longer,
// never more than the snippet has, with the span in the middle of the window
// and the window kept inside the snippet. A span of up to maxSnippetLines
// starting within the first lines leaves the window at the top, which is how
// every snippet sent with the default context is shown.
func snippetWindow(lo, hi, lines int) (first, n int) {
	span := hi - lo + 1
	n = min(max(maxSnippetLines, span), lines)
	first = min(max(lo-(n-span)/2, 0), lines-n)
	return first, n
}
