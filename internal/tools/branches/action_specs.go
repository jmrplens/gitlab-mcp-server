package branches

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for branch and protected branch actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		branchSpec("create", toolutil.RouteAction(client, Create), "gitlab_branch_create", false, false),
		branchSpec("get", toolutil.RouteAction(client, Get), "gitlab_branch_get", true, true),
		branchSpec("list", toolutil.RouteAction(client, List), "gitlab_branch_list", true, true),
		branchSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_branch_delete", false, true),
		branchSpec("delete_merged", toolutil.DestructiveVoidAction(client, DeleteMerged), "gitlab_branch_delete_merged", false, true),
		branchSpec("protect", toolutil.RouteAction(client, Protect), "gitlab_branch_protect", false, true),
		branchSpec("unprotect", toolutil.DestructiveAction(client, Unprotect), "gitlab_branch_unprotect", false, true),
		branchSpec("list_protected", toolutil.RouteAction(client, ProtectedList), "gitlab_protected_branches_list", true, true),
		branchSpec("get_protected", toolutil.RouteAction(client, ProtectedGet), "gitlab_protected_branch_get", true, true),
		branchSpec("update_protected", toolutil.RouteAction(client, ProtectedUpdate), "gitlab_protected_branch_update", false, true),
	}
}

func branchSpec(name string, route toolutil.ActionRoute, individualTool string, readOnly, idempotent bool) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:           []string{"branch"},
		RelatedActions: []string{"branch.list", "branch.get", "repository.tree", "merge_request.create"},
		ReadOnly:       readOnly,
		Idempotent:     idempotent,
		OpenWorld:      true,
		OwnerPackage:   "branches",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
