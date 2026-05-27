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
	return toolutil.NewReadActionSpec(name, route, groupImportExportOptions(individualTool))
}

func groupImportExportCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupImportExportOptions(individualTool))
}

func groupImportExportOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupimportexport domain action.", Tags: []string{"group", "import", "export"},
		RelatedActions: []string{"group.get"},
		OpenWorld:      true,
		OwnerPackage:   "groupimportexport",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
