// markdown.go provides Markdown formatting functions for tag MCP tool output.
package tags

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdownString renders a single tag as a Markdown summary.
func FormatOutputMarkdownString(t Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Tag: %s\n\n", toolutil.EscapeMdHeading(t.Name))
	fmt.Fprintf(&b, toolutil.FmtMdTarget, t.Target)
	fmt.Fprintf(&b, "- **Protected**: %v\n", t.Protected)
	if t.Message != "" {
		fmt.Fprintf(&b, "- **Message**: %s\n", t.Message)
	}
	if t.CommitSHA != "" {
		fmt.Fprintf(&b, "- **Commit SHA**: %s\n", t.CommitSHA)
	}
	if t.CommitMessage != "" {
		fmt.Fprintf(&b, "- **Commit Message**: %s\n", t.CommitMessage)
	}
	if t.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(t.CreatedAt))
	}
	toolutil.WriteHints(&b,
		"Use action 'delete' to remove this tag",
		"Use gitlab_release action 'create' with this tag to create a release",
	)
	return b.String()
}

// FormatOutputMarkdown renders a single tag as an MCP CallToolResult.
func FormatOutputMarkdown(t Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatOutputMarkdownString(t))
}

// FormatListMarkdownString renders a list of tags as a Markdown table.
func FormatListMarkdownString(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Tags (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Tags), out.Pagination)
	if len(out.Tags) == 0 {
		b.WriteString("No tags found.\n")
		return b.String()
	}
	b.WriteString("| Name | Target | Protected |\n")
	b.WriteString(toolutil.TblSep3Col)
	for _, t := range out.Tags {
		fmt.Fprintf(&b, "| %s | %s | %v |\n", toolutil.EscapeMdTableCell(t.Name), toolutil.EscapeMdTableCell(t.Target), t.Protected)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'get' with a tag_name to see tag details",
		"Use action 'create' to create a new tag",
	)
	return b.String()
}

// FormatListMarkdown renders a list of tags as an MCP CallToolResult.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatSignatureMarkdownString renders a tag signature as a Markdown summary.
func FormatSignatureMarkdownString(out SignatureOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Tag Signature\n\n")
	fmt.Fprintf(&b, "- **Signature Type**: %s\n", out.SignatureType)
	fmt.Fprintf(&b, "- **Verification Status**: %s\n", out.VerificationStatus)
	cert := out.X509Certificate
	fmt.Fprintf(&b, "\n### X.509 Certificate\n\n")
	fmt.Fprintf(&b, "- **Subject**: %s\n", cert.Subject)
	fmt.Fprintf(&b, toolutil.FmtMdEmail, cert.Email)
	fmt.Fprintf(&b, toolutil.FmtMdStatus, cert.CertificateStatus)
	if cert.SerialNumber != "" {
		fmt.Fprintf(&b, "- **Serial Number**: %s\n", cert.SerialNumber)
	}
	issuer := cert.X509Issuer
	fmt.Fprintf(&b, "\n### Issuer\n\n")
	fmt.Fprintf(&b, "- **Subject**: %s\n", issuer.Subject)
	if issuer.CrlURL != "" {
		fmt.Fprintf(&b, "- **CRL URL**: %s\n", issuer.CrlURL)
	}
	toolutil.WriteHints(&b,
		"Use action 'get' to see full tag details",
		"Use action 'list' to browse all tags",
	)
	return b.String()
}

// FormatSignatureMarkdown renders a tag signature as an MCP CallToolResult.
func FormatSignatureMarkdown(out SignatureOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatSignatureMarkdownString(out))
}

// FormatProtectedTagMarkdownString renders a protected tag as Markdown.
func FormatProtectedTagMarkdownString(out ProtectedTagOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Protected Tag: %s\n\n", toolutil.EscapeMdHeading(out.Name))
	if len(out.CreateAccessLevels) > 0 {
		b.WriteString("### Create Access Levels\n\n")
		b.WriteString("| ID | Access Level | Description | User ID | Group ID | Deploy Key ID |\n")
		b.WriteString("|----|-------------|-------------|---------|----------|---------------|\n")
		for _, al := range out.CreateAccessLevels {
			fmt.Fprintf(&b, "| %d | %d | %s | %s | %s | %s |\n",
				al.ID, al.AccessLevel, toolutil.EscapeMdTableCell(al.AccessLevelDescription),
				formatIDCell(al.UserID), formatIDCell(al.GroupID), formatIDCell(al.DeployKeyID))
		}
	} else {
		b.WriteString("No create access levels defined.\n")
	}
	toolutil.WriteHints(&b,
		"Use action 'list_protected' to see all protected tags",
		"Use action 'unprotect' to remove tag protection",
	)
	return b.String()
}

// FormatProtectedTagMarkdown renders a protected tag as an MCP CallToolResult.
func FormatProtectedTagMarkdown(out ProtectedTagOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatProtectedTagMarkdownString(out))
}

// FormatListProtectedTagsMarkdownString renders a list of protected tags as Markdown.
func FormatListProtectedTagsMarkdownString(out ListProtectedTagsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Protected Tags (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Tags), out.Pagination)
	if len(out.Tags) == 0 {
		b.WriteString("No protected tags found.\n")
		return b.String()
	}
	b.WriteString("| Name | Create Access Levels |\n")
	b.WriteString("|------|---------------------|\n")
	for _, t := range out.Tags {
		levels := make([]string, len(t.CreateAccessLevels))
		for i, al := range t.CreateAccessLevels {
			levels[i] = formatAccessLevelSummary(al)
		}
		fmt.Fprintf(&b, "| %s | %s |\n", toolutil.EscapeMdTableCell(t.Name), strings.Join(levels, ", "))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'get_protected' with tag name for full details",
		"Use action 'protect' to add a new protected tag",
	)
	return b.String()
}

// FormatListProtectedTagsMarkdown renders a list of protected tags as an MCP CallToolResult.
func FormatListProtectedTagsMarkdown(out ListProtectedTagsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListProtectedTagsMarkdownString(out))
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatSignatureMarkdownString)
	toolutil.RegisterMarkdown(FormatProtectedTagMarkdownString)
	toolutil.RegisterMarkdown(FormatListProtectedTagsMarkdownString)
}
