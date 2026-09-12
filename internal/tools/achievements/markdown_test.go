// markdown_test.go asserts the Markdown rendering of every achievement output
// type: the whole document each formatter writes, the optional rows that only
// appear when the API returned them, and the next-step hints each result
// carries.
//
// Every expectation is the complete rendered document rather than a fragment of
// one. A substring assertion is how the defect class this migration closes
// survived: a formatter opened a table, wrote a list row into it, and every
// later "| Label | value |" rendered as literal pipes while a test asserting
// that same substring kept passing.
package achievements

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// fullAchievement is an achievement with every optional field populated.
var fullAchievement = Achievement{
	ID:          1,
	NamespaceID: 10,
	Name:        "First Commit",
	AvatarURL:   "https://example.com/badge.png",
	Description: "Awarded for the first commit",
	CreatedAt:   "2025-05-25T13:47:41Z",
	UpdatedAt:   "2025-05-26T09:00:00Z",
}

// bareAchievement has only the fields the API always returns, so the optional
// rows must be absent rather than rendered empty.
var bareAchievement = Achievement{ID: 2, NamespaceID: 10, Name: "Second Commit"}

// fullUserAchievement is a revoked award with every optional field populated.
var fullUserAchievement = UserAchievement{
	ID:              88,
	AchievementID:   1,
	UserID:          2,
	AwardedByUserID: 3,
	RevokedByUserID: new(int64(4)),
	CreatedAt:       "2025-05-25T13:47:41Z",
	UpdatedAt:       "2025-05-26T09:00:00Z",
	RevokedAt:       "2025-05-26T09:00:00Z",
	Priority:        new(int64(1)),
	ShowOnProfile:   true,
	AwardMessage:    "Shipped the first release",
}

// bareUserAchievement is a live award with no message and no priority.
var bareUserAchievement = UserAchievement{ID: 89, AchievementID: 1, UserID: 5, AwardedByUserID: 3}

// The rows the two shared writers produce for the two fixtures, so an
// expectation names them once.
const (
	fullAchievementRows = "- **ID**: 1\n" +
		"- **Name**: First Commit\n" +
		"- **Namespace ID**: 10\n" +
		"- **Description**: Awarded for the first commit\n" +
		"- **Avatar**: [image](https://example.com/badge.png)\n" +
		"- **Created**: 25 May 2025 13:47 UTC\n" +
		"- **Updated**: 26 May 2025 09:00 UTC\n"

	fullUserAchievementRows = "- **Award ID**: 88\n" +
		"- **Achievement ID**: 1\n" +
		"- **User ID**: 2\n" +
		"- **Awarded By**: 3\n" +
		"- **Shown On Profile**: ✅\n" +
		"- **Message**: Shipped the first release\n" +
		"- **Priority**: 1\n" +
		"- **Revoked**: 26 May 2025 09:00 UTC\n" +
		"- **Revoked By**: 4\n" +
		"- **Created**: 25 May 2025 13:47 UTC\n" +
		"- **Updated**: 26 May 2025 09:00 UTC\n"

	awardTableHeader = "| Award ID | Achievement ID | User ID | Priority | On Profile | Revoked | Message |\n" +
		"| --- | --- | --- | --- | --- | --- | --- |\n"

	fullAwardRow = "| 88 | 1 | 2 | 1 | ✅ | 26 May 2025 09:00 UTC | Shipped the first release |\n"
	bareAwardRow = "| 89 | 1 | 5 | - | ❌ | - | - |\n"
)

