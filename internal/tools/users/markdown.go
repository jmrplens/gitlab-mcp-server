// markdown.go provides Markdown formatting functions for user MCP tool output.
package users

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const tblSep5Col = "|---|---|---|---|---|\n"

// FormatMarkdownString renders the authenticated user profile as a Markdown summary.
func FormatMarkdownString(u Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## GitLab User: %s\n\n", toolutil.EscapeMdHeading(u.Name))
	fmt.Fprintf(&b, toolutil.FmtMdID, u.ID)
	fmt.Fprintf(&b, toolutil.FmtMdUsername, u.Username)
	fmt.Fprintf(&b, toolutil.FmtMdEmail, u.Email)
	fmt.Fprintf(&b, toolutil.FmtMdState, u.State)
	if u.Bio != "" {
		fmt.Fprintf(&b, "- **Bio**: %s\n", u.Bio)
	}
	fmt.Fprintf(&b, "- **Admin**: %v\n", u.IsAdmin)
	fmt.Fprintf(&b, toolutil.FmtMdURL, u.WebURL)
	if u.AvatarURL != "" {
		fmt.Fprintf(&b, "- **Avatar**: %s\n", u.AvatarURL)
	}
	toolutil.WriteHints(&b,
		"Use action 'get_status' to check user's current status",
		"Use action 'ssh_keys' to list SSH keys",
	)
	return b.String()
}

// FormatMarkdown renders the user as an MCP CallToolResult.
func FormatMarkdown(u Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(u))
}

// FormatListMarkdownString renders a user list as a Markdown string.
func FormatListMarkdownString(o ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## GitLab Users (%d)\n\n", len(o.Users))
	toolutil.WriteListSummary(&b, len(o.Users), o.Pagination)
	if len(o.Users) == 0 {
		b.WriteString("No users found.\n")
	} else {
		b.WriteString("| ID | Username | Name | Email | State |\n")
		b.WriteString(tblSep5Col)
		for _, u := range o.Users {
			fmt.Fprintf(&b, "| %d | [@%s](%s) | %s | %s | %s |\n",
				u.ID, toolutil.EscapeMdTableCell(u.Username), u.WebURL,
				toolutil.EscapeMdTableCell(u.Name),
				toolutil.EscapeMdTableCell(u.Email), u.State)
		}
	}
	toolutil.WritePagination(&b, o.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'get' with user_id to see full user details",
	)
	return b.String()
}

// FormatListMarkdown renders a user list as an MCP CallToolResult.
func FormatListMarkdown(o ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(o))
}

// FormatStatusMarkdownString renders a user status as a Markdown string.
func FormatStatusMarkdownString(o StatusOutput) string {
	var b strings.Builder
	b.WriteString("## User Status\n\n")
	if o.Emoji != "" {
		fmt.Fprintf(&b, "- **Emoji**: %s\n", o.Emoji)
	}
	if o.Message != "" {
		fmt.Fprintf(&b, "- **Message**: %s\n", o.Message)
	}
	if o.Availability != "" {
		fmt.Fprintf(&b, "- **Availability**: %s\n", o.Availability)
	}
	if o.ClearStatusAt != "" {
		fmt.Fprintf(&b, "- **Clear At**: %s\n", toolutil.FormatTime(o.ClearStatusAt))
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_set_user_status` to update your status",
	)
	return b.String()
}

// FormatStatusMarkdown renders a user status as an MCP CallToolResult.
func FormatStatusMarkdown(o StatusOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatStatusMarkdownString(o))
}

// FormatSSHKeyMarkdownString renders a single SSH key as a Markdown string.
func FormatSSHKeyMarkdownString(o SSHKeyOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## SSH Key: %s\n\n", toolutil.EscapeMdHeading(o.Title))
	fmt.Fprintf(&b, toolutil.FmtMdID, o.ID)
	fmt.Fprintf(&b, "- **Title**: %s\n", o.Title)
	fmt.Fprintf(&b, "- **Key**: `%.40s...`\n", o.Key)
	if o.UsageType != "" {
		fmt.Fprintf(&b, "- **Usage Type**: %s\n", o.UsageType)
	}
	if o.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(o.CreatedAt))
	}
	if o.ExpiresAt != "" {
		fmt.Fprintf(&b, "- **Expires At**: %s\n", toolutil.FormatTime(o.ExpiresAt))
	}
	return b.String()
}

// FormatSSHKeyMarkdown renders a single SSH key as an MCP CallToolResult.
func FormatSSHKeyMarkdown(o SSHKeyOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatSSHKeyMarkdownString(o))
}

// FormatSSHKeyListMarkdownString renders an SSH key list as a Markdown string.
func FormatSSHKeyListMarkdownString(o SSHKeyListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## SSH Keys (%d)\n\n", len(o.Keys))
	toolutil.WriteListSummary(&b, len(o.Keys), o.Pagination)
	if len(o.Keys) == 0 {
		b.WriteString("No SSH keys found.\n")
	} else {
		b.WriteString("| ID | Title | Usage Type | Created At | Expires At |\n")
		b.WriteString(tblSep5Col)
		for _, k := range o.Keys {
			fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n",
				k.ID, toolutil.EscapeMdTableCell(k.Title), k.UsageType, k.CreatedAt, k.ExpiresAt)
		}
	}
	toolutil.WritePagination(&b, o.Pagination)
	toolutil.WriteHints(&b,
		"Use `gitlab_list_ssh_keys` to view all SSH keys",
	)
	return b.String()
}

// FormatSSHKeyListMarkdown renders an SSH key list as an MCP CallToolResult.
func FormatSSHKeyListMarkdown(o SSHKeyListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatSSHKeyListMarkdownString(o))
}

// FormatEmailListMarkdownString renders an email list as a Markdown string.
func FormatEmailListMarkdownString(o EmailListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Email Addresses (%d)\n\n", len(o.Emails))
	if len(o.Emails) == 0 {
		b.WriteString("No email addresses found.\n")
	} else {
		b.WriteString("| ID | Email | Confirmed At |\n")
		b.WriteString("|---|---|---|\n")
		for _, e := range o.Emails {
			fmt.Fprintf(&b, "| %d | %s | %s |\n", e.ID, toolutil.EscapeMdTableCell(e.Email), toolutil.FormatTime(e.ConfirmedAt))
		}
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_user_current` to view your full profile",
	)
	return b.String()
}

