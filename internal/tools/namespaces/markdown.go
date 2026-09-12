package namespaces

import (
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatListMarkdown formats a list of namespaces as a Markdown CallToolResult.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatListMarkdownString renders a page of namespaces as the table a
// collection of objects sharing columns takes, headed with the count the
// response can vouch for rather than with the number of rows shown.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.Namespaces) == 0 {
		return toolutil.EmptyMessage("namespaces")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Namespaces", len(out.Namespaces), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Kind", "Full Path"))
	for _, ns := range out.Namespaces {
		// For a user namespace GitLab keeps the name in step with the account
		// holder's display name, which is free text; the paths are slugs a
		// person chose.
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(ns.ID, 10),
			toolutil.EscapeMdTableCell(ns.Name),
			toolutil.EscapeMdTableCell(ns.Kind),
			toolutil.MdCodeSpanCell(ns.FullPath),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionNamespaceGet, "view details of one namespace"))
	return b.String()
}

// FormatMarkdown formats a single namespace as a Markdown CallToolResult.
func FormatMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(out))
}

// FormatMarkdownString renders one namespace as a card. Every optional field
// is written only when GitLab sent it: the administrator's figures, the
// compute-minute and storage limits a caller who may change them is shown, and
// the two subscription dates.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	// For a user namespace GitLab keeps the name in step with the account
	// holder's display name, which is free text; the paths are slugs a person
	// chose, and the card escapes each of them.
	c := toolutil.NewCard(&b, "Namespace: "+out.Name)
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Code("Path", out.Path)
	c.Code("Full Path", out.FullPath)
	c.Field("Kind", out.Kind)
	c.Count("Parent ID", out.ParentID)
	// GitLab builds the web URL from the instance URL and the full path above,
	// so it inherits whatever that path can hold.
	c.URL(out.WebURL)
	c.Count("Members Count With Descendants", out.MembersCountWithDescendants)
	c.Count("Projects Count", out.ProjectsCount)
	c.Count("Root Repository Size", out.RootRepositorySize)
	c.Count("Billable Members Count", out.BillableMembersCount)
	c.Field("Plan", out.Plan)
	c.Flag(toolutil.EmojiInfo, "Trial", out.Trial)
	c.Time("Trial Ends On", out.TrialEndsOn)
	c.Time("Subscription End Date", out.EndDate)
	writeSeatRows(c, out)
	c.End(toolutil.HintAction(actionNamespaceGet, "use this namespace ID with the project and group actions"))
	return b.String()
}

// writeSeatRows writes the seat, compute-minute and purchased-storage rows,
// each present only for a caller GitLab shows it to.
func writeSeatRows(c *toolutil.Card, out Output) {
	writeCountPtr(c, "Max Seats Used", out.MaxSeatsUsed)
	c.Time("Max Seats Used Changed At", out.MaxSeatsUsedChangedAt)
	writeCountPtr(c, "Seats In Use", out.SeatsInUse)
	writeCountPtr(c, "Shared Runners Minutes Limit", out.SharedRunnersMinutesLimit)
	writeCountPtr(c, "Extra Shared Runners Minutes Limit", out.ExtraSharedRunnersMinutesLimit)
	writeCountPtr(c, "Additional Purchased Storage Size", out.AdditionalPurchasedStorageSize)
	c.Time("Additional Purchased Storage Ends On", out.AdditionalPurchasedStorageEndsOn)
}

// writeCountPtr writes a figure GitLab sends only to some callers: nil is "not
// shown to you", and zero is an answer.
func writeCountPtr(c *toolutil.Card, label string, v *int64) {
	if v == nil {
		return
	}
	c.Int(label, *v)
}

// FormatExistsMarkdown formats a namespace existence check as a Markdown CallToolResult.
func FormatExistsMarkdown(out ExistsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatExistsMarkdownString(out))
}

// FormatExistsMarkdownString renders the availability answer as a card, with
// the alternative paths GitLab offered as the collection they are. The
// suggestions used to be joined into one line as they arrived, so a suggestion
// carrying Markdown was rendered as Markdown.
func FormatExistsMarkdownString(out ExistsOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Namespace Availability")
	c.Bool("Exists", out.Exists)
	c.Field("Path", pathState(out.Exists))
	if len(out.Suggests) > 0 {
		suggestions := c.Table("Suggested Paths", "Path")
		for _, suggestion := range out.Suggests {
			suggestions.Row(toolutil.MdCodeSpanCell(suggestion))
		}
		c.End("Try one of the suggested paths: the one asked about is taken")
		return b.String()
	}
	c.End()
	return b.String()
}

// pathState says what the existence answer means for a caller choosing a path.
func pathState(exists bool) string {
	if exists {
		return "taken"
	}
	return "available"
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatExistsMarkdownString)
}
