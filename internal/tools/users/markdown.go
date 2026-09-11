package users

import (
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The next steps a user result offers, each naming the canonical catalog ID
// every surface accepts rather than an individual tool name the default
// surface does not register.
var (
	hintGetUser           = toolutil.HintAction(actionUserGet, "see full user details")
	hintUserProfile       = toolutil.HintAction(actionUserGet, "view the user's profile")
	hintUserStatus        = toolutil.HintAction("user.get_status", "check the user's current status")
	hintSetStatus         = toolutil.HintAction("user.set_status", "update your status")
	hintListSSHKeys       = toolutil.HintAction(actionUserSSHKeys, "list the account's SSH keys")
	hintGetSSHKey         = toolutil.HintAction(actionUserGetSSHKey, "view one key in full")
	hintCurrentUser       = toolutil.HintAction(actionUserCurrent, "view your full profile")
	hintContributionEvent = toolutil.HintAction("user.contribution_events", "see recent activity")
	hintUserDetails       = toolutil.HintAction(actionUserGet, "view full details for a user")

	hintCreateServiceAccount = toolutil.HintAction("user.create_service_account", "add a service account")
)

// sshKeyPreviewRunes is how much of a public key the card shows: the algorithm
// name and the start of the base64 body, which identify the key, and short of
// the free-text comment its owner may have put at the end.
const sshKeyPreviewRunes = 40

type userNotFoundOutput struct {
	Identifier string `json:"identifier"`
}

func formatUserNotFound(out userNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult("User", out.Identifier,
		"Use gitlab_list_users to search users by username or email",
		"The user may have been blocked or deleted")
}

// FormatMarkdownString renders a user as a card.
//
// The avatar upload answers with an avatar URL and nothing else on GitLab 19,
// so a response carrying no identity is rendered as the avatar result it is:
// the user card used to print an ID of zero, an empty email and an empty
// address for it, which reads as a user whose profile GitLab lost rather than
// as the narrow answer it is.
func FormatMarkdownString(u Output) string {
	var b strings.Builder
	if u.ID == 0 && u.Username == "" && u.AvatarURL != "" {
		card := toolutil.NewCard(&b, "Avatar Updated")
		card.Link("Avatar", u.AvatarURL, u.AvatarURL)
		card.End(u.NextSteps...)
		return b.String()
	}
	card := toolutil.NewCard(&b, "GitLab User: "+u.Name)
	card.Int("ID", u.ID)
	card.Field("Username", u.Username)
	// GitLab validates an address with a regexp that excludes whitespace and a
	// second '@' and admits both '|' and '<'.
	card.Field("Email", u.Email)
	card.Field("State", u.State)
	// A bio is free profile text, and GitLab allows newlines in it.
	card.Text("Bio", u.Bio)
	card.Bool("Admin", u.IsAdmin)
	card.Bool("Bot", u.Bot)
	card.Bool("External", u.External)
	card.Warn("Locked", u.Locked)
	card.URL(u.WebURL)
	card.Link("Avatar", u.AvatarURL, u.AvatarURL)
	writeSCIMIdentities(card, u.SCIMIdentities)
	card.End(hintUserStatus, hintListSSHKeys)
	return b.String()
}

// writeSCIMIdentities writes the user's SCIM identities as the nested
// collection they are, under a heading of their own.
func writeSCIMIdentities(card *toolutil.Card, identities []SCIMIdentityOutput) {
	if len(identities) == 0 {
		return
	}
	table := card.Table("SCIM Identities", "Extern UID", "Group ID", "Active")
	for _, identity := range identities {
		table.Row(
			toolutil.EscapeMdTableCell(identity.ExternUID),
			strconv.FormatInt(identity.GroupID, 10),
			toolutil.BoolEmoji(identity.Active),
		)
	}
}

// FormatMarkdown renders the user as an MCP CallToolResult.
func FormatMarkdown(u Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(u))
}

