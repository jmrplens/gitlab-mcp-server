// markdown.go provides Markdown formatting functions for group credential
// MCP tool output.
package groupcredentials

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatPATMarkdown formats a single personal access token as Markdown.
func FormatPATMarkdown(out PATOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Personal Access Token: %s (ID: %d)\n\n", out.Name, out.ID)
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| **User ID** | %d |\n", out.UserID)
	fmt.Fprintf(&sb, "| **State** | %s |\n", out.State)
	fmt.Fprintf(&sb, "| **Revoked** | %t |\n", out.Revoked)
	if len(out.Scopes) > 0 {
		fmt.Fprintf(&sb, "| **Scopes** | %s |\n", strings.Join(out.Scopes, ", "))
	}
	if out.ExpiresAt != "" {
		fmt.Fprintf(&sb, "| **Expires At** | %s |\n", out.ExpiresAt)
	}
	fmt.Fprintf(&sb, "| **Created At** | %s |\n", out.CreatedAt)
	if out.LastUsedAt != "" {
		fmt.Fprintf(&sb, "| **Last Used At** | %s |\n", out.LastUsedAt)
	}
	return sb.String()
}

// FormatPATListMarkdown formats a list of personal access tokens as Markdown.
func FormatPATListMarkdown(out PATListOutput) string {
	if len(out.Tokens) == 0 {
		return "No personal access tokens found."
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Personal Access Tokens (%d)\n\n", len(out.Tokens))
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("| ID | Name | User ID | State | Revoked | Scopes | Expires At |\n")
	sb.WriteString("|---|---|---|---|---|---|---|\n")
	for _, t := range out.Tokens {
		scopes := strings.Join(t.Scopes, ", ")
		fmt.Fprintf(&sb, "| %d | %s | %d | %s | %t | %s | %s |\n",
			t.ID, t.Name, t.UserID, t.State, t.Revoked, scopes, t.ExpiresAt)
	}
	sb.WriteString(toolutil.FormatPagination(out.Pagination))
	return sb.String()
}

// FormatSSHKeyMarkdown formats a single SSH key as Markdown.
func FormatSSHKeyMarkdown(out SSHKeyOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## SSH Key: %s (ID: %d)\n\n", out.Title, out.ID)
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| **User ID** | %d |\n", out.UserID)
	key := out.Key
	if len(key) > 60 {
		key = key[:57] + "..."
	}
	fmt.Fprintf(&sb, "| **Key** | `%s` |\n", key)
	fmt.Fprintf(&sb, "| **Created At** | %s |\n", out.CreatedAt)
	if out.ExpiresAt != "" {
		fmt.Fprintf(&sb, "| **Expires At** | %s |\n", out.ExpiresAt)
	}
	return sb.String()
}

// FormatSSHKeyListMarkdown formats a list of SSH keys as Markdown.
func FormatSSHKeyListMarkdown(out SSHKeyListOutput) string {
	if len(out.Keys) == 0 {
		return "No SSH keys found."
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## SSH Keys (%d)\n\n", len(out.Keys))
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("| ID | Title | User ID | Created At | Expires At |\n")
	sb.WriteString("|---|---|---|---|---|\n")
	for _, k := range out.Keys {
		fmt.Fprintf(&sb, "| %d | %s | %d | %s | %s |\n",
			k.ID, k.Title, k.UserID, k.CreatedAt, k.ExpiresAt)
	}
	sb.WriteString(toolutil.FormatPagination(out.Pagination))
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatPATMarkdown)
	toolutil.RegisterMarkdown(FormatPATListMarkdown)
	toolutil.RegisterMarkdown(FormatSSHKeyMarkdown)
	toolutil.RegisterMarkdown(FormatSSHKeyListMarkdown)
}
