package topics

import (
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The topic routes a card or a list points at, by the canonical catalog ID
// every surface resolves. Topics are instance-wide, so they are actions on the
// admin group rather than a group of their own.
const (
	actionTopicGet    = "admin.topic_get"
	actionTopicUpdate = "admin.topic_update"
)

// FormatListMarkdown formats a list of topics.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	if len(out.Topics) == 0 {
		return toolutil.ToolResultWithMarkdown(toolutil.EmptyMessage("topics"))
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Topics", len(out.Topics), out.Pagination)
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Title", "Projects"))
	for _, t := range out.Topics {
		sb.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(t.ID, 10),
			toolutil.EscapeMdTableCell(t.Name),
			toolutil.EscapeMdTableCell(t.Title),
			strconv.FormatUint(t.TotalProjectsCount, 10),
		))
	}
	toolutil.WriteListFooter(&sb, out.Pagination, false,
		toolutil.HintAction(actionTopicGet, "view one topic's details"),
	)
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatTopicMarkdown formats a single topic as its card.
func FormatTopicMarkdown(t TopicItem) *mcp.CallToolResult {
	var sb strings.Builder
	// A topic's name and title are both free text an administrator types.
	c := toolutil.NewCard(&sb, "Topic: "+t.Name)
	c.Int("ID", t.ID)
	c.Field("Title", t.Title)
	c.Text("Description", t.Description)
	c.Field("Projects", strconv.FormatUint(t.TotalProjectsCount, 10))
	c.Count("Organization ID", t.OrganizationID)
	c.Link("Avatar", t.AvatarURL, t.AvatarURL)
	c.End(toolutil.HintAction(actionTopicUpdate, "modify this topic"))
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatGetMarkdown formats a single get_topic response.
func FormatGetMarkdown(out GetOutput) *mcp.CallToolResult {
	return FormatTopicMarkdown(out.Topic)
}

// FormatCreateMarkdown formats a single create_topic response.
func FormatCreateMarkdown(out CreateOutput) *mcp.CallToolResult {
	return FormatTopicMarkdown(out.Topic)
}

// FormatUpdateMarkdown formats a single update_topic response.
func FormatUpdateMarkdown(out UpdateOutput) *mcp.CallToolResult {
	return FormatTopicMarkdown(out.Topic)
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatTopicMarkdown)
	toolutil.RegisterMarkdownResult(FormatGetMarkdown)
	toolutil.RegisterMarkdownResult(FormatCreateMarkdown)
	toolutil.RegisterMarkdownResult(FormatUpdateMarkdown)
}
