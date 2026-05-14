package memberroles

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for custom member role actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		memberRoleReadSpec("list_instance", toolutil.RouteAction(client, ListInstance), "gitlab_list_instance_member_roles"),
		memberRoleCreateSpec("create_instance", toolutil.RouteAction(client, CreateInstance), "gitlab_create_instance_member_role"),
		memberRoleDeleteSpec("delete_instance", toolutil.DestructiveVoidAction(client, DeleteInstance), "gitlab_delete_instance_member_role"),
		memberRoleReadSpec("list_group", toolutil.RouteAction(client, ListGroup), "gitlab_list_group_member_roles"),
		memberRoleCreateSpec("create_group", toolutil.RouteAction(client, CreateGroup), "gitlab_create_group_member_role"),
		memberRoleDeleteSpec("delete_group", toolutil.DestructiveVoidAction(client, DeleteGroup), "gitlab_delete_group_member_role"),
	}
}

func memberRoleReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := memberRoleOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func memberRoleCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, memberRoleOptions(individualTool))
}

func memberRoleDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := memberRoleOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func memberRoleOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"member_role"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "memberroles",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
