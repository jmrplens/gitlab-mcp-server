package events

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func init() {
	toolutil.RegisterMarkdown(FormatContributionListMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
}

// formatTarget builds the target description with an optional clickable link.
func formatTarget(targetType string, targetIID int64, targetTitle, targetURL string) string {
	if targetType == "" {
		return ""
	}
	label := fmt.Sprintf("%s #%d", targetType, targetIID)
	if targetURL != "" {
		label = fmt.Sprintf("[%s](%s)", label, targetURL)
	}
	if targetTitle != "" {
		label += fmt.Sprintf(" %q", targetTitle)
	}
	return " " + label
}

// FormatContributionListMarkdown formats contribution events as a Markdown CallToolResult.
func FormatContributionListMarkdown(out ListContributionEventsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatContributionListMarkdownString(out))
}

// FormatContributionListMarkdownString renders contribution events as a Markdown string.
func FormatContributionListMarkdownString(out ListContributionEventsOutput) string {
	if len(out.Events) == 0 {
		return "No contribution events found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Contribution Events (%d)\n\n", len(out.Events))
	toolutil.WriteListSummary(&b, len(out.Events), out.Pagination)
	for _, e := range out.Events {
		target := formatTarget(e.TargetType, e.TargetIID, e.TargetTitle, e.TargetURL)
		author := formatAuthor(e.AuthorUsername)
		fmt.Fprintf(&b, "- **%s**%s by %s — %s\n", e.ActionName, target, author, toolutil.FormatTime(e.CreatedAt))
	}
	b.WriteString(toolutil.FormatPagination(out.Pagination))
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Filter events using action and target_type parameters",
	)
	return b.String()
}

// FormatListMarkdown formats project events as a Markdown CallToolResult.
func FormatListMarkdown(out ListProjectEventsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatListMarkdownString renders project events as a Markdown string.
func FormatListMarkdownString(out ListProjectEventsOutput) string {
	if len(out.Events) == 0 {
		return "No project events found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Project Events (%d)\n\n", len(out.Events))
	toolutil.WriteListSummary(&b, len(out.Events), out.Pagination)
	for _, e := range out.Events {
		target := formatTarget(e.TargetType, e.TargetIID, e.TargetTitle, e.TargetURL)
		author := formatAuthor(e.AuthorUsername)
		fmt.Fprintf(&b, "- **%s**%s by %s — %s\n", e.ActionName, target, author, toolutil.FormatTime(e.CreatedAt))
	}
	b.WriteString(toolutil.FormatPagination(out.Pagination))
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Filter events using action and target_type parameters",
	)
	return b.String()
}
