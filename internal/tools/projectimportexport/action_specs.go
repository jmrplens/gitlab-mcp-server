package projectimportexport

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project import and export actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		projectImportExportCreateSpec("export_schedule", toolutil.RouteAction(client, ScheduleExport), "gitlab_schedule_project_export"),
		projectImportExportReadSpec("export_status", toolutil.RouteAction(client, GetExportStatus), "gitlab_get_project_export_status"),
		projectImportExportReadSpec("export_download", toolutil.RouteAction(client, ExportDownload), "gitlab_download_project_export"),
		projectImportExportCreateSpec("import_from_file", toolutil.RouteAction(client, ImportFromFile), "gitlab_import_project_from_file"),
		projectImportExportReadSpec("import_status", toolutil.RouteAction(client, GetImportStatus), "gitlab_get_project_import_status"),
	}
}

func projectImportExportReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, projectImportExportOptions(individualTool))
}

func projectImportExportCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, projectImportExportOptions(individualTool))
}

func projectImportExportOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"project", "import", "export"},
		RelatedActions: []string{"project.get"},
		OpenWorld:      true,
		OwnerPackage:   "projectimportexport",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
