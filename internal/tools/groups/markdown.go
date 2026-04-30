// markdown.go provides Markdown formatting functions for group MCP tool output.
package groups

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single group as a Markdown summary.
func FormatOutputMarkdown(g Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group: %s\n\n", toolutil.EscapeMdHeading(g.Name))
	fmt.Fprintf(&b, toolutil.FmtMdID, g.ID)
	fmt.Fprintf(&b, toolutil.FmtMdPath, g.FullPath)
	if g.FullName != "" {
		fmt.Fprintf(&b, "- **Full Name**: %s\n", g.FullName)
	}
	fmt.Fprintf(&b, toolutil.FmtMdVisibility, g.Visibility)
	if g.Description != "" {
		fmt.Fprintf(&b, toolutil.FmtMdDescription, g.Description)
	}
	fmt.Fprintf(&b, toolutil.FmtMdURL, g.WebURL)
	if g.ParentID != 0 {
		fmt.Fprintf(&b, "- **Parent ID**: %d\n", g.ParentID)
	}
	if g.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(g.CreatedAt))
	}
	if g.MarkedForDeletion != "" {
		fmt.Fprintf(&b, "- %s **Marked for deletion**: %s\n", toolutil.EmojiWarning, g.MarkedForDeletion)
	}
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'projects' to see projects in this group",
		"Use action 'members' to see group members",
	)
	return b.String()
}

// FormatListMarkdown renders a list of groups as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Groups (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Groups), out.Pagination)
	if len(out.Groups) == 0 {
		b.WriteString("No groups found.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | Path | Visibility |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, g := range out.Groups {
		fmt.Fprintf(&b, "| %d | %s | %s | %s |\n", g.ID, toolutil.EscapeMdTableCell(g.Name), toolutil.EscapeMdTableCell(g.FullPath), g.Visibility)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'get' with a group_id to see group details",
		"Use action 'projects' to see projects in a group",
	)
	return b.String()
}

// FormatMemberListMarkdown renders a list of group members as a Markdown table.
func FormatMemberListMarkdown(out MemberListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Members (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Members), out.Pagination)
	if len(out.Members) == 0 {
		b.WriteString("No members found.\n")
		return b.String()
	}
	b.WriteString("| Username | Name | Access Level | State |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, m := range out.Members {
		fmt.Fprintf(&b, toolutil.FmtRow4Str, toolutil.EscapeMdTableCell(m.Username), toolutil.EscapeMdTableCell(m.Name), toolutil.EscapeMdTableCell(m.AccessLevelDescription), m.State)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use `gitlab_group_member_add` to add a new member",
		"Use `gitlab_group_member_edit` to change access level",
	)
	return b.String()
}

// FormatListProjectsMarkdown renders a list of group projects as a Markdown table.
func FormatListProjectsMarkdown(out ListProjectsOutput) string {
	var b strings.Builder
	if len(out.Projects) == 0 {
		b.WriteString("No projects found.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | Path | Visibility | Archived |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, p := range out.Projects {
		archived := "No"
		if p.Archived {
			archived = "Yes"
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n",
			p.ID,
			toolutil.EscapeMdTableCell(p.Name),
			toolutil.EscapeMdTableCell(p.PathWithNamespace),
			p.Visibility,
			archived,
		)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use `gitlab_project_get` to view project details",
		"Use `gitlab_project_create` to add a new project to this group",
	)
	return b.String()
}

// FormatHookMarkdown renders a single group hook as a Markdown summary.
func FormatHookMarkdown(h HookOutput) string {
	var b strings.Builder
	title := h.URL
	if h.Name != "" {
		title = h.Name
	}
	fmt.Fprintf(&b, "## Group Hook: %s\n\n", toolutil.EscapeMdHeading(title))
	fmt.Fprintf(&b, toolutil.FmtMdID, h.ID)
	fmt.Fprintf(&b, toolutil.FmtMdURL, h.URL)
	if h.Name != "" {
		fmt.Fprintf(&b, toolutil.FmtMdName, h.Name)
	}
	if h.Description != "" {
		fmt.Fprintf(&b, toolutil.FmtMdDescription, h.Description)
	}
	fmt.Fprintf(&b, "- **Group ID**: %d\n", h.GroupID)
	fmt.Fprintf(&b, "- **SSL Verification**: %v\n", h.EnableSSLVerification)
	fmt.Fprintf(&b, "- **Events**: %s\n", enabledEvents(h))
	if h.AlertStatus != "" {
		fmt.Fprintf(&b, "- **Alert Status**: %s\n", h.AlertStatus)
	}
	if h.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(h.CreatedAt))
	}
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_group_hook_edit` to modify this hook",
		"Use `gitlab_group_hook_delete` to remove it",
	)
	return b.String()
}

// FormatHookListMarkdown renders a paginated list of group hooks as a Markdown table.
func FormatHookListMarkdown(out HookListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Hooks (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Hooks), out.Pagination)
	if len(out.Hooks) == 0 {
		b.WriteString("No group webhooks found.\n")
		return b.String()
	}
	b.WriteString("| ID | URL | Events | SSL |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, h := range out.Hooks {
		ssl := "No"
		if h.EnableSSLVerification {
			ssl = "Yes"
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s |\n", h.ID, toolutil.MdTitleLink(toolutil.EscapeMdTableCell(h.URL), h.URL), toolutil.EscapeMdTableCell(enabledEvents(h)), ssl)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_group_hook_get` to view hook details",
		"Use `gitlab_group_hook_add` to add a new hook",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatMemberListMarkdown)
	toolutil.RegisterMarkdown(FormatListProjectsMarkdown)
	toolutil.RegisterMarkdown(FormatHookMarkdown)
	toolutil.RegisterMarkdown(FormatHookListMarkdown)
}
