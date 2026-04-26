// markdown.go provides Markdown formatting functions for repository submodule MCP tool output.

package repositorysubmodules

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatListMarkdown renders the submodule list as a Markdown table.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	if out.Count == 0 {
		return toolutil.ToolResultWithMarkdown("## Repository Submodules\n\nNo submodules found.")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Repository Submodules (%d)\n\n", out.Count)
	b.WriteString("| Name | Path | Commit SHA | Resolved Project |\n")
	b.WriteString("|------|------|------------|------------------|\n")
	for _, s := range out.Submodules {
		sha := s.CommitSHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		fmt.Fprintf(&b, "| %s | `%s` | `%s` | %s |\n", s.Name, s.Path, sha, s.ResolvedProject)
	}
	toolutil.WriteHints(&b, "Use `gitlab_read_repository_submodule_file` to view submodule content details")
	return toolutil.ToolResultWithMarkdown(b.String())
}

// FormatReadMarkdown renders the submodule file read result as Markdown.
func FormatReadMarkdown(out ReadOutput) *mcp.CallToolResult {
	ext := ""
	if idx := strings.LastIndex(out.FileName, "."); idx >= 0 {
		ext = out.FileName[idx+1:]
	}
	sha := out.CommitSHA
	if len(sha) > 8 {
		sha = sha[:8]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## File from Submodule\n\n")
	fmt.Fprintf(&b, "- **Submodule**: `%s`\n", out.SubmodulePath)
	fmt.Fprintf(&b, "- **Resolved Project**: %s\n", out.ResolvedProject)
	fmt.Fprintf(&b, "- **Commit**: `%s`\n", sha)
	fmt.Fprintf(&b, "- **File**: `%s` (%d bytes)\n\n", out.FilePath, out.Size)
	fmt.Fprintf(&b, "```%s\n%s\n```\n", ext, out.Content)
	toolutil.WriteHints(&b, "Use `gitlab_update_repository_submodule` to change the commit SHA reference")
	return toolutil.ToolResultWithMarkdown(b.String())
}

// FormatUpdateMarkdown formats the submodule update result as markdown.
func FormatUpdateMarkdown(out UpdateOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(fmt.Sprintf(
		"## Submodule Updated\n\n- **Commit**: %s (%s)\n- **Title**: %s\n- **Author**: %s <%s>\n- **Message**: %s",
		out.ShortID, out.ID, out.Title, out.AuthorName, out.AuthorEmail, out.Message,
	))
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatReadMarkdown)
	toolutil.RegisterMarkdownResult(FormatUpdateMarkdown)
}
