// markdown.go provides Markdown formatting functions for group SSH certificate
// MCP tool output.
package groupsshcerts

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single SSH certificate as Markdown.
func FormatOutputMarkdown(o Output) string {
	if o.ID == 0 {
		return ""
	}
	key := o.Key
	if len(key) > 60 {
		key = key[:57] + "..."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## SSH Certificate #%d\n\n", o.ID)
	fmt.Fprintf(&b, "- **Title**: %s\n", o.Title)
	fmt.Fprintf(&b, "- **Key**: `%s`\n", key)
	if o.CreatedAt != "" {
		fmt.Fprintf(&b, "- **Created**: %s\n", o.CreatedAt)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_delete_group_ssh_certificate` to revoke this certificate",
		"Use `gitlab_list_group_ssh_certificates` to view all certificates",
	)
	return b.String()
}

// FormatListMarkdown renders a list of SSH certificates as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Certificates) == 0 {
		return "No SSH certificates found."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## SSH Certificates (%d)\n\n", len(out.Certificates))
	b.WriteString("| ID | Title | Created |\n")
	b.WriteString("| --: | ----- | ------- |\n")
	for _, c := range out.Certificates {
		fmt.Fprintf(&b, "| %d | %s | %s |\n",
			c.ID,
			toolutil.EscapeMdTableCell(c.Title),
			toolutil.EscapeMdTableCell(c.CreatedAt),
		)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_create_group_ssh_certificate` to add a new certificate",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown) // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)   // ListOutput
}
