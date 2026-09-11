package releases

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The canonical catalog IDs the hints name that the specs do not already
// spell. The asset-link actions are projected under the release domain, so the
// ID every surface resolves is "release.link_list" and not the
// "release_link.list" the cross-link constant in action_specs.go spells.
const (
	hintActionReleaseLinkList   = "release.link_list"
	actionReleaseCreate         = "release.create"
	actionReleaseUpdate         = "release.update"
	actionReleaseLinkCreate     = "release.link_create"
	actionReleaseLinkCreateBulk = "release.link_create_batch"
	actionPackagePublishAndLink = "package.publish_and_link"
	actionTagList               = "tag.list"
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

// releaseWebURL derives the release page URL from the _links object, preferring
// the self link GitLab sends for the page itself and falling back to the
// edit_url with its trailing "/edit" segment trimmed. Returns "" when no links
// are present.
func releaseWebURL(r Output) string {
	if r.Links == nil {
		return ""
	}
	if r.Links.Self != "" {
		return r.Links.Self
	}
	if r.Links.EditURL == "" {
		return ""
	}
	return strings.TrimSuffix(r.Links.EditURL, "/edit")
}

// milestoneTitles extracts the milestone titles from the nested milestone
// objects, each escaped on its own: joining them first and escaping the join
// let a title carrying a pipe split the row it landed in, since the escaper
// cannot tell the separator this function wrote from one a title holds.
func milestoneTitles(ms []*toolutil.MilestoneOutput) []string {
	if len(ms) == 0 {
		return nil
	}
	titles := make([]string, 0, len(ms))
	for _, m := range ms {
		if m != nil && m.Title != "" {
			titles = append(titles, toolutil.EscapeMdTableCell(m.Title))
		}
	}
	return titles
}

// releaseHeading names the release the way a reader asked for it: its title
// when GitLab has one, and the tag it was cut from otherwise.
func releaseHeading(r Output) string {
	if strings.TrimSpace(r.Name) != "" {
		return "Release: " + r.Name
	}
	return "Release: " + r.TagName
}

// releasedCell renders the date a release went out, falling back to when it was
// created, and marks a release GitLab flagged as upcoming with the calendar
// glyph: without it a date in the future read as one already past.
func releasedCell(released, created string, upcoming bool) string {
	if released == "" {
		released = created
	}
	cell := toolutil.FormatTime(released)
	if upcoming && cell != "" {
		return toolutil.EmojiCalendar + " " + cell
	}
	return cell
}

// commitSHA is the commit a release points at, in the short form GitLab sends
// beside the full one and in the full form when it sent no short one.
func commitSHA(c toolutil.CommitOutput) string {
	if c.ShortID != "" {
		return c.ShortID
	}
	return c.ID
}

// FormatMarkdown renders one release as the card of a single object: its own
// fields, then its notes as quoted prose under their label.
func FormatMarkdown(r Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, releaseHeading(r))
	// A tag name is a git ref, and check-ref-format permits '|', '<' and '>'.
	c.Field("Tag", r.TagName)
	if r.Author != nil {
		c.Markdown("Author", toolutil.MdUserLink(r.Author.Username, r.Author.WebURL))
	}
	c.Time("Created", r.CreatedAt)
	c.Markdown("Released", releasedCell(r.ReleasedAt, "", r.UpcomingRelease))
	c.Flag(toolutil.EmojiCalendar, "Upcoming release", r.UpcomingRelease)
	if r.Commit != nil {
		// A commit SHA, hexadecimal by construction, shown the short way GitLab
		// shows it when it sent one and in full when it did not.
		c.Code("Commit", commitSHA(*r.Commit))
		c.Field("Commit Title", r.Commit.Title)
	}
	// A milestone title is free text, escaped one title at a time.
	c.Markdown("Milestones", strings.Join(milestoneTitles(r.Milestones), ", "))
	if r.Assets != nil {
		c.Count("Assets", r.Assets.Count)
	}
	c.URL(releaseWebURL(r))
	c.Text("Description", r.Description)
	c.End(
		toolutil.HintAction(hintActionReleaseLinkList, "see the assets linked to this release"),
		toolutil.HintAction(actionReleaseLinkCreate, "add a single asset link"),
		toolutil.HintAction(actionReleaseLinkCreateBulk, "add several asset links in one call"),
		toolutil.HintAction(actionPackagePublishAndLink, "upload a binary and link it to this release"),
		toolutil.HintAction(actionReleaseUpdate, "edit the release notes"),
	)
	return b.String()
}

// FormatListMarkdown renders a page of a project's releases as a Markdown
// table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Releases) == 0 {
		return toolutil.EmptyMessage("releases")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Releases", len(out.Releases), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Tag", "Name", "Author", "Released"))
	for _, r := range out.Releases {
		var author string
		if r.Author != nil {
			author = toolutil.MdUserLink(r.Author.Username, r.Author.WebURL)
		}
		// The cell escaper leaves ']' alone, so a hand-built link's label could
		// be closed from inside it by a tag holding one.
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(r.TagName, releaseWebURL(r)),
			toolutil.EscapeMdTableCell(r.Name),
			author,
			releasedCell(r.ReleasedAt, r.CreatedAt, r.UpcomingRelease),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionReleaseGet, "see one release in full, with its notes and assets"),
		toolutil.HintAction(actionReleaseCreate, "create a new release"),
		toolutil.HintAction(actionTagList, "see the tags a release can be cut from"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdownResult(formatReleaseNotFound)
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
