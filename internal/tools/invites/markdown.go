package invites

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatListPendingMarkdown formats pending invitations as a Markdown CallToolResult.
func FormatListPendingMarkdown(out ListPendingInvitationsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListPendingMarkdownString(out))
}

// FormatListPendingMarkdownString renders pending invitations as a Markdown string.
func FormatListPendingMarkdownString(out ListPendingInvitationsOutput) string {
	if len(out.Invitations) == 0 {
		return "No pending invitations found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Pending Invitations (%d)\n\n", len(out.Invitations))
	toolutil.WriteListSummary(&b, len(out.Invitations), out.Pagination)
	for _, inv := range out.Invitations {
		fmt.Fprintf(&b, "- **%s** (ID: %d) — Access Level: %d", inv.InviteEmail, inv.ID, inv.AccessLevel)
		if inv.UserName != "" {
			fmt.Fprintf(&b, ", User: %s", inv.UserName)
		}
		if inv.ExpiresAt != "" {
			fmt.Fprintf(&b, ", Expires: %s", toolutil.FormatTime(inv.ExpiresAt))
		}
		b.WriteString("\n")
	}
	b.WriteString(toolutil.FormatPagination(out.Pagination))
	toolutil.WriteHints(&b, "Manage pending invitations by approving, revoking, or resending them")
	return b.String()
}

// FormatInviteResultMarkdown formats an invitation result as a Markdown CallToolResult.
func FormatInviteResultMarkdown(out InviteResultOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatInviteResultMarkdownString(out))
}

// FormatInviteResultMarkdownString renders an invitation result as a Markdown string.
func FormatInviteResultMarkdownString(out InviteResultOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Invitation Result\n\n**Status**: %s\n", out.Status)
	if len(out.Message) > 0 {
		b.WriteString("\n**Messages**:\n")
		for k, v := range out.Message {
			fmt.Fprintf(&b, "- %s: %s\n", k, v)
		}
	}
	toolutil.WriteHints(&b, "Check invitation status or resend if the invite was not received")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListPendingMarkdownString)
	toolutil.RegisterMarkdown(FormatInviteResultMarkdownString)
}
