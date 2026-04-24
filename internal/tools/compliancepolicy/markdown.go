// markdown.go provides human-readable Markdown formatters for compliance policy settings.

package compliancepolicy

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown formats compliance policy settings as Markdown.
func FormatOutputMarkdown(out Output) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Compliance Policy Settings\n\n")
	fmt.Fprintf(&sb, "| Field | Value |\n")
	fmt.Fprintf(&sb, "|-------|-------|\n")
	if out.CSPNamespaceID != nil {
		fmt.Fprintf(&sb, "| CSP Namespace ID | %d |\n", *out.CSPNamespaceID)
	} else {
		fmt.Fprintf(&sb, "| CSP Namespace ID | _not set_ |\n")
	}
	sb.WriteString("\n")
	toolutil.WriteHints(&sb,
		"Use `gitlab_update_compliance_policy_settings` to modify these settings",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown) // Output
}
