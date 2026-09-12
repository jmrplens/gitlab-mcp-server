// markdown_test.go contains unit tests for the group relations export status
// list formatter.
//
// Every expectation here is the whole rendered response: the formatter used to
// write its guidance section before the table, which glued the table header to
// a list item and left no table at all, and a substring assertion saw none of
// that.
package grouprelationsexport

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestFormatListExportStatusMarkdownString verifies the whole status list: the
// heading counts what GitLab reported, the status code is named as well as
// shown, the batched flag is a glyph, and the guidance closes the response.
func TestFormatListExportStatusMarkdownString(t *testing.T) {
	md := FormatListExportStatusMarkdownString(ListExportStatusOutput{
		Statuses: []ExportStatusItem{
			{Relation: "projects", Status: 1, Batched: false, BatchesCount: 0, UpdatedAt: "2026-01-01T00:00:00Z"},
			{Relation: "milestones", Status: 0, Error: "timeout", Batched: true, BatchesCount: 3, UpdatedAt: "2026-01-02T00:00:00Z"},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 2},
	})

	want := "## Group Relations Export Status (2)\n\n" +
		"| Relation | Status | Batched | Batches | Error |\n| --- | --- | --- | --- | --- |\n" +
		"| projects | finished (1) | ❌ | 0 |  |\n" +
		"| milestones | started (0) | ✅ | 3 | timeout |\n" +
		"\nPage 1 of 1 | 2 items total\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'group.group_relations_schedule' to start a new export\n"
	if md != want {
		t.Errorf("export status list:\n got %q\nwant %q", md, want)
	}
}

// TestFormatListExportStatusMarkdownString_Empty verifies an empty list is the
// one sentence and nothing else.
func TestFormatListExportStatusMarkdownString_Empty(t *testing.T) {
	if md := FormatListExportStatusMarkdownString(ListExportStatusOutput{}); md != "No export statuses found.\n" {
		t.Errorf("empty status list = %q, want the one-sentence empty message", md)
	}
}

// TestFormatListExportStatusMarkdownString_EscapesTheRelation verifies a
// relation name carrying a pipe cannot end its cell.
func TestFormatListExportStatusMarkdownString_EscapesTheRelation(t *testing.T) {
	md := FormatListExportStatusMarkdownString(ListExportStatusOutput{
		Statuses: []ExportStatusItem{{Relation: "test|pipe", Status: 1}},
	})
	if !strings.Contains(md, "| test&#124;pipe | finished (1) |") {
		t.Errorf("the pipe was not neutralized:\n%s", md)
	}
}

// TestExportStatusLabel verifies every state GitLab reports as a number is
// named, with the number kept, and that a value outside the three is shown as
// the number alone rather than given an invented word.
func TestExportStatusLabel(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int64
		want   string
	}{
		{name: "failed", status: -1, want: "failed (-1)"},
		{name: "started", status: 0, want: "started (0)"},
		{name: "finished", status: 1, want: "finished (1)"},
		{name: "unknown", status: 7, want: "7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := exportStatusLabel(tc.status); got != tc.want {
				t.Errorf("exportStatusLabel(%d) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}
