package groupprotectedbranches

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group protected branch actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupProtectedBranchReadSpec("protected_branch_list", toolutil.RouteAction(client, List), "gitlab_group_protected_branch_list",
			"List group-level protected branches with search and offset or keyset pagination. Returns: protected branches with push, merge, and unprotect access levels (access_level, access_level_description, user_id, group_id), force-push and code-owner-approval flags, plus pagination metadata. See also: gitlab_group_protected_branch_get, gitlab_group_protected_branch_protect, gitlab_group_get."),
		groupProtectedBranchReadSpec("protected_branch_get", toolutil.RouteAction(client, Get), "gitlab_group_protected_branch_get",
			"Get a single group-level protected branch by name or wildcard. Returns: the protected branch with push, merge, and unprotect access levels (access_level, access_level_description, user_id, group_id) and force-push and code-owner-approval flags. See also: gitlab_group_protected_branch_list, gitlab_group_protected_branch_update, gitlab_group_protected_branch_unprotect."),
		groupProtectedBranchCreateSpec("protected_branch_protect", toolutil.RouteAction(client, Protect), "gitlab_group_protected_branch_protect",
			"Protect a group-level branch or wildcard, optionally with per-user/per-group allowed-to-push, merge, and unprotect entries. Returns: the created protected branch with its push, merge, and unprotect access levels and force-push and code-owner-approval flags. See also: gitlab_group_protected_branch_get, gitlab_group_protected_branch_update, gitlab_group_protected_branch_unprotect."),
		groupProtectedBranchUpdateSpec("protected_branch_update", toolutil.RouteAction(client, Update), "gitlab_group_protected_branch_update",
			"Update a group-level protected branch, adding or removing (_destroy) allowed-to-push, merge, and unprotect entries. Returns: the updated protected branch with its push, merge, and unprotect access levels and force-push and code-owner-approval flags. See also: gitlab_group_protected_branch_get, gitlab_group_protected_branch_list, gitlab_group_protected_branch_unprotect."),
		groupProtectedBranchDeleteSpec("protected_branch_unprotect", toolutil.DestructiveVoidAction(client, Unprotect), "gitlab_group_protected_branch_unprotect",
			"Unprotect a group-level branch, cascading the removal to all subgroup projects. Returns: a success confirmation naming the unprotected branch. See also: gitlab_group_protected_branch_get, gitlab_group_protected_branch_list, gitlab_group_protected_branch_protect."),
	}
}

func groupProtectedBranchReadSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupProtectedBranchOptions(individualTool, description))
}

func groupProtectedBranchCreateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupProtectedBranchOptions(individualTool, description))
}

func groupProtectedBranchUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupProtectedBranchOptions(individualTool, description))
}

func groupProtectedBranchDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupProtectedBranchOptions(individualTool, description))
}

func groupProtectedBranchOptions(individualTool, description string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"group", "protected-branch"},
		Usage:          "Use group protected branch actions for group-level branch rules inherited by subgroup projects. protect uses params.name for the branch or wildcard; get/update/unprotect use params.branch. Access levels are numeric: 0 no access, 30 developer, 40 maintainer.",
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupprotectedbranches",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
}
