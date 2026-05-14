package labels

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for project label actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		labelReadSpec("label_list", toolutil.RouteAction(client, List), "gitlab_label_list"),
		labelReadSpec("label_get", toolutil.RouteAction(client, Get), "gitlab_label_get"),
		labelCreateSpec("label_create", toolutil.RouteAction(client, Create), "gitlab_label_create"),
		labelUpdateSpec("label_update", toolutil.RouteAction(client, Update), "gitlab_label_update"),
		labelDeleteSpec("label_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_label_delete"),
		labelUpdateSpec("label_subscribe", toolutil.RouteAction(client, Subscribe), "gitlab_label_subscribe"),
		labelUpdateSpec("label_unsubscribe", toolutil.RouteVoidAction(client, Unsubscribe), "gitlab_label_unsubscribe"),
		labelUpdateSpec("label_promote", toolutil.RouteVoidAction(client, Promote), "gitlab_label_promote"),
	}
}

func labelReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := labelOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func labelCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, labelOptions(individualTool))
}

func labelUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := labelOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func labelDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := labelOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func labelOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"project", "label"},
		RelatedActions: []string{"project.get", "issue.list"},
		OpenWorld:      true,
		OwnerPackage:   "labels",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
