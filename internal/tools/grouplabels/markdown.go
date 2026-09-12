package grouplabels

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/labeldata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatMarkdown renders a single group label as a Markdown card.
//
// This package and the project label package alias one output type, so the
// Markdown registry keys them together and keeps whichever init ran first.
// Both now render through the shared formatter, which picks the copy from the
// label's own scope, so a group label is no longer titled "Label" and pointed
// at the project actions because the other package's init won the race.
func FormatMarkdown(l Output) string {
	return labeldata.FormatMarkdown(l)
}

// FormatListMarkdownString renders a paginated list of group labels as a Markdown table string.
func FormatListMarkdownString(out ListOutput) string {
	return toolutil.FormatLabelListMarkdownFunc(out.Labels, out.Pagination, labeldata.GroupMarkdownOptions, labeldata.ToMarkdown)
}

// FormatListMarkdown renders a paginated list of group labels as an MCP Markdown result.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
}
