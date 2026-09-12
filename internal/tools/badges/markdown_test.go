// markdown_test.go asserts the whole Markdown document each badge formatter
// writes: the project and group tables, the badge card, and the preview card a
// preview gets instead of the stored badge's, since a preview has no ID, no
// name and no kind to show.
package badges

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// storedBadge is a badge GitLab returns from a get, add or edit: it has an ID,
// a name, a kind, and the URLs GitLab rendered from its templates.
var storedBadge = BadgeItem{
	ID:               1,
	Name:             "coverage",
	LinkURL:          "https://example.com/%{project_path}",
	ImageURL:         "https://img.shields.io/%{project_path}",
	RenderedLinkURL:  "https://example.com/group/project",
	RenderedImageURL: "https://img.shields.io/group/project",
	Kind:             "project",
}

// previewBadge is what the preview endpoint answers with: the two templates and
// the two rendered URLs, and nothing that identifies a stored badge.
var previewBadge = BadgeItem{
	LinkURL:          "https://example.com/%{project_path}",
	ImageURL:         "https://img.shields.io/%{project_path}",
	RenderedLinkURL:  "https://example.com/group/project",
	RenderedImageURL: "https://img.shields.io/group/project",
}

// badgeCardHints is the guidance section every stored-badge card closes with.
const badgeCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'project.badge_edit' to change this badge on a project\n" +
	"- Use action 'group.badge_edit' to change it on a group\n"

// badgeListHints is the guidance section every badge table closes with.
const badgeListHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'project.badge_get' to read one project badge\n" +
	"- Use action 'group.badge_get' to read one group badge\n"

