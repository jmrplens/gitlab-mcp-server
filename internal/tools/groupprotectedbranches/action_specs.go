package groupprotectedbranches

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group protected branch actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupProtectedBranchReadSpec("protected_branch_list", toolutil.RouteAction(client, List), "gitlab_group_protected_branch_list"),
		groupProtectedBranchReadSpec("protected_branch_get", toolutil.RouteAction(client, Get), "gitlab_group_protected_branch_get"),
		groupProtectedBranchCreateSpec("protected_branch_protect", toolutil.RouteAction(client, Protect), "gitlab_group_protected_branch_protect"),
		groupProtectedBranchUpdateSpec("protected_branch_update", toolutil.RouteAction(client, Update), "gitlab_group_protected_branch_update"),
		groupProtectedBranchDeleteSpec("protected_branch_unprotect", toolutil.DestructiveVoidAction(client, Unprotect), "gitlab_group_protected_branch_unprotect"),
	}
}

func groupProtectedBranchReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupProtectedBranchOptions(individualTool))
}

func groupProtectedBranchCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupProtectedBranchOptions(individualTool))
}

func groupProtectedBranchUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupProtectedBranchOptions(individualTool))
}

func groupProtectedBranchDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupProtectedBranchOptions(individualTool))
}

func groupProtectedBranchOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"group", "protected-branch"},
		Usage:          "Use group protected branch actions for group-level branch rules inherited by subgroup projects. protect uses params.name for the branch or wildcard; get/update/unprotect use params.branch. Access levels are numeric: 0 no access, 30 developer, 40 maintainer.",
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupprotectedbranches",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
