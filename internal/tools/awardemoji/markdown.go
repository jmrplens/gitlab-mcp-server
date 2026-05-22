package awardemoji

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

type awardEmojiNotFoundOutput struct {
	Identifier string `json:"identifier"`
	ListHint   string `json:"list_hint"`
	VerifyHint string `json:"verify_hint"`
}

func formatAwardEmojiNotFound(out awardEmojiNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(awardEmojiResourceName, out.Identifier, out.ListHint, out.VerifyHint)
}

// FormatListMarkdown formats award emoji list as a Markdown CallToolResult.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatListMarkdownString renders award emoji list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.AwardEmoji) == 0 {
		return "No award emoji found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Award Emoji (%d)\n\n", len(out.AwardEmoji))
	toolutil.WriteListSummary(&b, len(out.AwardEmoji), out.Pagination)
	for _, e := range out.AwardEmoji {
		fmt.Fprintf(&b, "- :%s: by %s (ID: %d) - %s\n", e.Name, awardEmojiUserMarkdown(e), e.ID, toolutil.FormatTime(e.CreatedAt))
	}
	b.WriteString(toolutil.FormatPagination(out.Pagination))
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks, "Use the selected tool surface's matching award emoji actions for this resource; delete actions require explicit confirm=true plus the same resource identifiers and award_id")
	return b.String()
}

func awardEmojiUserMarkdown(out Output) string {
	if out.UserWebURL == "" {
		return out.Username
	}
	return toolutil.MdTitleLink(out.Username, out.UserWebURL)
}

// FormatMarkdown formats a single award emoji as a Markdown CallToolResult.
func FormatMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(out))
}

// FormatMarkdownString renders a single award emoji as Markdown.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Award Emoji\n\n")
	fmt.Fprintf(&b, "- **Name**: :%s:\n", out.Name)
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&b, "- **User**: %s (ID: %d)\n", out.Username, out.UserID)
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(out.CreatedAt))
	}
	toolutil.WriteHints(&b, "Use the selected tool surface's matching award emoji delete action with award_id, the same resource identifiers, and explicit confirm=true")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdownResult(formatAwardEmojiNotFound)
}
