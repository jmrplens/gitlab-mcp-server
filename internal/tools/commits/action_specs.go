package commits

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for commit actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		commitCreateSpec("commit_create", toolutil.RouteAction(client, Create), "gitlab_commit_create"),
		commitReadSpec("commit_list", toolutil.RouteAction(client, List), "gitlab_commit_list"),
		commitReadSpec("commit_get", toolutil.RouteAction(client, Get), "gitlab_commit_get"),
		commitReadSpec("commit_diff", toolutil.RouteAction(client, Diff), "gitlab_commit_diff"),
		commitReadSpec("commit_refs", toolutil.RouteAction(client, GetRefs), "gitlab_commit_refs"),
		commitReadSpec("commit_comments", toolutil.RouteAction(client, GetComments), "gitlab_commit_comments"),
		commitCreateSpec("commit_comment_create", toolutil.RouteAction(client, PostComment), "gitlab_commit_comment_create"),
		commitReadSpec("commit_statuses", toolutil.RouteAction(client, GetStatuses), "gitlab_commit_statuses"),
		commitUpdateSpec("commit_status_set", toolutil.RouteAction(client, SetStatus), "gitlab_commit_status_set"),
		commitReadSpec("commit_merge_requests", toolutil.RouteAction(client, ListMRsByCommit), "gitlab_commit_merge_requests"),
		commitCreateSpec("commit_cherry_pick", toolutil.RouteAction(client, CherryPick), "gitlab_commit_cherry_pick"),
		commitCreateSpec("commit_revert", toolutil.RouteAction(client, Revert), "gitlab_commit_revert"),
		commitReadSpec("commit_signature", toolutil.RouteAction(client, GetGPGSignature), "gitlab_commit_signature"),
		commitReadSpec("file_history", toolutil.RouteAction(client, List), "gitlab_commit_list"),
	}
}

func commitReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, commitOptions(individualTool))
}

func commitCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, commitOptions(individualTool))
}

func commitUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, commitOptions(individualTool))
}

func commitOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"repository", "commit"},
		RelatedActions: []string{"repository.tree", "branch.list"},
		OpenWorld:      true,
		OwnerPackage:   "commits",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
