package grouplabels

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/labeldata"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

var labelMarkdownOptions = toolutil.LabelMarkdownOptions{
	DetailTitle:   "Group Label",
	ListTitle:     "Group Labels",
	EmptyListText: "No group labels found.",
	DetailHints: []string{
		"If the workflow asks to fetch/get before update or delete, use the selected tool surface's group-label get action with the same group_id and this label_id next",
		"Use the selected tool surface's group-label update action with the same group_id and this label_id to modify this label",
		"Use the selected tool surface's group-label delete action with the same group_id, this label_id, and explicit confirm=true to remove this label",
		"Use the selected tool surface's group-label subscribe or unsubscribe actions with the same group_id and this label_id to follow or unfollow",
	},
	ListHints: []string{
		toolutil.HintPreserveLinks,
		"Use the selected tool surface's group-label get action with the same group_id and label_id for full details before update/delete workflows",
		"Use the selected tool surface's group-label create action with group_id to add a new group label",
	},
}

// FormatMarkdown renders a single group label as a Markdown summary.
func FormatMarkdown(l Output) string {
	return toolutil.FormatLabelMarkdown(toLabelMarkdown(l), labelMarkdownOptions)
}

// FormatListMarkdownString renders a paginated list of group labels as a Markdown table string.
func FormatListMarkdownString(out ListOutput) string {
	return toolutil.FormatLabelListMarkdownFunc(out.Labels, out.Pagination, labelMarkdownOptions, toLabelMarkdown)
}

// FormatListMarkdown renders a paginated list of group labels as an MCP Markdown result.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
}

func toLabelMarkdown(label Output) toolutil.LabelMarkdown {
	return labeldata.ToMarkdown(label)
}
