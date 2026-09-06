// markdown_test.go asserts the Markdown rendering of every achievement output
// type: the tables a model reads, the optional rows that only appear when the
// API returned them, and the next-step hints each result carries.
package achievements

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// assertContains fails when the rendered Markdown is missing a fragment.
func assertContains(t *testing.T, rendered string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(rendered, fragment) {
			t.Errorf("rendered Markdown is missing %q\n---\n%s", fragment, rendered)
		}
	}
}

// assertLacks fails when the rendered Markdown carries a fragment it should not.
func assertLacks(t *testing.T, rendered string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if strings.Contains(rendered, fragment) {
			t.Errorf("rendered Markdown unexpectedly contains %q\n---\n%s", fragment, rendered)
		}
	}
}

// TestFormatOutputMarkdown verifies a single achievement renders its identity
// and its optional rows, and drops the optional rows when they are empty.
func TestFormatOutputMarkdown(t *testing.T) {
	t.Run("every field populated", func(t *testing.T) {
		rendered := FormatOutputMarkdown(Output{Achievement: fullAchievement})
		assertContains(t, rendered,
			"## Achievement: First Commit",
			"| ID | 1 |",
			"| Namespace ID | 10 |",
			"| Description | Awarded for the first commit |",
			"[image](https://example.com/badge.png)",
			"25 May 2025",
			"gitlab_achievement_award")
	})
	t.Run("optional rows are omitted", func(t *testing.T) {
		rendered := FormatOutputMarkdown(Output{Achievement: bareAchievement})
		assertContains(t, rendered, "## Achievement: Second Commit", "| ID | 2 |")
		assertLacks(t, rendered, "| Description |", "| Avatar |", "| Created |", "| Updated |")
	})
}

// TestFormatDeleteOutputMarkdown verifies a deletion states what was removed
// and still shows the achievement it echoed back.
func TestFormatDeleteOutputMarkdown(t *testing.T) {
	rendered := FormatDeleteOutputMarkdown(DeleteOutput{
		Status:      "success",
		Message:     "Successfully deleted the achievement and every award made from it.",
		Achievement: fullAchievement,
	})
	assertContains(t, rendered,
		"## Achievement Deleted",
		"every award made from it",
		"| Name | First Commit |",
		"gitlab_achievement_create")
}

// TestFormatUserAchievementOutputMarkdown verifies one award renders its own ID
// separately from the achievement's, and shows the optional rows only when set.
func TestFormatUserAchievementOutputMarkdown(t *testing.T) {
	t.Run("every field populated", func(t *testing.T) {
		rendered := FormatUserAchievementOutputMarkdown(UserAchievementOutput{UserAchievement: fullUserAchievement})
		assertContains(t, rendered,
			"## Award 88",
			"| Award ID | 88 |",
			"| Achievement ID | 1 |",
			"| Shown On Profile | Yes |",
			"| Message | Shipped the first release |",
			"| Priority | 1 |",
			"| Revoked By | 4 |",
			"gitlab_achievement_revoke")
	})
	t.Run("optional rows are omitted", func(t *testing.T) {
		rendered := FormatUserAchievementOutputMarkdown(UserAchievementOutput{UserAchievement: bareUserAchievement})
		assertContains(t, rendered, "| Award ID | 89 |", "| Shown On Profile | No |")
		assertLacks(t, rendered, "| Message |", "| Priority |", "| Revoked |", "| Revoked By |")
	})
}

// TestFormatUserAchievementMutationOutputMarkdown verifies a revocation and a
// deletion each render their own message above the same award table.
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
			rendered := FormatUserAchievementMutationOutputMarkdown(UserAchievementMutationOutput{
				Status:          "success",
				Message:         tc.message,
				UserAchievement: fullUserAchievement,
			})
			assertContains(t, rendered, "## Award 88", tc.message, "| Award ID | 88 |")
		})
	}
}

// TestFormatListMarkdown verifies the achievement table, its link-preserving
// hint, its pagination footer, and the empty case that points at create.
func TestFormatListMarkdown(t *testing.T) {
	t.Run("with achievements", func(t *testing.T) {
		rendered := FormatListMarkdown(ListOutput{
			Achievements: []Achievement{fullAchievement, bareAchievement},
			Pagination:   toolutil.GraphQLPaginationOutput{HasNextPage: true, EndCursor: "cursor123"},
		})
		assertContains(t, rendered,
			"## Achievements (2)",
			toolutil.HintPreserveLinks,
			"| 1 | First Commit | 10 | Awarded for the first commit | [image](https://example.com/badge.png) |",
			"| 2 | Second Commit | 10 | - | - |",
			"next page cursor: `cursor123`")
	})
	t.Run("empty", func(t *testing.T) {
		rendered := FormatListMarkdown(ListOutput{})
		assertContains(t, rendered, "## Achievements (0)", "No achievements found", "gitlab_achievement_create")
		assertLacks(t, rendered, "| ID | Name |")
	})
}

