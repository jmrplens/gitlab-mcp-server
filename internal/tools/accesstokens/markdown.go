package accesstokens

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders an access token as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Access Token #%d\n\n", out.ID)
	fmt.Fprintf(&b, toolutil.FmtMdName, out.Name)
	if out.Description != "" {
		fmt.Fprintf(&b, toolutil.FmtMdDescription, out.Description)
	}
	fmt.Fprintf(&b, "- **Active**: %t\n", out.Active)
	fmt.Fprintf(&b, "- **Revoked**: %t\n", out.Revoked)
	if len(out.Scopes) > 0 {
		fmt.Fprintf(&b, "- **Scopes**: %s\n", strings.Join(out.Scopes, ", "))
	}
	if out.AccessLevel > 0 {
		fmt.Fprintf(&b, "- **Access Level**: %s\n", accessLevelName(out.AccessLevel))
	}
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(out.CreatedAt))
	}
	if out.ExpiresAt != "" {
		fmt.Fprintf(&b, "- **Expires**: %s\n", toolutil.FormatTime(out.ExpiresAt))
	}
	if out.Token != "" {
		fmt.Fprintf(&b, "- **Token**: `%s`\n", out.Token)
	}
	toolutil.WriteHints(&b,
		"Use action 'revoke' to revoke this token",
		"Use action 'rotate' to rotate this token",
	)
	return b.String()
}

// FormatListMarkdown renders a list of access tokens as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Tokens) == 0 {
		return "No access tokens found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Access Tokens (%d)\n\n", len(out.Tokens))
	toolutil.WriteListSummary(&b, len(out.Tokens), out.Pagination)
	b.WriteString("| ID | Name | Active | Scopes | Expires |\n")
	b.WriteString("|---:|------|--------|--------|--------|\n")
	for _, t := range out.Tokens {
		scopes := strings.Join(t.Scopes, ", ")
		expires := toolutil.FormatTime(t.ExpiresAt)
		if t.ExpiresAt == "" {
			expires = "never"
		}
		fmt.Fprintf(&b, "| %d | %s | %t | %s | %s |\n",
			t.ID, toolutil.EscapeMdTableCell(t.Name), t.Active,
			toolutil.EscapeMdTableCell(scopes), expires)
	}
	b.WriteString("\n")
	b.WriteString(toolutil.FormatPagination(out.Pagination))
	toolutil.WriteHints(&b,
		"Use action 'get' with token_id for full details",
		"Use action 'create' to generate a new access token",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
