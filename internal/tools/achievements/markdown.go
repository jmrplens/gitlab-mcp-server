package achievements

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Hints repeated across formatters, kept as constants so the same wording
// reaches the model whichever action produced the result.
const (
	hintAwardNext     = "Use `gitlab_achievement_award` to hand this achievement to a user"
	hintRecipientsNex = "Use `gitlab_achievement_recipients` to see who holds this achievement"
	hintListNext      = "Use `gitlab_achievement_list` to see the other achievements in the namespace"
	hintUserListNext  = "Use `gitlab_achievement_user_list` to see every award one user holds"
	hintCursorNext    = "Pass the `end_cursor` above as `after` to fetch the next page"
)

// writeAchievementRows renders the fields shared by every single-achievement
// view, so the detail and delete formatters cannot drift apart.
func writeAchievementRows(sb *strings.Builder, a Achievement) {
	fmt.Fprintf(sb, "| Field | Value |\n")
	fmt.Fprintf(sb, "|-------|-------|\n")
	fmt.Fprintf(sb, "| ID | %d |\n", a.ID)
	fmt.Fprintf(sb, "| Name | %s |\n", a.Name)
	fmt.Fprintf(sb, "| Namespace ID | %d |\n", a.NamespaceID)
	if a.Description != "" {
		fmt.Fprintf(sb, "| Description | %s |\n", a.Description)
	}
	if a.AvatarURL != "" {
		fmt.Fprintf(sb, "| Avatar | [image](%s) |\n", a.AvatarURL)
	}
	if a.CreatedAt != "" {
		fmt.Fprintf(sb, "| Created | %s |\n", toolutil.FormatTime(a.CreatedAt))
	}
	if a.UpdatedAt != "" {
		fmt.Fprintf(sb, "| Updated | %s |\n", toolutil.FormatTime(a.UpdatedAt))
	}
}

// writeUserAchievementRows renders the fields shared by every single-award view.
func writeUserAchievementRows(sb *strings.Builder, u UserAchievement) {
	fmt.Fprintf(sb, "| Field | Value |\n")
	fmt.Fprintf(sb, "|-------|-------|\n")
	fmt.Fprintf(sb, "| Award ID | %d |\n", u.ID)
	fmt.Fprintf(sb, "| Achievement ID | %d |\n", u.AchievementID)
	fmt.Fprintf(sb, "| User ID | %d |\n", u.UserID)
	fmt.Fprintf(sb, "| Awarded By | %d |\n", u.AwardedByUserID)
	fmt.Fprintf(sb, "| Shown On Profile | %s |\n", yesNo(u.ShowOnProfile))
	if u.AwardMessage != "" {
		fmt.Fprintf(sb, "| Message | %s |\n", u.AwardMessage)
	}
	if u.Priority != nil {
		fmt.Fprintf(sb, "| Priority | %d |\n", *u.Priority)
	}
	if u.RevokedAt != "" {
		fmt.Fprintf(sb, "| Revoked | %s |\n", toolutil.FormatTime(u.RevokedAt))
	}
	if u.RevokedByUserID != nil {
		fmt.Fprintf(sb, "| Revoked By | %d |\n", *u.RevokedByUserID)
	}
	if u.CreatedAt != "" {
		fmt.Fprintf(sb, "| Created | %s |\n", toolutil.FormatTime(u.CreatedAt))
	}
	if u.UpdatedAt != "" {
		fmt.Fprintf(sb, "| Updated | %s |\n", toolutil.FormatTime(u.UpdatedAt))
	}
}

// writeUserAchievementTable renders a set of awards as one table. The revoked
// column is what tells a reader that a listed award is no longer held, since
// the API returns revoked awards alongside live ones.
func writeUserAchievementTable(sb *strings.Builder, awards []UserAchievement) {
	fmt.Fprintf(sb, "| Award ID | Achievement ID | User ID | Priority | On Profile | Revoked | Message |\n")
	fmt.Fprintf(sb, "|---------:|---------------:|--------:|---------:|------------|---------|---------|\n")
	for _, award := range awards {
		priority := "-"
		if award.Priority != nil {
			priority = strconv.FormatInt(*award.Priority, 10)
		}
		revoked := "-"
		if award.RevokedAt != "" {
			revoked = toolutil.FormatTime(award.RevokedAt)
		}
		message := award.AwardMessage
		if message == "" {
			message = "-"
		}
		fmt.Fprintf(sb, "| %d | %d | %d | %s | %s | %s | %s |\n",
			award.ID, award.AchievementID, award.UserID, priority, yesNo(award.ShowOnProfile), revoked, message)
	}
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// FormatOutputMarkdown formats one achievement definition as Markdown.
func FormatOutputMarkdown(out Output) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Achievement: %s\n\n", out.Achievement.Name)
	writeAchievementRows(&sb, out.Achievement)
	toolutil.WriteHints(&sb, hintAwardNext, hintRecipientsNex, hintListNext)
	return sb.String()
}

