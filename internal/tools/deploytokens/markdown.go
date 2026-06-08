package deploytokens

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatOutputMarkdown formats a single deploy token.
func FormatOutputMarkdown(o Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Deploy Token: %s (ID: %d)\n\n", o.Name, o.ID)
	b.WriteString(toolutil.TblFieldValue)
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
	toolutil.WriteHints(
		&b,
		"Use the selected tool surface's deploy-token get action with the matching scope (project or group) and deploy_token_id to fetch this deploy token before changing it",
		"Use the selected tool surface's deploy-token delete action with the matching scope (project or group), this deploy_token_id, and explicit confirm=true to revoke this deploy token",
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
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Username", "Scopes", "Revoked", "Expired"))
	for _, t := range o.DeployTokens {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %t | %t |\n",
			t.ID, t.Name, t.Username, strings.Join(t.Scopes, ", "), t.Revoked, t.Expired)
	}
	toolutil.WritePagination(&b, o.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use the selected tool surface's deploy-token get action with the matching scope (project or group) and deploy_token_id for full details",
		"Use the selected tool surface's deploy-token create action with the matching scope (project or group) to generate a new deploy token",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
