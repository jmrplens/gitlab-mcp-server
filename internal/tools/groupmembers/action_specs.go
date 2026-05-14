package groupmembers

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for group member actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupMemberReadSpec("group_member_get", toolutil.RouteAction(client, GetMember), "gitlab_group_member_get"),
		groupMemberReadSpec("group_member_get_inherited", toolutil.RouteAction(client, GetInheritedMember), "gitlab_group_member_get_inherited"),
		groupMemberCreateSpec("group_member_add", toolutil.RouteAction(client, AddMember), "gitlab_group_member_add"),
		groupMemberUpdateSpec("group_member_edit", toolutil.RouteAction(client, EditMember), "gitlab_group_member_edit"),
		groupMemberDeleteSpec("group_member_remove", toolutil.DestructiveVoidAction(client, RemoveMember), "gitlab_group_member_remove"),
		groupMemberCreateSpec("group_member_share", toolutil.RouteAction(client, ShareGroup), "gitlab_group_share"),
		groupMemberUpdateSpec("group_member_unshare", toolutil.RouteVoidAction(client, UnshareGroup), "gitlab_group_unshare"),
	}
}

func groupMemberReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupMemberOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupMemberCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, groupMemberOptions(individualTool))
}

func groupMemberUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupMemberOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupMemberDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupMemberOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupMemberOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "member"},
		RelatedActions: []string{"group.get", "group.members"},
		OpenWorld:      true,
		OwnerPackage:   "groupmembers",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