// assertRendered fails when the rendered document is not exactly want.
func assertRendered(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("rendered Markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestFormatOutputMarkdown verifies a single achievement renders as a card with
// its identity and its optional rows, and drops the optional rows when they are
// empty.
func TestFormatOutputMarkdown(t *testing.T) {
	t.Run("every field populated", func(t *testing.T) {
		assertRendered(t, FormatOutputMarkdown(Output{Achievement: fullAchievement}),
			"## Achievement: First Commit\n\n"+
				fullAchievementRows+
				"\n---\n💡 **Next steps:**\n"+
				"- Use action 'achievement.award' to hand this achievement to a user\n"+
				"- Use action 'achievement.recipients' to see who holds this achievement\n"+
				"- Use action 'achievement.list' to see the other achievements in the namespace\n")
	})
	t.Run("optional rows are omitted", func(t *testing.T) {
		assertRendered(t, FormatOutputMarkdown(Output{Achievement: bareAchievement}),
			"## Achievement: Second Commit\n\n"+
				"- **ID**: 2\n"+
				"- **Name**: Second Commit\n"+
				"- **Namespace ID**: 10\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- Use action 'achievement.award' to hand this achievement to a user\n"+
				"- Use action 'achievement.recipients' to see who holds this achievement\n"+
				"- Use action 'achievement.list' to see the other achievements in the namespace\n")
	})
}

// TestFormatDeleteOutputMarkdown verifies a deletion states what was removed as
// a card row rather than as a bare paragraph, and still shows the achievement
// GitLab echoed back.
func TestFormatDeleteOutputMarkdown(t *testing.T) {
	assertRendered(t, FormatDeleteOutputMarkdown(DeleteOutput{
		Status:      "success",
		Message:     "Successfully deleted the achievement and every award made from it.",
		Achievement: fullAchievement,
	}),
		"## Achievement Deleted\n\n"+
			"- **Result**: Successfully deleted the achievement and every award made from it.\n"+
			fullAchievementRows+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'achievement.list' to see the other achievements in the namespace\n"+
			"- Use action 'achievement.create' to define a replacement achievement\n")
}

// TestFormatUserAchievementOutputMarkdown verifies one award renders its own ID
// separately from the achievement's, and shows the optional rows only when set.
func TestFormatUserAchievementOutputMarkdown(t *testing.T) {
	hints := "\n---\n💡 **Next steps:**\n" +
		"- Use action 'achievement.user_achievement_update' to change whether this award shows on the profile\n" +
		"- Use action 'achievement.revoke' to revoke it while keeping the record\n" +
		"- Use action 'achievement.user_list' to see every award one user holds\n"
	t.Run("every field populated", func(t *testing.T) {
		assertRendered(t, FormatUserAchievementOutputMarkdown(UserAchievementOutput{UserAchievement: fullUserAchievement}),
			"## Award 88\n\n"+fullUserAchievementRows+hints)
	})
	t.Run("optional rows are omitted", func(t *testing.T) {
		assertRendered(t, FormatUserAchievementOutputMarkdown(UserAchievementOutput{UserAchievement: bareUserAchievement}),
			"## Award 89\n\n"+
				"- **Award ID**: 89\n"+
				"- **Achievement ID**: 1\n"+
				"- **User ID**: 5\n"+
				"- **Awarded By**: 3\n"+
				"- **Shown On Profile**: ❌\n"+hints)
	})
}

// TestFormatUserAchievementMutationOutputMarkdown verifies a revocation and a
// deletion each render their own message as the card's first row.
func TestFormatUserAchievementMutationOutputMarkdown(t *testing.T) {
	cases := []struct {
		name    string
		message string
	}{
		{name: "revoked", message: "Successfully revoked the award. The record is kept and marked revoked."},
		{name: "deleted", message: "Successfully deleted the award record."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertRendered(t, FormatUserAchievementMutationOutputMarkdown(UserAchievementMutationOutput{
				Status:          "success",
				Message:         tc.message,
				UserAchievement: fullUserAchievement,
			}),
				"## Award 88\n\n"+
					"- **Result**: "+tc.message+"\n"+
					fullUserAchievementRows+
					"\n---\n💡 **Next steps:**\n"+
					"- Use action 'achievement.user_list' to see every award one user holds\n"+
					"- Use action 'achievement.recipients' to see who holds this achievement\n")
		})
	}
}

// TestFormatListMarkdown verifies the achievement table, the cursor footer, and
// that the guidance section closes the response rather than sitting between the
// heading and the table header, where it used to swallow the table.
func TestFormatListMarkdown(t *testing.T) {
	t.Run("with achievements", func(t *testing.T) {
		assertRendered(t, FormatListMarkdown(ListOutput{
			Achievements: []Achievement{fullAchievement, bareAchievement},
			Pagination:   toolutil.GraphQLPaginationOutput{HasNextPage: true, EndCursor: "cursor123"},
		}),
			"## Achievements (2)\n\n"+
				"| ID | Name | Namespace ID | Description | Avatar |\n"+
				"| --- | --- | --- | --- | --- |\n"+
				"| 1 | First Commit | 10 | Awarded for the first commit | [image](https://example.com/badge.png) |\n"+
				"| 2 | Second Commit | 10 | - | - |\n"+
				"\nShowing 2 items | next page cursor: `cursor123`\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- "+toolutil.HintPreserveLinks+"\n"+
				"- Use action 'achievement.award' to hand one of these achievements to a user\n"+
				"- Pass the `end_cursor` above as `after` to fetch the next page\n")
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, FormatListMarkdown(ListOutput{}),
			"No achievements found.\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- Use action 'achievement.create' to define the first achievement for this namespace\n")
	})
}

