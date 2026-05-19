package gitignoretemplates

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for gitignore template actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		gitignoreTemplateSpec("gitignore_list", toolutil.RouteAction(client, List), "gitlab_list_gitignore_templates"),
		gitignoreTemplateSpec("gitignore_get", toolutil.RouteAction(client, Get), "gitlab_get_gitignore_template"),
	}
}

func gitignoreTemplateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:           []string{"template", "gitignore"},
		RelatedActions: []string{"repository.file_create", "project.create"},
		ReadOnly:       true,
		Idempotent:     true,
		OpenWorld:      true,
		OwnerPackage:   "gitignoretemplates",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