// assertRendered fails when the rendered document is not exactly want.
func assertRendered(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("rendered Markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestFormatBadgeListMarkdown verifies the badge table, the heading count that
// follows the response's own total rather than the page length, and the
// sentence an empty page renders instead of a heading counting zero.
func TestFormatBadgeListMarkdown(t *testing.T) {
	t.Run("with badges", func(t *testing.T) {
		assertRendered(t, badgeText(t, FormatBadgeListMarkdown(
			[]BadgeItem{storedBadge},
			"Project Badges",
			toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
		)),
			"## Project Badges (1)\n\n"+
				"| ID | Name | Link URL | Image URL | Kind |\n"+
				"| --- | --- | --- | --- | --- |\n"+
				"| 1 | coverage | https://example.com/%{project_path} | https://img.shields.io/%{project_path} | project |\n"+
				"\nPage 1 of 1 | 1 items total | 20 per page\n"+
				badgeListHints)
	})
	t.Run("the heading counts what the response reports", func(t *testing.T) {
		assertRendered(t, badgeText(t, FormatBadgeListMarkdown(
			[]BadgeItem{storedBadge},
			"Group Badges",
			toolutil.PaginationOutput{TotalItems: 45, Page: 1, PerPage: 1, TotalPages: 45},
		)),
			"## Group Badges (45)\n\n"+
				"Showing 1 of 45 results (page 1 of 45)\n\n"+
				"| ID | Name | Link URL | Image URL | Kind |\n"+
				"| --- | --- | --- | --- | --- |\n"+
				"| 1 | coverage | https://example.com/%{project_path} | https://img.shields.io/%{project_path} | project |\n"+
				"\nPage 1 of 45 | 45 items total | 1 per page\n"+
				badgeListHints)
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, badgeText(t, FormatBadgeListMarkdown(nil, "Project Badges", toolutil.PaginationOutput{})),
			"No badges found.\n")
	})
}

// TestFormatBadgeMarkdown verifies the badge card, and that a badge GitLab sent
// no name for is headed by its ID rather than by "Badge: (ID: 1)".
func TestFormatBadgeMarkdown(t *testing.T) {
	t.Run("every field populated", func(t *testing.T) {
		assertRendered(t, badgeText(t, FormatBadgeMarkdown(storedBadge)),
			"## Badge: coverage (ID: 1)\n\n"+
				"- **Link URL**: https://example.com/%{project_path}\n"+
				"- **Image URL**: https://img.shields.io/%{project_path}\n"+
				"- **Rendered Link**: https://example.com/group/project\n"+
				"- **Rendered Image**: https://img.shields.io/group/project\n"+
				"- **Kind**: project\n"+
				badgeCardHints)
	})
	t.Run("optional rows are omitted", func(t *testing.T) {
		assertRendered(t, badgeText(t, FormatBadgeMarkdown(BadgeItem{ID: 1, Name: "test", LinkURL: "u", ImageURL: "i"})),
			"## Badge: test (ID: 1)\n\n"+
				"- **Link URL**: u\n"+
				"- **Image URL**: i\n"+
				badgeCardHints)
	})
	t.Run("no name falls back to the ID", func(t *testing.T) {
		assertRendered(t, badgeText(t, FormatBadgeMarkdown(BadgeItem{ID: 7, LinkURL: "u", ImageURL: "i"})),
			"## Badge #7\n\n"+
				"- **Link URL**: u\n"+
				"- **Image URL**: i\n"+
				badgeCardHints)
	})
}

// TestFormatBadgePreview verifies a preview renders as what it is: the two
// templates and what GitLab resolved them to, with no ID, no name, no kind and
// no offer to edit a badge that does not exist.
func TestFormatBadgePreview(t *testing.T) {
	want := "## Badge Preview\n\n" +
		"- **Link URL**: https://example.com/%{project_path}\n" +
		"- **Image URL**: https://img.shields.io/%{project_path}\n" +
		"- **Rendered Link**: https://example.com/group/project\n" +
		"- **Rendered Image**: https://img.shields.io/group/project\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'project.badge_add' to add the badge to a project once the rendered URLs look right\n" +
		"- Use action 'group.badge_add' to add it to a group instead\n"
	t.Run("project", func(t *testing.T) {
		assertRendered(t, badgeText(t, toolutil.MarkdownForResult(PreviewProjectOutput{Badge: previewBadge})), want)
	})
	t.Run("group", func(t *testing.T) {
		assertRendered(t, badgeText(t, toolutil.MarkdownForResult(PreviewGroupOutput{Badge: previewBadge})), want)
	})
}

// TestFormatBadgeNotFound_HostileResourceAndHints_ReachThePageAsText verifies
// the whole not-found card for a result whose label and hints carry a raw
// anchor. Both are words this package wrote, but both reach the formatter as
// fields of the result rather than as literals, so the sentence and the
// guidance lines show the tag as text and the card opens no link.
func TestFormatBadgeNotFound_HostileResourceAndHints_ReachThePageAsText(t *testing.T) {
	got := badgeText(t, formatBadgeNotFound(badgeNotFoundOutput{
		Resource:   "Project <a href=\"http://attacker.invalid\">Badge</a>",
		Identifier: "7",
		Hints:      []string{"<a href=\"http://attacker.invalid\">List</a> the badges", "Check the badge ID"},
	}))

	want := "## " + toolutil.EmojiQuestion + " Project &lt;a href=\"http://attacker.invalid\">Badge&lt;/a> Not Found\n\n" +
		"The project &lt;a href=\"http://attacker.invalid\">badge&lt;/a> **7** does not exist or is not accessible with your current permissions.\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- &lt;a href=\"http://attacker.invalid\">List&lt;/a> the badges\n" +
		"- Check the badge ID\n"
	assertRendered(t, got, want)
}

// TestMarkdownRegistry_BadgeOutputTypes verifies every badge output type is
// registered with the Markdown registry and routes to the expected formatter.
func TestMarkdownRegistry_BadgeOutputTypes(t *testing.T) {
	listProject := badgeText(t, FormatBadgeListMarkdown([]BadgeItem{storedBadge}, "Project Badges", toolutil.PaginationOutput{}))
	listGroup := badgeText(t, FormatBadgeListMarkdown([]BadgeItem{storedBadge}, "Group Badges", toolutil.PaginationOutput{}))
	card := badgeText(t, FormatBadgeMarkdown(storedBadge))
	preview := badgeText(t, formatBadgePreview(previewBadge))

	tests := []struct {
		name     string
		output   any
		rendered string
	}{
		{name: "list project output", output: ListProjectOutput{Badges: []BadgeItem{storedBadge}}, rendered: listProject},
		{name: "get project output", output: GetProjectOutput{Badge: storedBadge}, rendered: card},
		{name: "add project output", output: AddProjectOutput{Badge: storedBadge}, rendered: card},
		{name: "edit project output", output: EditProjectOutput{Badge: storedBadge}, rendered: card},
		{name: "preview project output", output: PreviewProjectOutput{Badge: previewBadge}, rendered: preview},
		{name: "list group output", output: ListGroupOutput{Badges: []BadgeItem{storedBadge}}, rendered: listGroup},
		{name: "get group output", output: GetGroupOutput{Badge: storedBadge}, rendered: card},
		{name: "add group output", output: AddGroupOutput{Badge: storedBadge}, rendered: card},
		{name: "edit group output", output: EditGroupOutput{Badge: storedBadge}, rendered: card},
		{name: "preview group output", output: PreviewGroupOutput{Badge: previewBadge}, rendered: preview},
		{name: "badge item", output: storedBadge, rendered: card},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertRendered(t, badgeText(t, toolutil.MarkdownForResult(tt.output)), tt.rendered)
		})
	}
}

// badgeText unwraps the single text block a registered formatter produces.
func badgeText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("expected non-nil markdown result")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected one content item, got %d", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return text.Text
}