// FormatDeleteOutputMarkdown formats a deleted achievement as Markdown.
func FormatDeleteOutputMarkdown(out DeleteOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Achievement Deleted\n\n%s\n\n", out.Message)
	writeAchievementRows(&sb, out.Achievement)
	toolutil.WriteHints(&sb, hintListNext, "Use `gitlab_achievement_create` to define a replacement achievement")
	return sb.String()
}

// FormatUserAchievementOutputMarkdown formats one award as Markdown.
func FormatUserAchievementOutputMarkdown(out UserAchievementOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Award %d\n\n", out.UserAchievement.ID)
	writeUserAchievementRows(&sb, out.UserAchievement)
	toolutil.WriteHints(&sb,
		"Use `gitlab_achievement_user_achievement_update` to change whether this award shows on the profile",
		"Use `gitlab_achievement_revoke` to revoke it while keeping the record",
		hintUserListNext)
	return sb.String()
}

// FormatUserAchievementMutationOutputMarkdown formats a revoked or deleted
// award as Markdown.
func FormatUserAchievementMutationOutputMarkdown(out UserAchievementMutationOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Award %d\n\n%s\n\n", out.UserAchievement.ID, out.Message)
	writeUserAchievementRows(&sb, out.UserAchievement)
	toolutil.WriteHints(&sb, hintUserListNext, hintRecipientsNex)
	return sb.String()
}

// FormatListMarkdown formats a page of achievement definitions as a table.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Achievements (%d)\n\n", len(out.Achievements))
	if len(out.Achievements) == 0 {
		sb.WriteString("No achievements found in this namespace.\n")
		toolutil.WriteHints(&sb, "Use `gitlab_achievement_create` to define the first achievement for this namespace")
		return sb.String()
	}
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks, hintAwardNext, hintCursorNext)
	fmt.Fprintf(&sb, "| ID | Name | Namespace ID | Description | Avatar |\n")
	fmt.Fprintf(&sb, "|---:|------|-------------:|-------------|--------|\n")
	for _, achievement := range out.Achievements {
		description := achievement.Description
		if description == "" {
			description = "-"
		}
		avatar := "-"
		if achievement.AvatarURL != "" {
			avatar = fmt.Sprintf("[image](%s)", achievement.AvatarURL)
		}
		fmt.Fprintf(&sb, "| %d | %s | %d | %s | %s |\n",
			achievement.ID, achievement.Name, achievement.NamespaceID, description, avatar)
	}
	fmt.Fprintf(&sb, "\n%s\n", toolutil.FormatGraphQLPagination(out.Pagination, len(out.Achievements)))
	return sb.String()
}

// FormatUserAchievementListMarkdown formats a page of awards as a table.
func FormatUserAchievementListMarkdown(out UserAchievementListOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Awards (%d)\n\n", len(out.UserAchievements))
	if len(out.UserAchievements) == 0 {
		sb.WriteString("No awards found.\n")
		toolutil.WriteHints(&sb, hintAwardNext)
		return sb.String()
	}
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks, hintRecipientsNex, hintCursorNext)
	writeUserAchievementTable(&sb, out.UserAchievements)
	fmt.Fprintf(&sb, "\n%s\n", toolutil.FormatGraphQLPagination(out.Pagination, len(out.UserAchievements)))
	return sb.String()
}

// FormatReorderOutputMarkdown formats a reordered set of awards as a table.
func FormatReorderOutputMarkdown(out ReorderOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Awards Reordered (%d)\n\n%s\n\n", len(out.UserAchievements), out.Message)
	if len(out.UserAchievements) == 0 {
		sb.WriteString("No awards were returned.\n")
		toolutil.WriteHints(&sb, hintUserListNext)
		return sb.String()
	}
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks, hintUserListNext)
	writeUserAchievementTable(&sb, out.UserAchievements)
	return sb.String()
}

// FormatUniqueUsersMarkdown formats a page of distinct recipients as a table.
func FormatUniqueUsersMarkdown(out UniqueUsersOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Achievement Recipients (%d)\n\n", len(out.Users))
	if len(out.Users) == 0 {
		sb.WriteString("No users hold this achievement.\n")
		toolutil.WriteHints(&sb, hintAwardNext)
		return sb.String()
	}
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks, hintRecipientsNex, hintCursorNext)
	fmt.Fprintf(&sb, "| ID | Username | Name | State |\n")
	fmt.Fprintf(&sb, "|---:|----------|------|-------|\n")
	for _, user := range out.Users {
		if user == nil {
			continue
		}
		username := user.Username
		if user.WebURL != "" {
			username = fmt.Sprintf("[%s](%s)", user.Username, user.WebURL)
		}
		fmt.Fprintf(&sb, "| %d | %s | %s | %s |\n", user.ID, username, user.Name, user.State)
	}
	fmt.Fprintf(&sb, "\n%s\n", toolutil.FormatGraphQLPagination(out.Pagination, len(out.Users)))
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
