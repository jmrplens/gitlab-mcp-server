package ffuserlists

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionFFUserListFeatureFlagUpdate = "feature_flags.feature_flag_update"
	actionFFUserListGet               = "feature_flags.ff_user_list_get"
	actionFFUserListList              = "feature_flags.ff_user_list_list"
)

// ActionSpecs returns canonical specs for feature flag user list actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		userListReadSpec("ff_user_list_list", toolutil.RouteAction(client, ListUserLists), "gitlab_ff_user_list_list"),
		userListReadSpec("ff_user_list_get", toolutil.RouteAction(client, GetUserList), "gitlab_ff_user_list_get"),
		userListCreateSpec("ff_user_list_create", toolutil.RouteAction(client, CreateUserList), "gitlab_ff_user_list_create"),
		userListUpdateSpec("ff_user_list_update", toolutil.RouteAction(client, UpdateUserList), "gitlab_ff_user_list_update"),
		userListDeleteSpec("ff_user_list_delete", toolutil.DestructiveAction(client, deleteUserListOutput), "gitlab_ff_user_list_delete"),
	}
}

func userListReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, userListOptions(name, individualTool))
}

func userListCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, userListOptions(name, individualTool))
}

func userListUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, userListOptions(name, individualTool))
}

func userListDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, userListOptions(name, individualTool))
}

func userListOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases:        userListAliases(actionName, individualTool),
		Usage:          userListUsage(actionName),
		Tags:           []string{"feature_flags", "user_list", "rollout"},
		RelatedActions: userListRelatedActions(actionName),
		OpenWorld:      true,
		OwnerPackage:   "ffuserlists",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if actionName == "ff_user_list_get" || actionName == "ff_user_list_update" || actionName == "ff_user_list_delete" {
		confusions := []string{
			"Do not use the user list name as user_list_iid.",
			"The ID is project-scoped and distinct from the feature flag name.",
		}
		if actionName == "ff_user_list_delete" {
			confusions = append(confusions,
				"This deletes a feature flag user-list cohort, NOT the feature flag itself — to delete the flag use feature_flags.feature_flag_delete (by name).")
		}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"user_list_iid": {
				SemanticRole:     "feature_flag_user_list_iid",
				ValueSource:      "Use the iid/user_list_iid returned by ff_user_list_list or ff_user_list_create.",
				CommonConfusions: confusions,
			},
		}
	}
	options.IndividualTool.Description = userListDescription(actionName)
	return options
}

// userListUsage returns the non-generic, action-specific usage guidance for
// each feature flag user list tool so the discovery surface explains exactly
// when to pick the action (1:1 audit R-META).
func userListUsage(actionName string) string {
	switch actionName {
	case "ff_user_list_list":
		return "List a project's feature flag user lists (the named cohorts of user_xids targeted by gradual rollout strategies) with search, ordering, and offset or keyset pagination; use it to discover a list's user_list_iid before getting, updating, or deleting it."
	case "ff_user_list_get":
		return "Get one feature flag user list by its user_list_iid to inspect the cohort name and the exact set of user_xids it targets for percentage or user-id rollout strategies."
	case "ff_user_list_create":
		return "Create a named feature flag user list of user_xids in a project so feature flag strategies can roll a flag out to that explicit cohort of users."
	case "ff_user_list_update":
		return "Update a feature flag user list's name or its set of user_xids by user_list_iid to change which users a rollout cohort targets without recreating the list."
	case "ff_user_list_delete":
		return "Delete a feature flag user list by its user_list_iid; only do this once no feature flag strategy references the cohort, since the user_xids it groups are removed permanently."
	default:
		return ""
	}
}

// userListAliases returns 2-4 distinctive natural-language aliases for each
// feature flag user list tool, phrased around user-list/cohort vocabulary so
// they stay distinct from the feature flag domain (1:1 audit R-META). The
// canonical tool name is always included as the first alias.
func userListAliases(actionName, individualTool string) []string {
	switch actionName {
	case "ff_user_list_list":
		return []string{individualTool, "list feature flag user lists", "show rollout user cohorts", "browse feature flag user lists"}
	case "ff_user_list_get":
		return []string{individualTool, "get feature flag user list", "show rollout cohort members", "fetch user list user_xids"}
	case "ff_user_list_create":
		return []string{individualTool, "create feature flag user list", "add rollout user cohort", "define feature flag user_xids list"}
	case "ff_user_list_update":
		return []string{individualTool, "update feature flag user list", "edit rollout cohort user_xids", "rename feature flag user list"}
	case "ff_user_list_delete":
		return []string{individualTool, "delete feature flag user list", "remove rollout user cohort", "drop feature flag user list"}
	default:
		return []string{individualTool}
	}
}

// userListRelatedActions returns the canonical related-action IDs for each
// feature flag user list tool so the discovery surface links sibling user-list
// operations and the feature flag actions that consume the cohort (R-META).
func userListRelatedActions(actionName string) []string {
	switch actionName {
	case "ff_user_list_list":
		return []string{actionFFUserListGet, "feature_flags.ff_user_list_create", actionFFUserListFeatureFlagUpdate}
	case "ff_user_list_get":
		return []string{actionFFUserListList, "feature_flags.ff_user_list_update", "feature_flags.ff_user_list_delete"}
	case "ff_user_list_create":
		return []string{actionFFUserListList, "feature_flags.ff_user_list_update", actionFFUserListFeatureFlagUpdate}
	case "ff_user_list_update":
		return []string{actionFFUserListGet, actionFFUserListList, "feature_flags.ff_user_list_delete"}
	case "ff_user_list_delete":
		return []string{actionFFUserListGet, actionFFUserListList}
	default:
		return []string{"feature_flags.feature_flag_get", actionFFUserListFeatureFlagUpdate}
	}
}

// userListDescription returns the non-generic "Returns: … See also: …"
// individual-tool description for each feature flag user list action so the
// discovery surface mirrors the issues domain (1:1 audit R-META).
func userListDescription(actionName string) string {
	switch actionName {
	case "ff_user_list_list":
		return "List feature flag user lists for a project with search, ordering, and offset or keyset pagination. Returns: each list's id, iid, project_id, name, user_xids, created_at, updated_at, plus pagination metadata. See also: gitlab_ff_user_list_get, gitlab_ff_user_list_create."
	case "ff_user_list_get":
		return "Get a single feature flag user list by its user_list_iid. Returns: the list's id, iid, project_id, name, user_xids, created_at, and updated_at. See also: gitlab_ff_user_list_list, gitlab_ff_user_list_update, gitlab_ff_user_list_delete."
	case "ff_user_list_create":
		return "Create a feature flag user list in a project. Returns: the created list with id, iid, project_id, name, user_xids, created_at, and updated_at. See also: gitlab_ff_user_list_list, gitlab_ff_user_list_get, gitlab_ff_user_list_update."
	case "ff_user_list_update":
		return "Update a feature flag user list's name or user_xids by its user_list_iid. Returns: the updated list with id, iid, project_id, name, user_xids, created_at, and updated_at. See also: gitlab_ff_user_list_get, gitlab_ff_user_list_list, gitlab_ff_user_list_delete."
	case "ff_user_list_delete":
		return "Delete a feature flag user list by its user_list_iid. Returns: a success confirmation naming the deleted list. See also: gitlab_ff_user_list_get, gitlab_ff_user_list_list."
	default:
		return ""
	}
}
