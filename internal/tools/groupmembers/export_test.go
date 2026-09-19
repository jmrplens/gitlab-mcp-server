package groupmembers

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
	actionGroupGet,
	actionGroupMembers,
	actionMemberGet,
	actionMemberGetInherited,
	actionMemberEdit,
	actionMemberRemove,
	actionMemberShare,
	actionMemberUnshare,
	actionBillableMembers,
	actionBillableMemberships,
	actionBillableMemberRemove,
}

// SpecNames is the bare name every spec in this package registers under, which
// the catalog publishes under the group domain. The external test holds each
// against the catalog, because that concatenation is the premise the IDs above
// are derived from.
var SpecNames = []string{
	specMemberGet,
	specMemberGetInherited,
	specMemberAdd,
	specMemberEdit,
	specMemberRemove,
	specMemberShare,
	specMemberUnshare,
	specBillableMembers,
	specBillableMemberships,
	specBillableMemberRemove,
}
