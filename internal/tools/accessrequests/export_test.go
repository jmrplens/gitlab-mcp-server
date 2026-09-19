package accessrequests

// PublishedActionIDs is every canonical catalog ID this package hands a model:
// the RelatedActions metadata on its specs and the HintAction calls its
// Markdown formatters write both read the same constants, and this slice names
// those constants rather than repeating their values. It exists so the
// external test can hold each of them against the built catalog, which this
// package cannot import itself: internal/tools imports this package, so a test
// in package accessrequests that reached for the catalog would be a cycle.
var PublishedActionIDs = []string{
	actionProjectMemberList,
	actionGroupMemberList,
	actionAccessApproveProject,
	actionAccessDenyProject,
	actionAccessApproveGroup,
	actionAccessDenyGroup,
	actionAccessRequestListProject,
	actionAccessRequestListGroup,
}
