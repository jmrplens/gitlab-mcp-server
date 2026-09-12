package groupreleases

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionGroupReleaseList = "group.release_list"
	actionReleaseGet       = "release.get"
	actionReleaseLinkList  = "release.link_list"
)

// releaseWebURL derives the release page URL from the _links object, preferring
// the self link and falling back to the edit_url with its trailing "/edit"
// segment trimmed. Returns "" when no links are present.
func releaseWebURL(r Output) string {
	if r.Links == nil {
		return ""
	}
	if r.Links.Self != "" {
		return r.Links.Self
	}
	if r.Links.EditURL != "" {
		return strings.TrimSuffix(r.Links.EditURL, "/edit")
	}
	return ""
}

// releaseAuthor renders the author as the handle linked to their profile, the
// way the project-scoped release list renders one, and nothing when no author
// is associated with the release.
func releaseAuthor(r Output) string {
	if r.Author == nil {
		return ""
	}
	return toolutil.MdUserLink(r.Author.Username, r.Author.WebURL)
}

// releasedCell renders the date a release went out, falling back to when it was
// created, and marks a release dated in the future with the calendar glyph
// GitLab's own upcoming_release flag stands for.
func releasedCell(r Output) string {
	released := r.ReleasedAt
	if released == "" {
		released = r.CreatedAt
	}
	cell := toolutil.FormatTime(released)
	if r.UpcomingRelease && cell != "" {
		return toolutil.EmojiCalendar + " " + cell
	}
	return cell
}

// FormatListMarkdown renders a page of a group's releases as a Markdown table:
// a collection of objects that share columns, under the heading the response
// can vouch for and above the pagination footer.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Releases) == 0 {
		return toolutil.EmptyMessage("group releases")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Releases", len(out.Releases), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Tag", "Name", "Released", "Author"))
	for _, r := range out.Releases {
		// A tag name may hold ']' and both angle brackets, which the cell
		// escaper leaves alone, so the label of a hand-built link could be
		// closed from inside it.
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(r.TagName, releaseWebURL(r)),
			toolutil.EscapeMdTableCell(r.Name),
			releasedCell(r),
			releaseAuthor(r),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionReleaseGet, "read one release in full, with its notes and assets"),
		toolutil.HintAction(actionReleaseLinkList, "list the asset links of a release"),
		toolutil.HintAction(actionGroupReleaseList, "page through the rest of the group's releases"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
