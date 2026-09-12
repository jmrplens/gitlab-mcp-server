package achievements

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The hints name the canonical action IDs declared in action_specs.go, the one
// form every surface resolves: the dynamic surface executes them directly and
// the meta and individual surfaces resolve them to their own tool names, so a
// hint written this way is never a name the serving surface does not register.
// They used to name the individual tools (`gitlab_achievement_award`), which
// the default dynamic surface does not register at all.

// hintCursorNext tells the reader how to spend the cursor the pagination line
// above it printed.
const hintCursorNext = "Pass the `end_cursor` above as `after` to fetch the next page"

// dash is the cell a list row shows where GitLab sent nothing, so a column
// keeps its width and an empty cell is never read as a missing value.
const dash = "-"

// writeAchievementRows writes the fields shared by every single-achievement
// card, so the detail and delete views cannot drift apart.
func writeAchievementRows(c *toolutil.Card, a Achievement) {
	c.Int("ID", a.ID)
	c.Field("Name", a.Name)
	c.Int("Namespace ID", a.NamespaceID)
	// The description is whatever the namespace's owner typed, so it is quoted
	// when it runs to more than one line rather than written as Markdown of the
	// response.
	c.Text("Description", a.Description)
	if a.AvatarURL != "" {
		c.Link("Avatar", "image", a.AvatarURL)
	}
	c.Time("Created", a.CreatedAt)
	c.Time("Updated", a.UpdatedAt)
}

// writeUserAchievementRows writes the fields shared by every single-award card.
func writeUserAchievementRows(c *toolutil.Card, u UserAchievement) {
	c.Int("Award ID", u.ID)
	c.Int("Achievement ID", u.AchievementID)
	c.Int("User ID", u.UserID)
	c.Int("Awarded By", u.AwardedByUserID)
	c.Bool("Shown On Profile", u.ShowOnProfile)
	// The award message is a note whoever awarded it typed.
	c.Text("Message", u.AwardMessage)
	if u.Priority != nil {
		c.Int("Priority", *u.Priority)
	}
	c.Time("Revoked", u.RevokedAt)
	if u.RevokedByUserID != nil {
		c.Int("Revoked By", *u.RevokedByUserID)
	}
	c.Time("Created", u.CreatedAt)
	c.Time("Updated", u.UpdatedAt)
}

// userAchievementColumns are the columns every award table shares.
var userAchievementColumns = []string{
	"Award ID", "Achievement ID", "User ID", "Priority", "On Profile", "Revoked", "Message",
}

// userAchievementCells renders one award as the cells of an award table. The
// revoked column is what tells a reader that a listed award is no longer held,
// since the API returns revoked awards alongside live ones.
func userAchievementCells(award UserAchievement) []string {
	priority := dash
	if award.Priority != nil {
		priority = strconv.FormatInt(*award.Priority, 10)
	}
	revoked := dash
	if award.RevokedAt != "" {
		revoked = toolutil.FormatTime(award.RevokedAt)
	}
	message := toolutil.EscapeMdTableCell(award.AwardMessage)
	if message == "" {
		message = dash
	}
	return []string{
		strconv.FormatInt(award.ID, 10),
		strconv.FormatInt(award.AchievementID, 10),
		strconv.FormatInt(award.UserID, 10),
		priority,
		toolutil.BoolEmoji(award.ShowOnProfile),
		revoked,
		message,
	}
}

// writeUserAchievementTable renders a set of awards as one table.
func writeUserAchievementTable(sb *strings.Builder, awards []UserAchievement) {
	sb.WriteString(toolutil.MarkdownTableHeader(userAchievementColumns...))
	for _, award := range awards {
		sb.WriteString(toolutil.MarkdownTableRow(userAchievementCells(award)...))
	}
}

// FormatOutputMarkdown renders one achievement definition as the card of one
// object.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Achievement: "+out.Achievement.Name)
	writeAchievementRows(c, out.Achievement)
	c.End(
		toolutil.HintAction(actionAward, "hand this achievement to a user"),
		toolutil.HintAction(actionRecipients, "see who holds this achievement"),
		toolutil.HintAction(actionList, "see the other achievements in the namespace"),
	)
	return b.String()
}

