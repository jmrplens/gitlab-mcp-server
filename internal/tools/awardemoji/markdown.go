package awardemoji

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// awardEmojiHint is the one next step an award emoji result offers: the delete
// action, which every surface spells with its own tool name.
const awardEmojiHint = "Use the selected tool surface's matching award emoji delete action with award_id, the same resource identifiers, and explicit confirm=true"

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

// FormatListMarkdownString renders award emoji as a Markdown table, the shape a
// collection of objects sharing columns takes. The heading counts what the
// response vouches for rather than the length of the page, and the pagination
// footer is written after the rows rather than against them.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.AwardEmoji) == 0 {
		return toolutil.EmptyMessage("award emoji")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Award Emoji", len(out.AwardEmoji), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Emoji", "User", "Awarded"))
	linked := false
	for _, e := range out.AwardEmoji {
		linked = linked || (e.User != nil && e.User.WebURL != "")
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(e.ID, 10),
			emojiCell(e.Name),
			awardEmojiUserMarkdown(e),
			toolutil.FormatTime(e.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, linked, awardEmojiHint)
	return b.String()
}

// emojiCell renders an emoji name in GitLab's own ":name:" spelling, or nothing
// for an award whose name is missing, so a cell never reads "::". An emoji name
// is not held to GitLab's own list: an instance may carry custom emoji whose
// name a person chose.
func emojiCell(name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	return ":" + toolutil.EscapeMdTableCell(name) + ":"
}

// awardEmojiUserMarkdown renders the awarding user as a cell: a link to their
// profile when GitLab gave one, the escaped handle otherwise, and nothing for an
// award whose user GitLab did not send. The result is written as it is, because
// the cell escaper turns a finished link back into text.
func awardEmojiUserMarkdown(out Output) string {
	if out.User == nil {
		return ""
	}
	return toolutil.MdUserLink(out.User.Username, out.User.WebURL)
}

// FormatMarkdown formats a single award emoji as a Markdown CallToolResult.
func FormatMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(out))
}

// FormatMarkdownString renders a single award emoji as a card: the emoji, what
// it was awarded on, who awarded it and when. The awardable is in the heading
// and in a row of its own because an award emoji is meaningless without it: the
// card used to name the emoji and the user and never say what had been reacted
// to, which is the one identifier a reader needs to find it again.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, awardEmojiHeading(out))
	c.Int("ID", out.ID)
	c.Markdown("Name", emojiCell(out.Name))
	c.Markdown("Awarded On", awardableCell(out))
	if out.User != nil {
		user := c.Sub("User")
		user.Int("ID", out.User.ID)
		user.Field("Name", out.User.Name)
		user.Markdown("Username", toolutil.MdUserLink(out.User.Username, out.User.WebURL))
	}
	c.Time("Created", out.CreatedAt)
	c.End(awardEmojiHint)
	return b.String()
}

// awardEmojiHeading composes the card's heading from the emoji and the object it
// sits on, "Award Emoji :thumbsup: on Issue 42". The parts are written as GitLab
// sent them: [toolutil.NewCard] escapes the whole composition.
func awardEmojiHeading(out Output) string {
	heading := "Award Emoji"
	if name := strings.TrimSpace(out.Name); name != "" {
		heading += " :" + name + ":"
	}
	if on := awardableText(out); on != "" {
		heading += " on " + on
	}
	return heading
}

// awardableCell renders the awardable as a card value, "Issue (ID 42)": the
// type GitLab names the object with, and the ID every award emoji action takes.
func awardableCell(out Output) string {
	kind := toolutil.EscapeMdTableCell(strings.TrimSpace(out.AwardableType))
	switch {
	case kind == "" && out.AwardableID == 0:
		return ""
	case kind == "":
		return "ID " + strconv.FormatInt(out.AwardableID, 10)
	case out.AwardableID == 0:
		return kind
	default:
		return fmt.Sprintf("%s (ID %d)", kind, out.AwardableID)
	}
}

// awardableText is the heading form of the awardable, escaped by the card's own
// heading escaper rather than here.
func awardableText(out Output) string {
	kind := strings.TrimSpace(out.AwardableType)
	switch {
	case kind == "" && out.AwardableID == 0:
		return ""
	case kind == "":
		return strconv.FormatInt(out.AwardableID, 10)
	case out.AwardableID == 0:
		return kind
	default:
		return kind + " " + strconv.FormatInt(out.AwardableID, 10)
	}
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdownResult(formatAwardEmojiNotFound)
}
