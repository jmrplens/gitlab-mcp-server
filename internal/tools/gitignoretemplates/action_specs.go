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
	return toolutil.NewReadActionSpec(name, route, gitignoreTemplateOptions(name, individualTool))
}

func gitignoreTemplateOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	usage := "List available .gitignore templates."
	guidance := map[string]toolutil.ParameterGuidance{}
	if actionName == "gitignore_get" {
		usage = "Get one .gitignore template by key for repository bootstrap workflows."
		guidance["key"] = toolutil.ParameterGuidance{
			SemanticRole:   "template_key",
			ValueSource:    "Template key returned by gitignore template list output.",
			ExampleBinding: `params.key:"Go"`,
		}
	}

	return toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"template", "gitignore"},
		Usage:             usage,
		RelatedActions:    []string{"repository.file_create", "project.create"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "gitignoretemplates",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