// FormatDeleteOutputMarkdown renders a deleted achievement as the card of the
// object as it looked when it was removed, which is what the mutation returns
// instead of a bare acknowledgement.
//
// The server's own confirmation sentence is a row rather than a paragraph of
// its own: it arrives on the output struct, and a value written as a bare
// paragraph line opens a heading of its own whenever it starts with a '#'.
func FormatDeleteOutputMarkdown(out DeleteOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Achievement Deleted")
	c.Field("Result", out.Message)
	writeAchievementRows(c, out.Achievement)
	c.End(
		toolutil.HintAction(actionList, "see the other achievements in the namespace"),
		toolutil.HintAction(actionCreate, "define a replacement achievement"),
	)
	return b.String()
}

// FormatUserAchievementOutputMarkdown renders one award as the card of one
// object.
func FormatUserAchievementOutputMarkdown(out UserAchievementOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Award %d", out.UserAchievement.ID))
	writeUserAchievementRows(c, out.UserAchievement)
	c.End(
		toolutil.HintAction(actionUserAchievementUpdate, "change whether this award shows on the profile"),
		toolutil.HintAction(actionRevoke, "revoke it while keeping the record"),
		toolutil.HintAction(actionUserList, "see every award one user holds"),
	)
	return b.String()
}

// FormatUserAchievementMutationOutputMarkdown renders a revoked or deleted
// award as the card of the award the call was about.
func FormatUserAchievementMutationOutputMarkdown(out UserAchievementMutationOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Award %d", out.UserAchievement.ID))
	c.Field("Result", out.Message)
	writeUserAchievementRows(c, out.UserAchievement)
	c.End(
		toolutil.HintAction(actionUserList, "see every award one user holds"),
		toolutil.HintAction(actionRecipients, "see who holds this achievement"),
	)
	return b.String()
}

// FormatListMarkdown renders a page of achievement definitions as a Markdown
// table: a collection of objects that share columns.
//
// The hints close the response. They used to be written between the heading and
// the table header, which both continued the guidance paragraph — so the table
// never rendered — and left the section neither leading nor trailing, where
// ExtractHints finds nothing and next_steps came back empty.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	if len(out.Achievements) == 0 {
		sb.WriteString(toolutil.EmptyMessage("achievements"))
		toolutil.WriteHints(&sb, toolutil.HintAction(actionCreate, "define the first achievement for this namespace"))
		return sb.String()
	}
	toolutil.WriteListHeading(&sb, "Achievements", len(out.Achievements), toolutil.PaginationOutput{})
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Namespace ID", "Description", "Avatar"))
	linked := false
	for _, achievement := range out.Achievements {
		description := toolutil.EscapeMdTableCell(achievement.Description)
		if description == "" {
			description = dash
		}
		avatar := dash
		if achievement.AvatarURL != "" {
			avatar = toolutil.MdTitleLink("image", achievement.AvatarURL)
			linked = true
		}
		sb.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(achievement.ID, 10),
			toolutil.EscapeMdTableCell(achievement.Name),
			strconv.FormatInt(achievement.NamespaceID, 10),
			description,
			avatar,
		))
	}
	writeCursorFooter(&sb, out.Pagination, len(out.Achievements), linked,
		toolutil.HintAction(actionAward, "hand one of these achievements to a user"),
		hintCursorNext,
	)
	return sb.String()
}

// writeCursorFooter closes a cursor-paginated list: the cursor summary, then
// the guidance section, with the instruction to keep links only when a cell
// carried one. The pagination the footer itself would write is empty because
// this list's own cursor line has already been written.
func writeCursorFooter(sb *strings.Builder, p toolutil.GraphQLPaginationOutput, shown int, linked bool, hints ...string) {
	toolutil.WriteGraphQLPagination(sb, p, shown)
	toolutil.WriteListFooter(sb, toolutil.PaginationOutput{}, linked, hints...)
}

