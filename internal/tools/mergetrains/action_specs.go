package mergetrains

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionMergeTrainListProject = "merge_train.list_project"
	actionMergeTrainListBranch  = "merge_train.list_branch"
	actionMergeTrainGet         = "merge_train.get"
	actionMergeTrainAdd         = "merge_train.add"
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
// merge train action, applying the action's discovery metadata
// (usage, aliases, related actions, and the "Returns: … See also: …"
// individual-tool description).
func mergeTrainReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mergeTrainOptions(individualTool)
	decorateMergeTrainMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

// mergeTrainCreateSpec builds a create-style [toolutil.ActionSpec] for
// a merge train action, applying the action's discovery metadata.
func mergeTrainCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mergeTrainOptions(individualTool)
	decorateMergeTrainMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

// mergeTrainActionMetaEntry is the discovery metadata for one merge train action.
type mergeTrainActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// mergeTrainActionMeta maps each individual merge train tool to its discovery
// metadata, giving every tool a non-generic usage, natural-language aliases,
// related actions, and a "Returns: … See also: …" individual-tool description
// (1:1 audit R-META).
var mergeTrainActionMeta = map[string]mergeTrainActionMetaEntry{
	"gitlab_list_project_merge_trains": {
		usage:       "List every merge train in a project. Use when the prompt asks for all merge trains, active trains, or completed trains in a known project, optionally filtered by scope and sorted.",
		aliases:     []string{"list project merge trains", "show merge trains", "find merge trains in project"},
		related:     []string{actionMergeTrainListBranch, actionMergeTrainGet, actionMergeTrainAdd},
		description: "List all merge trains in a project. Returns: merge train entries with the merge request, user, pipeline, target branch, status, duration, and pagination metadata. See also: gitlab_list_merge_request_in_merge_train, gitlab_get_merge_request_on_merge_train, gitlab_add_merge_request_to_merge_train.",
	},
	"gitlab_list_merge_request_in_merge_train": {
		usage:       "List the merge requests sitting on the merge train for one target branch. Use when work is scoped to a specific branch's merge train rather than the whole project.",
		aliases:     []string{"list merge requests in merge train", "show merge train for branch", "merge train queue for branch"},
		related:     []string{actionMergeTrainListProject, actionMergeTrainGet, actionMergeTrainAdd},
		description: "List the merge requests on a merge train for a target branch. Returns: merge train entries with the merge request, user, pipeline, status, duration, and pagination metadata. See also: gitlab_list_project_merge_trains, gitlab_get_merge_request_on_merge_train, gitlab_add_merge_request_to_merge_train.",
	},
	"gitlab_get_merge_request_on_merge_train": {
		usage:       "Fetch the merge train status of a single merge request by its IID. Use when the prompt names a concrete MR and asks whether or where it sits on a merge train.",
		aliases:     []string{"get merge request on merge train", "merge train status of mr", "show mr merge train"},
		related:     []string{actionMergeTrainListProject, actionMergeTrainListBranch, actionMergeTrainAdd},
		description: "Get the merge train status of a single merge request. Returns: the merge train entry with the merge request, user, pipeline, target branch, status, and duration. See also: gitlab_list_project_merge_trains, gitlab_list_merge_request_in_merge_train, gitlab_add_merge_request_to_merge_train.",
	},
	"gitlab_add_merge_request_to_merge_train": {
		usage:       "Add a merge request to its target branch's merge train. Requires the MR to be approved with a passing pipeline; optionally enable auto_merge, verify a head sha, or squash on merge.",
		aliases:     []string{"add merge request to merge train", "enqueue mr on merge train", "merge train an mr"},
		related:     []string{actionMergeTrainGet, actionMergeTrainListBranch, actionMergeTrainListProject},
		description: "Add a merge request to a merge train. Returns: the resulting merge train entries with the merge request, user, pipeline, target branch, status, and duration. See also: gitlab_get_merge_request_on_merge_train, gitlab_list_merge_request_in_merge_train, gitlab_list_project_merge_trains.",
	},
}

// decorateMergeTrainMeta fills non-generic Usage, natural-language Aliases,
// RelatedActions, and the "Returns: … See also: …" individual-tool description
// for a merge train action, replacing the generic placeholder metadata from
// [mergeTrainOptions].
func decorateMergeTrainMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := mergeTrainActionMeta[individualTool]
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
