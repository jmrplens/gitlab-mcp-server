package resourceevents

// PublishedActionIDs is every canonical action ID this package hands a model:
// the RelatedActions entries of the specs in action_specs.go, which are the
// one block of constants that file reads. This package writes no Markdown
// hints.
//
// It exists so the external test can hold them against the catalog the server
// really builds. An ID that resolves to nothing is answered "unknown action"
// the moment a model follows it, and what a model concludes from that is that
// the capability is missing rather than that the cross-link is wrong.
var PublishedActionIDs = []string{
	actionIssueGet,
	actionMRGet,
	actionGroupGet,
	actionIssueLabelList,
	actionIssueLabelGet,
	actionIssueStatList,
	actionIssueStateGet,
	actionIssueMilestoneList,
	actionIssueMilestoneGet,
	actionMRLabelList,
	actionMRLabelGet,
	actionMRStatList,
	actionMRStatGet,
	actionMRMilestoneList,
	actionMRMilestoneGet,
	actionEpicLabelList,
	actionEpicLabelGet,
	actionEpicGet,
	actionEpicList,
}

// EpicSpecNames is the bare name every epic-event spec registers under, which
// the catalog publishes under the group domain rather than an epic one. The
// external test holds each against the catalog, because that concatenation is
// the premise the epic IDs above are derived from.
var EpicSpecNames = []string{
	specEpicLabelList,
	specEpicLabelGet,
}