// FormatListMarkdownString renders a user list as a Markdown string.
func FormatListMarkdownString(o ListOutput) string {
	if len(o.Users) == 0 {
		return toolutil.EmptyMessage("users")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "GitLab Users", len(o.Users), o.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Username", "Name", "Email", "State"))
	for _, u := range o.Users {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(u.ID, 10),
			toolutil.MdUserLink(u.Username, u.WebURL),
			toolutil.EscapeMdTableCell(u.Name),
			toolutil.EscapeMdTableCell(u.Email),
			toolutil.EscapeMdTableCell(u.State),
		))
	}
	toolutil.WriteListFooter(&b, o.Pagination, true, hintGetUser)
	return b.String()
}

// FormatListMarkdown renders a user list as an MCP CallToolResult.
func FormatListMarkdown(o ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(o))
}

// FormatStatusMarkdownString renders a user status as a Markdown string.
func FormatStatusMarkdownString(o StatusOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "User Status")
	card.Field("Emoji", o.Emoji)
	card.Field("Message", o.Message)
	card.Field("Availability", o.Availability)
	card.Time("Clear At", o.ClearStatusAt)
	card.End(hintSetStatus)
	return b.String()
}

// FormatStatusMarkdown renders a user status as an MCP CallToolResult.
func FormatStatusMarkdown(o StatusOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatStatusMarkdownString(o))
}

// FormatSSHKeyMarkdownString renders a single SSH key as a Markdown string.
func FormatSSHKeyMarkdownString(o SSHKeyOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "SSH Key: "+o.Title)
	card.Int("ID", o.ID)
	card.Field("Title", o.Title)
	card.Code("Key", sshKeyPreview(o.Key))
	card.Field("Usage Type", o.UsageType)
	card.Time("Created", o.CreatedAt)
	card.Time("Expires At", o.ExpiresAt)
	card.End()
	return b.String()
}

// sshKeyPreview is the head of a public key, ellipsized only when there is
// more of it: the line used to append the ellipsis to every key, including one
// shorter than the cut.
func sshKeyPreview(key string) string {
	runes := []rune(key)
	if len(runes) <= sshKeyPreviewRunes {
		return key
	}
	return string(runes[:sshKeyPreviewRunes]) + "..."
}

// FormatSSHKeyMarkdown renders a single SSH key as an MCP CallToolResult.
func FormatSSHKeyMarkdown(o SSHKeyOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatSSHKeyMarkdownString(o))
}

// FormatSSHKeyListMarkdownString renders an SSH key list as a Markdown string.
func FormatSSHKeyListMarkdownString(o SSHKeyListOutput) string {
	if len(o.Keys) == 0 {
		return toolutil.EmptyMessage("SSH keys")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "SSH Keys", len(o.Keys), o.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Title", "Usage Type", "Created At", "Expires At"))
	for _, k := range o.Keys {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(k.ID, 10),
			toolutil.EscapeMdTableCell(k.Title),
			toolutil.EscapeMdTableCell(k.UsageType),
			toolutil.FormatTime(k.CreatedAt),
			toolutil.FormatTime(k.ExpiresAt),
		))
	}
	toolutil.WriteListFooter(&b, o.Pagination, false, hintGetSSHKey)
	return b.String()
}

// FormatSSHKeyListMarkdown renders an SSH key list as an MCP CallToolResult.
func FormatSSHKeyListMarkdown(o SSHKeyListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatSSHKeyListMarkdownString(o))
}

// FormatEmailListMarkdownString renders an email list as a Markdown string.
func FormatEmailListMarkdownString(o EmailListOutput) string {
	if len(o.Emails) == 0 {
		return toolutil.EmptyMessage("email addresses")
	}
	var b strings.Builder
	var pagination toolutil.PaginationOutput
	toolutil.WriteListHeading(&b, "Email Addresses", len(o.Emails), pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Email", "Confirmed"))
	for _, e := range o.Emails {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(e.ID, 10),
			toolutil.EscapeMdTableCell(e.Email),
			confirmationValue(e.ConfirmedAt),
		))
	}
	toolutil.WriteListFooter(&b, pagination, false, hintCurrentUser)
	return b.String()
}

// confirmationValue renders the confirmation state of an address: the instant
// GitLab confirmed it, or the cross and the reason when it never did, since an
// address waiting for its confirmation mail is the answer a reader is asking
// for and an empty cell said nothing.
func confirmationValue(confirmedAt string) string {
	if confirmedAt == "" {
		return toolutil.BoolEmoji(false) + " awaiting confirmation"
	}
	return toolutil.BoolEmoji(true) + " " + toolutil.FormatTime(confirmedAt)
}