// FormatEmailListMarkdown renders an email list as an MCP CallToolResult.
func FormatEmailListMarkdown(o EmailListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatEmailListMarkdownString(o))
}

// FormatContributionEventsMarkdownString renders contribution events as a Markdown string.
func FormatContributionEventsMarkdownString(o ContributionEventsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Contribution Events (%d)\n\n", len(o.Events))
	if len(o.Events) == 0 {
		b.WriteString("No contribution events found.\n")
	} else {
		b.WriteString("| ID | Action | Target Type | Target | Created At |\n")
		b.WriteString(tblSep5Col)
		for _, e := range o.Events {
			target := toolutil.FormatTarget(e.TargetType, e.TargetIID, e.TargetTitle, e.TargetURL)
			fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n",
				e.ID, e.ActionName, e.TargetType, target, e.CreatedAt)
		}
	}
	toolutil.WritePagination(&b, o.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_get_user` to view user profile details",
	)
	return b.String()
}

// FormatContributionEventsMarkdown renders contribution events as an MCP CallToolResult.
func FormatContributionEventsMarkdown(o ContributionEventsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatContributionEventsMarkdownString(o))
}

// FormatAssociationsCountMarkdownString renders user associations count as a Markdown string.
func FormatAssociationsCountMarkdownString(o AssociationsCountOutput) string {
	var b strings.Builder
	b.WriteString("## User Associations Count\n\n")
	fmt.Fprintf(&b, "- **Groups**: %d\n", o.GroupsCount)
	fmt.Fprintf(&b, "- **Projects**: %d\n", o.ProjectsCount)
	fmt.Fprintf(&b, "- **Issues**: %d\n", o.IssuesCount)
	fmt.Fprintf(&b, "- **Merge Requests**: %d\n", o.MergeRequestsCount)
	toolutil.WriteHints(&b,
		"Use `gitlab_get_user` to view the user's profile",
		"Use `gitlab_list_user_contribution_events` to see recent activity",
	)
	return b.String()
}

// FormatAssociationsCountMarkdown renders user associations count as an MCP CallToolResult.
func FormatAssociationsCountMarkdown(o AssociationsCountOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatAssociationsCountMarkdownString(o))
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatStatusMarkdownString)
	toolutil.RegisterMarkdown(FormatSSHKeyListMarkdownString)
	toolutil.RegisterMarkdown(FormatEmailListMarkdownString)
	toolutil.RegisterMarkdown(FormatContributionEventsMarkdownString)
	toolutil.RegisterMarkdown(FormatAssociationsCountMarkdownString)
}
