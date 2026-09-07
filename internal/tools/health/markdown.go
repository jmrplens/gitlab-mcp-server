package health

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	// Half of this report is the server describing itself rather than GitLab
	// describing anything, and those values are declared below instead of
	// escaped, because wrapping a compiled-in constant teaches the next reader
	// a rule that is not the rule. Everything read out of a GitLab response is
	// escaped, the error string first of all: client-go puts the whole response
	// body in its message when the body is not the JSON it expected, so a
	// proxy's HTML error page arrives here with its tags and line breaks intact.
	//
	//gitlab:allow-unescaped s.Status: one of the three literals Check assigns, "healthy", "degraded" or "unhealthy".
	//gitlab:allow-unescaped s.MCPServerVersion: this binary's own version, stamped by ldflags at build time or read from debug.BuildInfo.
	//gitlab:allow-unescaped s.Author: a constant of cmd/server, handed over once at startup through SetServerInfo.
	//gitlab:allow-unescaped s.Department: a constant of cmd/server, handed over once at startup through SetServerInfo.
	//gitlab:allow-unescaped s.Repository: a constant of cmd/server, handed over once at startup through SetServerInfo.
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
	// net/url permits '<' in a host and leaves it there, and under
	// --allow-any-gitlab-url the host is a value the caller supplied.
	fmt.Fprintf(&b, "- **GitLab URL**: %s\n", toolutil.EscapeMdTableCell(s.GitLabURL))
	if s.GitLabVersion != "" {
		fmt.Fprintf(&b, "- **Version**: %s (revision: %s)\n",
			toolutil.EscapeMdTableCell(s.GitLabVersion), toolutil.EscapeMdTableCell(s.GitLabRevision))
	}
	fmt.Fprintf(&b, "- **Authenticated**: %v\n", s.Authenticated)
	if s.Username != "" {
		fmt.Fprintf(&b, "- **User**: %s (ID: %d)\n", toolutil.EscapeMdTableCell(s.Username), s.UserID)
	}
	fmt.Fprintf(&b, "- **Response Time**: %d ms\n", s.ResponseTimeMS)
	if s.Error != "" {
		fmt.Fprintf(&b, "- **Error**: %s\n", toolutil.EscapeMdTableCell(s.Error))
	}
	toolutil.WriteHints(
		&b,
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
