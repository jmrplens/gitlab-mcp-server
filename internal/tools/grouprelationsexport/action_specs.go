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

const (
	usageScheduleExport = "Schedule an asynchronous export of a group's relations (issues, labels, milestones, boards, and more) by group_id. " +
		"Use this to start a group relations export before downloading it or before importing the group elsewhere; the export runs in the background, so poll status afterwards with group.group_relations_list_status."
	usageListExportStatus = "List the per-relation status of a group's relations export by group_id, optionally filtered by a single relation name. " +
		"Use this after scheduling an export to check whether each relation has finished, failed, or is still in progress before downloading the export archive."
)

// ActionSpecs returns canonical specs for group relations export actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupRelationsCreateSpec(
			"group_relations_schedule",
			toolutil.RouteVoidAction(client, ScheduleExport),
			"gitlab_schedule_group_relations_export",
			descScheduleExport,
			usageScheduleExport,
			[]string{"schedule group relations export", "start group relation export", "export group issues and labels"},
			[]string{"group.group_relations_list_status", "group.get"},
		),
		groupRelationsReadSpec(
			"group_relations_list_status",
			toolutil.RouteAction(client, ListExportStatus),
			"gitlab_list_group_relations_export_status",
			descListExportStatus,
			usageListExportStatus,
			[]string{"group relations export status", "check group relation export progress", "list group export relation statuses"},
			[]string{"group.group_relations_schedule", "group.get"},
		),
	}
}

func groupRelationsReadSpec(name string, route toolutil.ActionRoute, individualTool, description, usage string, aliases, related []string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupRelationsOptions(individualTool, description, usage, aliases, related))
}

func groupRelationsCreateSpec(name string, route toolutil.ActionRoute, individualTool, description, usage string, aliases, related []string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupRelationsOptions(individualTool, description, usage, aliases, related))
}

func groupRelationsOptions(individualTool, description, usage string, aliases, related []string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: append([]string{individualTool}, aliases...), Usage: usage, Tags: []string{"group", "export"},
		RelatedActions: related,
		OpenWorld:      true,
		OwnerPackage:   "grouprelationsexport",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
}
