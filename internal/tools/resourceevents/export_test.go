package resourceevents

// PublishedActionIDs is the block of canonical action ID constants at the top
// of action_specs.go, which is what its RelatedActions entries are built from.
// It is not every ID this package hands a model: three of them are written as
// literals in the eventActionMeta table, and the whole set is reached instead
// by TestActionSpecs_RelatedActions_NameCatalogActions, which walks the specs.
// Neither list covers the Markdown hints, which name individual tool names
// rather than canonical IDs.
//
// It exists so the external test can hold the constants against the catalog
// the server really builds. An ID that resolves to nothing is answered
// "unknown action" the moment a model follows it, and what a model concludes
// from that is that the capability is missing rather than that the cross-link
// is wrong.
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
