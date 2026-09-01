package jobtokenscope

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionJobTokenScopeAddProject    = "job.token_scope_add_project"
	actionJobTokenScopeAddGroup      = "job.token_scope_add_group"
	actionJobTokenScopeRemoveProject = "job.token_scope_remove_project"
	actionJobTokenScopeRemoveGroup   = "job.token_scope_remove_group"
	actionJobTokenScopeListInbound   = "job.token_scope_list_inbound"
	actionJobTokenScopeListGroups    = "job.token_scope_list_groups"
	actionJobTokenScopeGet           = "job.token_scope_get"
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
	options.Usage = "Remove a project from another project's CI/CD job token inbound allowlist, revoking its inbound job token access. Destructive. Confirm project_id and target_project_id first."
	options.Aliases = []string{"remove project from job token allowlist", "revoke project job token access", "delete project job token allowlist entry"}
	options.RelatedActions = []string{actionJobTokenScopeListInbound, actionJobTokenScopeAddProject, actionJobTokenScopeRemoveGroup}
	options.IndividualTool.Description = "Remove a project from a project's CI/CD job token inbound allowlist. Returns: a success confirmation naming the removed project. See also: gitlab_list_job_token_inbound_allowlist, gitlab_add_project_job_token_allowlist, gitlab_remove_group_job_token_allowlist."
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
// [jobTokenScopeOptions], decorated with non-generic discovery metadata.
func jobTokenScopeReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := jobTokenScopeOptions(individualTool)
	decorateJobTokenScopeMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

// jobTokenScopeCreateSpec builds a create-style [toolutil.ActionSpec]
// for a job token scope action using the package's default
// [jobTokenScopeOptions], decorated with non-generic discovery metadata.
func jobTokenScopeCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := jobTokenScopeOptions(individualTool)
	decorateJobTokenScopeMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

// jobTokenScopeUpdateSpec builds an update-style [toolutil.ActionSpec]
// for a job token scope action using the package's default
// [jobTokenScopeOptions], decorated with non-generic discovery metadata.
func jobTokenScopeUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := jobTokenScopeOptions(individualTool)
	decorateJobTokenScopeMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

// jobTokenScopeDeleteSpec builds a destructive [toolutil.ActionSpec]
// for a job token scope action using the package's default
// [jobTokenScopeOptions], decorated with non-generic discovery metadata.
func jobTokenScopeDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := jobTokenScopeOptions(individualTool)
	decorateJobTokenScopeMeta(&options, individualTool)
	return toolutil.NewDeleteActionSpec(name, route, options)
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

// decorateJobTokenScopeMeta fills non-generic Usage, natural-language
// Aliases, RelatedActions, and the "Returns: … See also: …" individual-tool
// description for the job token scope actions that would otherwise inherit
// the generic placeholder metadata from [jobTokenScopeOptions]. It is a
// no-op for tools whose dedicated spec builders already set rich metadata
// (gitlab_remove_project_job_token_allowlist).
func decorateJobTokenScopeMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := jobTokenScopeActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string(nil), meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}

