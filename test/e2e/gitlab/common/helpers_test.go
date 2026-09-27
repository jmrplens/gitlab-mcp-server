//go:build e2e

// helpers_test.go holds the two things every file here says about a
// refusal: that it names what a caller needs to act on it, and how it is
// quoted in a log line; the one question every listing here is asked; and
// what paging through a list one item at a time must answer.

package common

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// containsID reports whether an ID is among those listed. It is a name for
// the question every listing here is asked, so an assertion reads as one.
func containsID(ids []int64, want int64) bool {
	return slices.Contains(ids, want)
}

// assertPagesOneAtATime asks a list action for its first two pages one item
// at a time, over a list the calling scenario has given at least two items,
// and holds the two answers to what a caller paging through the list needs:
// one item on each page, the first naming the second as the page that
// follows, the second naming itself and the first as the page before, and a
// different item on each. read takes an answer apart into the keys of its
// items and its pagination block.
//
// The list must be one nothing else can reorder between the two reads: a
// list of the scenario's own fixture, one only the scenario writes to, or
// one read oldest first, where an item created meanwhile lands after both
// pages. A list others add to at its front could move the first page's item
// onto the second.
func assertPagesOneAtATime[O any](e *harness.Env, s *harness.Session, action harness.ActionID, params map[string]any, read func(O) ([]string, toolutil.PaginationOutput)) {
	e.T.Helper()
	firstKeys, first := read(harness.Do[O](s, action, onePerPage(params, 1)))
	secondKeys, second := read(harness.Do[O](s, action, onePerPage(params, 2)))
	if len(firstKeys) != 1 || len(secondKeys) != 1 {
		e.T.Fatalf("%s at per_page 1 answered %v on page 1 and %v on page 2, want one item on each", action, firstKeys, secondKeys)
	}
	if first.Page != 1 || first.PerPage != 1 || first.NextPage != 2 || !first.HasMore {
		e.T.Errorf("%s page 1 answered the pagination %+v, want page 1 at per_page 1 naming page 2 as the next", action, first)
	}
	if second.Page != 2 || second.PerPage != 1 || second.PrevPage != 1 {
		e.T.Errorf("%s page 2 answered the pagination %+v, want page 2 at per_page 1 naming page 1 as the previous", action, second)
	}
	if firstKeys[0] == secondKeys[0] {
		e.T.Errorf("%s answered %s on both pages, want a different item on each", action, firstKeys[0])
	}
}

// onePerPage is params asking for one page of one item, leaving params itself
// as it was.
func onePerPage(params map[string]any, page int) map[string]any {
	asked := maps.Clone(params)
	if asked == nil {
		asked = map[string]any{}
	}
	asked["page"] = page
	asked["per_page"] = 1
	return asked
}

// idKeys renders a page's IDs as the keys assertPagesOneAtATime compares.
func idKeys(ids []int64) []string {
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, strconv.FormatInt(id, 10))
	}
	return keys
}

// assertMentions checks that a refusal carries every substring, without
// regard to case. The substrings are what the tool promises a caller: the
// parameter to fix, the sibling action to call first, by its canonical ID,
// the state the object must be in. A refusal that lost one of them is a
// refusal a model cannot act on.
func assertMentions(e *harness.Env, what, text string, substrings ...string) {
	e.T.Helper()
	lowered := strings.ToLower(text)
	for _, want := range substrings {
		if !strings.Contains(lowered, strings.ToLower(want)) {
			e.T.Errorf("%s does not mention %q: %s", what, want, firstLine(text))
		}
	}
}

// firstLine returns the first line of a refusal for a log line, since the
// card a refusal comes as runs to a dozen lines and the first names the
// reason.
func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}
