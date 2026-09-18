// markdown_test.go asserts the whole Markdown document each custom emoji
// formatter writes: the list table, which now carries the global ID the delete
// action takes, and the card one create returns.
package customemoji

import (
	"strings"
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

// TestFormatListMarkdown_EmojiWithoutURL_RendersTheBareNameAndDropsTheLinkHint
// verifies that an emoji whose URL this server never received renders as the
// bare ":name:" form and that the table then claims no links at all.
//
// Both halves matter to a reader. A name wrapped in link syntax with nothing
// between the parentheses is a link to the document itself, so a model told to
// preserve the clickable links would hand the user a control that goes
// nowhere; and [toolutil.HintPreserveLinks] on a table carrying no link is an
// instruction about something that is not there, which the shared footer drops
// only if the formatter tells it the truth about what it wrote.
func TestFormatListMarkdown_EmojiWithoutURL_RendersTheBareNameAndDropsTheLinkHint(t *testing.T) {
	assertRendered(t, FormatListMarkdown(ListOutput{
		Emoji: []Item{{
			ID:        "gid://gitlab/CustomEmoji/3",
			Name:      "no_image",
			CreatedAt: "2026-06-20T08:00:00Z",
		}},
	}),
		"## Custom Emoji (1)\n\n"+
			"| ID | Name | External | Created |\n"+
			"| --- | --- | --- | --- |\n"+
			"| `gid://gitlab/CustomEmoji/3` | :no_image: | ❌ | 20 Jun 2026 08:00 UTC |\n"+
			"\nShowing 1 items | no more pages\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'custom_emoji.create' to add another emoji to the group\n"+
			"- Use action 'custom_emoji.delete' to remove one by the ID in the first column\n")
}

// TestFormatListMarkdown_EmojiWithoutName_RendersAnEmptyCellRatherThanALink
// verifies that an emoji carrying no name leaves the Name cell empty, even
// though GitLab sent an image URL the other rows are linked through.
//
// The link is built from the name, so a row with no name has no text to link:
// rendering it anyway produces "[](url)", an empty label whose destination a
// reader cannot see and cannot judge before following it. The row keeps its
// ID, which is the value the delete action takes, so the emoji stays
// actionable without being mislabelled. Because this row does carry a URL the
// preserve-links hint stays, which is what distinguishes "this table has no
// links" from "this cell has no name".
func TestFormatListMarkdown_EmojiWithoutName_RendersAnEmptyCellRatherThanALink(t *testing.T) {
	rendered := FormatListMarkdown(ListOutput{
		Emoji: []Item{{
			ID:        "gid://gitlab/CustomEmoji/4",
			URL:       "https://example.com/unnamed.png",
			External:  true,
			CreatedAt: "2026-06-20T09:00:00Z",
		}},
	})
	assertRendered(t, rendered,
		"## Custom Emoji (1)\n\n"+
			"| ID | Name | External | Created |\n"+
			"| --- | --- | --- | --- |\n"+
			"| `gid://gitlab/CustomEmoji/4` |  | ✅ | 20 Jun 2026 09:00 UTC |\n"+
			"\nShowing 1 items | no more pages\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- "+toolutil.HintPreserveLinks+"\n"+
			"- Use action 'custom_emoji.create' to add another emoji to the group\n"+
			"- Use action 'custom_emoji.delete' to remove one by the ID in the first column\n")
	// Asserted apart from the document above so that a future rewrite of the
	// expected text cannot carry the defect back in unnoticed: whatever else
	// the table says, the image URL of a nameless emoji is never the
	// destination of a link.
	if strings.Contains(rendered, "](https://example.com/unnamed.png)") {
		t.Errorf("a row with no name linked its image URL\n%s", rendered)
	}
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
