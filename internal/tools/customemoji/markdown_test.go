// markdown_test.go asserts the whole Markdown document each custom emoji
// formatter writes: the list table, which now carries the global ID the delete
// action takes, and the card one create returns.
package customemoji

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// partyParrot and shipIt are one internal and one external emoji, the two
// shapes GitLab's connection returns.
var (
	partyParrot = Item{
		ID:        "gid://gitlab/CustomEmoji/1",
		Name:      "party_parrot",
		URL:       "https://example.com/party_parrot.gif",
		External:  false,
		CreatedAt: "2026-06-01T10:00:00Z",
	}
	shipIt = Item{
		ID:        "gid://gitlab/CustomEmoji/2",
		Name:      "shipit",
		URL:       "https://example.com/shipit.png",
		External:  true,
		CreatedAt: "2026-06-15T14:30:00Z",
	}
)

// assertRendered fails when the rendered document is not exactly want.
func assertRendered(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("rendered Markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestFormatListMarkdown verifies the emoji table carries the ID a reader has
// to pass back to delete one, the display form of the creation time, and the
// sentence an empty page renders instead of a heading counting zero.
func TestFormatListMarkdown(t *testing.T) {
	t.Run("with emoji", func(t *testing.T) {
		assertRendered(t, FormatListMarkdown(ListOutput{
			Emoji:      []Item{partyParrot, shipIt},
			Pagination: toolutil.GraphQLPaginationOutput{HasNextPage: false},
		}),
			"## Custom Emoji (2)\n\n"+
				"| ID | Name | External | Created |\n"+
				"| --- | --- | --- | --- |\n"+
				"| `gid://gitlab/CustomEmoji/1` | [:party_parrot:](https://example.com/party_parrot.gif) | ❌ | 1 Jun 2026 10:00 UTC |\n"+
				"| `gid://gitlab/CustomEmoji/2` | [:shipit:](https://example.com/shipit.png) | ✅ | 15 Jun 2026 14:30 UTC |\n"+
				"\nShowing 2 items | no more pages\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- "+toolutil.HintPreserveLinks+"\n"+
				"- Use action 'custom_emoji.create' to add another emoji to the group\n"+
				"- Use action 'custom_emoji.delete' to remove one by the ID in the first column\n")
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, FormatListMarkdown(ListOutput{}), "No custom emoji found.\n")
	})
}

// TestFormatCreateMarkdown verifies the created emoji renders as the card of
// one object, with the global ID as a code span because it is the value the
// delete action takes.
func TestFormatCreateMarkdown(t *testing.T) {
	hints := "\n---\n💡 **Next steps:**\n" +
		"- Use action 'custom_emoji.list' to see every custom emoji in the group\n" +
		"- Use action 'custom_emoji.delete' to remove this emoji\n"
	t.Run("internal emoji", func(t *testing.T) {
		assertRendered(t, FormatCreateMarkdown(CreateOutput{Emoji: partyParrot}),
			"## "+toolutil.EmojiSuccess+" Custom Emoji Created\n\n"+
				"- **ID**: `gid://gitlab/CustomEmoji/1`\n"+
				"- **Name**: :party_parrot:\n"+
				"- **URL**: [https://example.com/party_parrot.gif](https://example.com/party_parrot.gif)\n"+
				"- **External**: ❌\n"+
				"- **Created**: 1 Jun 2026 10:00 UTC\n"+
				hints)
	})
	t.Run("external emoji", func(t *testing.T) {
		assertRendered(t, FormatCreateMarkdown(CreateOutput{Emoji: shipIt}),
			"## "+toolutil.EmojiSuccess+" Custom Emoji Created\n\n"+
				"- **ID**: `gid://gitlab/CustomEmoji/2`\n"+
				"- **Name**: :shipit:\n"+
				"- **URL**: [https://example.com/shipit.png](https://example.com/shipit.png)\n"+
				"- **External**: ✅\n"+
				"- **Created**: 15 Jun 2026 14:30 UTC\n"+
				hints)
	})
}

// TestCustomEmojiFormattersAreRegistered verifies each output type resolves
// through the shared Markdown registry, which is how a tool result reaches its
// formatter at runtime.
func TestCustomEmojiFormattersAreRegistered(t *testing.T) {
	cases := []struct {
		name     string
		output   any
		rendered string
	}{
		{
			name:     "ListOutput",
			output:   ListOutput{Emoji: []Item{partyParrot}},
			rendered: FormatListMarkdown(ListOutput{Emoji: []Item{partyParrot}}),
		},
		{
			name:     "CreateOutput",
			output:   CreateOutput{Emoji: partyParrot},
			rendered: FormatCreateMarkdown(CreateOutput{Emoji: partyParrot}),
		},
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
