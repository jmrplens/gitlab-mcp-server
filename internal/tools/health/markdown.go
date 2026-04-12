// markdown.go provides Markdown formatting functions for server health MCP tool output.

package health

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatMarkdownString renders the server health status as a Markdown string.
func FormatMarkdownString(s Output) string {
	var b strings.Builder
	var statusEmoji string
	switch s.Status {
	case "unhealthy":
		statusEmoji = toolutil.EmojiCross
	case "degraded":
		statusEmoji = toolutil.EmojiWarning
	default:
		statusEmoji = toolutil.EmojiSuccess
	}
	fmt.Fprintf(&b, "## %s GitLab Server Status: %s\n\n", statusEmoji, s.Status)
	if s.MCPServerVersion != "" {
		fmt.Fprintf(&b, "- **MCP Server Version**: %s\n", s.MCPServerVersion)
	}
	if s.Author != "" {
		fmt.Fprintf(&b, toolutil.FmtMdAuthor, s.Author)
	}
	if s.Department != "" {
		fmt.Fprintf(&b, "- **Department**: %s\n", s.Department)
	}
	if s.Repository != "" {
		fmt.Fprintf(&b, "- **Repository**: %s\n", s.Repository)
	}
	fmt.Fprintf(&b, "- **GitLab URL**: %s\n", s.GitLabURL)
	if s.GitLabVersion != "" {
		fmt.Fprintf(&b, "- **Version**: %s (revision: %s)\n", s.GitLabVersion, s.GitLabRevision)
	}
	fmt.Fprintf(&b, "- **Authenticated**: %v\n", s.Authenticated)
	if s.Username != "" {
		fmt.Fprintf(&b, "- **User**: %s (ID: %d)\n", s.Username, s.UserID)
	}
	fmt.Fprintf(&b, "- **Response Time**: %d ms\n", s.ResponseTimeMS)
	if s.Error != "" {
		fmt.Fprintf(&b, "- **Error**: %s\n", s.Error)
	}
	toolutil.WriteHints(&b,
		"Use gitlab_project action 'list' to explore available projects",
		"Use gitlab_user action 'me' to see current user details",
	)
	return b.String()
}

// FormatMarkdown renders the server health status as an MCP CallToolResult.
func FormatMarkdown(s Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(s))
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
}
