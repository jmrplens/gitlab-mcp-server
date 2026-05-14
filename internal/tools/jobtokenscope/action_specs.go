package jobtokenscope

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for CI/CD job token scope actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		jobTokenScopeReadSpec("token_scope_get", toolutil.RouteAction(client, GetAccessSettings), "gitlab_get_job_token_access_settings"),
		jobTokenScopeUpdateSpec("token_scope_patch", toolutil.RouteAction(client, PatchAccessSettings), "gitlab_patch_job_token_access_settings"),
		jobTokenScopeReadSpec("token_scope_list_inbound", toolutil.RouteAction(client, ListInboundAllowlist), "gitlab_list_job_token_inbound_allowlist"),
		jobTokenScopeCreateSpec("token_scope_add_project", toolutil.RouteAction(client, AddProjectAllowlist), "gitlab_add_project_job_token_allowlist"),
		jobTokenScopeRemoveProjectSpec(client),
		jobTokenScopeReadSpec("token_scope_list_groups", toolutil.RouteAction(client, ListGroupAllowlist), "gitlab_list_job_token_group_allowlist"),
		jobTokenScopeCreateSpec("token_scope_add_group", toolutil.RouteAction(client, AddGroupAllowlist), "gitlab_add_group_job_token_allowlist"),
		jobTokenScopeDeleteSpec("token_scope_remove_group", toolutil.DestructiveVoidAction(client, RemoveGroupAllowlist), "gitlab_remove_group_job_token_allowlist"),
	}
}

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
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec("token_scope_remove_project", toolutil.DestructiveVoidAction(client, RemoveProjectAllowlist), options)
}

func jobTokenScopeReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := jobTokenScopeOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func jobTokenScopeCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, jobTokenScopeOptions(individualTool))
}

func jobTokenScopeUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := jobTokenScopeOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func jobTokenScopeDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := jobTokenScopeOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func jobTokenScopeOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"job", "token-scope", "allowlist"},
		OpenWorld:      true,
		OwnerPackage:   "jobtokenscope",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
