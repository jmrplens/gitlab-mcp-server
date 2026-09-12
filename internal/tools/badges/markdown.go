package badges

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionProjectBadgeGet  = "project.badge_get"
	actionProjectBadgeEdit = "project.badge_edit"
	actionProjectBadgeAdd  = "project.badge_add"
	actionGroupBadgeGet    = "group.badge_get"
	actionGroupBadgeEdit   = "group.badge_edit"
	actionGroupBadgeAdd    = "group.badge_add"
)

type badgeNotFoundOutput struct {
	Resource   string   `json:"resource"`
	Identifier string   `json:"identifier"`
	Hints      []string `json:"hints,omitempty"`
}

// formatBadgeNotFound renders the not-found card. The resource label and the
// hints are this package's own words, but they reach the formatter as fields
// of the result rather than as literals, so each is escaped like any other
// value read off an output: the label lands in a sentence and every hint in a
// list item, and the inline escaper is what both take.
func formatBadgeNotFound(out badgeNotFoundOutput) *mcp.CallToolResult {
	hints := make([]string, 0, len(out.Hints))
	for _, hint := range out.Hints {
		hints = append(hints, toolutil.EscapeMdTableCell(hint))
	}
	return toolutil.NotFoundResult(toolutil.EscapeMdTableCell(out.Resource), out.Identifier, hints...)
}

// FormatBadgeListMarkdown formats a list of badges as a Markdown table: a
// collection of objects that share columns.
func FormatBadgeListMarkdown(badges []BadgeItem, title string, pagination toolutil.PaginationOutput) *mcp.CallToolResult {
	if len(badges) == 0 {
		return toolutil.ToolResultWithMarkdown(toolutil.EmptyMessage("badges"))
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, title, len(badges), pagination)
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Link URL", "Image URL", "Kind"))
	for _, b := range badges {
		sb.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(b.ID, 10),
			toolutil.EscapeMdTableCell(b.Name),
			// Every URL here is one a maintainer typed into the badge,
			// placeholders included, so it is shown as the template it is
			// rather than linked.
			toolutil.EscapeMdTableCell(b.LinkURL),
			toolutil.EscapeMdTableCell(b.ImageURL),
			toolutil.EscapeMdTableCell(b.Kind),
		))
	}
	toolutil.WriteListFooter(&sb, pagination, false,
		toolutil.HintAction(actionProjectBadgeGet, "read one project badge"),
		toolutil.HintAction(actionGroupBadgeGet, "read one group badge"),
	)
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// badgeHeading names a badge by the name its maintainer gave it, and by its ID
// when GitLab sent no name: a badge's name is optional, and "Badge: (ID: 7)"
// read as a badge whose name was lost rather than one that has none.
func badgeHeading(b BadgeItem) string {
	if b.Name == "" {
		return fmt.Sprintf("Badge #%d", b.ID)
	}
	return fmt.Sprintf("Badge: %s (ID: %d)", b.Name, b.ID)
}

// FormatBadgeMarkdown formats a single badge as the card of one object.
func FormatBadgeMarkdown(b BadgeItem) *mcp.CallToolResult {
	var sb strings.Builder
	c := toolutil.NewCard(&sb, badgeHeading(b))
	writeBadgeRows(c, b)
	c.End(
		toolutil.HintAction(actionProjectBadgeEdit, "change this badge on a project"),
		toolutil.HintAction(actionGroupBadgeEdit, "change it on a group"),
	)
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// writeBadgeRows writes the rows every badge view shares. Every URL is one a
// maintainer typed into the badge, placeholders (%{project_path}) included, and
// GitLab renders the last two by substituting into it, so each is shown as the
// value it is rather than linked.
func writeBadgeRows(c *toolutil.Card, b BadgeItem) {
	c.Field("Link URL", b.LinkURL)
	c.Field("Image URL", b.ImageURL)
	c.Field("Rendered Link", b.RenderedLinkURL)
	c.Field("Rendered Image", b.RenderedImageURL)
	c.Field("Kind", b.Kind)
}

// formatBadgePreview renders a badge preview as the card of what the badge
// would render to. A preview is not a stored badge: GitLab answers it with no
// ID, no name and no kind, so it gets a heading and a hint of its own rather
// than the stored badge's, which named an empty badge and offered to edit a
// badge that does not exist.
func formatBadgePreview(b BadgeItem) *mcp.CallToolResult {
	var sb strings.Builder
	c := toolutil.NewCard(&sb, "Badge Preview")
	c.Field("Link URL", b.LinkURL)
	c.Field("Image URL", b.ImageURL)
	c.Field("Rendered Link", b.RenderedLinkURL)
	c.Field("Rendered Image", b.RenderedImageURL)
	c.End(
		toolutil.HintAction(actionProjectBadgeAdd, "add the badge to a project once the rendered URLs look right"),
		toolutil.HintAction(actionGroupBadgeAdd, "add it to a group instead"),
	)
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
	return formatBadgePreview(out.Badge)
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
	return formatBadgePreview(out.Badge)
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
