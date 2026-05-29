package mrcontextcommits

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	return toolutil.NewReadActionSpec(name, route, contextCommitOptions(individualTool))
}

func contextCommitCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, contextCommitOptions(individualTool))
}

func contextCommitDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, contextCommitOptions(individualTool))
}

func contextCommitOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute mrcontextcommits domain action.", Tags: []string{"merge_request", "context_commit"},
		OpenWorld:      true,
		OwnerPackage:   "mrcontextcommits",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
