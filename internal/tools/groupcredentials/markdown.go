package groupcredentials

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// The three timestamps below are written by this package's own converters with
// a constant layout, so nothing a person typed survives into them. Both the
// token and the SSH key formatter name their parameter out, and one
// declaration covers a package, so each is declared once here rather than six
// times below.
//
//gitlab:allow-unescaped out.CreatedAt: a timestamp toPATOutput or toSSHKeyOutput rendered with the constant toolutil.DateTimeFormat layout.
//gitlab:allow-unescaped out.LastUsedAt: a timestamp toPATOutput or toSSHKeyOutput rendered with the constant toolutil.DateTimeFormat layout.
//gitlab:allow-unescaped out.ExpiresAt: a date rendered with a constant layout, gl.ISOTime.String for a token and toolutil.DateTimeFormat for an SSH key.

// FormatPATMarkdown formats a single personal access token as Markdown.
func FormatPATMarkdown(out PATOutput) string {
	var sb strings.Builder
	// A token name is free text whoever created the token typed.
	fmt.Fprintf(&sb, "## Personal Access Token: %s (ID: %d)\n\n", toolutil.EscapeMdHeading(out.Name), out.ID)
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| **User ID** | %d |\n", out.UserID)
	fmt.Fprintf(&sb, "| **Active** | %t |\n", out.Active)
	fmt.Fprintf(&sb, "| **Revoked** | %t |\n", out.Revoked)
	if len(out.Scopes) > 0 {
		//gitlab:allow-unescaped strings.Join(out.Scopes, ", "): token scopes, which GitLab refuses to store outside its own fixed set.
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
	sb.WriteString("| ID | Name | User ID | Active | Revoked | Scopes | Expires At |\n")
	sb.WriteString("|---|---|---|---|---|---|---|\n")
	for _, t := range out.Tokens {
		scopes := strings.Join(t.Scopes, ", ")
		fmt.Fprintf(&sb, "| %d | %s | %d | %t | %t | %s | %s |\n",
			//gitlab:allow-unescaped scopes: the same token scopes, joined for one row of the list.
			//gitlab:allow-unescaped t.ExpiresAt: a date gl.ISOTime rendered as YYYY-MM-DD.
			t.ID, toolutil.EscapeMdTableCell(t.Name), t.UserID, t.Active, t.Revoked, scopes, t.ExpiresAt)
	}
	sb.WriteString(toolutil.FormatPagination(out.Pagination))
	return sb.String()
}

// FormatSSHKeyMarkdown formats a single SSH key as Markdown.
func FormatSSHKeyMarkdown(out SSHKeyOutput) string {
	var sb strings.Builder
	// An SSH key title is the name its owner gave it, with GitLab falling back
	// to the key's own comment field, which the owner also wrote.
	fmt.Fprintf(&sb, "## SSH Key: %s (ID: %d)\n\n", toolutil.EscapeMdHeading(out.Title), out.ID)
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| **User ID** | %d |\n", out.UserID)
	if out.UsageType != "" {
		//gitlab:allow-unescaped out.UsageType: an SSH key usage GitLab picks from a fixed set (auth, signing, auth_and_signing).
		fmt.Fprintf(&sb, "| **Usage Type** | %s |\n", out.UsageType)
	}
	fmt.Fprintf(&sb, "| **Created At** | %s |\n", out.CreatedAt)
	if out.ExpiresAt != "" {
		fmt.Fprintf(&sb, "| **Expires At** | %s |\n", out.ExpiresAt)
	}
	if out.LastUsedAt != "" {
		fmt.Fprintf(&sb, "| **Last Used At** | %s |\n", out.LastUsedAt)
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
			//gitlab:allow-unescaped k.CreatedAt: a timestamp toSSHKeyOutput rendered with the constant toolutil.DateTimeFormat layout.
			//gitlab:allow-unescaped k.ExpiresAt: a timestamp toSSHKeyOutput rendered with the constant toolutil.DateTimeFormat layout.
			k.ID, toolutil.EscapeMdTableCell(k.Title), k.UserID, k.CreatedAt, k.ExpiresAt)
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
