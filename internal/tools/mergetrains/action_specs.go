package mergetrains

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge train actions exposed
// as MCP tools. The list, get, and add routes are projected into the
// dynamic, meta, individual, and audit surfaces by the action catalog
// (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_list_project_merge_trains — list every merge train in a project.
		mergeTrainReadSpec("list_project", toolutil.RouteAction(client, ListProjectMergeTrains), "gitlab_list_project_merge_trains"),
		// gitlab_list_merge_request_in_merge_train — list MRs on the merge train for a branch.
		mergeTrainReadSpec("list_branch", toolutil.RouteAction(client, ListMergeRequestInMergeTrain), "gitlab_list_merge_request_in_merge_train"),
		// gitlab_get_merge_request_on_merge_train — fetch a single MR's merge train status.
		mergeTrainReadSpec("get", toolutil.RouteAction(client, GetMergeRequestOnMergeTrain), "gitlab_get_merge_request_on_merge_train"),
		// gitlab_add_merge_request_to_merge_train — add an MR to the merge train.
		mergeTrainCreateSpec("add", toolutil.RouteAction(client, AddMergeRequestToMergeTrain), "gitlab_add_merge_request_to_merge_train"),
	}
}

// mergeTrainReadSpec builds a read-only [toolutil.ActionSpec] for a
// merge train action using the package's default [mergeTrainOptions].
func mergeTrainReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, mergeTrainOptions(individualTool))
}

// mergeTrainCreateSpec builds a create-style [toolutil.ActionSpec] for
// a merge train action using the package's default
// [mergeTrainOptions].
func mergeTrainCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, mergeTrainOptions(individualTool))
}

// mergeTrainOptions returns the base [toolutil.ActionSpecOptions]
// shared by every merge train action (tags, edition, owner,
// individual tool metadata).
func mergeTrainOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute mergetrains domain action.", Tags: []string{"merge_request", "merge_train"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "mergetrains",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
