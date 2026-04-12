// markdown.go provides Markdown formatting functions for release MCP tool output.

package releases

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatMarkdown renders a single release as a Markdown summary.
func FormatMarkdown(r Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Release: %s\n\n", toolutil.EscapeMdHeading(r.Name))
	fmt.Fprintf(&b, "- **Tag**: %s\n", r.TagName)
	if r.Author != "" {
		fmt.Fprintf(&b, toolutil.FmtMdAuthorAt, r.Author)
	}
	if r.WebURL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURL, r.WebURL)
	}
	fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(r.CreatedAt))
	if r.ReleasedAt != "" {
		fmt.Fprintf(&b, "- **Released**: %s\n", toolutil.FormatTime(r.ReleasedAt))
	}
	if r.CommitSHA != "" {
		fmt.Fprintf(&b, "- **Commit**: %s\n", r.CommitSHA)
	}
	if r.UpcomingRelease {
		b.WriteString("- " + toolutil.EmojiCalendar + " **Upcoming release**\n")
	}
	if len(r.Milestones) > 0 {
		fmt.Fprintf(&b, "- **Milestones**: %s\n", strings.Join(r.Milestones, ", "))
	}
	if r.Description != "" {
		fmt.Fprintf(&b, "\n### Description\n\n%s\n", toolutil.WrapGFMBody(r.Description))
	}
	toolutil.WriteHints(&b,
		"Use gitlab_release_link action 'link_list' to see release assets",
		"Use gitlab_release_link action 'link_create' to add a single asset link",
		"Use gitlab_release_link action 'link_create_batch' to add multiple asset links in one call",
		"Use gitlab_package action 'publish_and_link' to upload and link binaries to this release",
		"Use gitlab_release action 'update' to edit the release description",
	)
	return b.String()
}

// FormatListMarkdown renders a list of releases as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Releases (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Releases), out.Pagination)
	if len(out.Releases) == 0 {
		b.WriteString("No releases found.\n")
		return b.String()
	}
	b.WriteString("| Tag | Name | Author | Released |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, r := range out.Releases {
		released := r.ReleasedAt
		if released == "" {
			released = r.CreatedAt
		}
		tag := toolutil.EscapeMdTableCell(r.TagName)
		if r.WebURL != "" {
			tag = fmt.Sprintf("[%s](%s)", tag, r.WebURL)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", tag, toolutil.EscapeMdTableCell(r.Name), toolutil.EscapeMdTableCell(r.Author), toolutil.FormatTime(released))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'get' with a tag_name to see full release details",
		"Use action 'create' to create a new release",
		"Use gitlab_tag action 'list' to see available tags",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
