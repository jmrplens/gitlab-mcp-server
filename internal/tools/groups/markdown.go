package groups

import (
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The routes these cards and lists point at, by the canonical catalog ID every
// surface resolves: the dynamic surface executes it, and the meta and
// individual surfaces resolve it to their own tool names, so a hint written
// this way never names something the serving surface does not register. The
// member routes live in internal/tools/groupmembers and the project ones in
// internal/tools/projects; both are actions of a catalog group, which is what
// the ID spells.
const (
	actionGroupMemberAdd  = "group.group_member_add"
	actionGroupMemberEdit = "group.group_member_edit"
	actionProjectGet      = "project.get"
	actionProjectCreate   = "project.create"
	actionGroupHookAdd    = "group.hook_add"
	actionGroupHookEdit   = "group.hook_edit"
	actionGroupHookDelete = "group.hook_delete"
	actionGroupTransfer   = "group.transfer"
)

type groupNotFoundOutput struct {
	Identifier string
}

func formatGroupNotFound(out groupNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Group", out.Identifier,
		"Use gitlab_group_list to list accessible groups",
		"If using a path, ensure it is URL-encoded (e.g. my%2Fgroup)",
		"Verify your token has access to this group",
	)
}

// FormatOutputMarkdown renders a single group as its card.
func FormatOutputMarkdown(g Output) string {
	var b strings.Builder
	writeGroupCard(&b, g).End(groupCardHints()...)
	return b.String()
}

// writeGroupCard writes the card of the group entity and returns it, so that
// [FormatDetailOutputMarkdown] can add the rows only a single-group route
// carries before [toolutil.Card.End] closes the card rather than after them.
func writeGroupCard(b *strings.Builder, g Output) *toolutil.Card {
	c := toolutil.NewCard(b, "Group: "+g.Name)
	c.Int("ID", g.ID)
	c.Field("Path", g.FullPath)
	// A full name is the group names of the ancestry joined, and a group name
	// is free text a person types.
	c.Field("Full Name", g.FullName)
	c.Field("Visibility", g.Visibility)
	// An archived group is read-only, which is the one thing a reader acting
	// on it has to know before they try; it was in the JSON and nowhere in the
	// text until the markdown audit (issue 697).
	c.Flag(toolutil.EmojiArchived, "Archived", g.Archived)
	c.Text("Description", g.Description)
	c.URL(g.WebURL)
	c.Count("Parent ID", g.ParentID)
	c.Time("Created", g.CreatedAt)
	c.Time(toolutil.EmojiWarning+" Marked for deletion", g.MarkedForDeletion)
	return c
}

// groupCardHints closes a group card with its next steps. The preserve-links
// hint is deliberately absent: it is about the links of a table, and a card
// has none.
func groupCardHints() []string {
	return []string{
		toolutil.HintAction(actionGroupProjects, "see the projects in this group"),
		toolutil.HintAction(actionGroupMembers, "see the group's members"),
	}
}

// archivedCell renders a project row's archived flag for the two group project
// tables. A row GitLab rendered as BasicProjectDetails carries no flag, and
// its cell stays empty rather than answering for it.
func archivedCell(p ProjectItem) string {
	if p.Archived == nil {
		return ""
	}
	return toolutil.BoolEmoji(*p.Archived)
}

// FormatListMarkdown renders a list of groups as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Groups) == 0 {
		return toolutil.EmptyMessage("groups")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Groups", len(out.Groups), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Path", "Visibility", "Archived"))
	for _, g := range out.Groups {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(g.ID, 10),
			toolutil.EscapeMdTableCell(g.Name),
			toolutil.EscapeMdTableCell(g.FullPath),
			toolutil.EscapeMdTableCell(g.Visibility),
			toolutil.BoolEmoji(g.Archived),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionGroupGet, "see one group's details"),
		toolutil.HintAction(actionGroupProjects, "see the projects in a group"),
	)
	return b.String()
}

