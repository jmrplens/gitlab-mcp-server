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

func resourceGroupOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	usage := "List CI resource groups configured for a project."
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
		usage = "Update one resource group process mode by key."
		guidance["process_mode"] = toolutil.ParameterGuidance{
			SemanticRole:   "resource_group_process_mode",
			ValueSource:    "Requested process mode (unordered, oldest_first, newest_first, newest_ready_first).",
			ExampleBinding: `params.process_mode:"newest_first"`,
		}
	}
	if actionName == "resource_group_get" {
		usage = "Get one resource group by key."
	}
	if actionName == "resource_group_upcoming_jobs" {
		usage = "List upcoming jobs queued for a resource group by key."
	}

	return toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"ci", "pipeline", "resource_group"},
		Usage:             usage,
		RelatedActions:    []string{"pipeline.list", "job.list"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "resourcegroups",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
