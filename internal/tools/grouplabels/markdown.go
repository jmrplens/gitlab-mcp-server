package grouplabels

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/labeldata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatMarkdown renders a single group label as a Markdown card.
//
// This package and the project label package answer with one output type,
// [labeldata.Output], so the card is registered once, there, and picks its
// copy from the label's own scope: a group label is titled as one whichever
// action fetched it.
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
	toolutil.RegisterMarkdown(FormatListMarkdownString)
}
