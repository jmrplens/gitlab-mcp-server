package members

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project member actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		memberReadSpec("members", toolutil.RouteAction(client, List), "gitlab_project_members_list"),
		memberReadSpec("member_get", toolutil.RouteAction(client, Get), "gitlab_project_member_get"),
		memberReadSpec("member_inherited", toolutil.RouteAction(client, GetInherited), "gitlab_project_member_get_inherited"),
		memberCreateSpec("member_add", toolutil.RouteAction(client, Add), "gitlab_project_member_add"),
		memberUpdateSpec("member_edit", toolutil.RouteAction(client, Edit), "gitlab_project_member_edit"),
		memberDeleteSpec("member_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_project_member_delete"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("project member")
	return out, nil
}

func memberReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, memberOptions(individualTool))
}

func memberCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, memberOptions(individualTool))
}

func memberUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, memberOptions(individualTool))
}

func memberDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, memberOptions(individualTool))
}

func memberOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute members domain action.", Tags: []string{"project", "member", "access"},
		RelatedActions: []string{"project.get", "user.get"},
		OpenWorld:      true,
		OwnerPackage:   "members",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
