package members

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for project member actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		memberReadSpec("members", toolutil.RouteAction(client, List), "gitlab_project_members_list"),
		memberReadSpec("member_get", toolutil.RouteAction(client, Get), "gitlab_project_member_get"),
		memberReadSpec("member_inherited", toolutil.RouteAction(client, GetInherited), "gitlab_project_member_get_inherited"),
		memberCreateSpec("member_add", toolutil.RouteAction(client, Add), "gitlab_project_member_add"),
		memberUpdateSpec("member_edit", toolutil.RouteAction(client, Edit), "gitlab_project_member_edit"),
		memberDeleteSpec("member_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_project_member_delete"),
	}
}

func memberReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := memberOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func memberCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, memberOptions(individualTool))
}

func memberUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := memberOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func memberDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := memberOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func memberOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"project", "member", "access"},
		RelatedActions: []string{"project.get", "user.get"},
		OpenWorld:      true,
		OwnerPackage:   "members",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
