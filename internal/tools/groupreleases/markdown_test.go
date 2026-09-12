// markdown_test.go contains unit tests for group release Markdown formatting functions.
package groupreleases

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// listHints is the guidance section every group release listing closes with,
// the preserve-links instruction first because the Tag column carries links.
const listHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- " + toolutil.HintPreserveLinks + "\n" +
	"- Use action 'release.get' to read one release in full, with its notes and assets\n" +
	"- Use action 'release.link_list' to list the asset links of a release\n" +
	"- Use action 'group.release_list' to page through the rest of the group's releases\n"

const listHeader = "| Tag | Name | Released | Author |\n| --- | --- | --- | --- |\n"

// TestFormatListMarkdown_Empty pins that a group with no releases renders the
// one sentence and nothing else: no heading counting zero above it.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{})

	if want := "No group releases found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_SinglePage pins the whole document of a one-page
// listing: the heading counting what GitLab said, the table, the pagination
// line separated from the last row, and the hints last.
func TestFormatListMarkdown_SinglePage(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Releases: []Output{{
			TagName:    "v1.0.0",
			Name:       "First Release",
			ReleasedAt: "2026-06-01T00:00:00Z",
			Author:     &toolutil.AuthorOutput{Username: "admin"},
			Links:      &toolutil.LinksOutput{Self: "https://git.example.com/g/p/-/releases/v1.0.0"},
		}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, TotalPages: 1},
	})

	want := "## Group Releases (1)\n\n" +
		listHeader +
		"| [v1.0.0](https://git.example.com/g/p/-/releases/v1.0.0) | First Release | 1 Jun 2026 00:00 UTC | @admin |\n" +
		"\n1 items total\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_MultiPage pins the heading, the summary line and the
// pagination footer of a page that is one of several: the heading counts the
// total GitLab reported rather than the rows shown.
func TestFormatListMarkdown_MultiPage(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Releases: []Output{
			{TagName: "v2.0.0", Name: "Second", ReleasedAt: "2026-07-01T10:00:00Z", Author: &toolutil.AuthorOutput{Username: "dev1"}},
			{TagName: "v1.0.0", Name: "First", ReleasedAt: "2026-06-01T09:00:00Z", Author: &toolutil.AuthorOutput{Username: "dev2"}},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 3, TotalItems: 45, PerPage: 20},
	})

	want := "## Group Releases (45)\n\n" +
		"Showing 2 of 45 results (page 1 of 3)\n\n" +
		listHeader +
		"| v2.0.0 | Second | 1 Jul 2026 10:00 UTC | @dev1 |\n" +
		"| v1.0.0 | First | 1 Jun 2026 09:00 UTC | @dev2 |\n" +
		"\nPage 1 of 3 | 45 items total | 20 per page\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_UpcomingAndCreatedFallback pins the two things the
// Released column answers besides a release date: a release GitLab marked
// upcoming carries the calendar glyph, and one with no released_at falls back
// to when it was created rather than rendering an empty cell.
func TestFormatListMarkdown_UpcomingAndCreatedFallback(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Releases: []Output{
			{TagName: "v9.0.0", Name: "Planned", ReleasedAt: "2027-01-01T00:00:00Z", UpcomingRelease: true},
			{TagName: "v0.1.0", Name: "Early", CreatedAt: "2026-01-02T03:04:00Z"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, TotalPages: 1},
	})

	want := "## Group Releases (2)\n\n" +
		listHeader +
		"| v9.0.0 | Planned | \U0001F4C5 1 Jan 2027 00:00 UTC |  |\n" +
		"| v0.1.0 | Early | 2 Jan 2026 03:04 UTC |  |\n" +
		"\n2 items total\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_HostileTagAndEditURLFallback pins both halves of the
// Tag column: a tag name carrying a pipe and a bracket cannot split the row or
// close a link label, and a release whose only link is the edit URL is linked
// to the page that URL is the edit form of.
func TestFormatListMarkdown_HostileTagAndEditURLFallback(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Releases: []Output{
			{TagName: "v1|beta](http://attacker.invalid/)", Name: "Rel|ease"},
			{TagName: "v5.0.0", Name: "EditFallback", Links: &toolutil.LinksOutput{EditURL: "https://git.example.com/g/p/-/releases/v5.0.0/edit"}},
			{TagName: "v3.9.0", Name: "NoURL", Links: &toolutil.LinksOutput{ClosedIssuesURL: "https://ci.example.com"}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 3, TotalPages: 1},
	})

	want := "## Group Releases (3)\n\n" +
		listHeader +
		"| v1&#124;beta](http://attacker.invalid/) | Rel&#124;ease |  |  |\n" +
		"| [v5.0.0](https://git.example.com/g/p/-/releases/v5.0.0) | EditFallback |  |  |\n" +
		"| v3.9.0 | NoURL |  |  |\n" +
		"\n3 items total\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
