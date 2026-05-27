package grouplabels

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group label actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupLabelReadSpec("group_label_list", toolutil.RouteAction(client, List), "gitlab_group_label_list"),
		groupLabelReadSpec("group_label_get", toolutil.RouteAction(client, Get), "gitlab_group_label_get"),
		groupLabelCreateSpec("group_label_create", toolutil.RouteAction(client, Create), "gitlab_group_label_create"),
		groupLabelUpdateSpec("group_label_update", toolutil.RouteAction(client, Update), "gitlab_group_label_update"),
		groupLabelDeleteSpec("group_label_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_group_label_delete"),
		groupLabelUpdateSpec("group_label_subscribe", toolutil.RouteAction(client, Subscribe), "gitlab_group_label_subscribe"),
		groupLabelUpdateSpec("group_label_unsubscribe", toolutil.RouteVoidAction(client, Unsubscribe), "gitlab_group_label_unsubscribe"),
	}
}

func groupLabelReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupLabelOptions(individualTool))
}

func groupLabelCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupLabelOptions(individualTool))
}

func groupLabelUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupLabelOptions(individualTool))
}

func groupLabelDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupLabelOptions(individualTool))
}

func groupLabelOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute grouplabels domain action.", Tags: []string{"group", "label"},
		RelatedActions: []string{"group.get", "group.issues"},
		OpenWorld:      true,
		OwnerPackage:   "grouplabels",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
