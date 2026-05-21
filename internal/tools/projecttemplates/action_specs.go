package projecttemplates

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project template actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		projectTemplateSpec("project_template_list", toolutil.RouteAction(client, List), "gitlab_list_project_templates"),
		projectTemplateSpec("project_template_get", toolutil.RouteAction(client, Get), "gitlab_get_project_template"),
	}
}

func projectTemplateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:           []string{"template", "project"},
		RelatedActions: []string{"project.create", "project.list"},
		OpenWorld:      true,
		OwnerPackage:   "projecttemplates",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
