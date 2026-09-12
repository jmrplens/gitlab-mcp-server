package todos

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionList        = "todo.list"
	actionMarkDone    = "todo.mark_done"
	actionMarkAllDone = "todo.mark_all_done"
)

// targetTitle returns the to-do target's title, or "" when no target is set.
func targetTitle(t Output) string {
	if t.Target == nil {
		return ""
	}
	return t.Target.Title
}

// targetIID returns the to-do target's IID, or 0 when no target is set. It is
// what names the target when GitLab sent no title, so a to-do about an issue
// reads "Issue #4" rather than as a link with nothing in it.
func targetIID(t Output) int64 {
	if t.Target == nil {
		return 0
	}
	return t.Target.IID
}

// targetState returns the state of the issue or merge request the to-do points
// at, or "" when there is no target. A to-do whose target is already closed is
// the commonest one to skip, and the state is what says so.
func targetState(t Output) string {
	if t.Target == nil {
		return ""
	}
	return t.Target.State
}

// targetCell renders the to-do's target as its title linked to the target,
// falling back to "<type> #<iid>" as the label when GitLab sent no title, and
// to nothing at all when it sent neither. It used to be built as a link whose
// label was the title straight from Target, which rendered "[](url)" for every
// to-do GitLab sent a target URL and no target object for.
func targetCell(t Output) string {
	return toolutil.FormatTarget(string(t.TargetType), targetIID(t), targetTitle(t), t.TargetURL)
}

// scopeLabel and scopeValue name where the to-do was raised: the project path
// for a project to-do, and the group path for one raised in a group, which is
// the only shape that carries no project at all.
//
// The row used to print Project.Name, which GitLab does not send on the to-do
// entity, so every to-do rendered "**Project:**" with nothing after it.
func scopeLabel(t Output) string {
	if t.Project == nil && t.Group != nil {
		return "Group"
	}
	return "Project"
}

func scopeValue(t Output) string {
	if t.Project != nil {
		if t.Project.PathWithNamespace != "" {
			return t.Project.PathWithNamespace
		}
		return t.Project.Name
	}
	if t.Group != nil {
		if t.Group.FullPath != "" {
			return t.Group.FullPath
		}
		return t.Group.Name
	}
	return ""
}

// authorCell renders the to-do's author as the handle linked to the profile,
// or nothing when GitLab sent no author.
func authorCell(t Output) string {
	if t.Author == nil {
		return ""
	}
	return toolutil.MdUserLink(t.Author.Username, t.Author.WebURL)
}

// FormatOutputMarkdownString renders one to-do item as the card of one object.
//
// It used to write four bullet-less "**Label:** value" lines, which a Markdown
// renderer joins into one run-on paragraph, and then appended the to-do body at
// block level, where a body opening with "## " added a heading to the response
// and a body opening with "- " added items to it. Every row is a card row now,
// and the body is the card's long text, which is quoted.
func FormatOutputMarkdownString(t Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("To-Do #%d", t.ID))
	c.Field("Action", string(t.ActionName))
	target := targetCell(t)
	c.Markdown("Target", target)
	c.Field("Target Type", string(t.TargetType))
	c.Field("Target State", targetState(t))
	if target == "" {
		// The target object is absent but GitLab still said where the to-do
		// points, so the address is the card's own row rather than nothing.
		c.URL(t.TargetURL)
	}
	c.Field("State", t.State)
	c.Field(scopeLabel(t), scopeValue(t))
	c.Markdown("Author", authorCell(t))
	c.Time("Created", t.CreatedAt)
	c.Time("Updated", t.UpdatedAt)
	c.Text("Body", t.Body)
	c.End(
		toolutil.HintAction(actionMarkDone, "mark this to-do item as done"),
		toolutil.HintAction(actionList, "see the rest of your to-do items"),
	)
	return b.String()
}

// FormatOutputMarkdown returns an MCP tool result for a single to-do item.
func FormatOutputMarkdown(t Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatOutputMarkdownString(t))
}

// FormatListMarkdownString renders a page of to-do items as a Markdown table:
// a collection of objects that share columns.
func FormatListMarkdownString(v ListOutput) string {
	if len(v.Todos) == 0 {
		return toolutil.EmptyMessage("to-do items")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "To-Do Items", len(v.Todos), v.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Action", "Target", "Type", "State", "Project"))
	for _, t := range v.Todos {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(t.ID, 10),
			toolutil.EscapeMdTableCell(string(t.ActionName)),
			targetCell(t),
			toolutil.EscapeMdTableCell(string(t.TargetType)),
			toolutil.EscapeMdTableCell(t.State),
			toolutil.EscapeMdTableCell(scopeValue(t)),
		))
	}
	toolutil.WriteListFooter(&b, v.Pagination, true,
		toolutil.HintAction(actionMarkDone, "mark one to-do item as done"),
		toolutil.HintAction(actionMarkAllDone, "clear every pending to-do item"),
	)
	return b.String()
}

// FormatListMarkdown returns an MCP tool result for a to-do list.
func FormatListMarkdown(v ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(v))
}

// confirmation renders the one-line confirmation a void result carries.
//
// The message is the handler's own sentence, built from the to-do ID the caller
// passed, so escaping it changes nothing GitLab can influence; it is escaped
// anyway because the value reaches here as a field of a published output type,
// and one line that cannot open a block is what a confirmation is.
func confirmation(message string) string {
	return toolutil.EmojiSuccess + " " + toolutil.EscapeMdTableCell(message) + "\n"
}

// FormatMarkDoneMarkdownString formats a mark-done result as Markdown.
func FormatMarkDoneMarkdownString(v MarkDoneOutput) string {
	var b strings.Builder
	b.WriteString(confirmation(v.Message))
	toolutil.WriteHints(&b, toolutil.HintAction(actionList, "see the remaining to-do items"))
	return b.String()
}

// FormatMarkDoneMarkdown returns an MCP tool result for marking a to-do as done.
func FormatMarkDoneMarkdown(v MarkDoneOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkDoneMarkdownString(v))
}

// FormatMarkAllDoneMarkdownString formats a mark-all-done result as Markdown.
func FormatMarkAllDoneMarkdownString(v MarkAllDoneOutput) string {
	var b strings.Builder
	b.WriteString(confirmation(v.Message))
	toolutil.WriteHints(&b, toolutil.HintAction(actionList, "confirm there is nothing left pending"))
	return b.String()
}

// FormatMarkAllDoneMarkdown returns an MCP tool result for marking all to-dos as done.
func FormatMarkAllDoneMarkdown(v MarkAllDoneOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkAllDoneMarkdownString(v))
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkDoneMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkAllDoneMarkdownString)
}
