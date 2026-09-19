package deploymentmergerequests

// Canonical action IDs this package publishes, the one form every surface
// resolves.
//
// The hints a Markdown formatter writes and the RelatedActions the ActionSpec
// carries read this one block rather than each keeping its own. Kept apart,
// the two drifted: the formatter named merge_request.get and the spec named
// mergerequest.get, a domain the catalog does not have, so the cross-link a
// model was invited to follow answered "unknown action".
const (
	actionMergeRequestGet     = "merge_request.get"
	actionMergeRequestList    = "merge_request.list"
	actionMergeRequestChanges = "mr_review.changes_get"

	// The deployment this action's merge requests belong to. Its actions are
	// aggregated into the environment group, which is why the ID is not
	// "deployment.get".
	actionDeploymentGet = "environment.deployment_get"
)

// publishedActionIDs returns every canonical action ID this package hands a
// model as a cross-link, which is what the catalog test holds against the
// catalog.
func publishedActionIDs() []string {
	return []string{
		actionMergeRequestGet,
		actionMergeRequestList,
		actionMergeRequestChanges,
		actionDeploymentGet,
	}
}
