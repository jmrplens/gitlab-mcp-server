package labels

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/labeldata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

type labelNotFoundOutput struct {
	Identifier string
}

// formatLabelNotFound renders a [labelNotFoundOutput] as a structured
// MCP not-found result with follow-up hints.
func formatLabelNotFound(out labelNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Label", out.Identifier,
		"Use gitlab_label_list with project_id to list labels",
		"Labels can be referenced by ID or name - verify the value is correct",
	)
}

// FormatMarkdown renders a single label as a Markdown card, with the copy of
// the scope the label itself belongs to: a project label list carries the
// group labels the project inherits, and one of those is not a project label
// whichever action fetched it.
func FormatMarkdown(l Output) string {
	return labeldata.FormatMarkdown(l)
}

// FormatListMarkdownString renders a paginated list of labels as a Markdown table string.
func FormatListMarkdownString(out ListOutput) string {
	return toolutil.FormatLabelListMarkdownFunc(out.Labels, out.Pagination, labeldata.ProjectMarkdownOptions, labeldata.ToMarkdown)
}

// FormatListMarkdown renders a paginated list of labels as an MCP Markdown result.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

func init() {
	toolutil.RegisterMarkdownResult(formatLabelNotFound)
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
}
