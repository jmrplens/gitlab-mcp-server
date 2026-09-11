package invites

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// accessLevel renders an invitation's numeric access level as the name GitLab
// gives it with the number beside it, "Developer (30)". The invitation list
// used to print the bare integer, which is the one thing a reader cannot act
// on without a lookup table.
func accessLevel(level int) string {
	return fmt.Sprintf("%s (%d)", toolutil.AccessLevelDescription(gl.AccessLevelValue(level)), level)
}

// FormatListPendingMarkdown formats pending invitations as a Markdown CallToolResult.
func FormatListPendingMarkdown(out ListPendingInvitationsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListPendingMarkdownString(out))
}

// FormatListPendingMarkdownString renders pending invitations as a Markdown
// string.
//
// The invitation token is deliberately absent from every column: it is carried
// in the JSON, where a caller who asked for it finds it, and a rendered table
// would paste a live credential into the conversation.
func FormatListPendingMarkdownString(out ListPendingInvitationsOutput) string {
	if len(out.Invitations) == 0 {
		return toolutil.EmptyMessage("pending invitations")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Pending Invitations", len(out.Invitations), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Email", "Access Level", "User", "Invited By", "Created", "Expires"))
	for _, inv := range out.Invitations {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(inv.InviteEmail),
			accessLevel(inv.AccessLevel),
			toolutil.EscapeMdTableCell(inv.UserName),
			toolutil.EscapeMdTableCell(inv.CreatedByName),
			toolutil.FormatTime(inv.CreatedAt),
			toolutil.FormatTime(inv.ExpiresAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"Manage pending invitations by approving, revoking, or resending them",
	)
	return b.String()
}

// FormatInviteResultMarkdown formats an invitation result as a Markdown CallToolResult.
func FormatInviteResultMarkdown(out InviteResultOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatInviteResultMarkdownString(out))
}

// FormatInviteResultMarkdownString renders an invitation result as a Markdown string.
func FormatInviteResultMarkdownString(out InviteResultOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Invitation Result")
	c.Field("Status", out.Status)
	// GitLab keys these messages by the address that was invited and answers
	// with its own text about it.
	writeKeyedTable(c, "Messages", "Invitee", out.Message)
	// Keyed by username, against GitLab's own reason for queueing it, which is
	// what member promotion management answers with instead of inviting.
	writeKeyedTable(c, "Queued for Administrator Approval", "User", out.QueuedUsers)
	c.End("Check invitation status or resend if the invite was not received")
	return b.String()
}

// writeKeyedTable writes one of GitLab's keyed message maps as a nested
// collection of the card, rows in key order.
//
// The order is sorted rather than the map's own because Go randomizes map
// iteration: the same answer rendered two different documents on two identical
// calls, which is a diff a reader cannot tell from a change on the instance.
func writeKeyedTable(c *toolutil.Card, title, keyColumn string, entries map[string]string) {
	if len(entries) == 0 {
		return
	}
	t := c.Table(title, keyColumn, "Message")
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		t.Row(toolutil.EscapeMdTableCell(key), toolutil.EscapeMdTableCell(entries[key]))
	}
}

func init() {
	toolutil.RegisterMarkdown(FormatListPendingMarkdownString)
	toolutil.RegisterMarkdown(FormatInviteResultMarkdownString)
}