// TestFormatUserAchievementListMarkdown verifies the award table shows the
// revoked column, which is the only signal that a listed award is not held, and
// that a table with no link column does not ask the model to preserve links.
func TestFormatUserAchievementListMarkdown(t *testing.T) {
	t.Run("with awards", func(t *testing.T) {
		assertRendered(t, FormatUserAchievementListMarkdown(UserAchievementListOutput{
			UserAchievements: []UserAchievement{fullUserAchievement, bareUserAchievement},
			Pagination:       toolutil.GraphQLPaginationOutput{HasPreviousPage: true, StartCursor: "cursor000"},
		}),
			"## Awards (2)\n\n"+
				awardTableHeader+fullAwardRow+bareAwardRow+
				"\nShowing 2 items | prev page cursor: `cursor000`\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- Use action 'achievement.recipients' to see who holds this achievement\n"+
				"- Pass the `end_cursor` above as `after` to fetch the next page\n")
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, FormatUserAchievementListMarkdown(UserAchievementListOutput{}),
			"No awards found.\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- Use action 'achievement.award' to hand an achievement to a user\n")
	})
}

// TestFormatReorderOutputMarkdown verifies the reordered set renders as the card
// of the reorder with the awards as its nested collection, and that an empty
// payload says so in the server's own words.
func TestFormatReorderOutputMarkdown(t *testing.T) {
	t.Run("with awards", func(t *testing.T) {
		assertRendered(t, FormatReorderOutputMarkdown(ReorderOutput{
			Status:           "success",
			Message:          "Successfully reordered the awards, highest priority first.",
			UserAchievements: []UserAchievement{fullUserAchievement},
		}),
			"## Awards Reordered\n\n"+
				"- **Result**: Successfully reordered the awards, highest priority first.\n"+
				"- **Awards**: 1\n\n"+
				"### Awards\n\n"+
				awardTableHeader+fullAwardRow+
				"\n---\n💡 **Next steps:**\n"+
				"- Use action 'achievement.user_list' to see every award one user holds\n")
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, FormatReorderOutputMarkdown(ReorderOutput{Status: "success", Message: "Nothing to reorder."}),
			"## Awards Reordered\n\n"+
				"- **Result**: Nothing to reorder.\n\n"+
				"GitLab returned no awards for this reorder.\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- Use action 'achievement.user_list' to see every award one user holds\n")
	})
}

// TestFormatUniqueUsersMarkdown verifies distinct holders render as linked
// profiles, that a user without a web URL degrades to a plain handle, and that
// a nil entry is skipped rather than counted in the heading.
func TestFormatUniqueUsersMarkdown(t *testing.T) {
	t.Run("with users", func(t *testing.T) {
		assertRendered(t, FormatUniqueUsersMarkdown(UniqueUsersOutput{
			Users: []*toolutil.BasicUserOutput{
				{ID: 2, Username: "octocat", Name: "Octo Cat", State: "active", WebURL: "https://example.com/octocat"},
				{ID: 3, Username: "hubot", Name: "Hubot", State: "active"},
				nil,
			},
		}),
			"## Achievement Recipients (2)\n\n"+
				"| ID | Username | Name | State |\n"+
				"| --- | --- | --- | --- |\n"+
				"| 2 | [@octocat](https://example.com/octocat) | Octo Cat | active |\n"+
				"| 3 | @hubot | Hubot | active |\n"+
				"\nShowing 2 items | no more pages\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- "+toolutil.HintPreserveLinks+"\n"+
				"- Use action 'achievement.recipients' to see who holds this achievement\n"+
				"- Pass the `end_cursor` above as `after` to fetch the next page\n")
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, FormatUniqueUsersMarkdown(UniqueUsersOutput{}),
			"No recipients of this achievement found.\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- Use action 'achievement.award' to hand this achievement to a user\n")
	})
}

