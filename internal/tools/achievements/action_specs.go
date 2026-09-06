package achievements

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action IDs, hoisted so every RelatedActions list references the
// same value rather than repeating the literal.
const (
	actionCreate                 = "achievement.create"
	actionUpdate                 = "achievement.update"
	actionDelete                 = "achievement.delete"
	actionAward                  = "achievement.award"
	actionRevoke                 = "achievement.revoke"
	actionUserAchievementUpdate  = "achievement.user_achievement_update"
	actionUserAchievementDelete  = "achievement.user_achievement_delete"
	actionUserAchievementReorder = "achievement.user_achievement_reorder"
	actionUserList               = "achievement.user_list"
	actionList                   = "achievement.list"
	actionRecipients             = "achievement.recipients"
	actionUniqueUsers            = "achievement.unique_users"
)

// Individual-tool names. They are declared, never derived, so the projection
// and the documentation cannot drift from each other.
const (
	toolCreate                 = "gitlab_achievement_create"
	toolUpdate                 = "gitlab_achievement_update"
	toolDelete                 = "gitlab_achievement_delete"
	toolAward                  = "gitlab_achievement_award"
	toolRevoke                 = "gitlab_achievement_revoke"
	toolUserAchievementUpdate  = "gitlab_achievement_user_achievement_update"
	toolUserAchievementDelete  = "gitlab_achievement_user_achievement_delete"
	toolUserAchievementReorder = "gitlab_achievement_user_achievement_reorder"
	toolUserList               = "gitlab_achievement_user_list"
	toolList                   = "gitlab_achievement_list"
	toolRecipients             = "gitlab_achievement_recipients"
	toolUniqueUsers            = "gitlab_achievement_unique_users"
)

// Sentences repeated across the metadata. The distinction between an
// achievement and an award of it is the one thing a model gets wrong here, so
// it is stated the same way everywhere rather than paraphrased per action.
const (
	noteTwoIDs = "An achievement_id names a badge the namespace defines. A user_achievement_id names one award of that badge to one person, and the two are different numbers."
	noteAvatar = "Send an avatar as exactly one of avatar_file_path (a local image the MCP server reads, unavailable when the server is reached over HTTP) or avatar_content_base64 (inline bytes), together with avatar_filename."
)

// ActionSpecs returns canonical specs for the achievement actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		createSpec(toolutil.RouteAction(client, Create)),
		updateSpec(toolutil.RouteAction(client, Update)),
		deleteSpec(toolutil.DestructiveAction(client, Delete)),
		awardSpec(toolutil.RouteAction(client, Award)),
		revokeSpec(toolutil.DestructiveAction(client, Revoke)),
		userAchievementUpdateSpec(toolutil.RouteAction(client, UserAchievementUpdate)),
		userAchievementDeleteSpec(toolutil.DestructiveAction(client, UserAchievementDelete)),
		userAchievementReorderSpec(toolutil.RouteAction(client, UserAchievementReorder)),
		userListSpec(toolutil.RouteAction(client, UserList)),
		listSpec(toolutil.RouteAction(client, List)),
		recipientsSpec(toolutil.RouteAction(client, Recipients)),
		uniqueUsersSpec(toolutil.RouteAction(client, UniqueUsers)),
	}
}

func createSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := achievementOptions(toolCreate)
	opts.Usage = "Define a new achievement in a group or project namespace, addressed by its numeric namespace_id rather than its path. Creating an achievement does not give it to anyone. Hand it out afterwards with achievement.award. " + noteAvatar
	opts.Aliases = append(opts.Aliases, "create achievement", "define a new badge", "add achievement to namespace")
	opts.RelatedActions = []string{actionList, actionAward, actionUpdate}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"namespace_id": {
			SemanticRole:     "scope_namespace",
			ValueSource:      "Numeric ID of the owning group or project namespace, from group.get or project.get.",
			ExampleBinding:   "params.namespace_id:42",
			CommonConfusions: []string{"namespace_id is numeric here. The list and recipients actions take a path instead, in full_path."},
		},
		"name": {
			ValueSource:    "Display name for the badge, chosen by the caller.",
			ExampleBinding: `params.name:"First Contribution"`,
		},
		"avatar_file_path":      avatarPathGuidance(),
		"avatar_content_base64": avatarBase64Guidance(),
		"avatar_filename":       avatarFilenameGuidance(),
	}
	opts.IndividualTool.Description = "Define a new achievement in a group or project namespace. Creating one awards it to nobody. Returns: the created achievement with id, namespace_id, name, description, avatar_url, and timestamps. See also: gitlab_achievement_award, gitlab_achievement_list, gitlab_achievement_update."
	return toolutil.NewCreateActionSpec("create", route, opts)
}

func updateSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := achievementOptions(toolUpdate)
	opts.Usage = "Change an existing achievement's name, description, or avatar, addressed by its numeric achievement_id. Every field is optional and an omitted one keeps its current value, so this cannot be used to clear a description. " + noteAvatar
	opts.Aliases = append(opts.Aliases, "update achievement", "rename achievement", "change achievement avatar")
	opts.RelatedActions = []string{actionList, actionCreate, actionDelete}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"achievement_id":        achievementIDGuidance(),
		"avatar_file_path":      avatarPathGuidance(),
		"avatar_content_base64": avatarBase64Guidance(),
		"avatar_filename":       avatarFilenameGuidance(),
	}
	opts.IndividualTool.Description = "Change an existing achievement's name, description, or avatar. Omitted fields keep their current value. Returns: the updated achievement with id, namespace_id, name, description, avatar_url, and timestamps. See also: gitlab_achievement_list, gitlab_achievement_create, gitlab_achievement_delete."
	return toolutil.NewUpdateActionSpec("update", route, opts)
}

func deleteSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := achievementOptions(toolDelete)
	opts.Usage = "Delete an achievement definition from its namespace. Every award ever made from it goes with it, so prefer achievement.revoke when the intent is to take the badge back from one person rather than to retire the badge itself."
	opts.Aliases = append(opts.Aliases, "delete achievement", "remove a badge", "retire achievement")
	opts.RelatedActions = []string{actionList, actionRevoke, actionCreate}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"achievement_id": achievementIDGuidance(),
	}
	opts.IndividualTool.Description = "Delete an achievement definition and every award made from it. Returns: deletion confirmation plus the achievement as it looked when removed. See also: gitlab_achievement_revoke, gitlab_achievement_list, gitlab_achievement_create."
	return toolutil.NewDeleteActionSpec("delete", route, opts)
}

func awardSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := achievementOptions(toolAward)
	opts.Usage = "Award an achievement to one user, creating an award record with its own id. That new id, not the achievement_id passed in, is what achievement.revoke and the user_achievement actions take. " + noteTwoIDs
	opts.Aliases = append(opts.Aliases, "award achievement to user", "give a badge to someone", "grant achievement")
	opts.RelatedActions = []string{actionRevoke, actionRecipients, actionUserList}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"achievement_id": achievementIDGuidance(),
		"user_id": {
			SemanticRole:     "scope_user",
			ValueSource:      "Numeric ID of the recipient, from user.list or user.get.",
			ExampleBinding:   "params.user_id:7",
			CommonConfusions: []string{"user_id is numeric. The username string belongs to achievement.user_list instead."},
		},
		"award_message": {
			ValueSource:      "Short note shown with the award, written by the caller, up to 200 characters.",
			ExampleBinding:   `params.award_message:"Shipped the first release"`,
			CommonConfusions: []string{"GitLab rejects an award_message longer than 200 characters."},
		},
	}
	opts.IndividualTool.Description = "Award an achievement to a user. Returns: the new award with its own id, achievement_id, user_id, awarded_by_user_id, award_message, priority, show_on_profile, and timestamps. See also: gitlab_achievement_revoke, gitlab_achievement_recipients, gitlab_achievement_user_list."
	return toolutil.NewCreateActionSpec("award", route, opts)
}

func revokeSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := achievementOptions(toolRevoke)
	opts.Usage = "Revoke one award, taking the badge back from its holder while keeping the record and stamping who revoked it and when. Use achievement.user_achievement_delete instead to erase the record outright, and achievement.delete to retire the badge for everyone. " + noteTwoIDs
	opts.Aliases = append(opts.Aliases, "revoke an award", "take back an achievement", "withdraw a badge from a user")
	opts.RelatedActions = []string{actionUserAchievementDelete, actionAward, actionRecipients}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"user_achievement_id": userAchievementIDGuidance(),
	}
	opts.IndividualTool.Description = "Revoke one award while keeping its record, which is then stamped with revoked_at and revoked_by_user_id. Returns: confirmation plus the revoked award. See also: gitlab_achievement_user_achievement_delete, gitlab_achievement_award, gitlab_achievement_recipients."
	return toolutil.NewDeleteActionSpec("revoke", route, opts)
}

func userAchievementUpdateSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := achievementOptions(toolUserAchievementUpdate)
	opts.Usage = "Change one award, which today means whether the recipient's profile displays it. This edits an award, not the badge behind it, so use achievement.update to rename or re-illustrate the achievement everyone holds. " + noteTwoIDs
	opts.Aliases = append(opts.Aliases, "hide an award from a profile", "show award on profile", "change award visibility")
	opts.RelatedActions = []string{actionUserList, actionUserAchievementReorder, actionUpdate}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"user_achievement_id": userAchievementIDGuidance(),
		"show_on_profile": {
			ValueSource:      "Whether the recipient's profile displays the award.",
			ExampleBinding:   "params.show_on_profile:false",
			CommonConfusions: []string{"Omitting the field leaves the current visibility alone rather than hiding the award."},
		},
	}
	opts.IndividualTool.Description = "Change one award, which today means its show_on_profile visibility. Returns: the updated award with id, achievement_id, user_id, priority, show_on_profile, and timestamps. See also: gitlab_achievement_user_list, gitlab_achievement_user_achievement_reorder, gitlab_achievement_update."
	return toolutil.NewUpdateActionSpec("user_achievement_update", route, opts)
}

func userAchievementDeleteSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := achievementOptions(toolUserAchievementDelete)
	opts.Usage = "Erase one award record outright, leaving no trace that the badge was ever held. Use achievement.revoke instead when the history should be kept, and achievement.delete to retire the badge itself. " + noteTwoIDs
	opts.Aliases = append(opts.Aliases, "delete an award record", "erase an award", "remove award from user")
	opts.RelatedActions = []string{actionRevoke, actionUserList, actionDelete}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"user_achievement_id": userAchievementIDGuidance(),
	}
	opts.IndividualTool.Description = "Erase one award record outright, keeping no revocation history. Returns: confirmation plus the deleted award. See also: gitlab_achievement_revoke, gitlab_achievement_user_list, gitlab_achievement_delete."
	return toolutil.NewDeleteActionSpec("user_achievement_delete", route, opts)
}

func userAchievementReorderSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := achievementOptions(toolUserAchievementReorder)
	opts.Usage = "Set the order one user's awards appear in on their profile, highest priority first. Pass every award ID of that one user in the wanted order, since the mutation reads the whole sequence rather than moving a single entry. Read the current order from achievement.user_list."
	opts.Aliases = append(opts.Aliases, "reorder a user's awards", "set achievement display order", "prioritize awards on a profile")
	opts.RelatedActions = []string{actionUserList, actionUserAchievementUpdate, actionAward}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"user_achievement_ids": {
			SemanticRole:     "user_achievement_id_list",
			ValueSource:      "Award IDs from a prior achievement.user_list response, listed in the wanted order.",
			ExampleBinding:   "params.user_achievement_ids:[12, 9, 30]",
			CommonConfusions: []string{"These are award IDs, not achievement IDs, and they must all belong to one user.", "At least one ID is required, and the whole sequence is replaced rather than merged."},
		},
	}
	opts.IndividualTool.Description = "Set the display order of one user's awards, highest priority first. Returns: confirmation plus every award in its new order with the assigned priority. See also: gitlab_achievement_user_list, gitlab_achievement_user_achievement_update, gitlab_achievement_award."
	return toolutil.NewUpdateActionSpec("user_achievement_reorder", route, opts)
}

func userListSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := achievementOptions(toolUserList)
	opts.ContentKind = toolutil.ActionSpecContentList
	opts.Usage = "List the awards one user holds, found by username and paginated by cursor. This is the person-centered view: achievement.recipients answers the opposite question, who holds one badge. Set include_hidden to also return awards the user hid from their profile, which only they and namespace maintainers or owners can see."
	opts.Aliases = append(opts.Aliases, "list a user's achievements", "what badges does this person hold", "show achievements on a profile")
	opts.RelatedActions = []string{actionRecipients, actionUserAchievementReorder, actionAward}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"username": {
			SemanticRole:     "scope_user",
			ValueSource:      "Account name from user.list, without a leading at sign.",
			ExampleBinding:   `params.username:"jsmith"`,
			CommonConfusions: []string{"This action takes the username string. The numeric user_id belongs to achievement.award instead."},
		},
		"include_hidden": {
			ValueSource:      "Whether to include awards the recipient hid from their profile.",
			ExampleBinding:   "params.include_hidden:true",
			CommonConfusions: []string{"Hidden awards come back only for the user themself and for namespace or instance maintainers and owners, so another caller sees no difference."},
		},
		"after": afterCursorGuidance(),
		"first": pageSizeGuidance(),
	}
	opts.IndividualTool.Description = "List the awards one user holds, by username, with cursor pagination. Returns: each award with id, achievement_id, user_id, priority, show_on_profile, award_message, and revocation stamps, plus pagination cursors. See also: gitlab_achievement_recipients, gitlab_achievement_user_achievement_reorder, gitlab_achievement_award."
	return toolutil.NewReadActionSpec("user_list", route, opts)
}

func listSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := achievementOptions(toolList)
	opts.ContentKind = toolutil.ActionSpecContentList
	opts.Usage = "List the achievements a namespace defines, found by its full_path and paginated by cursor. This is the action that turns a namespace path into the numeric achievement_id every other action needs. It returns badge definitions, not who holds them, which is what achievement.recipients answers."
	opts.Aliases = append(opts.Aliases, "list achievements in a namespace", "what badges does this group define", "find an achievement id")
	opts.RelatedActions = []string{actionRecipients, actionCreate, actionAward}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"full_path": fullPathGuidance(),
		"ids":       idsGuidance(),
		"after":     afterCursorGuidance(),
		"first":     pageSizeGuidance(),
	}
	opts.IndividualTool.Description = "List the achievements a group or project namespace defines, with cursor pagination. Returns: each achievement with id, namespace_id, name, description, avatar_url, and timestamps, plus pagination cursors. See also: gitlab_achievement_recipients, gitlab_achievement_create, gitlab_achievement_award."
	return toolutil.NewReadActionSpec("list", route, opts)
}

func recipientsSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := achievementOptions(toolRecipients)
	opts.ContentKind = toolutil.ActionSpecContentList
	opts.Usage = "List every award of one achievement, including repeat awards to the same person and awards already revoked. Use achievement.unique_users instead to count distinct holders, and achievement.user_list to look at one person rather than one badge. Each entry carries the user_achievement_id that revoke and delete take."
	opts.Aliases = append(opts.Aliases, "who holds this achievement", "list awards of a badge", "show achievement recipients")
	opts.RelatedActions = []string{actionUniqueUsers, actionUserList, actionRevoke}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"full_path":      fullPathGuidance(),
		"achievement_id": achievementIDGuidance(),
		"after":          afterCursorGuidance(),
		"first":          pageSizeGuidance(),
	}
	opts.IndividualTool.Description = "List every award of one achievement, revoked awards and repeat awards included, with cursor pagination. Returns: each award with its own id, achievement_id, user_id, award_message, priority, show_on_profile, and revocation stamps, plus pagination cursors. See also: gitlab_achievement_unique_users, gitlab_achievement_user_list, gitlab_achievement_revoke."
	return toolutil.NewReadActionSpec("recipients", route, opts)
}

func uniqueUsersSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := achievementOptions(toolUniqueUsers)
	opts.ContentKind = toolutil.ActionSpecContentList
	opts.Usage = "List the distinct users who hold one achievement, counting a person once however many times they were awarded it. Use achievement.recipients instead when the award records themselves are wanted, because this action returns user profiles and carries no user_achievement_id to revoke or delete with."
	opts.Aliases = append(opts.Aliases, "distinct holders of an achievement", "how many people have this badge", "unique achievement recipients")
	opts.RelatedActions = []string{actionRecipients, actionUserList, actionList}
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"full_path":      fullPathGuidance(),
		"achievement_id": achievementIDGuidance(),
		"after":          afterCursorGuidance(),
		"first":          pageSizeGuidance(),
	}
	opts.IndividualTool.Description = "List the distinct users who hold one achievement, deduplicated across repeat awards, with cursor pagination. Returns: each user with id, username, name, state, avatar_url, and web_url, plus pagination cursors. See also: gitlab_achievement_recipients, gitlab_achievement_user_list, gitlab_achievement_list."
	return toolutil.NewReadActionSpec("unique_users", route, opts)
}

