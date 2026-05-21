package dockerfiletemplates

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for Dockerfile template actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		dockerfileTemplateSpec("dockerfile_list", toolutil.RouteAction(client, List), "gitlab_list_dockerfile_templates"),
		dockerfileTemplateSpec("dockerfile_get", toolutil.RouteAction(client, Get), "gitlab_get_dockerfile_template"),
	}
}

func dockerfileTemplateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:           []string{"template", "dockerfile"},
		RelatedActions: []string{"repository.file_create", "template.gitignore_get"},
		OpenWorld:      true,
		OwnerPackage:   "dockerfiletemplates",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
