package mrchanges

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action IDs for the mr_review-projected change actions, used for
// RelatedActions cross-links so discovery surfaces resolve to real routes.
const (
	actionChangesGet        = "mr_review.changes_get"
	actionRawDiffs          = "mr_review.raw_diffs"
	actionDiffVersionsList  = "mr_review.diff_versions_list"
	actionDiffVersionGet    = "mr_review.diff_version_get"
	actionMRGet             = "merge_request.get"
	actionMRDiscussionsList = "mr_review.discussion_list"
)

// ActionSpecs returns canonical specs for merge request changes and diff version actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		mrChangeReadSpec("changes_get", toolutil.RouteAction(client, Get), "gitlab_mr_changes_get"),
		mrChangeReadSpec("raw_diffs", toolutil.RouteAction(client, RawDiffs), "gitlab_mr_raw_diffs"),
		mrChangeReadSpec("diff_versions_list", toolutil.RouteAction(client, ListDiffVersions), "gitlab_mr_diff_versions_list"),
		mrChangeReadSpec("diff_version_get", toolutil.RouteAction(client, GetDiffVersion), "gitlab_mr_diff_version_get"),
	}
}

// mrChangeReadSpec builds a read-only [toolutil.ActionSpec] for an MR-changes
// action and fills in the action-specific Usage, natural-language Aliases,
// canonical RelatedActions, ParameterGuidance, and the "Returns: … See also: …"
// individual-tool description (1:1 audit R-META).
func mrChangeReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mrChangeOptions(individualTool)
	decorateMRChangeMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

// decorateMRChangeMeta applies the per-action discovery metadata, replacing the
// generic placeholder Usage and toolname-only aliases inherited from
// mrChangeOptions. No-op for unknown tools.
func decorateMRChangeMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := mrChangeActionMeta[individualTool]
	if !ok {
		return
	}
	options.Usage = meta.usage
	options.Aliases = append([]string{individualTool}, meta.aliases...)
	options.RelatedActions = append([]string(nil), meta.related...)
	options.IndividualTool.Description = meta.description
	if len(meta.params) > 0 {
		options.ParameterGuidance = meta.params
	}
}

// mrChangeActionMetaEntry is the discovery metadata for one MR-changes action.
type mrChangeActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
	params      map[string]toolutil.ParameterGuidance
}

// scopeProjectGuidance and mrIIDGuidance are the shared parameter guidance for
// the project_id and merge_request_iid inputs common to all MR-changes actions.
var (
	scopeProjectGuidance = toolutil.ParameterGuidance{
		SemanticRole:     "scope_project",
		ValueSource:      "Project ID or full namespace path that owns the merge request.",
		ExampleBinding:   `params.project_id:"group/project"`,
		CommonConfusions: []string{"Use the MR's parent project here, not a group path or the MR's global ID."},
	}
	mrIIDGuidance = toolutil.ParameterGuidance{
		SemanticRole:     "merge_request_iid",
		ValueSource:      "Merge request number visible in the project, from the MR URL or prior list output.",
		ExampleBinding:   "params.merge_request_iid:42",
		CommonConfusions: []string{"Use the project-scoped IID, not the MR's global database ID."},
	}
)

