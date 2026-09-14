//go:build e2e

// helpers_test.go holds the two assertions every file here makes about a
// refusal: that it names the things a caller needs to act on it, and how it
// is quoted in a log line.

package ee

import (
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// assertMentions checks that a refusal carries every substring, without
// regard to case. The substrings are what the tool promises a caller: the
// parameter to fix, the sibling tool to call first, the state the object
// must be in. A refusal that lost one of them is a refusal a model cannot
// act on.
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

// mentionsAny reports whether text carries any of the substrings, without
// regard to case. It is the question a test asks before accepting a refusal:
// a scenario that accepts every error accepts a transport failure as the
// refusal it was looking for.
func mentionsAny(text string, substrings ...string) bool {
	lowered := strings.ToLower(text)
	for _, want := range substrings {
		if strings.Contains(lowered, strings.ToLower(want)) {
			return true
		}
	}
	return false
}

// containsID reports whether an ID is among those listed. It is a name for
// the question every listing here is asked, so an assertion reads as one.
func containsID(ids []int64, want int64) bool {
	return slices.Contains(ids, want)
}
