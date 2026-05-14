package mergetrains

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge train actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		mergeTrainReadSpec("list_project", toolutil.RouteAction(client, ListProjectMergeTrains), "gitlab_list_project_merge_trains"),
		mergeTrainReadSpec("list_branch", toolutil.RouteAction(client, ListMergeRequestInMergeTrain), "gitlab_list_merge_request_in_merge_train"),
		mergeTrainReadSpec("get", toolutil.RouteAction(client, GetMergeRequestOnMergeTrain), "gitlab_get_merge_request_on_merge_train"),
		mergeTrainCreateSpec("add", toolutil.RouteAction(client, AddMergeRequestToMergeTrain), "gitlab_add_merge_request_to_merge_train"),
	}
}

func mergeTrainReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mergeTrainOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func mergeTrainCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, mergeTrainOptions(individualTool))
}

func mergeTrainOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"merge_request", "merge_train"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "mergetrains",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
