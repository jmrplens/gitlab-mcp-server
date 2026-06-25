package grouprelationsexport

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	descScheduleExport = "Schedule a new export of a group's relations (issues, labels, milestones, and more). " +
		"Returns: a confirmation that the asynchronous export was scheduled. " +
		"See also: gitlab_list_group_relations_export_status, gitlab_group_get."
	descListExportStatus = "List the status of a group's relations export, optionally filtered by relation. " +
		"Returns: per-relation export status, error detail, batched flag, batch count, per-batch progress, and pagination metadata. " +
		"See also: gitlab_schedule_group_relations_export, gitlab_group_get."
)

// ActionSpecs returns canonical specs for group relations export actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupRelationsCreateSpec(
			"group_relations_schedule",
			toolutil.RouteVoidAction(client, ScheduleExport),
			"gitlab_schedule_group_relations_export",
			descScheduleExport,
			[]string{"group.group_relations_list_status", "group.get"},
		),
		groupRelationsReadSpec(
			"group_relations_list_status",
			toolutil.RouteAction(client, ListExportStatus),
			"gitlab_list_group_relations_export_status",
			descListExportStatus,
			[]string{"group.group_relations_schedule", "group.get"},
		),
	}
}

func groupRelationsReadSpec(name string, route toolutil.ActionRoute, individualTool, description string, related []string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupRelationsOptions(individualTool, description, related))
}

func groupRelationsCreateSpec(name string, route toolutil.ActionRoute, individualTool, description string, related []string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupRelationsOptions(individualTool, description, related))
}

func groupRelationsOptions(individualTool, description string, related []string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute grouprelationsexport domain action.", Tags: []string{"group", "export"},
		RelatedActions: related,
		OpenWorld:      true,
		OwnerPackage:   "grouprelationsexport",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
}
