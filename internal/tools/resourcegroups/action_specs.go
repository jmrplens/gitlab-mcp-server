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
	options := resourceGroupOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func resourceGroupUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := resourceGroupOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func resourceGroupOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"ci", "pipeline", "resource_group"},
		OpenWorld:      true,
		OwnerPackage:   "resourcegroups",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
