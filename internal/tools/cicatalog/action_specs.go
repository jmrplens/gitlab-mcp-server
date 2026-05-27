package cicatalog

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for CI/CD Catalog actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		catalogSpec("list", toolutil.RouteAction(client, List), "gitlab_list_catalog_resources"),
		catalogSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_catalog_resource"),
	}
}

func catalogSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute cicatalog domain action.", Tags: []string{"ci_catalog", "component", "graphql"},
		RelatedActions: []string{"template.lint", "pipeline.create", "project.get"},
		OpenWorld:      true,
		OwnerPackage:   "cicatalog",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
