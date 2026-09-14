// exclusion.go narrows the completion surface the way --exclude-tools narrows
// the tool surface.
//
// Completion was the fourth request path to GitLab and the last one an
// operator could not narrow. It runs handler code holding the caller's
// credential and enumerates projects, groups, users, merge requests, issues,
// branches, tags, pipelines, commits, labels, milestones and jobs, every one of
// which duplicates a catalog action an operator can remove. The repository
// already states the invariant twice, for resources and for prompts: an
// operator who removes an action and finds it still readable through another
// surface has been given a guard that does not guard.

package completions

// Canonical catalog action IDs whose data a completer serves. Named as
// constants so the table below and the completers cannot drift apart, and so a
// rename shows up as a compile error in both places at once.
const (
	actionProjectList       = "project.list"
	actionGroupList         = "group.list"
	actionUserList          = "user.list"
	actionMergeRequestList  = "merge_request.list"
	actionIssueList         = "issue.list"
	actionBranchList        = "branch.list"
	actionTagList           = "tag.list"
	actionPipelineList      = "pipeline.list"
	actionCommitList        = "repository.commit_list"
	actionLabelList         = "project.label_list"
	actionMilestoneList     = "project.milestone_list"
	actionGroupMilestoneLst = "group.group_milestone_list"
	actionJobList           = "job.list"
)

// completionBackingActions maps each completion argument this package answers
// to the canonical catalog actions that return the same GitLab data through a
// tool.
//
// It is a hand-kept table for the same reason the resource one is: nothing
// relates a completer to the action serving its data, and keeping the mapping
// here is the point, since it is the one place a reviewer can see the overlap
// between the two surfaces. Two tests hold it honest: one fails when an
// argument the dispatch answers is missing from the table, the other when an
// action ID in the table is not in the real catalog.
//
// An argument backed by several actions is withheld only when **every** one of
// them is excluded. That is deliberate: `from` completes branches and tags
// together, and an operator who removed only the tag listing still asked for
// branches to work.
var completionBackingActions = map[string][]string{
	"project_id":        {actionProjectList},
	"group_id":          {actionGroupList},
	"username":          {actionUserList},
	"merge_request_iid": {actionMergeRequestList},
	"issue_iid":         {actionIssueList},
	"from":              {actionBranchList, actionTagList},
	"to":                {actionBranchList, actionTagList},
	"ref":               {actionBranchList, actionTagList},
	"branch":            {actionBranchList},
	"source_branch":     {actionBranchList},
	"target_branch":     {actionBranchList},
	"tag":               {actionTagList},
	"pipeline_id":       {actionPipelineList},
	"sha":               {actionCommitList},
	"label":             {actionLabelList},
	"milestone_id":      {actionMilestoneList},
	"milestone":         {actionMilestoneList, actionGroupMilestoneLst},
	"job_id":            {actionJobList},
}

// PublishExcludedActions records the canonical catalog action IDs the operator
// removed, so a completer serving the same data answers an empty list instead
// of reaching GitLab.
//
// Late publication, like [Handler.PublishPrompts] and for the same reason: this
// handler is built before the catalog it would be filtered against exists. A
// handler nobody published to withholds nothing, which is what every test and
// every deployment with no exclusions wants.
func (h *Handler) PublishExcludedActions(actions []string) {
	excluded := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		excluded[action] = struct{}{}
	}
	h.excluded.Store(&excluded)
}

// withholds reports whether every action backing this data was excluded.
//
// Empty actions never withhold: an argument with no backing action in the
// table is one nothing can exclude, and treating "no actions" as "all excluded"
// would silently withhold it.
func (h *Handler) withholds(actions ...string) bool {
	excluded := h.excluded.Load()
	if excluded == nil || len(actions) == 0 {
		return false
	}
	for _, action := range actions {
		if _, ok := (*excluded)[action]; !ok {
			return false
		}
	}
	return true
}
