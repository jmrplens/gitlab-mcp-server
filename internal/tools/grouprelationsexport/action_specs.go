package grouprelationsexport

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group relations export actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupRelationsCreateSpec("group_relations_schedule", toolutil.RouteVoidAction(client, ScheduleExport), "gitlab_schedule_group_relations_export"),
		groupRelationsReadSpec("group_relations_list_status", toolutil.RouteAction(client, ListExportStatus), "gitlab_list_group_relations_export_status"),
	}
}

func groupRelationsReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupRelationsOptions(individualTool))
}

func groupRelationsCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupRelationsOptions(individualTool))
}

func groupRelationsOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute grouprelationsexport domain action.", Tags: []string{"group", "export"},
		RelatedActions: []string{"group.get"},
		OpenWorld:      true,
		OwnerPackage:   "grouprelationsexport",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
