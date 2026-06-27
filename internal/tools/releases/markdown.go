package releases

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

type releaseNotFoundOutput struct {
	Identifier string
}

func formatReleaseNotFound(out releaseNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Release", out.Identifier,
		"Use gitlab_release_list with project_id to list releases",
		"Verify the tag_name is correct (case-sensitive)",
		"A tag may exist without a release - check with gitlab_tag_get",
	)
}

// releaseWebURL derives the release page URL from the _links.edit_url field by
// trimming the trailing "/edit" segment, or returns "" when no links exist.
func releaseWebURL(r Output) string {
	if r.Links == nil || r.Links.EditURL == "" {
		return ""
	}
	return strings.TrimSuffix(r.Links.EditURL, "/edit")
}

// milestoneTitles extracts the milestone titles from the nested milestone
// objects for compact Markdown rendering.
func milestoneTitles(ms []*toolutil.MilestoneOutput) []string {
	if len(ms) == 0 {
		return nil
	}
	titles := make([]string, 0, len(ms))
	for _, m := range ms {
		if m != nil && m.Title != "" {
			titles = append(titles, m.Title)
		}
	}
	return titles
}

// FormatMarkdown renders a single release as a Markdown summary.
func FormatMarkdown(r Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Release: %s\n\n", toolutil.EscapeMdHeading(r.Name))
	fmt.Fprintf(&b, "- **Tag**: %s\n", r.TagName)
	if r.Author != nil && r.Author.Username != "" {
		fmt.Fprintf(&b, toolutil.FmtMdAuthorAt, r.Author.Username)
	}
	if webURL := releaseWebURL(r); webURL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURL, webURL)
	}
	fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(r.CreatedAt))
	if r.ReleasedAt != "" {
		fmt.Fprintf(&b, "- **Released**: %s\n", toolutil.FormatTime(r.ReleasedAt))
	}
	if r.Commit != nil && r.Commit.ID != "" {
		fmt.Fprintf(&b, "- **Commit**: %s\n", r.Commit.ID)
	}
	if r.UpcomingRelease {
		b.WriteString("- " + toolutil.EmojiCalendar + " **Upcoming release**\n")
	}
	if titles := milestoneTitles(r.Milestones); len(titles) > 0 {
		fmt.Fprintf(&b, "- **Milestones**: %s\n", strings.Join(titles, ", "))
	}
	if r.Description != "" {
		fmt.Fprintf(&b, "\n### Description\n\n%s\n", toolutil.WrapGFMBody(r.Description))
	}
	toolutil.WriteHints(
		&b,
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
		if webURL := releaseWebURL(r); webURL != "" {
			tag = fmt.Sprintf("[%s](%s)", tag, webURL)
		}
		var author string
		if r.Author != nil {
			author = r.Author.Username
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", tag, toolutil.EscapeMdTableCell(r.Name), toolutil.EscapeMdTableCell(author), toolutil.FormatTime(released))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use action 'get' with a tag_name to see full release details",
		"Use action 'create' to create a new release",
		"Use gitlab_tag action 'list' to see available tags",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdownResult(formatReleaseNotFound)
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
