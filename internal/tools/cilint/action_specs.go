package cilint

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for CI lint actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		ciLintSpec("lint", toolutil.RouteAction(client, LintContent), "gitlab_ci_lint"),
		ciLintSpec("lint_project", toolutil.RouteAction(client, LintProject), "gitlab_ci_lint_project"),
	}
}

func ciLintSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute cilint domain action.", Tags: []string{"template", "ci", "lint"},
		RelatedActions: []string{"template.ci_yml_get", "pipeline.create", "repository.file_get"},
		OpenWorld:      true,
		OwnerPackage:   "cilint",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
