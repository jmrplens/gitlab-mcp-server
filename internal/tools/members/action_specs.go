package members

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project member actions exposed
// as MCP tools. The list, get, get-inherited, add, edit, and delete
// routes are projected into the dynamic, meta, individual, and audit
// surfaces by the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_project_members_list — list project members (direct + inherited).
		memberReadSpec("members", toolutil.RouteAction(client, List), "gitlab_project_members_list"),
		// gitlab_project_member_get — fetch a direct project member.
		memberReadSpec("member_get", toolutil.RouteAction(client, Get), "gitlab_project_member_get"),
		// gitlab_project_member_get_inherited — fetch a project member including inherited.
		memberReadSpec("member_inherited", toolutil.RouteAction(client, GetInherited), "gitlab_project_member_get_inherited"),
		// gitlab_project_member_add — add a user to a project.
		memberCreateSpec("member_add", toolutil.RouteAction(client, Add), "gitlab_project_member_add"),
		// gitlab_project_member_edit — edit a project member's access level.
		memberUpdateSpec("member_edit", toolutil.RouteAction(client, Edit), "gitlab_project_member_edit"),
		// gitlab_project_member_delete — remove a member from a project (destructive).
		memberDeleteSpec("member_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_project_member_delete"),
	}
}

// deleteOutput adapts the package's [Delete] handler to the
// [toolutil.DestructiveAction] contract, returning a structured success
// result that names the resource in the message.
func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("project member")
	return out, nil
}

// memberReadSpec builds a read-only [toolutil.ActionSpec] for a project
// member action using the package's default [memberOptions].
func memberReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, memberOptions(individualTool))
}

// memberCreateSpec builds a create-style [toolutil.ActionSpec] for a
// project member action using the package's default [memberOptions].
func memberCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, memberOptions(individualTool))
}

// memberUpdateSpec builds an update-style [toolutil.ActionSpec] for a
// project member action using the package's default [memberOptions].
func memberUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, memberOptions(individualTool))
}

// memberDeleteSpec builds a destructive [toolutil.ActionSpec] for a
// project member action using the package's default [memberOptions].
func memberDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, memberOptions(individualTool))
}

// memberOptions returns the base [toolutil.ActionSpecOptions] shared by
// every project member action (tags, owner, individual tool metadata).
func memberOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute members domain action.", Tags: []string{"project", "member", "access"},
		RelatedActions: []string{"project.get", "user.get"},
		OpenWorld:      true,
		OwnerPackage:   "members",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