// TestFormatUserAchievementListMarkdown verifies the award table shows the
// revoked column, which is the only signal that a listed award is not held.
func TestFormatUserAchievementListMarkdown(t *testing.T) {
	t.Run("with awards", func(t *testing.T) {
		rendered := FormatUserAchievementListMarkdown(UserAchievementListOutput{
			UserAchievements: []UserAchievement{fullUserAchievement, bareUserAchievement},
			Pagination:       toolutil.GraphQLPaginationOutput{HasPreviousPage: true, StartCursor: "cursor000"},
		})
		assertContains(t, rendered,
			"## Awards (2)",
			toolutil.HintPreserveLinks,
			"| 88 | 1 | 2 | 1 | Yes | 26 May 2025 09:00 UTC | Shipped the first release |",
			"| 89 | 1 | 5 | - | No | - | - |",
			"prev page cursor: `cursor000`")
	})
	t.Run("empty", func(t *testing.T) {
		rendered := FormatUserAchievementListMarkdown(UserAchievementListOutput{})
		assertContains(t, rendered, "## Awards (0)", "No awards found", "gitlab_achievement_award")
	})
}

// TestFormatReorderOutputMarkdown verifies the reordered set renders with its
// new priorities, and that an empty payload still says so.
func TestFormatReorderOutputMarkdown(t *testing.T) {
	t.Run("with awards", func(t *testing.T) {
		rendered := FormatReorderOutputMarkdown(ReorderOutput{
			Status:           "success",
			Message:          "Successfully reordered the awards, highest priority first.",
			UserAchievements: []UserAchievement{fullUserAchievement},
		})
		assertContains(t, rendered, "## Awards Reordered (1)", "highest priority first", "| 88 | 1 | 2 | 1 | Yes |")
	})
	t.Run("empty", func(t *testing.T) {
		rendered := FormatReorderOutputMarkdown(ReorderOutput{Status: "success", Message: "Nothing to reorder."})
		assertContains(t, rendered, "## Awards Reordered (0)", "No awards were returned")
	})
}

// TestFormatUniqueUsersMarkdown verifies distinct holders render as linked
// profiles, that a user without a web URL degrades to a plain name, and that a
// nil entry is skipped rather than printed as a blank row.
func TestFormatUniqueUsersMarkdown(t *testing.T) {
	t.Run("with users", func(t *testing.T) {
		rendered := FormatUniqueUsersMarkdown(UniqueUsersOutput{
			Users: []*toolutil.BasicUserOutput{
				{ID: 2, Username: "octocat", Name: "Octo Cat", State: "active", WebURL: "https://example.com/octocat"},
				{ID: 3, Username: "hubot", Name: "Hubot", State: "active"},
				nil,
			},
			Pagination: toolutil.GraphQLPaginationOutput{},
		})
		assertContains(t, rendered,
			"## Achievement Recipients (3)",
			toolutil.HintPreserveLinks,
			"| 2 | [octocat](https://example.com/octocat) | Octo Cat | active |",
			"| 3 | hubot | Hubot | active |",
			"no more pages")
		if strings.Count(rendered, "| active |") != 2 {
			t.Errorf("rendered %d user rows, want the nil entry skipped\n---\n%s", strings.Count(rendered, "| active |"), rendered)
		}
	})
	t.Run("empty", func(t *testing.T) {
		rendered := FormatUniqueUsersMarkdown(UniqueUsersOutput{})
		assertContains(t, rendered, "## Achievement Recipients (0)", "No users hold this achievement", "gitlab_achievement_award")
	})
}

// TestFormattersAreRegistered verifies each output type resolves through the
// shared Markdown registry, which is how a tool result reaches its formatter at
// runtime. A formatter written but not registered renders as raw JSON.
func TestFormattersAreRegistered(t *testing.T) {
	cases := []struct {
		name   string
		output any
		want   string
	}{
		{name: "Output", output: Output{Achievement: fullAchievement}, want: "## Achievement: First Commit"},
		{name: "DeleteOutput", output: DeleteOutput{Achievement: fullAchievement}, want: "## Achievement Deleted"},
		{name: "UserAchievementOutput", output: UserAchievementOutput{UserAchievement: fullUserAchievement}, want: "## Award 88"},
		{name: "UserAchievementMutationOutput", output: UserAchievementMutationOutput{UserAchievement: fullUserAchievement}, want: "## Award 88"},
		{name: "ListOutput", output: ListOutput{Achievements: []Achievement{fullAchievement}}, want: "## Achievements (1)"},
		{name: "UserAchievementListOutput", output: UserAchievementListOutput{UserAchievements: []UserAchievement{fullUserAchievement}}, want: "## Awards (1)"},
		{name: "ReorderOutput", output: ReorderOutput{UserAchievements: []UserAchievement{fullUserAchievement}}, want: "## Awards Reordered (1)"},
		{name: "UniqueUsersOutput", output: UniqueUsersOutput{Users: []*toolutil.BasicUserOutput{{ID: 2, Username: "octocat"}}}, want: "## Achievement Recipients (1)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rendered := markdownText(t, toolutil.MarkdownForResult(tc.output))
			if !strings.Contains(rendered, tc.want) {
				t.Errorf("MarkdownForResult(%s) = %q, want it to contain %q", tc.name, rendered, tc.want)
			}
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
			if !strings.Contains(tc.rendered, "&#124;") {
				t.Errorf("rendered Markdown does not escape the pipe\n---\n%s", tc.rendered)
			}
			if strings.Contains(tc.rendered, "It\nsecond line") {
				t.Errorf("rendered Markdown keeps the newline inside a cell\n---\n%s", tc.rendered)
			}
		})
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
	if !strings.HasPrefix(rendered, "## Achievement: &lt;b>Boom second") {
		t.Errorf("heading = %q, want the name neutralized and kept on one line", strings.SplitN(rendered, "\n", 2)[0])
	}
}
