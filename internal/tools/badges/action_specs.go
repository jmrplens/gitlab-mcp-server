package badges

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ProjectActionSpecs returns canonical specs for project badge actions.
func ProjectActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		badgeReadSpec("badge_list", toolutil.RouteAction(client, ListProject), "gitlab_list_project_badges"),
		badgeReadSpec("badge_get", toolutil.RouteAction(client, GetProject), "gitlab_get_project_badge"),
		badgeCreateSpec("badge_add", toolutil.RouteAction(client, AddProject), "gitlab_add_project_badge"),
		badgeUpdateSpec("badge_edit", toolutil.RouteAction(client, EditProject), "gitlab_edit_project_badge"),
		badgeDeleteSpec("badge_delete", toolutil.DestructiveVoidAction(client, DeleteProject), "gitlab_delete_project_badge"),
		badgeReadSpec("badge_preview", toolutil.RouteAction(client, PreviewProject), "gitlab_preview_project_badge"),
	}
}

func badgeReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := badgeOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func badgeCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, badgeOptions(individualTool))
}

func badgeUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := badgeOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func badgeDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := badgeOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func badgeOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"project", "badge"},
		RelatedActions: []string{"project.get"},
		OpenWorld:      true,
		OwnerPackage:   "badges",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
