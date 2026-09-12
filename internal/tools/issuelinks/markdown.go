package issuelinks

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders a single issue link as the card of one object:
// the link's own id and type, then the two issues it joins as nested objects,
// each with the reference GitLab renders it by, its title linked to the issue
// and the confidential marker when it carries one.
func FormatOutputMarkdown(v Output) string {
	if v.ID == 0 {
		return ""
	}
	var b strings.Builder
	c := toolutil.NewCard(&b, "Issue Link")
	c.Int("ID", int64(v.ID))
	c.Field("Link Type", v.LinkType)
	writeIssueRef(c, "Source Issue", v.SourceIssue)
	writeIssueRef(c, "Target Issue", v.TargetIssue)
	c.End(toolutil.HintAction(actionLinkList, "see all links for this issue"))
	return b.String()
}

// writeIssueRef writes one side of the link as a nested object, or a row
// saying the object was not sent: a link whose issue the response omitted is
// an answer, and a silent row would read as a link to nothing.
func writeIssueRef(c *toolutil.Card, label string, ref *IssueRefOutput) {
	if ref == nil {
		c.Field(label, "(not available)")
		return
	}
	side := c.Sub(label)
	side.Int("IID", ref.IID)
	side.Int("Project ID", ref.ProjectID)
	side.Link("Title", ref.Title, ref.WebURL)
	side.Markdown("State", issueStateCell(ref.State))
	side.Flag(toolutil.EmojiConfidential, "Confidential", ref.Confidential)
}

// FormatListMarkdown renders the issues related to one issue as a Markdown
// table: a collection of objects that share columns.
//
// The row carries the link ID because it is what the unlink action takes, the
// state with the emoji every issue row in the tree shows, and the confidential
// marker, without which a reader cannot tell a restricted issue from an open
// one at a glance.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Relations) == 0 {
		return toolutil.EmptyMessage("linked issues")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Issue Relations", len(out.Relations), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "IID", "Title", "State", "Link Type", "Link ID", "Author"))
	for _, r := range out.Relations {
		author := ""
		if r.Author != nil {
			author = r.Author.Username
		}
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.Itoa(r.ID),
			strconv.Itoa(r.IID),
			relationTitleCell(r),
			issueStateCell(r.State),
			toolutil.EscapeMdTableCell(r.LinkType),
			strconv.Itoa(r.IssueLinkID),
			toolutil.MdUserHandle(author),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, true,
		toolutil.HintPreserveLinks,
		toolutil.HintAction(actionLinkCreate, "add a new link between issues"),
	)
	return b.String()
}

// relationTitleCell renders the linked issue's title as a link to it, with the
// confidential marker appended when the issue is restricted.
func relationTitleCell(r RelationOutput) string {
	cell := toolutil.MdTitleLink(r.Title, r.WebURL)
	if r.Confidential {
		cell += " " + toolutil.EmojiConfidential
	}
	return cell
}

// issueStateCell renders an issue state with the emoji the issue tables and
// cards share, and nothing at all when GitLab sent no state.
func issueStateCell(state string) string {
	if strings.TrimSpace(state) == "" {
		return ""
	}
	return fmt.Sprintf("%s %s", toolutil.IssueStateEmoji(state), toolutil.EscapeMdTableCell(state))
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
