package issues

// PublishedActionIDs is every canonical action ID this package hands a model:
// the RelatedActions entries of the specs in action_specs.go and the IDs the
// Markdown hints in markdown.go name, which are one block of constants both
// files read.
//
// It exists so the external test can hold them against the catalog the server
// really builds. An ID that resolves to nothing is answered "unknown action"
// the moment a model follows it, and what a model concludes from that is that
// the capability is missing rather than that the cross-link is wrong.
var PublishedActionIDs = []string{
	actionIssueCreate,
	actionIssueGet,
	actionIssueList,
	actionIssueListGroup,
	actionIssueUpdate,
	actionIssueDelete,
	actionIssueSubscribe,
	actionIssueUnsubscribe,
	actionIssueTimeEstSet,
	actionIssueTimeEstReset,
	actionIssueSpentTimeAdd,
	actionIssueSpentTimeReset,
	actionIssueTimeStatsGet,
	actionIssueMRsClosing,
	actionIssueMRsRelated,
	actionIssueNoteList,
	actionIssueNoteCreate,
	actionGroupGet,
	actionSearchIssues,
	actionTodoMarkDone,
	actionMRGet,
	actionMRChangesGet,
}

// SpecNames is the bare name every spec this package contributes to the issue
// catalog group registers under, which the catalog publishes under the issue
// domain. The external test holds each against the catalog, because that
// concatenation is the premise the IDs above are derived from. specGroupIssues
// is deliberately absent: it is a route on the group catalog group and is
// covered by its own assertion.
var SpecNames = []string{
	specCreate,
	specGet,
	specGetByID,
	specList,
	specListAll,
	specListGroup,
	specUpdate,
	specDelete,
	specReorder,
	specMove,
	specSubscribe,
	specUnsubscribe,
	specCreateTodo,
	specTimeEstimateSet,
	specTimeEstimateReset,
	specSpentTimeAdd,
	specSpentTimeReset,
	specTimeStatsGet,
	specParticipants,
	specMRsClosing,
	specMRsRelated,
}

// GroupSpecName is the name the group-scoped issue listing registers under,
// which the catalog publishes under the group domain rather than the issue
// one.
var GroupSpecName = specGroupIssues
