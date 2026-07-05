package securityscanprofiles

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	schemaType     = "type"
	schemaTypeArr  = "array"
	schemaMinItems = "minItems"

	actionAttach              = "security_scan_profile.attach"
	actionDetach              = "security_scan_profile.detach"
	actionListProjectStatuses = "security_scan_profile.list_project_statuses"
	actionProjectGet          = "project.get"
	actionVulnList            = "vulnerability.list"

	descriptionAttach = "Attach a GitLab security scan profile to one or more projects and/or groups via GraphQL. Requires Ultimate. Returns: attach confirmation with the resolved profile and targets. See also: gitlab_detach_security_scan_profile, gitlab_list_project_scan_profile_statuses, gitlab_project. API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecurityscanprofileattach"
	descriptionDetach = "Detach a GitLab security scan profile from one or more projects and/or groups via GraphQL. Requires Ultimate. Returns: detach confirmation with the resolved profile and targets. See also: gitlab_attach_security_scan_profile, gitlab_list_project_scan_profile_statuses, gitlab_project. API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecurityscanprofiledetach"
	descriptionList   = "List the security scan profile statuses for a GitLab project via GraphQL. Requires Ultimate. Returns: per-scan-type profile status (NOT_CONFIGURED, PENDING, ACTIVE, WARNING, FAILED, or STALE). See also: gitlab_attach_security_scan_profile, gitlab_detach_security_scan_profile, gitlab_project. API docs: https://docs.gitlab.com/api/graphql/reference/#project-scanprofilestatuses"
)

// ActionSpecs returns canonical specs for security scan profile actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_attach_security_scan_profile — attach a scan profile to projects/groups.
		attachSpec("attach", toolutil.RouteAction(client, Attach), "gitlab_attach_security_scan_profile", descriptionAttach),
		// gitlab_detach_security_scan_profile — detach a scan profile from projects/groups.
		detachSpec("detach", toolutil.DestructiveAction(client, Detach), "gitlab_detach_security_scan_profile", descriptionDetach),
		// gitlab_list_project_scan_profile_statuses — list a project's scan profile statuses.
		listProjectStatusesSpec("list_project_statuses", toolutil.RouteAction(client, ListProjectStatuses), "gitlab_list_project_scan_profile_statuses", descriptionList),
	}
}

// targetSchemaOverrides constrains project_ids/group_ids to non-empty arrays and
// requires at least one of them. Shared by the attach and detach specs, whose
// project/group target shape is identical.
func targetSchemaOverrides() []toolutil.InputSchemaOverride {
	return []toolutil.InputSchemaOverride{
		toolutil.SchemaAnyOfRequired("project_ids", "group_ids"),
		toolutil.SchemaPropertyOverride("project_ids", map[string]any{schemaType: schemaTypeArr, schemaMinItems: 1}),
		toolutil.SchemaPropertyOverride("group_ids", map[string]any{schemaType: schemaTypeArr, schemaMinItems: 1}),
	}
}

// attachSpec builds the canonical create spec for attaching a scan profile.
func attachSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := scanProfileOptions(individualTool, description)
	options.InputSchemaOverrides = targetSchemaOverrides()
	options.Usage = "Attach a security scan profile to one or more projects and/or groups. Supply security_scan_profile_id (a built-in scan type — dependency_scanning, sast, secret_detection, or container_scanning — which creates the namespace's default profile on the fly) and at least one of project_ids or group_ids. Targets must belong to a group namespace (not a personal namespace) and share one root namespace. Requires Maintainer or Owner on the targets."
	options.Aliases = []string{"attach security scan profile", "enable scan profile on project", "assign scan profile to group", "apply security scan configuration"}
	options.RelatedActions = []string{actionDetach, actionListProjectStatuses, actionProjectGet}
	return toolutil.NewCreateActionSpec(name, route, options)
}

// detachSpec builds the canonical destructive spec for detaching a scan profile.
func detachSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := scanProfileOptions(individualTool, description)
	options.InputSchemaOverrides = targetSchemaOverrides()
	options.Usage = "Detach a security scan profile from one or more projects and/or groups, disabling that scanning configuration on the targets. Supply security_scan_profile_id (the persisted profile's numeric ID from gitlab_list_project_scan_profile_statuses, not a scan-type name) and at least one of project_ids or group_ids. This is reversible with attach."
	options.Aliases = []string{"detach security scan profile", "disable scan profile on project", "remove scan profile from group", "unassign security scan configuration"}
	options.RelatedActions = []string{actionAttach, actionListProjectStatuses, actionProjectGet}
	return toolutil.NewDeleteActionSpec(name, route, options)
}

// listProjectStatusesSpec builds the canonical read spec for listing scan
// profile statuses on a project.
func listProjectStatusesSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := scanProfileOptions(individualTool, description)
	options.ContentKind = toolutil.ActionSpecContentList
	options.Usage = "List the security scan profile statuses for a project by full path (namespace/project). Use this to check which scan profiles are active, pending, failing, or not configured before attaching or detaching."
	options.Aliases = []string{"list scan profile statuses", "get project scan profile status", "show security scan configuration status", "check scan profile status"}
	options.RelatedActions = []string{actionAttach, actionDetach, actionVulnList, actionProjectGet}
	return toolutil.NewReadActionSpec(name, route, options)
}

func scanProfileOptions(individualTool, description string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute securityscanprofiles domain action.", Tags: []string{"security", "scan", "profile", "graphql"},
		RelatedActions: []string{actionAttach, actionDetach, actionListProjectStatuses, actionProjectGet},
		OpenWorld:      true,
		Edition:        "ultimate",
		OwnerPackage:   "securityscanprofiles",
		ContentKind:    toolutil.ActionSpecContentMutate,
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
}
