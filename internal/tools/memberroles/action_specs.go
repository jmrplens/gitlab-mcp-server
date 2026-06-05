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
	return toolutil.NewReadActionSpec(name, route, memberRoleOptions(name, individualTool))
}

func memberRoleCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, memberRoleOptions(name, individualTool))
}

func memberRoleDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, memberRoleOptions(name, individualTool))
}

func memberRoleOptions(name, individualTool string) toolutil.ActionSpecOptions {
	opts := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: usageFor(name), Tags: []string{"member_role"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "memberroles",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if rel := relatedActionsFor(name); len(rel) > 0 {
		opts.RelatedActions = rel
	}
	return opts
}

// usageFor returns a disambiguation note for the named action. The default
// ("Use to execute memberroles domain action") is replaced for the two
// list actions that are most often confused with each other and for the
// deprecated-on-self-managed path. The Usage string is also the surface
// that the dynamic find surfaces when a model searches with a generic
// term like "custom member roles" or "list member roles".
func usageFor(name string) string {
	switch name {
	case "list_instance":
		return "Lists all custom member roles defined at the instance level (Ultimate on self-managed, all tiers on GitLab.com). Use this when the prompt asks for custom member roles in general — list_group is deprecated on self-managed 17+ and returns 400 in that environment."
	case "list_group":
		return "Lists custom member roles available for a specific group_id. Deprecated on self-managed GitLab 17+; use list_instance instead on self-managed. Still valid on GitLab.com Ultimate."
	}
	return "Use to execute memberroles domain action."
}

// relatedActionsFor returns the cross-sell action IDs (in canonical
// `domain.action` form, matching the IDs the dynamic catalog assigns
// to member_role) that the dynamic find surfaces as RelatedActions for
// the named action. list_instance and list_group are linked so a model
// that finds one can recover quickly to the other.
func relatedActionsFor(name string) []string {
	switch name {
	case "list_instance":
		return []string{"member_role.list_group"}
	case "list_group":
		return []string{"member_role.list_instance"}
	}
	return nil
}
