package epics

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// userName returns the username of a nested user object, or "" when nil.
func userName(u *BasicUserOutput) string {
	if u == nil {
		return ""
	}
	return u.Username
}

// userNames maps a slice of nested user objects to their usernames, skipping
// nil entries.
func userNames(users []*BasicUserOutput) []string {
	if len(users) == 0 {
		return nil
	}
	names := make([]string, 0, len(users))
	for _, u := range users {
		if u != nil {
			names = append(names, u.Username)
		}
	}
	return names
}

// FormatOutputMarkdown renders a single epic as a Markdown summary.
func FormatOutputMarkdown(e Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Epic &%d: %s\n\n", e.IID, toolutil.EscapeMdTableCell(e.Title))
	//gitlab:allow-unescaped e.State: an epic state GitLab writes, opened or closed on the REST epic and OPEN or CLOSED on the work item.
	fmt.Fprintf(&b, toolutil.FmtMdState, e.State)
	if e.Status != "" {
		// The status widget carries the display name of a status in the
		// namespace's lifecycle, which a group owner can create and rename.
		fmt.Fprintf(&b, "- **Status**: %s\n", toolutil.EscapeMdTableCell(e.Status))
	}
	fmt.Fprintf(&b, toolutil.FmtMdAuthor, toolutil.EscapeMdTableCell(userName(e.Author)))
	if len(e.Assignees) > 0 {
		fmt.Fprintf(&b, "- **Assignees**: %s\n", toolutil.EscapeMdTableCell(strings.Join(userNames(e.Assignees), ", ")))
	}
	if e.Confidential {
		b.WriteString("- **Confidential**: yes\n")
	}
	if len(e.Labels) > 0 {
		// A label title is free text: GitLab's only rule on one is that it
		// carries no comma.
		fmt.Fprintf(&b, "- **Labels**: %s\n", toolutil.EscapeMdTableCell(strings.Join(e.Labels, ", ")))
	}
	if e.HealthStatus != "" {
		//gitlab:allow-unescaped e.HealthStatus: the GraphQL HealthStatus enum, one of onTrack, needsAttention and atRisk.
		fmt.Fprintf(&b, "- **Health**: %s\n", e.HealthStatus)
	}
	if e.Weight != nil {
		fmt.Fprintf(&b, "- **Weight**: %d\n", *e.Weight)
	}
	if e.StartDate != "" {
		//gitlab:allow-unescaped e.StartDate: a date this package wrote itself, with time.Format on the DateOnly layout.
		fmt.Fprintf(&b, "- **Start date**: %s\n", e.StartDate)
	}
	if e.DueDate != "" {
		//gitlab:allow-unescaped e.DueDate: a date this package wrote itself, with time.Format on the DateOnly layout.
		fmt.Fprintf(&b, "- **Due date**: %s\n", e.DueDate)
	}
	if e.Color != "" {
		//gitlab:allow-unescaped e.Color: a hex code or a CSS color name, which GitLab validates before it stores it.
		fmt.Fprintf(&b, "- **Color**: %s\n", e.Color)
	}
	if e.ParentIID > 0 {
		fmt.Fprintf(&b, "- **Parent**: &%d (%s)\n", e.ParentIID, toolutil.EscapeMdTableCell(e.ParentPath))
	}
	fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(e.CreatedAt))
	if e.ClosedAt != "" {
		fmt.Fprintf(&b, "- **Closed**: %s\n", toolutil.FormatTime(e.ClosedAt))
	}
	if e.WebURL != "" {
		toolutil.WriteMdURL(&b, e.WebURL)
	}
	if len(e.LinkedItems) > 0 {
		b.WriteString("\n### Linked Items\n\n")
		b.WriteString("| IID | Link Type | Path |\n")
		b.WriteString("| --- | --- | --- |\n")
		for _, li := range e.LinkedItems {
			//gitlab:allow-unescaped li.LinkType: a link type the Work Items API documents, one of blocks, is_blocked_by and relates_to.
			fmt.Fprintf(&b, "| %d | %s | %s |\n", li.IID, li.LinkType, toolutil.EscapeMdTableCell(li.Path))
		}
	}
	if e.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", toolutil.WrapGFMBody(e.Description))
	}
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use action 'update' with iid to modify this epic",
		"Use action 'epic_get_links' with iid to see child epics",
		"Use gitlab_epic_note_list to see comments on this epic",
	)
	return b.String()
}

// FormatListMarkdown renders a list of epics as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Epics (%d)\n\n", len(out.Epics))
	if len(out.Epics) == 0 {
		b.WriteString("No epics found.\n")
		return b.String()
	}
	b.WriteString("| IID | Title | State | Author | Labels | Created |\n")
	b.WriteString(toolutil.TblSep6Col)
	for _, e := range out.Epics {
		labels := ""
		if len(e.Labels) > 0 {
			labels = strings.Join(e.Labels, ", ")
		}
		fmt.Fprintf(
			&b, "| &%d | %s | %s | %s | %s | %s |\n",
			e.IID,
			toolutil.MdTitleLink(toolutil.EscapeMdTableCell(e.Title), e.WebURL),
			e.State,
			toolutil.EscapeMdTableCell(userName(e.Author)),
			toolutil.EscapeMdTableCell(labels),
			toolutil.FormatTime(e.CreatedAt),
		)
	}
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use action 'get' with iid to see full details",
		"Use action 'create' to add a new epic",
	)
	return b.String()
}

// FormatLinksMarkdown renders child epics as a Markdown table.
func FormatLinksMarkdown(out LinksOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Child Epics (%d)\n\n", len(out.ChildEpics))
	if len(out.ChildEpics) == 0 {
		b.WriteString("No child epics found.\n")
		return b.String()
	}
	b.WriteString("| IID | Title | State | Author | Created |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, e := range out.ChildEpics {
		fmt.Fprintf(
			&b, "| &%d | %s | %s | %s | %s |\n",
			e.IID,
			toolutil.MdTitleLink(toolutil.EscapeMdTableCell(e.Title), e.WebURL),
			e.State,
			toolutil.EscapeMdTableCell(userName(e.Author)),
			toolutil.FormatTime(e.CreatedAt),
		)
	}
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use action 'get' with iid to see child epic details",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatLinksMarkdown)
}
