package resourcegroups

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for resource group actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		resourceGroupReadSpec("resource_group_list", toolutil.RouteAction(client, ListAll), "gitlab_list_resource_groups"),
		resourceGroupReadSpec("resource_group_get", toolutil.RouteAction(client, Get), "gitlab_get_resource_group"),
		resourceGroupUpdateSpec("resource_group_edit", toolutil.RouteAction(client, Edit), "gitlab_edit_resource_group"),
		resourceGroupReadSpec("resource_group_upcoming_jobs", toolutil.RouteAction(client, ListUpcomingJobs), "gitlab_list_resource_group_upcoming_jobs"),
	}
}

func resourceGroupReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, resourceGroupOptions(name, individualTool))
}

func resourceGroupUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, resourceGroupOptions(name, individualTool))
}

// resourceGroupMeta carries the non-generic discovery metadata for one
// resource group action: action-specific Usage, domain-specific
// natural-language Aliases (distinctive CI resource-group / concurrency
// phrasing), canonical RelatedActions, and the "Returns: … See also: …"
// individual-tool description.
type resourceGroupMeta struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// resourceGroupMetaFor returns the discovery metadata for the given action.
func resourceGroupMetaFor(actionName string) resourceGroupMeta {
	switch actionName {
	case "resource_group_get":
		return resourceGroupMeta{
			usage: "Get one resource group by key.",
			aliases: []string{
				"get resource group",
				"show resource group concurrency settings",
				"inspect resource group process mode",
			},
			related: []string{"resource_group.list", "resource_group.edit", "resource_group.upcoming_jobs"},
			description: "Get one CI resource group in a project by key. Returns: the resource group ID, key, and process mode " +
				"(the concurrency mode that controls how jobs sharing the resource group are serialized). " +
				"See also: gitlab_list_resource_groups, gitlab_edit_resource_group, gitlab_list_resource_group_upcoming_jobs.",
		}
	case "resource_group_edit":
		return resourceGroupMeta{
			usage: "Update one resource group process mode by key.",
			aliases: []string{
				"edit resource group process mode",
				"change job concurrency mode for resource group",
				"set resource group ordering (oldest_first, newest_first, unordered)",
			},
			related: []string{"resource_group.get", "resource_group.list", "resource_group.upcoming_jobs"},
			description: "Update the process mode of one CI resource group by key. Returns: the updated resource group ID, key, " +
				"and new process mode that controls how queued jobs sharing the resource group are serialized. " +
				"See also: gitlab_get_resource_group, gitlab_list_resource_groups, gitlab_list_resource_group_upcoming_jobs.",
		}
	case "resource_group_upcoming_jobs":
		return resourceGroupMeta{
			usage: "List upcoming jobs queued for a resource group by key.",
			aliases: []string{
				"list upcoming jobs for resource group",
				"show jobs queued behind resource group concurrency lock",
				"resource group pending job queue",
			},
			related: []string{"resource_group.get", "resource_group.list", "job.list"},
			description: "List the upcoming CI jobs queued for one resource group by key. Returns: each pending job's ID, name, " +
				"status, and stage, ordered as they will run under the resource group's process mode. " +
				"See also: gitlab_get_resource_group, gitlab_list_resource_groups, gitlab_list_resource_group_upcoming_jobs.",
		}
	default: // resource_group_list
		return resourceGroupMeta{
			usage: "List CI resource groups configured for a project.",
			aliases: []string{
				"list resource groups",
				"show CI concurrency resource groups in project",
				"find resource groups controlling job serialization",
			},
			related: []string{"resource_group.get", "resource_group.edit", "resource_group.upcoming_jobs"},
			description: "List the CI resource groups configured for a project. Returns: each resource group's ID, key, and " +
				"process mode that controls how jobs sharing the group are serialized to limit pipeline concurrency. " +
				"See also: gitlab_get_resource_group, gitlab_edit_resource_group, gitlab_list_resource_group_upcoming_jobs.",
		}
	}
}

func resourceGroupOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	meta := resourceGroupMetaFor(actionName)
	guidance := map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole:   "scope_project",
			ValueSource:    "Project ID or path that owns the resource groups.",
			ExampleBinding: `params.project_id:"group/project"`,
		},
	}
	if actionName != "resource_group_list" {
		guidance["key"] = toolutil.ParameterGuidance{
			SemanticRole:   "resource_group_key",
			ValueSource:    "Resource group key from resource group list output.",
			ExampleBinding: `params.key:"production"`,
		}
	}
	if actionName == "resource_group_edit" {
		guidance["process_mode"] = toolutil.ParameterGuidance{
			SemanticRole:   "resource_group_process_mode",
			ValueSource:    "Requested process mode (unordered, oldest_first, newest_first, newest_ready_first).",
			ExampleBinding: `params.process_mode:"newest_first"`,
		}
	}

	return toolutil.ActionSpecOptions{
		Aliases:           append([]string{individualTool}, meta.aliases...),
		Tags:              []string{"ci", "pipeline", "resource_group"},
		Usage:             meta.usage,
		RelatedActions:    meta.related,
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "resourcegroups",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: meta.description,
		},
	}
}
