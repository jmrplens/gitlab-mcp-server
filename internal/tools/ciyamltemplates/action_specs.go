package ciyamltemplates

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for CI YAML template actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		ciYMLTemplateSpec("ci_yml_list", toolutil.RouteAction(client, List), "gitlab_list_ci_yml_templates"),
		ciYMLTemplateSpec("ci_yml_get", toolutil.RouteAction(client, Get), "gitlab_get_ci_yml_template"),
	}
}

func ciYMLTemplateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute ciyamltemplates domain action.", Tags: []string{"template", "ci"},
		RelatedActions: []string{"template.lint", "repository.file_create"},
		OpenWorld:      true,
		OwnerPackage:   "ciyamltemplates",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
