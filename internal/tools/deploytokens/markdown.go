// markdown.go provides Markdown formatting functions for deploy token MCP tool output.

package deploytokens

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown formats a single deploy token.
func FormatOutputMarkdown(o Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Deploy Token: %s (ID: %d)\n\n", o.Name, o.ID)
	fmt.Fprintf(&b, "| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| ID | %d |\n", o.ID)
	fmt.Fprintf(&b, "| Name | %s |\n", o.Name)
	fmt.Fprintf(&b, "| Username | %s |\n", o.Username)
	if o.Token != "" {
		fmt.Fprintf(&b, "| Token | %s |\n", o.Token)
	}
	fmt.Fprintf(&b, "| Scopes | %s |\n", strings.Join(o.Scopes, ", "))
	fmt.Fprintf(&b, "| Revoked | %t |\n", o.Revoked)
	fmt.Fprintf(&b, "| Expired | %t |\n", o.Expired)
	if o.ExpiresAt != "" {
		fmt.Fprintf(&b, "| Expires | %s |\n", toolutil.FormatTime(o.ExpiresAt))
	}
	toolutil.WriteHints(&b,
		"Use action 'delete' to revoke this deploy token",
	)
	return b.String()
}

// FormatListMarkdown formats a list of deploy tokens.
func FormatListMarkdown(o ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Deploy Tokens (%d)\n\n", len(o.DeployTokens))
	toolutil.WriteListSummary(&b, len(o.DeployTokens), o.Pagination)
	if len(o.DeployTokens) == 0 {
		b.WriteString("No deploy tokens found.\n")
		toolutil.WritePagination(&b, o.Pagination)
		return b.String()
	}
	b.WriteString("| ID | Name | Username | Scopes | Revoked | Expired |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, t := range o.DeployTokens {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %t | %t |\n",
			t.ID, t.Name, t.Username, strings.Join(t.Scopes, ", "), t.Revoked, t.Expired)
	}
	toolutil.WritePagination(&b, o.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'create' to generate a new deploy token",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
