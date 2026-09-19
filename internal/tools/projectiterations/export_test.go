package projectiterations

// PublishedActionIDs is every canonical action ID this package hands a model:
// the entries of the RelatedActions list in action_specs.go and the two the
// Markdown hints in markdown.go name, which are one block of constants both
// files read.
//
// It exists so the external test can hold them against the catalog the server
// really builds. An ID that resolves to nothing is answered "unknown action"
// the moment a model follows it, and what a model concludes from that is that
// the capability is missing rather than that the cross-link is wrong.
var PublishedActionIDs = []string{
	actionListProject,
	actionListGroup,
	actionIssueList,
	actionMilestoneList,
}