// jobTokenScopeActionMetaEntry is the discovery metadata for one job
// token scope action.
type jobTokenScopeActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// jobTokenScopeActionMeta maps each individual job token scope tool to its
// discovery metadata. The gitlab_remove_project_job_token_allowlist tool is
// intentionally absent: its dedicated spec builder
// ([jobTokenScopeRemoveProjectSpec]) already sets rich usage, related actions,
// and parameter guidance.
var jobTokenScopeActionMeta = map[string]jobTokenScopeActionMetaEntry{
	"gitlab_get_job_token_access_settings": {
		usage:       "Read whether a project limits CI/CD job token access to an inbound allowlist. Use before changing allowlist membership to confirm the inbound scope is enabled.",
		aliases:     []string{"get job token access settings", "show job token scope", "is job token scope enabled"},
		related:     []string{"job.token_scope_patch", actionJobTokenScopeListInbound, actionJobTokenScopeListGroups},
		description: "Get a project's CI/CD job token access settings. Returns: whether inbound job token access is limited to the allowlist. See also: gitlab_patch_job_token_access_settings, gitlab_list_job_token_inbound_allowlist, gitlab_list_job_token_group_allowlist.",
	},
	"gitlab_patch_job_token_access_settings": {
		usage:       "Enable or disable the inbound CI/CD job token scope for a project. Enabling restricts which projects' job tokens may access this project to the inbound allowlist.",
		aliases:     []string{"enable job token scope", "disable job token scope", "toggle job token access settings"},
		related:     []string{actionJobTokenScopeGet, actionJobTokenScopeAddProject, actionJobTokenScopeAddGroup},
		description: "Update a project's CI/CD job token access settings. Returns: a confirmation that the inbound scope setting was updated. See also: gitlab_get_job_token_access_settings, gitlab_add_project_job_token_allowlist, gitlab_add_group_job_token_allowlist.",
	},
	"gitlab_list_job_token_inbound_allowlist": {
		usage:       "List the projects whose CI/CD job tokens are allowed inbound access to this project. Use to audit allowlist membership or before adding or removing a project.",
		aliases:     []string{"list job token allowlist projects", "show inbound job token allowlist", "audit job token project allowlist"},
		related:     []string{actionJobTokenScopeAddProject, actionJobTokenScopeRemoveProject, actionJobTokenScopeGet},
		description: "List projects on a project's CI/CD job token inbound allowlist. Returns: allowlisted projects with id, name, path, and web URL plus pagination metadata. See also: gitlab_add_project_job_token_allowlist, gitlab_remove_project_job_token_allowlist, gitlab_get_job_token_access_settings.",
	},
	"gitlab_add_project_job_token_allowlist": {
		usage:       "Add a project to this project's CI/CD job token inbound allowlist so its job tokens may access this project. Requires the inbound scope to be enabled.",
		aliases:     []string{"add project to job token allowlist", "allow project job token access", "grant inbound job token access"},
		related:     []string{actionJobTokenScopeListInbound, actionJobTokenScopeRemoveProject, actionJobTokenScopeAddGroup},
		description: "Add a project to a project's CI/CD job token inbound allowlist. Returns: the new allowlist entry with source and target project IDs. See also: gitlab_list_job_token_inbound_allowlist, gitlab_remove_project_job_token_allowlist, gitlab_add_group_job_token_allowlist.",
	},
	"gitlab_list_job_token_group_allowlist": {
		usage:       "List the groups allowed inbound CI/CD job token access to this project. Use to audit group allowlist membership. Requires GitLab 17.0+.",
		aliases:     []string{"list job token allowlist groups", "show group job token allowlist", "audit job token group allowlist"},
		related:     []string{actionJobTokenScopeAddGroup, actionJobTokenScopeRemoveGroup, actionJobTokenScopeGet},
		description: "List groups on a project's CI/CD job token allowlist. Returns: allowlisted groups with id, name, full path, and web URL plus pagination metadata. See also: gitlab_add_group_job_token_allowlist, gitlab_remove_group_job_token_allowlist, gitlab_get_job_token_access_settings.",
	},
	"gitlab_add_group_job_token_allowlist": {
		usage:       "Add a group to this project's CI/CD job token allowlist so job tokens from the group's projects may access this project. Requires GitLab 17.0+.",
		aliases:     []string{"add group to job token allowlist", "allow group job token access", "grant group inbound job token access"},
		related:     []string{actionJobTokenScopeListGroups, actionJobTokenScopeRemoveGroup, actionJobTokenScopeAddProject},
		description: "Add a group to a project's CI/CD job token allowlist. Returns: the new allowlist entry with source project ID and target group ID. See also: gitlab_list_job_token_group_allowlist, gitlab_remove_group_job_token_allowlist, gitlab_add_project_job_token_allowlist.",
	},
	"gitlab_remove_group_job_token_allowlist": {
		usage:       "Remove a group from this project's CI/CD job token allowlist, revoking inbound job token access for the group's projects. Destructive. Confirm project_id and target_group_id first.",
		aliases:     []string{"remove group from job token allowlist", "revoke group job token access", "delete group job token allowlist entry"},
		related:     []string{actionJobTokenScopeListGroups, actionJobTokenScopeAddGroup, actionJobTokenScopeRemoveProject},
		description: "Remove a group from a project's CI/CD job token allowlist. Returns: a success confirmation naming the removed group. See also: gitlab_list_job_token_group_allowlist, gitlab_add_group_job_token_allowlist, gitlab_remove_project_job_token_allowlist.",
	},
}
