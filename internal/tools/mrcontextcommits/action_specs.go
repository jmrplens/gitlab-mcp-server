package mrcontextcommits

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action IDs referenced by the MR context-commit RelatedActions.
// Context commits live under the gitlab_merge_request meta-group, so the
// merge_request.* IDs are siblings and the commit.* IDs cross-reference the
// repository commit history the context SHAs are drawn from.
const (
	actionContextCommitsList   = "merge_request.context_commits_list"
	actionContextCommitsCreate = "merge_request.context_commits_create"
	actionContextCommitsDelete = "merge_request.context_commits_delete"

	actionMergeRequestGet     = "merge_request.get"
	actionMergeRequestList    = "merge_request.list"
	actionMergeRequestCommits = "merge_request.commits"

	actionCommitGet  = "commit.get"
	actionCommitList = "commit.list"
)

// ActionSpecs returns canonical specs for merge request context commit actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		contextCommitReadSpec("context_commits_list", toolutil.RouteAction(client, List), "gitlab_list_mr_context_commits"),
		contextCommitCreateSpec("context_commits_create", toolutil.RouteAction(client, Create), "gitlab_create_mr_context_commits"),
		contextCommitDeleteSpec("context_commits_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_delete_mr_context_commits"),
	}
}

func contextCommitReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, contextCommitOptions(individualTool))
}

func contextCommitCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, contextCommitOptions(individualTool))
}

func contextCommitDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, contextCommitOptions(individualTool))
}

// contextCommitOptions builds action-specific discovery metadata for a single
// MR context-commit tool. Each individual tool gets a distinctive Usage,
// natural-language Aliases phrased around MR context commits, canonical
// RelatedActions that link the sibling context-commit actions plus the
// merge_request.* and commit.* surfaces, and an IndividualTool.Description in
// "Returns: … See also: …" form.
func contextCommitOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Tags:           []string{"merge_request", "context_commit"},
		OpenWorld:      true,
		OwnerPackage:   "mrcontextcommits",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch individualTool {
	case "gitlab_list_mr_context_commits":
		options.Usage = "List the context commits attached to a merge request: extra commits a reviewer added for context that are not part of the MR's diff range. Use after merge_request.get to inspect which supplementary commits are pinned to the review."
		options.Aliases = []string{
			individualTool,
			"list mr context commits",
			"show merge request context commits",
			"view pinned review commits",
			"get extra commits added to mr review",
		}
		options.RelatedActions = []string{actionMergeRequestGet, actionMergeRequestCommits, actionContextCommitsCreate, actionContextCommitsDelete}
		options.IndividualTool.Description = "List the context commits pinned to a merge request for review. Returns: commit summaries (full SHA, short SHA, title, author name/email, created date). See also: gitlab_mr_get, gitlab_create_mr_context_commits, gitlab_delete_mr_context_commits."
	case "gitlab_create_mr_context_commits":
		options.Usage = "Attach one or more existing repository commit SHAs to a merge request as context commits, adding them to the review without changing the MR's source/target branches. Resolve SHAs with commit.list or merge_request.commits first."
		options.Aliases = []string{
			individualTool,
			"add context commits to mr",
			"attach commits to merge request review",
			"pin extra commits to merge request",
			"include additional commits in mr context",
		}
		options.RelatedActions = []string{actionContextCommitsList, actionContextCommitsDelete, actionCommitList, actionMergeRequestCommits}
		options.IndividualTool.Description = "Attach existing repository commit SHAs to a merge request as context commits. Returns: the resulting context commit summaries (full SHA, short SHA, title, author name/email, created date). See also: gitlab_list_mr_context_commits, gitlab_delete_mr_context_commits, gitlab_commit_get."
	case "gitlab_delete_mr_context_commits":
		options.Usage = "Remove specified context commits from a merge request by SHA, detaching them from the review without deleting the underlying repository commits. List context commits first to confirm which SHAs are currently pinned."
		options.Aliases = []string{
			individualTool,
			"remove context commits from mr",
			"detach commits from merge request review",
			"unpin extra commits from merge request",
			"delete additional commits from mr context",
		}
		options.RelatedActions = []string{actionContextCommitsList, actionContextCommitsCreate, actionMergeRequestGet, actionCommitGet}
		options.IndividualTool.Description = "Remove context commits from a merge request by SHA, detaching them from the review. Returns: a success confirmation; the underlying repository commits are left intact. See also: gitlab_list_mr_context_commits, gitlab_create_mr_context_commits, gitlab_mr_get."
	}

	return options
}
