package commits

// Canonical action IDs this package publishes, the one form every surface
// resolves.
//
// The domain is repository and not commit: the commit actions are aggregated
// into the repository group beside the tree, file and submodule ones, so the
// package name is not the domain. "commit.get" reads as a perfectly plausible
// pair and resolves to nothing, which answers "unknown action" the moment a
// model follows it.
//
// The hints a Markdown formatter writes and the RelatedActions an ActionSpec
// carries read this one block rather than each keeping its own. Kept apart,
// the two drifted: the formatters named the catalog IDs and the specs named a
// commit domain that does not exist.
const (
	actionCommitList          = "repository.commit_list"
	actionCommitGet           = "repository.commit_get"
	actionCommitDiff          = "repository.commit_diff"
	actionCommitRefs          = "repository.commit_refs"
	actionCommitComments      = "repository.commit_comments"
	actionCommitCommentCreate = "repository.commit_comment_create"
	actionCommitStatuses      = "repository.commit_statuses"
	actionCommitStatusSet     = "repository.commit_status_set"
	actionCommitCherryPick    = "repository.commit_cherry_pick"
	actionCommitRevert        = "repository.commit_revert"

	actionRepositoryTree = "repository.tree"
	actionFileGet        = "repository.file_get"

	actionBranchGet  = "branch.get"
	actionBranchList = "branch.list"
	actionTagGet     = "tag.get"
	actionTagList    = "tag.list"

	actionPipelineList = "pipeline.list"

	// The merge request reads a commit points at. The diff of a merge request
	// is owned by the review group rather than the merge request one, which is
	// why the two IDs do not share a domain.
	actionMRGet        = "merge_request.get"
	actionMRChangesGet = "mr_review.changes_get"
)

// publishedActionIDs returns every canonical action ID this package hands a
// model as a cross-link, which is what the catalog test holds against the
// catalog.
func publishedActionIDs() []string {
	return []string{
		actionCommitList,
		actionCommitGet,
		actionCommitDiff,
		actionCommitRefs,
		actionCommitComments,
		actionCommitCommentCreate,
		actionCommitStatuses,
		actionCommitStatusSet,
		actionCommitCherryPick,
		actionCommitRevert,
		actionRepositoryTree,
		actionFileGet,
		actionBranchGet,
		actionBranchList,
		actionTagGet,
		actionTagList,
		actionPipelineList,
		actionMRGet,
		actionMRChangesGet,
	}
}
