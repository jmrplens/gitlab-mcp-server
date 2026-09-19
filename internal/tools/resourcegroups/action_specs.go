package resourcegroups

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// catalogDomain is the catalog group these specs are aggregated into with its
// gitlab_ prefix removed: resource group actions are routes on gitlab_pipeline
// rather than a group of their own (buildPipelineActionSpecs in
// internal/tools/action_specs.go). A canonical action ID is that domain, a dot,
// and the spec name.
const catalogDomain = "pipeline"

// The names ActionSpecs registers each action under.
const (
	specResourceGroupList         = "resource_group_list"
	specResourceGroupGet          = "resource_group_get"
	specResourceGroupEdit         = "resource_group_edit"
	specResourceGroupUpcomingJobs = "resource_group_upcoming_jobs"
)

// The canonical catalog IDs those four actions resolve to, which is what a
// RelatedActions entry and a Markdown hint have to name. This block and one in
// markdown.go each held a copy, and the two had drifted: every RelatedActions
// entry named a resource_group.* domain the catalog has never held, so a model
// following one was answered "unknown action". One block now, spelled out
// rather than concatenated so it reads as the string a model receives, with
// TestActionSpecs_CanonicalIDConstantsMatchTheRegisteredSpecs holding each to
// catalogDomain and the name beside it.
const (
	actionResourceGroupList         = "pipeline.resource_group_list"
	actionResourceGroupGet          = "pipeline.resource_group_get"
	actionResourceGroupEdit         = "pipeline.resource_group_edit"
	actionResourceGroupUpcomingJobs = "pipeline.resource_group_upcoming_jobs"
)

// Actions of the gitlab_job group that this package's hints and related
// metadata point at. They come from another package's specs, so
// TestActionSpecs_RelatedActionsNameActionsThatExist cannot derive them and
// holds every foreign reference to this declared list instead.
const (
	actionJobGet   = "job.get"
	actionJobTrace = "job.trace"
	actionJobList  = "job.list"
)

// ActionSpecs returns canonical specs for resource group actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		resourceGroupReadSpec(specResourceGroupList, toolutil.RouteAction(client, ListAll), "gitlab_list_resource_groups"),
		resourceGroupReadSpec(specResourceGroupGet, toolutil.RouteAction(client, Get), "gitlab_get_resource_group"),
		resourceGroupUpdateSpec(specResourceGroupEdit, toolutil.RouteAction(client, Edit), "gitlab_edit_resource_group"),
		resourceGroupReadSpec(specResourceGroupUpcomingJobs, toolutil.RouteAction(client, ListUpcomingJobs), "gitlab_list_resource_group_upcoming_jobs"),
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
	case specResourceGroupGet:
		return resourceGroupMeta{
			usage: "Get one resource group by key.",
			aliases: []string{
				"get resource group",
				"show resource group concurrency settings",
				"inspect resource group process mode",
			},
			related: []string{actionResourceGroupList, actionResourceGroupEdit, actionResourceGroupUpcomingJobs},
			description: "Get one CI resource group in a project by key. Returns: the resource group ID, key, and process mode " +
				"(the concurrency mode that controls how jobs sharing the resource group are serialized). " +
				"See also: gitlab_list_resource_groups, gitlab_edit_resource_group, gitlab_list_resource_group_upcoming_jobs.",
		}
	case specResourceGroupEdit:
		return resourceGroupMeta{
			usage: "Update one resource group process mode by key.",
			aliases: []string{
				"edit resource group process mode",
				"change job concurrency mode for resource group",
				"set resource group ordering (oldest_first, newest_first, unordered)",
			},
			related: []string{actionResourceGroupGet, actionResourceGroupList, actionResourceGroupUpcomingJobs},
			description: "Update the process mode of one CI resource group by key. Returns: the updated resource group ID, key, " +
				"and new process mode that controls how queued jobs sharing the resource group are serialized. " +
				"See also: gitlab_get_resource_group, gitlab_list_resource_groups, gitlab_list_resource_group_upcoming_jobs.",
		}
	case specResourceGroupUpcomingJobs:
		return resourceGroupMeta{
			usage: "List upcoming jobs queued for a resource group by key.",
			aliases: []string{
				"list upcoming jobs for resource group",
				"show jobs queued behind resource group concurrency lock",
				"resource group pending job queue",
			},
			related: []string{actionResourceGroupGet, actionResourceGroupList, actionJobList},
			description: "List the upcoming CI jobs queued for one resource group by key. Returns: each pending job's ID, name, " +
				"status, and stage, ordered as they will run under the resource group's process mode. " +
				"See also: gitlab_get_resource_group, gitlab_list_resource_groups, gitlab_job_list.",
		}
	default: // specResourceGroupList
		return resourceGroupMeta{
			usage: "List CI resource groups configured for a project.",
			aliases: []string{
				"list resource groups",
				"show CI concurrency resource groups in project",
				"find resource groups controlling job serialization",
			},
			related: []string{actionResourceGroupGet, actionResourceGroupEdit, actionResourceGroupUpcomingJobs},
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
	if actionName != specResourceGroupList {
		guidance["key"] = toolutil.ParameterGuidance{
			SemanticRole:   "resource_group_key",
			ValueSource:    "Resource group key from resource group list output.",
			ExampleBinding: `params.key:"production"`,
		}
	}
	var overrides []toolutil.InputSchemaOverride
	if actionName == specResourceGroupEdit {
		guidance["process_mode"] = toolutil.ParameterGuidance{
			SemanticRole:   "resource_group_process_mode",
			ValueSource:    "Requested process mode (unordered, oldest_first, newest_first, newest_ready_first).",
			ExampleBinding: `params.process_mode:"newest_first"`,
		}
		overrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("process_mode", "unordered", "oldest_first", "newest_first", "newest_ready_first"),
		}
	}

	return toolutil.ActionSpecOptions{
		Aliases:              append([]string{individualTool}, meta.aliases...),
		Tags:                 []string{"ci", "pipeline", "resource_group"},
		Usage:                meta.usage,
		RelatedActions:       meta.related,
		ParameterGuidance:    guidance,
		InputSchemaOverrides: overrides,
		OpenWorld:            true,
		OwnerPackage:         "resourcegroups",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: meta.description,
		},
	}
}