// FormatMemberListMarkdown renders a list of group members as a Markdown table.
//
// The state column is the user account's state (active, blocked, deactivated,
// banned), which is not the membership's own state; the membership column is
// written only when GitLab sent one, since it is an Enterprise field.
func FormatMemberListMarkdown(out MemberListOutput) string {
	if len(out.Members) == 0 {
		return toolutil.EmptyMessage("group members")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Members", len(out.Members), out.Pagination)
	columns := []string{"Username", "Name", "Access Level", "Account State"}
	membership := memberListHasMembershipState(out.Members)
	if membership {
		columns = append(columns, "Membership")
	}
	b.WriteString(toolutil.MarkdownTableHeader(columns...))
	for _, m := range out.Members {
		cells := []string{
			toolutil.MdUserHandle(m.Username),
			toolutil.EscapeMdTableCell(m.Name),
			toolutil.EscapeMdTableCell(toolutil.AccessLevelDescription(gl.AccessLevelValue(m.AccessLevel))),
			toolutil.EscapeMdTableCell(m.State),
		}
		if membership {
			cells = append(cells, toolutil.EscapeMdTableCell(m.MembershipState))
		}
		b.WriteString(toolutil.MarkdownTableRow(cells...))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionGroupMemberAdd, "add a member to this group"),
		toolutil.HintAction(actionGroupMemberEdit, "change a member's access level"),
	)
	return b.String()
}

// memberListHasMembershipState reports whether any row carries the Enterprise
// membership state, which decides whether the column is written at all.
func memberListHasMembershipState(members []MemberOutput) bool {
	for _, m := range members {
		if m.MembershipState != "" {
			return true
		}
	}
	return false
}

// FormatListProjectsMarkdown renders a list of group projects as a Markdown table.
func FormatListProjectsMarkdown(out ListProjectsOutput) string {
	if len(out.Projects) == 0 {
		return toolutil.EmptyMessage("projects")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Projects", len(out.Projects), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Path", "Visibility", "Archived"))
	for _, p := range out.Projects {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(p.ID, 10),
			toolutil.EscapeMdTableCell(p.Name),
			toolutil.EscapeMdTableCell(p.PathWithNamespace),
			toolutil.EscapeMdTableCell(p.Visibility),
			archivedCell(p),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionProjectGet, "view a project's details"),
		toolutil.HintAction(actionProjectCreate, "add a new project to this group"),
	)
	return b.String()
}

// FormatHookMarkdown renders a single group hook as its card. The URL
// variables and the custom headers are written as their keys with every value
// redacted: both are secrets GitLab masks on read, and the key is what says
// which ones are set.
func FormatHookMarkdown(h HookOutput) string {
	var b strings.Builder
	title := h.URL
	if h.Name != "" {
		title = h.Name
	}
	c := toolutil.NewCard(&b, "Group Hook: "+title)
	c.Int("ID", h.ID)
	c.URL(h.URL)
	c.Field("Name", h.Name)
	c.Text("Description", h.Description)
	c.Int("Group ID", h.GroupID)
	c.Bool("SSL Verification", h.EnableSSLVerification)
	c.Bool("Token Present", h.TokenPresent)
	c.Bool("Signing Token Present", h.SigningTokenPresent)
	c.Field("Events", enabledEvents(h))
	c.Field("Alert Status", h.AlertStatus)
	c.Time("Disabled Until", h.DisabledUntil)
	c.Time("Created", h.CreatedAt)
	toolutil.WriteHookSecretKeys(&b, hookURLVariableKeys(h), hookCustomHeaderKeys(h))
	c.End(
		toolutil.HintAction(actionGroupHookEdit, "modify this hook"),
		toolutil.HintAction(actionGroupHookDelete, "remove it"),
	)
	return b.String()
}

// hookURLVariableKeys is the hook's templated URL variable names, values left
// behind.
func hookURLVariableKeys(h HookOutput) []string {
	keys := make([]string, 0, len(h.URLVariables))
	for _, variable := range h.URLVariables {
		keys = append(keys, variable.Key)
	}
	return keys
}

// hookCustomHeaderKeys is the hook's custom header names, values left behind.
func hookCustomHeaderKeys(h HookOutput) []string {
	keys := make([]string, 0, len(h.CustomHeaders))
	for _, header := range h.CustomHeaders {
		keys = append(keys, header.Key)
	}
	return keys
}

