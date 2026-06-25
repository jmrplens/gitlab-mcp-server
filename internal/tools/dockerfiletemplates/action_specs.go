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
	return toolutil.NewReadActionSpec(name, route, dockerfileTemplateOptions(name, individualTool))
}

func dockerfileTemplateOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	if actionName == "dockerfile_get" {
		return toolutil.ActionSpecOptions{
			Aliases: []string{individualTool},
			Tags:    []string{"template", "dockerfile"},
			Usage:   "Get one Dockerfile template by key for scaffold/bootstrap workflows.",
			ParameterGuidance: map[string]toolutil.ParameterGuidance{
				"key": {SemanticRole: "template_key", ValueSource: "Template key returned by dockerfile template list output.", ExampleBinding: `params.key:"Go"`},
			},
			RelatedActions: []string{"repository.file_create", "template.gitignore_get"},
			OpenWorld:      true,
			OwnerPackage:   "dockerfiletemplates",
			IndividualTool: toolutil.IndividualToolSpec{
				Name:        individualTool,
				Title:       toolutil.TitleFromName(individualTool),
				Description: "Get a single Dockerfile template by key. Returns: the template name and full Dockerfile content. See also: gitlab_list_dockerfile_templates, gitlab_get_gitignore_template.",
			},
		}
	}
	return toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"template", "dockerfile"},
		Usage:          "List available Dockerfile templates.",
		RelatedActions: []string{"repository.file_create", "template.gitignore_get"},
		OpenWorld:      true,
		OwnerPackage:   "dockerfiletemplates",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: "List available Dockerfile templates with pagination and sorting. Returns: matching templates with key and name plus pagination metadata. See also: gitlab_get_dockerfile_template, gitlab_list_gitignore_templates.",
		},
	}
}
