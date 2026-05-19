package groupimportexport

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group import and export actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupImportExportCreateSpec("group_export_schedule", toolutil.RouteAction(client, ScheduleExport), "gitlab_schedule_group_export"),
		groupImportExportReadSpec("group_export_download", toolutil.RouteAction(client, ExportDownload), "gitlab_download_group_export"),
		groupImportExportCreateSpec("group_import_file", toolutil.RouteAction(client, ImportFile), "gitlab_import_group_from_file"),
	}
}

func groupImportExportReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupImportExportOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupImportExportCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, groupImportExportOptions(individualTool))
}

func groupImportExportOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "import", "export"},
		RelatedActions: []string{"group.get"},
		OpenWorld:      true,
		OwnerPackage:   "groupimportexport",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
