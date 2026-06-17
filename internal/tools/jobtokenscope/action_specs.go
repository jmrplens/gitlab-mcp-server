package jobtokenscope

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for CI/CD job token scope actions
// exposed as MCP tools. The read, update, create, and delete routes
// for the project access settings, inbound project allowlist, and
// group allowlist are projected into the dynamic, meta, individual,
// and audit surfaces by the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_get_job_token_access_settings — read the CI/CD job token access settings for a project.
		jobTokenScopeReadSpec("token_scope_get", toolutil.RouteAction(client, GetAccessSettings), "gitlab_get_job_token_access_settings"),
		// gitlab_patch_job_token_access_settings — toggle the CI/CD job token access for a project.
		jobTokenScopeUpdateSpec("token_scope_patch", toolutil.RouteAction(client, PatchAccessSettings), "gitlab_patch_job_token_access_settings"),
		// gitlab_list_job_token_inbound_allowlist — list the project allowlist.
		jobTokenScopeReadSpec("token_scope_list_inbound", toolutil.RouteAction(client, ListInboundAllowlist), "gitlab_list_job_token_inbound_allowlist"),
		// gitlab_add_project_job_token_allowlist — add a project to the inbound allowlist.
		jobTokenScopeCreateSpec("token_scope_add_project", toolutil.RouteAction(client, AddProjectAllowlist), "gitlab_add_project_job_token_allowlist"),
		// gitlab_remove_project_job_token_allowlist — remove a project (destructive, with custom guidance).
		jobTokenScopeRemoveProjectSpec(client),
		// gitlab_list_job_token_group_allowlist — list the group allowlist.
		jobTokenScopeReadSpec("token_scope_list_groups", toolutil.RouteAction(client, ListGroupAllowlist), "gitlab_list_job_token_group_allowlist"),
		// gitlab_add_group_job_token_allowlist — add a group to the allowlist.
		jobTokenScopeCreateSpec("token_scope_add_group", toolutil.RouteAction(client, AddGroupAllowlist), "gitlab_add_group_job_token_allowlist"),
		// gitlab_remove_group_job_token_allowlist — remove a group (destructive, with confirmation).
		jobTokenScopeDeleteSpec("token_scope_remove_group", toolutil.DestructiveAction(client, removeGroupAllowlistOutput), "gitlab_remove_group_job_token_allowlist"),
	}
}

// removeProjectAllowlistOutput adapts [RemoveProjectAllowlist] to the
// [toolutil.DestructiveAction] contract, returning a structured success
// result that names the resource in the message.
func removeProjectAllowlistOutput(ctx context.Context, client *gitlabclient.Client, input RemoveProjectAllowlistInput) (toolutil.DeleteOutput, error) {
	if err := RemoveProjectAllowlist(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("project from job token allowlist")
	return out, nil
}

// removeGroupAllowlistOutput adapts [RemoveGroupAllowlist] to the
// [toolutil.DestructiveAction] contract, returning a structured success
// result that names the resource in the message.
func removeGroupAllowlistOutput(ctx context.Context, client *gitlabclient.Client, input RemoveGroupAllowlistInput) (toolutil.DeleteOutput, error) {
	if err := RemoveGroupAllowlist(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("group from job token allowlist")
	return out, nil
}

// jobTokenScopeRemoveProjectSpec builds the destructive
// [toolutil.ActionSpec] for the gitlab_remove_project_job_token_allowlist
// individual tool, with custom parameter guidance to disambiguate the
// owning project (project_id) from the project being removed
// (target_project_id).
func jobTokenScopeRemoveProjectSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := jobTokenScopeOptions("gitlab_remove_project_job_token_allowlist")
	options.Usage = "Use when removing a target project from another project's CI job token inbound allowlist."
	options.RelatedActions = []string{"job.token_scope_list_inbound", "job.token_scope_add_project", "job.token_scope_remove_group"}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole:     "scope_owner_project",
			ValueSource:      "Owning project whose CI job token allowlist is being changed.",
			CommonConfusions: []string{"Do not use the project being removed as project_id."},
			ExampleBinding:   "Remove project ID 51 from allowlist of project 1 => project_id=1.",
		},
		"target_project_id": {
			SemanticRole:     "target_project",
			ValueSource:      "Project being removed from or added to the allowlist.",
			CommonConfusions: []string{"Do not put the allowlist owner project here."},
			ExampleBinding:   "Remove project ID 51 from allowlist of project 1 => target_project_id=51.",
		},
	}
	return toolutil.NewDeleteActionSpec("token_scope_remove_project", toolutil.DestructiveAction(client, removeProjectAllowlistOutput), options)
}

// jobTokenScopeReadSpec builds a read-only [toolutil.ActionSpec] for a
// job token scope action using the package's default
// [jobTokenScopeOptions].
func jobTokenScopeReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, jobTokenScopeOptions(individualTool))
}

// jobTokenScopeCreateSpec builds a create-style [toolutil.ActionSpec]
// for a job token scope action using the package's default
// [jobTokenScopeOptions].
func jobTokenScopeCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, jobTokenScopeOptions(individualTool))
}

// jobTokenScopeUpdateSpec builds an update-style [toolutil.ActionSpec]
// for a job token scope action using the package's default
// [jobTokenScopeOptions].
func jobTokenScopeUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, jobTokenScopeOptions(individualTool))
}

// jobTokenScopeDeleteSpec builds a destructive [toolutil.ActionSpec]
// for a job token scope action using the package's default
// [jobTokenScopeOptions].
func jobTokenScopeDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, jobTokenScopeOptions(individualTool))
}

// jobTokenScopeOptions returns the base [toolutil.ActionSpecOptions]
// shared by every job token scope action (tags, owner, individual
// tool metadata).
func jobTokenScopeOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute jobtokenscope domain action.", Tags: []string{"job", "token-scope", "allowlist"},
		OpenWorld:      true,
		OwnerPackage:   "jobtokenscope",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
