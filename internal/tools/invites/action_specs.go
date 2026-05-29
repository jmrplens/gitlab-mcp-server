package invites

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project and group invitation actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		inviteReadSpec("invite_list_project", toolutil.RouteAction(client, ListPendingProjectInvitations), "gitlab_project_invite_list_pending"),
		inviteReadSpec("invite_list_group", toolutil.RouteAction(client, ListPendingGroupInvitations), "gitlab_group_invite_list_pending"),
		inviteCreateSpec("invite_project", toolutil.RouteAction(client, ProjectInvites), "gitlab_project_invite"),
		inviteCreateSpec("invite_group", toolutil.RouteAction(client, GroupInvites), "gitlab_group_invite"),
	}
}

func inviteReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, inviteOptions(individualTool))
}

func inviteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, inviteOptions(individualTool))
}

func inviteOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute invites domain action.", Tags: []string{"access", "invite"},
		OpenWorld:      true,
		OwnerPackage:   "invites",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
