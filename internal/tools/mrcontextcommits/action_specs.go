package mrcontextcommits

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge request context commit actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		contextCommitReadSpec("context_commits_list", toolutil.RouteAction(client, List), "gitlab_list_mr_context_commits"),
		contextCommitCreateSpec("context_commits_create", toolutil.RouteAction(client, Create), "gitlab_create_mr_context_commits"),
		contextCommitDeleteSpec("context_commits_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_delete_mr_context_commits"),
	}
}

func contextCommitReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := contextCommitOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func contextCommitCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, contextCommitOptions(individualTool))
}

func contextCommitDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := contextCommitOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func contextCommitOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"merge_request", "context_commit"},
		OpenWorld:      true,
		OwnerPackage:   "mrcontextcommits",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
