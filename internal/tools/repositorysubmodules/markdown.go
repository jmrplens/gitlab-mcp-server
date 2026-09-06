package repositorysubmodules

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
		// The name, the path and the resolved project all come out of the
		// repository's own .gitmodules, so all three are text somebody typed:
		// resolveProjectPath returns the substring after the colon of an
		// SCP-style remote verbatim.
		//gitlab:allow-unescaped sha: the object id of a submodule tree entry, hexadecimal by construction.
		fmt.Fprintf(&b, "| %s | `%s` | `%s` | %s |\n",
			toolutil.EscapeMdTableCell(s.Name), toolutil.EscapeMdTableCell(s.Path), sha,
			toolutil.EscapeMdTableCell(s.ResolvedProject))
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
	fmt.Fprintf(&b, "- **Submodule**: `%s`\n", toolutil.EscapeMdTableCell(out.SubmodulePath))
	fmt.Fprintf(&b, "- **Resolved Project**: %s\n", toolutil.EscapeMdTableCell(out.ResolvedProject))
	fmt.Fprintf(&b, "- **Commit**: `%s`\n", sha)
	fmt.Fprintf(&b, "- **File**: `%s` (%d bytes)\n\n", toolutil.EscapeMdTableCell(out.FilePath), out.Size)
	fmt.Fprintf(&b, "```%s\n%s\n```\n", ext, out.Content)
	toolutil.WriteHints(&b, "Use `gitlab_update_repository_submodule` to change the commit SHA reference")
	return toolutil.ToolResultWithMarkdown(b.String())
}

// FormatUpdateMarkdown formats the submodule update result as markdown.
func FormatUpdateMarkdown(out UpdateOutput) *mcp.CallToolResult {
	var b strings.Builder
	// The title, the ident and the message are what a person wrote in the
	// commit, and this package's own update action supplies the message.
	//gitlab:allow-unescaped out.ShortID: an abbreviated commit SHA, hexadecimal by construction.
	//gitlab:allow-unescaped out.ID: the commit SHA GitLab returned for the commit the update created, hexadecimal by construction.
	fmt.Fprintf(&b, "## Submodule Updated\n\n- **Commit**: %s (%s)\n- **Title**: %s\n- **Author**: %s <%s>\n- **Message**: %s",
		out.ShortID, out.ID, toolutil.EscapeMdTableCell(out.Title), toolutil.EscapeMdTableCell(out.AuthorName),
		toolutil.EscapeMdTableCell(out.AuthorEmail), toolutil.EscapeMdTableCell(out.Message))
	if out.Status != "" {
		//gitlab:allow-unescaped out.Status: a client-go BuildStateValue, one of a closed set of lowercase words.
		fmt.Fprintf(&b, "\n- **Status**: %s", out.Status)
	}
	return toolutil.ToolResultWithMarkdown(b.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatReadMarkdown)
	toolutil.RegisterMarkdownResult(FormatUpdateMarkdown)
}
