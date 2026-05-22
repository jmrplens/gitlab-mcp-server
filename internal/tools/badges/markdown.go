package badges

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

type badgeNotFoundOutput struct {
	Resource   string   `json:"resource"`
	Identifier string   `json:"identifier"`
	Hints      []string `json:"hints,omitempty"`
}

func formatBadgeNotFound(out badgeNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(out.Resource, out.Identifier, out.Hints...)
}

// FormatBadgeListMarkdown formats a list of badges.
func FormatBadgeListMarkdown(badges []BadgeItem, title string, pagination toolutil.PaginationOutput) *mcp.CallToolResult {
	if len(badges) == 0 {
		return toolutil.ToolResultWithMarkdown("No badges found.\n")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s (%d)\n\n", title, len(badges))
	sb.WriteString("| ID | Name | Link URL | Image URL | Kind |\n")
	sb.WriteString("|----|------|----------|-----------|------|\n")
	for _, b := range badges {
		fmt.Fprintf(&sb, "| %d | %s | %s | %s | %s |\n",
			b.ID,
			toolutil.EscapeMdTableCell(b.Name),
			toolutil.EscapeMdTableCell(b.LinkURL),
			toolutil.EscapeMdTableCell(b.ImageURL),
			b.Kind)
	}
	toolutil.WritePagination(&sb, pagination)
	toolutil.WriteHints(&sb, "Use `gitlab_get_badge` to view details of a specific badge")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatBadgeMarkdown formats a single badge.
func FormatBadgeMarkdown(b BadgeItem) *mcp.CallToolResult {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Badge: %s (ID: %d)\n\n", b.Name, b.ID)
	fmt.Fprintf(&sb, "- **Link URL**: %s\n", b.LinkURL)
	fmt.Fprintf(&sb, "- **Image URL**: %s\n", b.ImageURL)
	if b.RenderedLinkURL != "" {
		fmt.Fprintf(&sb, "- **Rendered Link**: %s\n", b.RenderedLinkURL)
	}
	if b.RenderedImageURL != "" {
		fmt.Fprintf(&sb, "- **Rendered Image**: %s\n", b.RenderedImageURL)
	}
	if b.Kind != "" {
		fmt.Fprintf(&sb, "- **Kind**: %s\n", b.Kind)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_update_badge` to modify this badge")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func formatListProjectOutput(out ListProjectOutput) *mcp.CallToolResult {
	return FormatBadgeListMarkdown(out.Badges, "Project Badges", out.Pagination)
}

func formatGetProjectOutput(out GetProjectOutput) *mcp.CallToolResult {
	return FormatBadgeMarkdown(out.Badge)
}

func formatAddProjectOutput(out AddProjectOutput) *mcp.CallToolResult {
	return FormatBadgeMarkdown(out.Badge)
}

func formatEditProjectOutput(out EditProjectOutput) *mcp.CallToolResult {
	return FormatBadgeMarkdown(out.Badge)
}

func formatPreviewProjectOutput(out PreviewProjectOutput) *mcp.CallToolResult {
	return FormatBadgeMarkdown(out.Badge)
}

func formatListGroupOutput(out ListGroupOutput) *mcp.CallToolResult {
	return FormatBadgeListMarkdown(out.Badges, "Group Badges", out.Pagination)
}

func formatGetGroupOutput(out GetGroupOutput) *mcp.CallToolResult {
	return FormatBadgeMarkdown(out.Badge)
}

func formatAddGroupOutput(out AddGroupOutput) *mcp.CallToolResult {
	return FormatBadgeMarkdown(out.Badge)
}

func formatEditGroupOutput(out EditGroupOutput) *mcp.CallToolResult {
	return FormatBadgeMarkdown(out.Badge)
}

func formatPreviewGroupOutput(out PreviewGroupOutput) *mcp.CallToolResult {
	return FormatBadgeMarkdown(out.Badge)
}

func init() {
	toolutil.RegisterMarkdownResult(FormatBadgeMarkdown)
	toolutil.RegisterMarkdownResult(formatBadgeNotFound)
	toolutil.RegisterMarkdownResult(formatListProjectOutput)
	toolutil.RegisterMarkdownResult(formatGetProjectOutput)
	toolutil.RegisterMarkdownResult(formatAddProjectOutput)
	toolutil.RegisterMarkdownResult(formatEditProjectOutput)
	toolutil.RegisterMarkdownResult(formatPreviewProjectOutput)
	toolutil.RegisterMarkdownResult(formatListGroupOutput)
	toolutil.RegisterMarkdownResult(formatGetGroupOutput)
	toolutil.RegisterMarkdownResult(formatAddGroupOutput)
	toolutil.RegisterMarkdownResult(formatEditGroupOutput)
	toolutil.RegisterMarkdownResult(formatPreviewGroupOutput)
}