// FormatUserAchievementListMarkdown renders a page of awards as a Markdown
// table. No cell carries a link, so the hints do not ask for links to be kept.
func FormatUserAchievementListMarkdown(out UserAchievementListOutput) string {
	var sb strings.Builder
	if len(out.UserAchievements) == 0 {
		sb.WriteString(toolutil.EmptyMessage("awards"))
		toolutil.WriteHints(&sb, toolutil.HintAction(actionAward, "hand an achievement to a user"))
		return sb.String()
	}
	toolutil.WriteListHeading(&sb, "Awards", len(out.UserAchievements), toolutil.PaginationOutput{})
	writeUserAchievementTable(&sb, out.UserAchievements)
	writeCursorFooter(&sb, out.Pagination, len(out.UserAchievements), false,
		toolutil.HintAction(actionRecipients, "see who holds this achievement"),
		hintCursorNext,
	)
	return sb.String()
}

// FormatReorderOutputMarkdown renders a reordered set of awards as the card of
// the reorder, with the awards as the nested collection they are. The mutation
// returns the whole reordered set rather than a page, so there is no cursor.
func FormatReorderOutputMarkdown(out ReorderOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Awards Reordered")
	c.Field("Result", out.Message)
	c.Count("Awards", int64(len(out.UserAchievements)))
	if len(out.UserAchievements) == 0 {
		c.Note("GitLab returned no awards for this reorder.")
		c.End(toolutil.HintAction(actionUserList, "see every award one user holds"))
		return b.String()
	}
	table := c.Table("Awards", userAchievementColumns...)
	for _, award := range out.UserAchievements {
		table.Row(userAchievementCells(award)...)
	}
	c.End(toolutil.HintAction(actionUserList, "see every award one user holds"))
	return b.String()
}

// FormatUniqueUsersMarkdown renders a page of distinct recipients as a
// Markdown table.
func FormatUniqueUsersMarkdown(out UniqueUsersOutput) string {
	users := make([]*toolutil.BasicUserOutput, 0, len(out.Users))
	for _, user := range out.Users {
		if user != nil {
			users = append(users, user)
		}
	}
	var sb strings.Builder
	if len(users) == 0 {
		sb.WriteString(toolutil.EmptyMessage("recipients of this achievement"))
		toolutil.WriteHints(&sb, toolutil.HintAction(actionAward, "hand this achievement to a user"))
		return sb.String()
	}
	toolutil.WriteListHeading(&sb, "Achievement Recipients", len(users), toolutil.PaginationOutput{})
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Username", "Name", "State"))
	linked := false
	for _, user := range users {
		linked = linked || (user.WebURL != "" && user.Username != "")
		handle := toolutil.MdUserLink(user.Username, user.WebURL)
		if handle == "" {
			handle = dash
		}
		sb.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(user.ID, 10),
			handle,
			toolutil.EscapeMdTableCell(user.Name),
			toolutil.EscapeMdTableCell(user.State),
		))
	}
	writeCursorFooter(&sb, out.Pagination, len(users), linked,
		toolutil.HintAction(actionRecipients, "see who holds this achievement"),
		hintCursorNext,
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)                        // Output
	toolutil.RegisterMarkdown(FormatDeleteOutputMarkdown)                  // DeleteOutput
	toolutil.RegisterMarkdown(FormatUserAchievementOutputMarkdown)         // UserAchievementOutput
	toolutil.RegisterMarkdown(FormatUserAchievementMutationOutputMarkdown) // UserAchievementMutationOutput
	toolutil.RegisterMarkdown(FormatListMarkdown)                          // ListOutput
	toolutil.RegisterMarkdown(FormatUserAchievementListMarkdown)           // UserAchievementListOutput
	toolutil.RegisterMarkdown(FormatReorderOutputMarkdown)                 // ReorderOutput
	toolutil.RegisterMarkdown(FormatUniqueUsersMarkdown)                   // UniqueUsersOutput
}
