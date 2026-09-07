package events

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	// The target type is a class name GitLab writes and the IID an integer, so
	// the label is safe; the URL and the title are not, and MdTitleLink is what
	// escapes both halves of the link they end up in.
	label := toolutil.MdTitleLink(fmt.Sprintf("%s #%d", targetType, targetIID), targetURL)
	if targetTitle != "" {
		label += fmt.Sprintf(" %q", toolutil.EscapeMdTableCell(targetTitle))
	}
	return " " + label
}

type markdownEvent struct {
	ActionName     string
	TargetType     string
	TargetIID      int64
	TargetTitle    string
	TargetURL      string
	AuthorUsername string
	CreatedAt      string
}

// FormatContributionListMarkdown formats contribution events as a Markdown CallToolResult.
func FormatContributionListMarkdown(out ListContributionEventsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatContributionListMarkdownString(out))
}

// FormatContributionListMarkdownString renders contribution events as a Markdown string.
func FormatContributionListMarkdownString(out ListContributionEventsOutput) string {
	return formatEventListMarkdown("Contribution Events", "No contribution events found.", contributionMarkdownEvents(out.Events), out.Pagination)
}

// FormatListMarkdown formats project events as a Markdown CallToolResult.
func FormatListMarkdown(out ListProjectEventsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatListMarkdownString renders project events as a Markdown string.
func FormatListMarkdownString(out ListProjectEventsOutput) string {
	return formatEventListMarkdown("Project Events", "No project events found.", projectMarkdownEvents(out.Events), out.Pagination)
}

func formatEventListMarkdown(title, emptyText string, events []markdownEvent, pagination toolutil.PaginationOutput) string {
	if len(events) == 0 {
		return emptyText + "\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s (%d)\n\n", title, len(events))
	toolutil.WriteListSummary(&b, len(events), pagination)
	for _, e := range events {
		target := formatTarget(e.TargetType, e.TargetIID, e.TargetTitle, e.TargetURL)
		author := formatAuthor(e.AuthorUsername)
		//gitlab:allow-unescaped e.ActionName: a contribution-event action GitLab writes from its own vocabulary (opened, closed, pushed to and the rest).
		fmt.Fprintf(&b, "- **%s**%s by %s, %s\n", e.ActionName, target, toolutil.EscapeMdTableCell(author), toolutil.FormatTime(e.CreatedAt))
	}
	b.WriteString(toolutil.FormatPagination(pagination))
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Filter events using action and target_type parameters",
	)
	return b.String()
}

func contributionMarkdownEvents(events []ContributionEventOutput) []markdownEvent {
	items := make([]markdownEvent, len(events))
	for i, event := range events {
		items[i] = markdownEvent{
			ActionName:     event.ActionName,
			TargetType:     event.TargetType,
			TargetIID:      event.TargetIID,
			TargetTitle:    event.TargetTitle,
			TargetURL:      event.TargetURL,
			AuthorUsername: event.AuthorUsername,
			CreatedAt:      event.CreatedAt,
		}
	}
	return items
}

func projectMarkdownEvents(events []ProjectEventOutput) []markdownEvent {
	items := make([]markdownEvent, len(events))
	for i, event := range events {
		items[i] = markdownEvent{
			ActionName:     event.ActionName,
			TargetType:     event.TargetType,
			TargetIID:      event.TargetIID,
			TargetTitle:    event.TargetTitle,
			TargetURL:      event.TargetURL,
			AuthorUsername: event.AuthorUsername,
			CreatedAt:      event.CreatedAt,
		}
	}
	return items
}