// TestFormattersAreRegistered verifies each output type resolves through the
// shared Markdown registry, which is how a tool result reaches its formatter at
// runtime. A formatter written but not registered renders as raw JSON.
func TestFormattersAreRegistered(t *testing.T) {
	cases := []struct {
		name     string
		output   any
		rendered string
	}{
		{name: "Output", output: Output{Achievement: fullAchievement}, rendered: FormatOutputMarkdown(Output{Achievement: fullAchievement})},
		{name: "DeleteOutput", output: DeleteOutput{Achievement: fullAchievement}, rendered: FormatDeleteOutputMarkdown(DeleteOutput{Achievement: fullAchievement})},
		{name: "UserAchievementOutput", output: UserAchievementOutput{UserAchievement: fullUserAchievement}, rendered: FormatUserAchievementOutputMarkdown(UserAchievementOutput{UserAchievement: fullUserAchievement})},
		{name: "UserAchievementMutationOutput", output: UserAchievementMutationOutput{UserAchievement: fullUserAchievement}, rendered: FormatUserAchievementMutationOutputMarkdown(UserAchievementMutationOutput{UserAchievement: fullUserAchievement})},
		{name: "ListOutput", output: ListOutput{Achievements: []Achievement{fullAchievement}}, rendered: FormatListMarkdown(ListOutput{Achievements: []Achievement{fullAchievement}})},
		{name: "UserAchievementListOutput", output: UserAchievementListOutput{UserAchievements: []UserAchievement{fullUserAchievement}}, rendered: FormatUserAchievementListMarkdown(UserAchievementListOutput{UserAchievements: []UserAchievement{fullUserAchievement}})},
		{name: "ReorderOutput", output: ReorderOutput{UserAchievements: []UserAchievement{fullUserAchievement}}, rendered: FormatReorderOutputMarkdown(ReorderOutput{UserAchievements: []UserAchievement{fullUserAchievement}})},
		{name: "UniqueUsersOutput", output: UniqueUsersOutput{Users: []*toolutil.BasicUserOutput{{ID: 2, Username: "octocat"}}}, rendered: FormatUniqueUsersMarkdown(UniqueUsersOutput{Users: []*toolutil.BasicUserOutput{{ID: 2, Username: "octocat"}}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertRendered(t, markdownText(t, toolutil.MarkdownForResult(tc.output)), tc.rendered)
		})
	}
}

// markdownText unwraps the single text block a registered string formatter
// produces, failing when the registry returned nothing for the type.
func markdownText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
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
	return text.Text
}

// hostileText is what a person can type into an achievement's name, its
// description or an award message: a pipe, which ends a Markdown cell, and a
// newline, which ends a row.
const hostileText = "Ship | It\nsecond line"

var hostileAchievement = Achievement{ID: 1, NamespaceID: 10, Name: hostileText, Description: hostileText}

var hostileUserAchievement = UserAchievement{ID: 88, AchievementID: 1, UserID: 2, AwardedByUserID: 3, AwardMessage: hostileText}

// assertTableRowsWellFormed fails when a row of a Markdown table carries a
// different number of cells than the header above it, which is exactly what an
// unescaped pipe or newline in a cell produces.
//
// It counts structure rather than comparing against a golden string, so it
// keeps failing for the right reason after a column is added or a label is
// reworded.
func assertTableRowsWellFormed(t *testing.T, rendered string) {
	t.Helper()
	want := -1
	for line := range strings.SplitSeq(rendered, "\n") {
		if !strings.HasPrefix(line, "|") {
			want = -1
			continue
		}
		got := strings.Count(line, "|")
		switch {
		case want < 0:
			want = got
		case got != want:
			t.Errorf("table row %q has %d pipes, want %d\n---\n%s", line, got, want, rendered)
		}
	}
}

