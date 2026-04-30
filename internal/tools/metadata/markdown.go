// markdown.go provides Markdown formatting functions for GitLab metadata MCP tool output.
package metadata

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatGetMarkdown formats metadata as markdown.
func FormatGetMarkdown(out GetOutput) string {
	var sb strings.Builder
	sb.WriteString("## GitLab Metadata\n\n")
	sb.WriteString("| Property | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Version | %s |\n", toolutil.EscapeMdTableCell(out.Version))
	fmt.Fprintf(&sb, "| Revision | %s |\n", toolutil.EscapeMdTableCell(out.Revision))
	fmt.Fprintf(&sb, "| Enterprise | %v |\n", out.Enterprise)
	fmt.Fprintf(&sb, "| KAS Enabled | %v |\n", out.KAS.Enabled)
	if out.KAS.Version != "" {
		fmt.Fprintf(&sb, "| KAS Version | %s |\n", toolutil.EscapeMdTableCell(out.KAS.Version))
	}
	if out.KAS.ExternalURL != "" {
		fmt.Fprintf(&sb, "| KAS URL | %s |\n", toolutil.EscapeMdTableCell(out.KAS.ExternalURL))
	}
	toolutil.WriteHints(&sb, "Use version information to verify API compatibility")
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}