// mrChangeActionMeta maps each individual MR-changes tool to its discovery metadata.
var mrChangeActionMeta = map[string]mrChangeActionMetaEntry{
	"gitlab_mr_changes_get": {
		usage:   "Get the current file diffs (changes) for a merge request by project_id and merge_request_iid. Use this first when the prompt asks to inspect, view, or analyze MR changes without running an LLM analyzer. Returns structured per-file diffs with old/new paths and line metadata for inline review.",
		aliases: []string{"get merge request changes", "view mr changes", "inspect mr diff", "list merge request changes", "mr code diff"},
		related: []string{actionRawDiffs, actionDiffVersionsList, actionMRGet, actionMRDiscussionsList},
		description: "Get the current file diffs (changes) for a merge request. " +
			"Returns: per-file diffs with old_path, new_path, diff content, file status (added/deleted/renamed), file modes, and truncated_files when GitLab collapses large diffs. " +
			"See also: gitlab_mr_diff_versions_list, gitlab_mr_raw_diffs, gitlab_mr_get.",
		params: map[string]toolutil.ParameterGuidance{
			"project_id":        scopeProjectGuidance,
			"merge_request_iid": mrIIDGuidance,
		},
	},
	"gitlab_mr_raw_diffs": {
		usage:   "Get the raw unified-diff output for a merge request in plain-text git-apply format. Use this when a raw patch or unified diff is requested, or when changes_get reports truncated_files and the full diff payload is needed.",
		aliases: []string{"get raw merge request diff", "mr unified diff", "mr patch", "raw mr diff", "git apply patch for mr"},
		related: []string{actionChangesGet, actionDiffVersionsList, actionMRGet},
		description: "Get the raw unified-diff output for a merge request in plain-text git-apply format. " +
			"Returns: raw_diff, a single plain-text unified diff for the MR head suitable for patch application or programmatic analysis. " +
			"See also: gitlab_mr_changes_get, gitlab_mr_diff_versions_list.",
		params: map[string]toolutil.ParameterGuidance{
			"project_id":        scopeProjectGuidance,
			"merge_request_iid": mrIIDGuidance,
		},
	},
	"gitlab_mr_diff_versions_list": {
		usage:   "List the diff versions (historical snapshots) of a merge request. Use when the prompt asks for the MR's diff revisions or version history. Returns version IDs and SHAs but not the diffs themselves — pass a version_id to diff_version_get to retrieve the actual diff.",
		aliases: []string{"list merge request diff versions", "mr diff revisions", "list mr diff snapshots", "merge request version history"},
		related: []string{actionDiffVersionGet, actionChangesGet, actionMRGet},
		description: "List all diff versions (historical snapshots) of a merge request. " +
			"Returns: an array of diff versions with id, head/base/start SHAs, state, real_size, and created_at, plus pagination metadata. " +
			"See also: gitlab_mr_diff_version_get, gitlab_mr_changes_get.",
		params: map[string]toolutil.ParameterGuidance{
			"project_id":        scopeProjectGuidance,
			"merge_request_iid": mrIIDGuidance,
			"order_by": {
				ValueSource:      "Column to order keyset-paginated results by.",
				ExampleBinding:   `params.order_by:"id"`,
				CommonConfusions: []string{"Combine order_by with sort and pagination='keyset'. Not a free-text phrase."},
			},
		},
	},
	"gitlab_mr_diff_version_get": {
		usage:   "Get a single merge request diff version with its commits and file diffs. Use the version_id from diff_versions_list. Set unidiff to return diffs in unified-diff format.",
		aliases: []string{"get merge request diff version", "show mr diff snapshot", "get mr diff revision", "merge request version detail"},
		related: []string{actionDiffVersionsList, actionChangesGet, actionMRGet},
		description: "Get a single merge request diff version with its commits and file diffs. " +
			"Returns: the diff version with id, SHAs, state, real_size, the full commit list, and per-file diffs (optionally in unified-diff format). " +
			"See also: gitlab_mr_diff_versions_list, gitlab_mr_changes_get.",
		params: map[string]toolutil.ParameterGuidance{
			"project_id":        scopeProjectGuidance,
			"merge_request_iid": mrIIDGuidance,
			"version_id": {
				SemanticRole:     "mr_diff_version_id",
				ValueSource:      "Diff version ID taken from a prior diff_versions_list result.",
				ExampleBinding:   "params.version_id:1001",
				CommonConfusions: []string{"Use the diff version id, not the merge_request_iid or a commit SHA."},
			},
			"unidiff": {
				ValueSource:    "Set true to request diffs rendered in unified-diff format.",
				ExampleBinding: "params.unidiff:true",
			},
		},
	},
}

// mrChangeOptions returns the base [toolutil.ActionSpecOptions] shared by every
// MR-changes action (tags, owner, individual tool metadata). The Usage, Aliases,
// RelatedActions, and Description are overridden per action by decorateMRChangeMeta.
func mrChangeOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Usage:          "Use to execute mrchanges domain action.",
		Tags:           []string{"merge_request", "review", "diff", "changes", "inspect"},
		OpenWorld:      true,
		OwnerPackage:   "mrchanges",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