// TestMarkdownFormatters_PipeAndNewlineInText_StayInOneCell pins every
// achievement table against free text a person typed.
//
// A name, a description and an award message are unvalidated strings that the
// create handler trims and nothing more, so a pipe or a newline reaches the
// formatter through this server's own achievement.create and has to leave it
// as one cell. Every formatter in the package is driven, because the defect
// this guards was in all of them at once: a new domain shipped without calling
// the escaping helper 105 sibling packages call, and no gate could see it.
func TestMarkdownFormatters_PipeAndNewlineInText_StayInOneCell(t *testing.T) {
	cases := []struct {
		name     string
		rendered string
	}{
		{name: "Output", rendered: FormatOutputMarkdown(Output{Achievement: hostileAchievement})},
		{name: "DeleteOutput", rendered: FormatDeleteOutputMarkdown(DeleteOutput{Achievement: hostileAchievement, Message: "deleted"})},
		{name: "UserAchievementOutput", rendered: FormatUserAchievementOutputMarkdown(UserAchievementOutput{UserAchievement: hostileUserAchievement})},
		{name: "UserAchievementMutationOutput", rendered: FormatUserAchievementMutationOutputMarkdown(UserAchievementMutationOutput{UserAchievement: hostileUserAchievement, Message: "revoked"})},
		{name: "ListOutput", rendered: FormatListMarkdown(ListOutput{Achievements: []Achievement{hostileAchievement}})},
		{name: "UserAchievementListOutput", rendered: FormatUserAchievementListMarkdown(UserAchievementListOutput{UserAchievements: []UserAchievement{hostileUserAchievement}})},
		{name: "ReorderOutput", rendered: FormatReorderOutputMarkdown(ReorderOutput{UserAchievements: []UserAchievement{hostileUserAchievement}, Message: "reordered"})},
		{name: "UniqueUsersOutput", rendered: FormatUniqueUsersMarkdown(UniqueUsersOutput{Users: []*toolutil.BasicUserOutput{{ID: 3, Username: "jdoe", Name: hostileText, State: "active"}}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertTableRowsWellFormed(t, tc.rendered)
			assertHostileTextContained(t, tc.rendered)
		})
	}
}

// assertHostileTextContained fails when the pipe a person typed reaches a line
// that is neither a quote nor an escaped value.
//
// There are three shapes and the formatter picks by position: a value written
// into a card row or a table cell has its pipe written as &#124;, a multi-line
// body is quoted, where a pipe is literal text and can break nothing, and a
// heading carries the pipe as it is, since a heading has no cells to end.
// Requiring the entity everywhere would fail the quote, which is the stronger
// containment of the three.
func assertHostileTextContained(t *testing.T, rendered string) {
	t.Helper()
	for line := range strings.SplitSeq(rendered, "\n") {
		if !strings.Contains(line, "Ship") {
			continue
		}
		trimmed := strings.TrimLeft(line, " ")
		contained := strings.HasPrefix(trimmed, ">") ||
			strings.HasPrefix(trimmed, "#") ||
			strings.Contains(line, "&#124;")
		if !contained {
			t.Errorf("line %q carries the raw pipe outside a quote or a heading\n---\n%s", line, rendered)
		}
	}
	if strings.Contains(rendered, "It\nsecond line") {
		t.Errorf("rendered Markdown keeps the newline inside a cell\n---\n%s", rendered)
	}
}

// TestFormatOutputMarkdown_HostileName_DoesNotEscapeTheHeading pins the other
// place a GitLab-authored name is interpolated.
//
// A pipe in a heading is harmless, so the table check above cannot see this
// one: what matters here is that a name cannot promote itself to a new heading
// or open raw HTML a rendering client would obey.
func TestFormatOutputMarkdown_HostileName_DoesNotEscapeTheHeading(t *testing.T) {
	rendered := FormatOutputMarkdown(Output{Achievement: Achievement{Name: "# <b>Boom\nsecond"}})
	// The '#' survives because it is no longer at the start of the line: the
	// formatter composes "Achievement: # <b>Boom", and a hash mid-line is text.
	if !strings.HasPrefix(rendered, "## Achievement: # &lt;b>Boom second\n") {
		t.Errorf("heading = %q, want the name neutralized and kept on one line", strings.SplitN(rendered, "\n", 2)[0])
	}
}
