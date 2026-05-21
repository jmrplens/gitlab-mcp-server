package dependencies

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for dependency list and export actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		dependencyReadSpec("list", toolutil.RouteAction(client, ListDeps), "gitlab_list_project_dependencies"),
		dependencyCreateSpec("export_create", toolutil.RouteAction(client, CreateExport), "gitlab_create_dependency_list_export"),
		dependencyReadSpec("export_get", toolutil.RouteAction(client, GetExport), "gitlab_get_dependency_list_export"),
		dependencyReadSpec("export_download", toolutil.RouteAction(client, DownloadExport), "gitlab_download_dependency_list_export"),
	}
}

func dependencyReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, dependencyOptions(individualTool))
}

func dependencyCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, dependencyOptions(individualTool))
}

func dependencyOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"dependency", "sbom"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "dependencies",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
