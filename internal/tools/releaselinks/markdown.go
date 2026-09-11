package releaselinks

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The canonical catalog IDs the hints name that the specs do not already
// spell. The asset-link actions are projected under the release domain, so the
// ID every surface resolves is "release.link_list" and not the
// "release_link.list" the cross-link constant in action_specs.go spells.
const (
	hintActionLinkList        = "release.link_list"
	hintActionLinkGet         = "release.link_get"
	hintActionLinkUpdate      = "release.link_update"
	hintActionLinkDelete      = "release.link_delete"
	hintActionLinkCreate      = "release.link_create"
	hintActionLinkCreateBatch = "release.link_create_batch"
)

// linkColumns are the columns every collection of asset links shares, so the
// batch result and the listing cannot drift apart.
var linkColumns = []string{"ID", "Name", "Type", "URL"}

// linkRow renders one asset link as a row of that collection. The URL column
// is labeled with the URL rather than with the link's name, which the Name
// column beside it already carries: a row whose last two cells read alike says
// nothing about where the asset is.
func linkRow(l Output) string {
	return toolutil.MarkdownTableRow(
		strconv.FormatInt(l.ID, 10),
		toolutil.EscapeMdTableCell(l.Name),
		toolutil.EscapeMdTableCell(l.LinkType),
		toolutil.MdTitleLink(l.URL, l.URL),
	)
}

// FormatOutputMarkdown renders one release asset link as the card of a single
// object.
func FormatOutputMarkdown(l Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Release Link: "+l.Name)
	c.Int("ID", l.ID)
	// A release link type GitLab picks from a fixed set (other, runbook,
	// image, package).
	c.Field("Type", l.LinkType)
	c.URL(l.URL)
	c.Link("Direct Asset URL", l.DirectAssetURL, l.DirectAssetURL)
	c.End(
		toolutil.HintAction(hintActionLinkUpdate, "change this link's name, URL or type"),
		toolutil.HintAction(hintActionLinkDelete, "remove this link from the release"),
	)
	return b.String()
}

// FormatDeletedMarkdown renders the link GitLab removed. It is a card of its
// own rather than the one above because the object it describes no longer
// exists: rendered as an ordinary link card, a deletion answered with the
// link's own heading and offered the reader an update and a delete of
// something that had just gone.
func FormatDeletedMarkdown(l DeletedOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Release Link Deleted: "+l.Name)
	c.Int("ID", l.ID)
	c.Field("Type", l.LinkType)
	c.URL(l.URL)
	c.Note("The link is removed from the release. The file or package it pointed at is untouched.")
	c.End(
		toolutil.HintAction(hintActionLinkList, "see the links the release still has"),
		toolutil.HintAction(hintActionLinkCreate, "link another asset to the release"),
	)
	return b.String()
}

// FormatBatchMarkdown renders the result of a batch link creation: the links
// created as a collection, then the ones GitLab refused.
func FormatBatchMarkdown(out CreateBatchOutput) string {
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Release Links Created", len(out.Created), toolutil.PaginationOutput{})
	if len(out.Created) > 0 {
		b.WriteString(toolutil.MarkdownTableHeader(linkColumns...))
		for _, l := range out.Created {
			b.WriteString(linkRow(l))
		}
	}
	if len(out.Failed) > 0 {
		// The blank line is what separates the section from the table above it:
		// a heading written straight after a row ends that row's table where
		// the reader cannot see why.
		fmt.Fprintf(&b, "\n### Failures (%d)\n\n", len(out.Failed))
		for _, f := range out.Failed {
			// Each failure carries the link's own name and GitLab's message.
			fmt.Fprintf(&b, "- %s\n", toolutil.EscapeMdTableCell(f))
		}
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, len(out.Created) > 0,
		toolutil.HintPreserveLinks,
		toolutil.HintAction(hintActionLinkList, "see every link the release now has"),
	)
	return b.String()
}

// FormatListMarkdown renders a release's asset links as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Links) == 0 {
		return toolutil.EmptyMessage("release links")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Release Links", len(out.Links), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader(linkColumns...))
	for _, l := range out.Links {
		b.WriteString(linkRow(l))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(hintActionLinkGet, "see one link on its own"),
		toolutil.HintAction(hintActionLinkCreate, "add a new release asset link"),
		toolutil.HintAction(hintActionLinkCreateBatch, "add several asset links in one call"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatDeletedMarkdown)
	toolutil.RegisterMarkdown(FormatBatchMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