// FormatHookListMarkdown renders a paginated list of group hooks as a Markdown table.
func FormatHookListMarkdown(out HookListOutput) string {
	if len(out.Hooks) == 0 {
		return toolutil.EmptyMessage("group webhooks")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Hooks", len(out.Hooks), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "URL", "Events", "SSL"))
	for _, h := range out.Hooks {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(h.ID, 10),
			toolutil.MdTitleLink(h.URL, h.URL),
			toolutil.EscapeMdTableCell(enabledEvents(h)),
			toolutil.BoolEmoji(h.EnableSSLVerification),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionGroupHookGet, "view one hook's details"),
		toolutil.HintAction(actionGroupHookAdd, "add a new hook"),
	)
	return b.String()
}

// FormatTransferLocationsListMarkdown renders the candidate parent groups for a group transfer.
func FormatTransferLocationsListMarkdown(out TransferLocationsListOutput) string {
	if len(out.Locations) == 0 {
		return toolutil.EmptyMessage("transfer locations")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Transfer Locations", len(out.Locations), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Full Path"))
	for _, l := range out.Locations {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(l.ID, 10),
			toolutil.MdTitleLink(l.Name, l.WebURL),
			toolutil.EscapeMdTableCell(l.FullPath),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionGroupTransfer, "move the group into one of these parents"),
	)
	return b.String()
}

// FormatProvisionedUsersListMarkdown renders a paginated list of users
// provisioned for a group through SAML/SCIM as a Markdown table.
func FormatProvisionedUsersListMarkdown(out ProvisionedUsersListOutput) string {
	if len(out.Users) == 0 {
		return toolutil.EmptyMessage("provisioned users")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Provisioned Users", len(out.Users), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Username", "Name", "Account State", "Email"))
	for _, u := range out.Users {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(u.ID, 10),
			toolutil.MdUserLink(u.Username, u.WebURL),
			toolutil.EscapeMdTableCell(u.Name),
			toolutil.EscapeMdTableCell(u.State),
			toolutil.EscapeMdTableCell(u.Email),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionGroupMembers, "see the group's members and access levels"),
		"Provisioned users are managed through the group's SAML/SCIM identity provider",
	)
	return b.String()
}

// FormatDetailOutputMarkdown renders a group as a route that answers with one
// group returns it. The registry keys a formatter by its Go type, so
// [DetailOutput] needs its own even though it embeds [Output]: without it the
// group get, create, update, restore and transfer tools would fall through to
// no formatter at all.
//
// runners_token is deliberately absent. It is a live credential, and this
// package renders into a conversation transcript; the JSON keeps it for a
// caller that needs it, which is the decision internal/tools/invites already
// took for invite_token and the reasoning is the same.
func FormatDetailOutputMarkdown(g DetailOutput) string {
	var b strings.Builder
	c := writeGroupCard(&b, g.Output)
	// Only what a single-group route adds, and only when GitLab sent it: each
	// of these is behind a condition of its own, so an absent key is an answer
	// rather than a gap.
	c.Field("Git Access Protocol", g.EnabledGitAccessProtocol)
	c.Field("Step-up Auth Provider", g.StepUpAuthRequiredOAuthProvider)
	c.Count("Shared With Groups", int64(len(g.SharedWithGroups)))
	c.Count("Projects", int64(len(g.Projects)))
	c.BoolPtr("Auto-ban on Excessive Downloads", g.AutoBanUserOnExcessiveProjectsDownload)
	c.End(groupCardHints()...)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdownResult(formatGroupNotFound)
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatDetailOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatMemberListMarkdown)
	toolutil.RegisterMarkdown(FormatListProjectsMarkdown)
	toolutil.RegisterMarkdown(FormatHookMarkdown)
	toolutil.RegisterMarkdown(FormatHookListMarkdown)
	toolutil.RegisterMarkdown(FormatTransferLocationsListMarkdown)
	toolutil.RegisterMarkdown(FormatProvisionedUsersListMarkdown)
}
