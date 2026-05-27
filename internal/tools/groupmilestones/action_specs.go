package groupmilestones

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group milestone actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupMilestoneReadSpec("group_milestone_list", toolutil.RouteAction(client, List), "gitlab_group_milestone_list"),
		groupMilestoneReadSpec("group_milestone_get", toolutil.RouteAction(client, Get), "gitlab_group_milestone_get"),
		groupMilestoneCreateSpec("group_milestone_create", toolutil.RouteAction(client, Create), "gitlab_group_milestone_create"),
		groupMilestoneUpdateSpec("group_milestone_update", toolutil.RouteAction(client, Update), "gitlab_group_milestone_update"),
		groupMilestoneDeleteSpec("group_milestone_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_group_milestone_delete"),
		groupMilestoneReadSpec("group_milestone_issues", toolutil.RouteAction(client, GetIssues), "gitlab_group_milestone_issues"),
		groupMilestoneReadSpec("group_milestone_merge_requests", toolutil.RouteAction(client, GetMergeRequests), "gitlab_group_milestone_merge_requests"),
		groupMilestoneReadSpec("group_milestone_burndown", toolutil.RouteAction(client, GetBurndownChartEvents), "gitlab_group_milestone_burndown_events"),
	}
}

func groupMilestoneReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupMilestoneOptions(individualTool))
}

func groupMilestoneCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupMilestoneOptions(individualTool))
}

func groupMilestoneUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupMilestoneOptions(individualTool))
}

func groupMilestoneDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupMilestoneOptions(individualTool))
}

func groupMilestoneOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupmilestones domain action.", Tags: []string{"group", "milestone"},
		RelatedActions: []string{"group.get", "group.issues"},
		OpenWorld:      true,
		OwnerPackage:   "groupmilestones",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
