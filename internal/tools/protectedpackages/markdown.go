// markdown.go provides Markdown formatting functions for protected package
// MCP tool output.

package protectedpackages

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single package protection rule as Markdown.
func FormatOutputMarkdown(r Output) string {
	if r.ID == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Package Protection Rule #%d\n\n", r.ID)
	fmt.Fprintf(&b, "- **Pattern**: `%s`\n", r.PackageNamePattern)
	fmt.Fprintf(&b, "- **Package Type**: %s\n", r.PackageType)
	if r.MinimumAccessLevelForPush != "" {
		fmt.Fprintf(&b, "- **Min Push Level**: %s\n", r.MinimumAccessLevelForPush)
	}
	if r.MinimumAccessLevelForDelete != "" {
		fmt.Fprintf(&b, "- **Min Delete Level**: %s\n", r.MinimumAccessLevelForDelete)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_update_package_protection_rule` to modify this rule",
		"Use `gitlab_delete_package_protection_rule` to remove it",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of package protection rules as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Rules) == 0 {
		return "No package protection rules found."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Package Protection Rules (%d)\n\n", len(out.Rules))
	b.WriteString("| ID | Pattern | Type | Min Push | Min Delete |\n")
	b.WriteString("| --: | ------- | ---- | -------- | ---------- |\n")
	for _, r := range out.Rules {
		fmt.Fprintf(&b, "| %d | `%s` | %s | %s | %s |\n",
			r.ID,
			toolutil.EscapeMdTableCell(r.PackageNamePattern),
			toolutil.EscapeMdTableCell(r.PackageType),
			toolutil.EscapeMdTableCell(r.MinimumAccessLevelForPush),
			toolutil.EscapeMdTableCell(r.MinimumAccessLevelForDelete),
		)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown) // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)   // ListOutput
}
