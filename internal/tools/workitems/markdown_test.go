package workitems

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestHandleList_BlankNameAmongThem_IsLeftOut verifies the "@handle" list a
// card row shows: every name becomes its escaped handle, and a blank one,
// which GitLab sends for a user it no longer names, is left out rather than
// written as a bare "@".
func TestHandleList_BlankNameAmongThem_IsLeftOut(t *testing.T) {
	if got, want := handleList([]string{"alice", " ", "bob"}), "@alice, @bob"; got != want {
		t.Errorf("handleList = %q, want %q", got, want)
	}
	if got := handleList(nil); got != "" {
		t.Errorf("handleList(nil) = %q, want nothing", got)
	}
}

// TestFormatListMarkdown_OnlyTheFirstItemLinked_KeepsTheLinksHint verifies
// that a page asks the reader to keep the links as soon as one item carries
// an address, whichever item it is: the flag is set by the first linked row
// and kept by the rows after it that carry none.
func TestFormatListMarkdown_OnlyTheFirstItemLinked_KeepsTheLinksHint(t *testing.T) {
	result := FormatListMarkdown(ListOutput{WorkItems: []WorkItemItem{
		{IID: 1, Title: "linked", WebURL: "https://gitlab.example.com/g/p/-/work_items/1"},
		{IID: 2, Title: "unlinked"},
	}})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, toolutil.HintPreserveLinks) {
		t.Errorf("FormatListMarkdown =\n%s\nwant the hint to keep the links", text)
	}
}

// TestStateCell_EitherSpellingOfAState_GetsItsEmojiAndKeepsItsWords verifies
// the state cell of a work item: GitLab's GraphQL spelling (OPEN, CLOSED) and
// the REST one (opened) find the same emoji, the state is shown as GitLab
// sent it, a state the emoji table has no entry for keeps its words beside
// the unknown mark rather than borrowing another state's emoji, and an empty
// state is an empty cell.
func TestStateCell_EitherSpellingOfAState_GetsItsEmojiAndKeepsItsWords(t *testing.T) {
	cases := []struct {
		name  string
		state string
		want  string
	}{
		{name: "graphql open", state: "OPEN", want: toolutil.EmojiGreen + " OPEN"},
		{name: "rest opened", state: "opened", want: toolutil.EmojiGreen + " opened"},
		{name: "graphql closed", state: " CLOSED ", want: toolutil.EmojiRed + "  CLOSED "},
		{name: "a state the table does not know", state: "LOCKED", want: toolutil.EmojiQuestion + " LOCKED"},
		{name: "blank", state: "  ", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stateCell(tc.state); got != tc.want {
				t.Errorf("stateCell(%q) = %q, want %q", tc.state, got, tc.want)
			}
		})
	}
}
