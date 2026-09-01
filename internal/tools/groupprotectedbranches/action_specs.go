package groupprotectedbranches

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action IDs for group protected branch cross-links. They are
// projected under the gitlab_group meta-tool, so RelatedActions reference the
// group.-qualified form consistent with the rest of the group surface.
const (
	actionList      = "group.protected_branch_list"
	actionGet       = "group.protected_branch_get"
	actionProtect   = "group.protected_branch_protect"
	actionUpdate    = "group.protected_branch_update"
	actionUnprotect = "group.protected_branch_unprotect"
	actionGroupGet  = "group.get"
)

// ActionSpecs returns canonical specs for group protected branch actions.
//
// Each action carries action-specific Usage, distinctive natural-language
// aliases (group-scoped branch-protection phrasing, kept distinct from the
// per-project branches domain and from group protected environments), and
// non-empty canonical RelatedActions cross-links so discovery surfaces and the
// individual-tool descriptions stay non-generic (R-META, 1:1 audit).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupProtectedBranchReadSpec("protected_branch_list", toolutil.RouteAction(client, List), "gitlab_group_protected_branch_list",
			"List group-level protected branches with search and offset or keyset pagination. Returns: protected branches with push, merge, and unprotect access levels (access_level, access_level_description, user_id, group_id), force-push and code-owner-approval flags, plus pagination metadata. See also: gitlab_group_protected_branch_get, gitlab_group_protected_branch_protect, gitlab_group_get.",
			"List the protected-branch rules defined on a group so subgroup projects inherit them. Use params.group_id. Supports search plus offset or keyset pagination. Prefer this over inspecting a single rule when enumerating or auditing a group's inherited branch protections.",
			[]string{"list group protected branches", "show group branch protection rules", "audit inherited branch protections"},
			[]string{actionGet, actionProtect, actionGroupGet}),
		groupProtectedBranchReadSpec("protected_branch_get", toolutil.RouteAction(client, Get), "gitlab_group_protected_branch_get",
			"Get a single group-level protected branch by name or wildcard. Returns: the protected branch with push, merge, and unprotect access levels (access_level, access_level_description, user_id, group_id) and force-push and code-owner-approval flags. See also: gitlab_group_protected_branch_list, gitlab_group_protected_branch_update, gitlab_group_protected_branch_unprotect.",
			"Fetch one group-level protected-branch rule by exact name or wildcard via params.group_id plus params.branch. Use after a list call or when the rule name is already known. Prefer this over list when inspecting a single inherited rule's access levels.",
			[]string{"get group protected branch", "show group branch protection details", "inspect inherited branch rule"},
			[]string{actionList, actionUpdate, actionUnprotect}),
		groupProtectedBranchCreateSpec("protected_branch_protect", toolutil.RouteAction(client, Protect), "gitlab_group_protected_branch_protect",
			"Protect a group-level branch or wildcard, optionally with per-user/per-group allowed-to-push, merge, and unprotect entries. Returns: the created protected branch with its push, merge, and unprotect access levels and force-push and code-owner-approval flags. See also: gitlab_group_protected_branch_get, gitlab_group_protected_branch_update, gitlab_group_protected_branch_unprotect.",
			"Create a group-level protected-branch rule that cascades to subgroup projects. Use params.group_id plus params.name for the branch or wildcard (e.g. 'release/*'). Optionally pass per-user/per-group allowed-to-push, merge, and unprotect entries. Numeric access levels: 0 no access, 30 developer, 40 maintainer.",
			[]string{"protect group branch", "create group branch protection rule", "restrict pushes across subgroup projects"},
			[]string{actionGet, actionUpdate, actionUnprotect}),
		groupProtectedBranchUpdateSpec("protected_branch_update", toolutil.RouteAction(client, Update), "gitlab_group_protected_branch_update",
			"Update a group-level protected branch, adding or removing (_destroy) allowed-to-push, merge, and unprotect entries. Returns: the updated protected branch with its push, merge, and unprotect access levels and force-push and code-owner-approval flags. See also: gitlab_group_protected_branch_get, gitlab_group_protected_branch_list, gitlab_group_protected_branch_unprotect.",
			"Modify an existing group-level protected-branch rule via params.group_id plus params.branch. Partial updates merge with the current config. Add or remove allowed-to-push, merge, and unprotect entries (pass _destroy=true to drop an entry). Unset fields keep their current values.",
			[]string{"update group protected branch", "edit group branch protection rule", "change inherited push or merge access"},
			[]string{actionGet, actionList, actionUnprotect}),
		groupProtectedBranchDeleteSpec("protected_branch_unprotect", toolutil.DestructiveVoidAction(client, Unprotect), "gitlab_group_protected_branch_unprotect",
			"Unprotect a group-level branch, cascading the removal to all subgroup projects. Returns: a success confirmation naming the unprotected branch. See also: gitlab_group_protected_branch_get, gitlab_group_protected_branch_list, gitlab_group_protected_branch_protect.",
			"Remove a group-level protected-branch rule via params.group_id plus params.branch. The removal cascades to every subgroup project and is irreversible. Requires Owner plus Premium/Ultimate. Verify the rule name with the list action first.",
			[]string{"unprotect group branch", "delete group branch protection rule", "remove inherited branch protection"},
			[]string{actionGet, actionList, actionProtect}),
	}
}

func groupProtectedBranchReadSpec(name string, route toolutil.ActionRoute, individualTool, description, usage string, aliases, related []string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupProtectedBranchOptions(individualTool, description, usage, aliases, related))
}

func groupProtectedBranchCreateSpec(name string, route toolutil.ActionRoute, individualTool, description, usage string, aliases, related []string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupProtectedBranchOptions(individualTool, description, usage, aliases, related))
}

func groupProtectedBranchUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description, usage string, aliases, related []string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupProtectedBranchOptions(individualTool, description, usage, aliases, related))
}

func groupProtectedBranchDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description, usage string, aliases, related []string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupProtectedBranchOptions(individualTool, description, usage, aliases, related))
}

func groupProtectedBranchOptions(individualTool, description, usage string, aliases, related []string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:        append([]string{individualTool}, aliases...),
		Tags:           []string{"group", "protected-branch"},
		Usage:          usage,
		RelatedActions: related,
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupprotectedbranches",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
}
