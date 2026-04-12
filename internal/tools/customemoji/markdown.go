// markdown.go provides Markdown formatting for Custom Emoji outputs.

package customemoji

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown renders a paginated list of custom emoji as Markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("## Custom Emoji\n\n")

	if len(out.Emoji) == 0 {
		sb.WriteString("No custom emoji found.\n")
		return sb.String()
	}

	sb.WriteString("| Name | External | Created |\n")
	sb.WriteString("|------|----------|---------|\n")

	for _, e := range out.Emoji {
		external := "No"
		if e.External {
			external = "Yes"
		}
		created := "-"
		if e.CreatedAt != "" {
			created = toolutil.EscapeMdTableCell(e.CreatedAt)
		}

		fmt.Fprintf(&sb, "| :%s: | %s | %s |\n",
			toolutil.EscapeMdTableCell(e.Name),
			external,
			created,
		)
	}

	sb.WriteString("\n")
	sb.WriteString(toolutil.FormatGraphQLPagination(out.Pagination, len(out.Emoji)))
	sb.WriteString("\n")
	return sb.String()
}

// FormatCreateMarkdown renders a single created custom emoji as Markdown.
func FormatCreateMarkdown(out CreateOutput) string {
	var sb strings.Builder
	sb.WriteString(toolutil.EmojiSuccess + " Custom emoji created.\n\n")
	sb.WriteString("| Field | Value |\n")
	sb.WriteString("|-------|-------|\n")
	fmt.Fprintf(&sb, "| ID | `%s` |\n", out.Emoji.ID)
	fmt.Fprintf(&sb, "| Name | :%s: |\n", toolutil.EscapeMdTableCell(out.Emoji.Name))
	fmt.Fprintf(&sb, "| URL | %s |\n", toolutil.MdTitleLink(out.Emoji.Name, out.Emoji.URL))

	external := "No"
	if out.Emoji.External {
		external = "Yes"
	}
	fmt.Fprintf(&sb, "| External | %s |\n", external)

	if out.Emoji.CreatedAt != "" {
		fmt.Fprintf(&sb, "| Created | %s |\n", toolutil.EscapeMdTableCell(out.Emoji.CreatedAt))
	}
	toolutil.WriteHints(&sb,
		toolutil.HintPreserveLinks,
		"Use `gitlab_list_custom_emoji` to view all custom emoji",
		"Use `gitlab_delete_custom_emoji` to remove this emoji",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatCreateMarkdown)
}
