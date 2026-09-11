package groupscim

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders a single SCIM identity as Markdown.
func FormatOutputMarkdown(o Output) string {
	if o.UserID == 0 && o.ExternUID == "" {
		return ""
	}
	var b strings.Builder
	c := toolutil.NewCard(&b, "SCIM Identity")
	// The external UID is whatever the identity provider sent for the user,
	// and a reader copies it back verbatim, so it is a code span rather than
	// an escaped cell: an entity renders literally inside a span.
	c.Code("External UID", o.ExternUID)
	c.Int("User ID", o.UserID)
	c.Bool("Active", o.Active)
	c.End(
		toolutil.HintAction("group_scim.update", "modify the external UID"),
		toolutil.HintAction("group_scim.delete", "remove this identity"),
	)
	return b.String()
}

// FormatListMarkdown renders a list of SCIM identities as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Identities) == 0 {
		return toolutil.EmptyMessage("SCIM identities")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "SCIM Identities", len(out.Identities), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("External UID", "User ID", "Active"))
	for _, id := range out.Identities {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdCodeSpanCell(id.ExternUID),
			strconv.FormatInt(id.UserID, 10),
			toolutil.BoolEmoji(id.Active),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction("group_scim.get", "view one identity in full"),
	)
	return b.String()
}

// FormatUpdateMarkdown renders the SCIM identity update confirmation as Markdown.
func FormatUpdateMarkdown(out UpdateOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "SCIM Identity Updated")
	c.Bool("Updated", out.Updated)
	c.Field("Message", out.Message)
	c.End(toolutil.HintAction("group_scim.get", "verify the new external UID"))
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown) // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)   // ListOutput
	toolutil.RegisterMarkdown(FormatUpdateMarkdown) // UpdateOutput
}
