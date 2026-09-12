// markdown_test.go asserts the whole Markdown document each feature flag user
// list formatter writes: the card one list renders as, and the table a page of
// them renders as.
package ffuserlists

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// listCardHints is the guidance section every user list card closes with.
const listCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'feature_flags.ff_user_list_update' to modify the user XIDs on this list\n" +
	"- Use action 'feature_flags.ff_user_list_delete' to remove this user list\n"

// listTableHints is the guidance section every user list table closes with.
const listTableHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'feature_flags.ff_user_list_get' to read one list by its user_list_iid\n" +
	"- Use action 'feature_flags.ff_user_list_create' to add a new user list\n"

// assertRendered fails when the rendered document is not exactly want.
func assertRendered(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("rendered Markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestFormatUserListMarkdown verifies the user list card: the ID and the IID as
// rows of their own, since the IID is what every other action takes, and the
// timestamps in the display form.
func TestFormatUserListMarkdown(t *testing.T) {
	t.Run("without timestamps", func(t *testing.T) {
		assertRendered(t, FormatUserListMarkdown(Output{ID: 5, IID: 3, ProjectID: 10, Name: "my-list", UserXIDs: "x1"}),
			"## Feature Flag User List: my-list\n\n"+
				"- **ID**: 5\n"+
				"- **IID**: 3\n"+
				"- **Project ID**: 10\n"+
				"- **Name**: my-list\n"+
				"- **User XIDs**: x1\n"+
				listCardHints)
	})
	t.Run("with timestamps", func(t *testing.T) {
		assertRendered(t, FormatUserListMarkdown(Output{
			ID: 1, IID: 10, ProjectID: 42,
			Name: "cov-list", UserXIDs: "a,b",
			CreatedAt: "2026-06-01T12:00:00Z",
			UpdatedAt: "2026-06-02T12:00:00Z",
		}),
			"## Feature Flag User List: cov-list\n\n"+
				"- **ID**: 1\n"+
				"- **IID**: 10\n"+
				"- **Project ID**: 42\n"+
				"- **Name**: cov-list\n"+
				"- **User XIDs**: a,b\n"+
				"- **Created**: 1 Jun 2026 12:00 UTC\n"+
				"- **Updated**: 2 Jun 2026 12:00 UTC\n"+
				listCardHints)
	})
}

// TestFormatListUserListsMarkdown verifies the user list table, which keys on
// the IID the other actions take and never on the internal ID, and the sentence
// an empty page renders instead of a heading counting zero.
func TestFormatListUserListsMarkdown(t *testing.T) {
	t.Run("with lists", func(t *testing.T) {
		assertRendered(t, FormatListUserListsMarkdown(ListOutput{
			UserLists: []Output{
				{ID: 1, IID: 10, Name: "list-1", UserXIDs: "u1"},
				{ID: 2, IID: 20, Name: "list-2", UserXIDs: "u2"},
			},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1},
		}),
			"## Feature Flag User Lists (2)\n\n"+
				"| IID | Name | User XIDs |\n"+
				"| --- | --- | --- |\n"+
				"| 10 | list-1 | u1 |\n"+
				"| 20 | list-2 | u2 |\n"+
				"\nPage 1 of 1\n"+
				listTableHints)
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, FormatListUserListsMarkdown(ListOutput{UserLists: []Output{}}),
			"No feature flag user lists found.\n")
	})
}

// TestUserListFormattersAreRegistered verifies each output type resolves
// through the shared Markdown registry, which is how a tool result reaches its
// formatter at runtime.
func TestUserListFormattersAreRegistered(t *testing.T) {
	card := Output{ID: 1, IID: 10, Name: "list-1", UserXIDs: "u1"}
	list := ListOutput{UserLists: []Output{card}}
	cases := []struct {
		name     string
		output   any
		rendered string
	}{
		{name: "Output", output: card, rendered: FormatUserListMarkdown(card)},
		{name: "ListOutput", output: list, rendered: FormatListUserListsMarkdown(list)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := toolutil.MarkdownForResult(tc.output)
			if result == nil {
				t.Fatal("MarkdownForResult returned nil, want a registered formatter for the type")
			}
			if len(result.Content) != 1 {
				t.Fatalf("content blocks = %d, want 1", len(result.Content))
			}
			text, ok := result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("content block = %T, want *mcp.TextContent", result.Content[0])
			}
			assertRendered(t, text.Text, tc.rendered)
		})
	}
}
