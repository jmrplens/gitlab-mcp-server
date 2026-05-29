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
	return toolutil.NewReadActionSpec(name, route, dependencyOptions(name, individualTool))
}

func dependencyCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, dependencyOptions(name, individualTool))
}

func dependencyOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	guidance := map[string]toolutil.ParameterGuidance{}
	aliases := []string{}
	usage := ""
	switch actionName {
	case "list":
		aliases = []string{"list project dependencies", "dependency list", "dependencies inventory"}
		usage = "List project dependency inventory."
		guidance["project_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "scope_project",
			ValueSource:    "Project ID or path containing dependencies.",
			ExampleBinding: `params.project_id:"group/project"`,
		}
	case "export_create":
		aliases = []string{"create dependency export", "sbom export create", "dependency export create"}
		usage = "Create a dependency list export from a pipeline."
		guidance["pipeline_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "pipeline_id",
			ValueSource:    "Pipeline numeric ID to export dependencies from.",
			ExampleBinding: "params.pipeline_id:100",
		}
	case "export_get", "export_download":
		if actionName == "export_get" {
			aliases = []string{"get dependency export", "dependency export status", "sbom export status"}
			usage = "Get dependency list export status and metadata."
		} else {
			aliases = []string{"download dependency export", "download sbom export", "dependency export download"}
			usage = "Download generated dependency list export content."
		}
		guidance["export_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "dependency_export_id",
			ValueSource:    "Export numeric ID returned by create/get export operations.",
			ExampleBinding: "params.export_id:1",
		}
	}

	return toolutil.ActionSpecOptions{
		Aliases:           aliases,
		Tags:              []string{"dependency", "sbom"},
		Usage:             usage,
		RelatedActions:    []string{"pipeline.get", "security.vulnerability_list", "package.list"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		Edition:           "premium",
		OwnerPackage:      "dependencies",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:  individualTool,
			Title: toolutil.TitleFromName(individualTool),
		},
	}
}
