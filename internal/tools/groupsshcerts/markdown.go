package groupsshcerts

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders one SSH CA certificate as a card. A certificate
// GitLab sent nothing for renders as nothing.
func FormatOutputMarkdown(o Output) string {
	if o.ID == 0 {
		return ""
	}
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("SSH Certificate #%d", o.ID))
	c.Int("ID", o.ID)
	// The title is the name whoever added the certificate gave it, and the key
	// is truncated here rather than constrained.
	c.Field("Title", o.Title)
	c.Code("Key", truncateKey(o.Key))
	c.Time("Created", o.CreatedAt)
	c.End(
		toolutil.HintAction(actionDelete, "revoke this certificate"),
		toolutil.HintAction(actionList, "see the group's other certificates"),
	)
	return b.String()
}

// FormatListMarkdown renders one page of a group's SSH CA certificates as a
// Markdown table, with the pagination GitLab sent so a reader can tell the
// page from the whole.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Certificates) == 0 {
		return toolutil.EmptyMessage("SSH certificates")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "SSH Certificates", len(out.Certificates), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Title", "Created"))
	for _, c := range out.Certificates {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(c.ID, 10),
			toolutil.EscapeMdTableCell(c.Title),
			toolutil.FormatTime(c.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionCreate, "add another certificate"),
	)
	return b.String()
}

// truncateKey shortens a CA public key to the head a reader recognizes it by.
// The whole key is in the JSON result; the card shows enough to tell two apart.
func truncateKey(key string) string {
	if len(key) > 60 {
		return key[:57] + "..."
	}
	return key
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown) // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)   // ListOutput
}
