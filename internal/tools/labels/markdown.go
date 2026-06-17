package labels

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/labeldata"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// labelMarkdownOptions configures the shared [toolutil.LabelMarkdownOptions]
// used by the package's single-label and list Markdown formatters. It
// centralises the title, empty-state copy, and follow-up hints so both
// renderers stay in sync.
var labelMarkdownOptions = toolutil.LabelMarkdownOptions{
	DetailTitle:       "Label",
	ListTitle:         "Labels",
	EmptyListText:     "No labels found.",
	EscapeDescription: true,
	DetailHints: []string{
		"Use action 'label_update' to change label name, color, or description",
		"Use action 'label_delete' to remove this label",
	},
	ListHints: []string{
		toolutil.HintPreserveLinks,
		"Use action 'label_get' with a label_id to see label details",
		"Use action 'label_create' to create a new label",
	},
}

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

// FormatMarkdown renders a single label as a Markdown summary.
func FormatMarkdown(l Output) string {
	return toolutil.FormatLabelMarkdown(toLabelMarkdown(l), labelMarkdownOptions)
}

// FormatListMarkdownString renders a paginated list of labels as a Markdown table string.
func FormatListMarkdownString(out ListOutput) string {
	return toolutil.FormatLabelListMarkdownFunc(out.Labels, out.Pagination, labelMarkdownOptions, toLabelMarkdown)
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

// toLabelMarkdown adapts a package [Output] into the
// [toolutil.LabelMarkdown] shape used by the shared formatter helpers.
func toLabelMarkdown(label Output) toolutil.LabelMarkdown {
	return labeldata.ToMarkdown(label)
}
