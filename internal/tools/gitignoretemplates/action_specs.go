package gitignoretemplates

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionProjectCreate      = "project.create"
	actionRepositoryFileCreate = "repository.file_create"
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
	opts := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"template", "gitignore"},
		Usage:          "List available .gitignore templates.",
		RelatedActions: []string{actionRepositoryFileCreate, actionProjectCreate},
		OpenWorld:      true,
		OwnerPackage:   "gitignoretemplates",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if actionName == "gitignore_list" {
		opts.Aliases = []string{individualTool, "list dedicated gitignore templates", "show available .gitignore presets", "browse gitignore boilerplate"}
		opts.RelatedActions = []string{"gitignoretemplates.gitignore_get", actionRepositoryFileCreate, actionProjectCreate}
		opts.IndividualTool.Description = "List available .gitignore templates with order_by, sort, and offset or keyset pagination. Returns: each template's key and name, plus pagination metadata. See also: gitlab_get_gitignore_template, gitlab_file_create."
	}
	if actionName == "gitignore_get" {
		opts.Aliases = []string{individualTool, "get dedicated gitignore template", "fetch .gitignore template content", "show gitignore preset for language"}
		opts.Usage = "Get one .gitignore template by key for repository bootstrap workflows."
		opts.RelatedActions = []string{"gitignoretemplates.gitignore_list", actionRepositoryFileCreate, actionProjectCreate}
		opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"key": {SemanticRole: "template_key", ValueSource: "Template key returned by gitignore template list output.", ExampleBinding: `params.key:"Go"`},
		}
		opts.IndividualTool.Description = "Get one .gitignore template by key (e.g. Go, Python, Node). Returns: the template's name and full file content ready to copy into .gitignore. See also: gitlab_list_gitignore_templates, gitlab_file_create."
	}
	return opts
}
