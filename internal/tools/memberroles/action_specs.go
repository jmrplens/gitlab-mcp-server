package memberroles

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for custom member role actions
// exposed as MCP tools. The instance and group list, create, and delete
// routes are projected into the dynamic, meta, individual, and audit
// surfaces by the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_list_instance_member_roles — list instance-level custom member roles.
		memberRoleReadSpec("list_instance", toolutil.RouteAction(client, ListInstance), "gitlab_list_instance_member_roles"),
		// gitlab_create_instance_member_role — create an instance-level custom role.
		memberRoleCreateSpec("create_instance", toolutil.RouteAction(client, CreateInstance), "gitlab_create_instance_member_role"),
		// gitlab_delete_instance_member_role — remove an instance-level custom role (destructive).
		memberRoleDeleteSpec("delete_instance", toolutil.DestructiveAction(client, deleteInstanceOutput), "gitlab_delete_instance_member_role"),
		// gitlab_list_group_member_roles — list group-level custom member roles.
		memberRoleReadSpec("list_group", toolutil.RouteAction(client, ListGroup), "gitlab_list_group_member_roles"),
		// gitlab_create_group_member_role — create a group-level custom role.
		memberRoleCreateSpec("create_group", toolutil.RouteAction(client, CreateGroup), "gitlab_create_group_member_role"),
		// gitlab_delete_group_member_role — remove a group-level custom role (destructive).
		memberRoleDeleteSpec("delete_group", toolutil.DestructiveAction(client, deleteGroupOutput), "gitlab_delete_group_member_role"),
	}
}

// deleteInstanceOutput adapts the package's [DeleteInstance] handler
// to the [toolutil.DestructiveAction] contract, returning a structured
// success result that names the deleted role in the message.
func deleteInstanceOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInstanceInput) (toolutil.DeleteOutput, error) {
	if err := DeleteInstance(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("instance member role %d", input.MemberRoleID))
	return out, nil
}

// deleteGroupOutput adapts the package's [DeleteGroup] handler to the
// [toolutil.DestructiveAction] contract, returning a structured
// success result that names the role and group in the message.
func deleteGroupOutput(ctx context.Context, client *gitlabclient.Client, input DeleteGroupInput) (toolutil.DeleteOutput, error) {
	if err := DeleteGroup(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("member role %d from group %s", input.MemberRoleID, input.GroupID))
	return out, nil
}

// memberRoleReadSpec builds a read-only [toolutil.ActionSpec] for a
// custom member role action using the package's default
// [memberRoleOptions].
func memberRoleReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, memberRoleOptions(name, individualTool))
}

// memberRoleCreateSpec builds a create-style [toolutil.ActionSpec] for
// a custom member role action using the package's default
// [memberRoleOptions].
func memberRoleCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, memberRoleOptions(name, individualTool))
}

// memberRoleDeleteSpec builds a destructive [toolutil.ActionSpec] for
// a custom member role action using the package's default
// [memberRoleOptions].
func memberRoleDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, memberRoleOptions(name, individualTool))
}

// memberRoleOptions returns the base [toolutil.ActionSpecOptions] for
// a custom member role action, layering the package's Usage/RelatedActions
// customisations on top of the catalog defaults.
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
