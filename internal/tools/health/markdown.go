package health

import (
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// statusEmoji is the glyph the heading opens with, one per state Check
// assigns.
func statusEmoji(status string) string {
	switch status {
	case "unhealthy":
		return toolutil.EmojiCross
	case "degraded":
		return toolutil.EmojiWarning
	default:
		return toolutil.EmojiSuccess
	}
}

// FormatMarkdownString renders the server health status as the card of one
// object: what this binary is, what GitLab answered, and who the credential
// belongs to.
//
// Half of this report is the server describing itself rather than GitLab
// describing anything, and the card escapes both halves at the row that writes
// them, which is why the five directives that used to declare the server's own
// constants safe are gone: the rule is now the same for every value, and the
// one that most needs it is the error string, since client-go puts the whole
// response body in its message when the body is not the JSON it expected, so a
// proxy's HTML error page arrives here with its tags and line breaks intact.
func FormatMarkdownString(s Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, statusEmoji(s.Status)+" GitLab Server Status: "+s.Status)
	c.Field("MCP Server Version", s.MCPServerVersion)
	c.Field("Author", s.Author)
	c.Field("Department", s.Department)
	c.Field("Repository", s.Repository)
	// net/url permits '<' in a host and leaves it there, and under
	// --allow-any-gitlab-url the host is a value the caller supplied.
	c.Field("GitLab URL", s.GitLabURL)
	// The revision is a row of its own: joined to the version as
	// "%s (revision: %s)" it printed an empty parenthesis whenever GitLab sent
	// a version and no revision.
	c.Field("Version", s.GitLabVersion)
	c.Field("Revision", s.GitLabRevision)
	c.Bool("Authenticated", s.Authenticated)
	c.Markdown("User", toolutil.MdUserHandle(s.Username))
	c.Count("User ID", s.UserID)
	c.Field("Response Time", strconv.FormatInt(s.ResponseTimeMS, 10)+" ms")
	c.Text("Error", s.Error)
	c.End(
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
