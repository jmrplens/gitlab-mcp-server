package groupmembers

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group member actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupMemberReadSpec("group_member_get", toolutil.RouteAction(client, GetMember), "gitlab_group_member_get"),
		groupMemberReadSpec("group_member_get_inherited", toolutil.RouteAction(client, GetInheritedMember), "gitlab_group_member_get_inherited"),
		groupMemberCreateSpec("group_member_add", toolutil.RouteAction(client, AddMember), "gitlab_group_member_add"),
		groupMemberUpdateSpec("group_member_edit", toolutil.RouteAction(client, EditMember), "gitlab_group_member_edit"),
		groupMemberDeleteSpec("group_member_remove", toolutil.DestructiveAction(client, removeMemberOutput), "gitlab_group_member_remove"),
		groupMemberCreateSpec("group_member_share", toolutil.RouteAction(client, ShareGroup), "gitlab_group_share"),
		groupMemberDeleteSpec("group_member_unshare", toolutil.DestructiveAction(client, unshareGroupOutput), "gitlab_group_unshare"),
	}
}

func groupMemberReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupMemberOptions(individualTool))
}

func groupMemberCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupMemberOptions(individualTool))
}

func groupMemberUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupMemberOptions(individualTool))
}

func groupMemberDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupMemberOptions(individualTool))
}

func groupMemberOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupmembers domain action.", Tags: []string{"group", "member"},
		RelatedActions: []string{"group.get", "group.members"},
		OpenWorld:      true,
		OwnerPackage:   "groupmembers",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
