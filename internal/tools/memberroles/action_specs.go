package memberroles

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for custom member role actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		memberRoleReadSpec("list_instance", toolutil.RouteAction(client, ListInstance), "gitlab_list_instance_member_roles"),
		memberRoleCreateSpec("create_instance", toolutil.RouteAction(client, CreateInstance), "gitlab_create_instance_member_role"),
		memberRoleDeleteSpec("delete_instance", toolutil.DestructiveAction(client, deleteInstanceOutput), "gitlab_delete_instance_member_role"),
		memberRoleReadSpec("list_group", toolutil.RouteAction(client, ListGroup), "gitlab_list_group_member_roles"),
		memberRoleCreateSpec("create_group", toolutil.RouteAction(client, CreateGroup), "gitlab_create_group_member_role"),
		memberRoleDeleteSpec("delete_group", toolutil.DestructiveAction(client, deleteGroupOutput), "gitlab_delete_group_member_role"),
	}
}

func deleteInstanceOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInstanceInput) (toolutil.DeleteOutput, error) {
	if err := DeleteInstance(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("instance member role %d", input.MemberRoleID))
	return out, nil
}

func deleteGroupOutput(ctx context.Context, client *gitlabclient.Client, input DeleteGroupInput) (toolutil.DeleteOutput, error) {
	if err := DeleteGroup(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("member role %d from group %s", input.MemberRoleID, input.GroupID))
	return out, nil
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