// FormatEmailListMarkdown renders an email list as an MCP CallToolResult.
func FormatEmailListMarkdown(o EmailListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatEmailListMarkdownString(o))
}

// FormatContributionEventsMarkdownString renders contribution events as a Markdown string.
func FormatContributionEventsMarkdownString(o ContributionEventsOutput) string {
	if len(o.Events) == 0 {
		return toolutil.EmptyMessage("contribution events")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Contribution Events", len(o.Events), o.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Action", "Target Type", "Target", "Created At"))
	linked := false
	for _, e := range o.Events {
		target := toolutil.FormatTarget(e.TargetType, e.TargetIID, e.TargetTitle, e.TargetURL)
		if target != "" && e.TargetURL != "" {
			linked = true
		}
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(e.ID, 10),
			toolutil.EscapeMdTableCell(e.ActionName),
			toolutil.EscapeMdTableCell(e.TargetType),
			target,
			toolutil.FormatTime(e.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, o.Pagination, linked, hintUserProfile)
	return b.String()
}

// FormatContributionEventsMarkdown renders contribution events as an MCP CallToolResult.
func FormatContributionEventsMarkdown(o ContributionEventsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatContributionEventsMarkdownString(o))
}

// FormatAssociationsCountMarkdownString renders user associations count as a Markdown string.
func FormatAssociationsCountMarkdownString(o AssociationsCountOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "User Associations Count")
	card.Int("Groups", o.GroupsCount)
	card.Int("Projects", o.ProjectsCount)
	card.Int("Issues", o.IssuesCount)
	card.Int("Merge Requests", o.MergeRequestsCount)
	card.End(hintUserProfile, hintContributionEvent)
	return b.String()
}

// FormatAssociationsCountMarkdown renders user associations count as an MCP CallToolResult.
func FormatAssociationsCountMarkdown(o AssociationsCountOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatAssociationsCountMarkdownString(o))
}

// FormatDeleteUserMarkdownString renders user deletion output as Markdown.
func FormatDeleteUserMarkdownString(o DeleteOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "User Deleted")
	card.Int("ID", o.UserID)
	card.Bool("Deleted", o.Deleted)
	card.End()
	return b.String()
}

// FormatDeleteSSHKeyMarkdownString renders SSH key deletion output as Markdown.
func FormatDeleteSSHKeyMarkdownString(o DeleteSSHKeyOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "SSH Key Deleted")
	card.Int("ID", o.KeyID)
	card.Bool("Deleted", o.Deleted)
	card.End()
	return b.String()
}

func init() {
	toolutil.RegisterMarkdownResult(formatUserNotFound)
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatStatusMarkdownString)
	toolutil.RegisterMarkdown(FormatSSHKeyMarkdownString)
	toolutil.RegisterMarkdown(FormatSSHKeyListMarkdownString)
	toolutil.RegisterMarkdown(FormatEmailListMarkdownString)
	toolutil.RegisterMarkdown(FormatContributionEventsMarkdownString)
	toolutil.RegisterMarkdown(FormatAssociationsCountMarkdownString)
	toolutil.RegisterMarkdown(FormatAdminActionMarkdownString)
	toolutil.RegisterMarkdown(FormatDeleteUserMarkdownString)
	toolutil.RegisterMarkdown(FormatDeleteSSHKeyMarkdownString)
	toolutil.RegisterMarkdown(FormatUserActivitiesMarkdownString)
	toolutil.RegisterMarkdown(FormatUserMembershipsMarkdownString)
	toolutil.RegisterMarkdown(FormatUserRunnerMarkdownString)
	toolutil.RegisterMarkdown(FormatDeleteUserIdentityMarkdownString)
	toolutil.RegisterMarkdown(FormatServiceAccountMarkdownString)
	toolutil.RegisterMarkdown(FormatServiceAccountListMarkdownString)
	toolutil.RegisterMarkdown(FormatCurrentUserPATMarkdownString)
}
