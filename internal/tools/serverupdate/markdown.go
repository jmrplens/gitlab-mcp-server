// markdown.go provides Markdown formatting functions for server update MCP tool output.

package serverupdate

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatCheckMarkdownString renders the check result as Markdown.
func FormatCheckMarkdownString(o CheckOutput) string {
	var b strings.Builder
	if o.UpdateAvailable {
		fmt.Fprintf(&b, "## "+toolutil.EmojiUpArrow+" Update Available\n\n")
		fmt.Fprintf(&b, "- **Current Version**: %s\n", o.CurrentVersion)
		fmt.Fprintf(&b, "- **Latest Version**: %s\n", o.LatestVersion)
		if o.ReleaseURL != "" {
			fmt.Fprintf(&b, "- **Release URL**: %s\n", o.ReleaseURL)
		}
	} else {
		fmt.Fprintf(&b, "## "+toolutil.EmojiSuccess+" Up to Date\n\n")
		fmt.Fprintf(&b, "- **Current Version**: %s\n", o.CurrentVersion)
		fmt.Fprintf(&b, "- **Mode**: %s\n", o.Mode)
	}
	if o.Author != "" {
		fmt.Fprintf(&b, toolutil.FmtMdAuthor, o.Author)
	}
	if o.Department != "" {
		fmt.Fprintf(&b, "- **Department**: %s\n", o.Department)
	}
	if o.Repository != "" {
		fmt.Fprintf(&b, "- **Repository**: %s\n", o.Repository)
	}
	if o.UpdateAvailable && o.ReleaseNotes != "" {
		fmt.Fprintf(&b, "\n### Release Notes\n\n%s\n", o.ReleaseNotes)
	}
	toolutil.WriteHints(&b, "Use `gitlab_apply_server_update` to install the update")
	return b.String()
}

// FormatCheckMarkdown renders the check result as an MCP CallToolResult.
func FormatCheckMarkdown(o CheckOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatCheckMarkdownString(o))
}

// FormatApplyMarkdownString renders the apply result as Markdown.
func FormatApplyMarkdownString(o ApplyOutput) string {
	var b strings.Builder
	if o.Deferred {
		fmt.Fprintf(&b, "## "+toolutil.EmojiDownArrow+" Update Downloaded (Deferred)\n\n")
		fmt.Fprintf(&b, "- **Previous Version**: %s\n", o.PreviousVersion)
		fmt.Fprintf(&b, "- **New Version**: %s\n", o.NewVersion)
		fmt.Fprintf(&b, "- **Staging Path**: `%s`\n", o.StagingPath)
		if o.ScriptPath != "" {
			fmt.Fprintf(&b, "- **Update Script**: `%s`\n", o.ScriptPath)
		}
		fmt.Fprintf(&b, "\n> **Note**: The running binary cannot be replaced on Windows. "+
			"Stop the MCP server, then run the update script to apply.\n")
	} else if o.Applied {
		fmt.Fprintf(&b, "## "+toolutil.EmojiSuccess+" Update Applied\n\n")
		fmt.Fprintf(&b, "- **Previous Version**: %s\n", o.PreviousVersion)
		fmt.Fprintf(&b, "- **New Version**: %s\n", o.NewVersion)
		fmt.Fprintf(&b, "\n> **Note**: Restart the server to use the new version.\n")
	} else {
		fmt.Fprintf(&b, "## "+toolutil.EmojiInfo+" No Update Applied\n\n")
		fmt.Fprintf(&b, "- **Message**: %s\n", o.Message)
	}
	toolutil.WriteHints(&b, "Restart the MCP server for the update to take effect")
	return b.String()
}

// FormatApplyMarkdown renders the apply result as an MCP CallToolResult.
func FormatApplyMarkdown(o ApplyOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatApplyMarkdownString(o))
}

func init() {
	toolutil.RegisterMarkdown(FormatCheckMarkdownString)
	toolutil.RegisterMarkdown(FormatApplyMarkdownString)
}
