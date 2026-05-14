package repositorysubmodules

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for repository submodule actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		submoduleReadSpec("list_submodules", toolutil.RouteAction(client, List), "gitlab_list_repository_submodules"),
		submoduleReadSpec("read_submodule_file", toolutil.RouteAction(client, Read), "gitlab_read_repository_submodule_file"),
		submoduleUpdateSpec("update_submodule", toolutil.RouteAction(client, Update), "gitlab_update_repository_submodule"),
	}
}

func submoduleReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := submoduleOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func submoduleUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := submoduleOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func submoduleOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"repository", "submodule"},
		RelatedActions: []string{"repository.tree", "repository.file_get"},
		OpenWorld:      true,
		OwnerPackage:   "repositorysubmodules",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