// achievementOptions returns the metadata every achievement action shares. The
// Edition stays empty because the feature is Free on every offering: the
// documented availability is "Tier: Free, Premium, Ultimate", and the GraphQL
// reference marks no achievement type, mutation, or field as Premium or
// Ultimate.
//
// GitLab API docs: https://docs.gitlab.com/user/profile/achievements/
func achievementOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:      []string{individualTool},
		Tags:         []string{"achievement", "badge", "user", "namespace", "graphql"},
		OpenWorld:    true,
		OwnerPackage: "achievements",
		ContentKind:  toolutil.ActionSpecContentMutate,
		IndividualTool: toolutil.IndividualToolSpec{
			Name:  individualTool,
			Title: toolutil.TitleFromName(individualTool),
		},
	}
}

func achievementIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "achievement_id",
		ValueSource:      "Numeric id from a prior achievement.list response.",
		ExampleBinding:   "params.achievement_id:15",
		CommonConfusions: []string{noteTwoIDs, "Pass the plain number, not the gid://gitlab/Achievements::Achievement/15 global ID."},
	}
}

func userAchievementIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "user_achievement_id",
		ValueSource:      "Numeric award id from achievement.recipients or achievement.user_list.",
		ExampleBinding:   "params.user_achievement_id:88",
		CommonConfusions: []string{noteTwoIDs, "Pass the plain number, not the gid://gitlab/Achievements::UserAchievement/88 global ID."},
	}
}

func fullPathGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "scope_namespace",
		ValueSource:      "Full path of the owning group or project, from group.list or project.list.",
		ExampleBinding:   `params.full_path:"my-group/my-project"`,
		CommonConfusions: []string{"full_path is a path string. The numeric namespace_id belongs to achievement.create instead."},
	}
}

func idsGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		ValueSource:      "Numeric achievement ids to narrow the listing to.",
		ExampleBinding:   "params.ids:[15, 16]",
		CommonConfusions: []string{"Omitting ids lists every achievement in the namespace rather than none."},
	}
}

// afterCursorGuidance describes the forward-paging cursor. The backward pair
// (before, last) is documented in the same Usage text rather than repeated
// here, because a model that has the forward pair has the shape of both.
func afterCursorGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:   "pagination_cursor",
		ValueSource:    "The pagination.end_cursor value of the previous response.",
		ExampleBinding: `params.after:"eyJpZCI6IjEwIn0"`,
		CommonConfusions: []string{
			"These endpoints paginate by opaque cursor, so page and per_page are not accepted.",
			"Page backwards with before and last, taking the cursor from pagination.start_cursor.",
		},
	}
}

func pageSizeGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "pagination_limit",
		ValueSource:      "How many items to return, chosen by the caller, between 1 and 100 and defaulting to 20.",
		ExampleBinding:   "params.first:50",
		CommonConfusions: []string{"Set first for forward paging or last for backward paging, never both.", "A value above 100 is clamped to 100 rather than refused."},
	}
}

// avatarPathGuidance, avatarBase64Guidance and avatarFilenameGuidance describe
// the dual file shape create and update share, so the two actions cannot
// describe the same three parameters differently.
func avatarPathGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:   "local_file_path",
		ValueSource:    "Absolute path to an image on the machine the MCP server runs on.",
		ExampleBinding: `params.avatar_file_path:"/tmp/badge.png"`,
		CommonConfusions: []string{
			"Send exactly one of avatar_file_path or avatar_content_base64, never both.",
			"A server reached over HTTP refuses this parameter, because the path would name its own disk rather than the caller's. Send avatar_content_base64 there.",
		},
	}
}

func avatarBase64Guidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "inline_file_content",
		ValueSource:      "Base64-encoded image bytes, for when no local path is available or the server is remote.",
		ExampleBinding:   `params.avatar_content_base64:"iVBORw0KGgo..."`,
		CommonConfusions: []string{"Send exactly one of avatar_file_path or avatar_content_base64, never both."},
	}
}

func avatarFilenameGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		ValueSource:      "File name to record for the upload, such as badge.png.",
		ExampleBinding:   `params.avatar_filename:"badge.png"`,
		CommonConfusions: []string{"Required whenever an avatar is sent by either route, because GitLab identifies the upload part by its file name."},
	}
}
