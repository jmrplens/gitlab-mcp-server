package mergetrains

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	return toolutil.NewReadActionSpec(name, route, mergeTrainOptions(individualTool))
}

func mergeTrainCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, mergeTrainOptions(individualTool))
}

func mergeTrainOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute mergetrains domain action.", Tags: []string{"merge_request", "merge_train"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "mergetrains",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
