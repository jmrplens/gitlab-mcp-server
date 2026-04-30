// markdown.go provides Markdown formatting functions for project discovery MCP tool output.
package projectdiscovery

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatMarkdown renders the resolved project as a Markdown CallToolResult.
func FormatMarkdown(out ResolveOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Resolved GitLab Project\n\n")
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&b, toolutil.FmtMdName, out.Name)
	fmt.Fprintf(&b, toolutil.FmtMdPath, out.PathWithNamespace)
	fmt.Fprintf(&b, toolutil.FmtMdURL, out.WebURL)
	fmt.Fprintf(&b, "- **Default Branch**: %s\n", out.DefaultBranch)
	if out.Description != "" {
		fmt.Fprintf(&b, toolutil.FmtMdDescription, out.Description)
	}
	fmt.Fprintf(&b, toolutil.FmtMdVisibility, out.Visibility)
	fmt.Fprintf(&b, "\nUse `project_id: %d` or `project_id: \"%s\"` for subsequent operations.\n", out.ID, out.PathWithNamespace)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use the project_id in subsequent tool calls to operate on this project",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
}
