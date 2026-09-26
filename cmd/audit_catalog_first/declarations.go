package main

// orphanActionSpecsDeclaration is a package whose exported ActionSpecs is
// deliberately aggregated by nothing, with the reason it is.
type orphanActionSpecsDeclaration struct {
	// Category names the shape rather than the package, so a second entry of
	// the same kind is recognizable as one.
	Category string
	// Reason is what makes it right here, in a sentence a reader who did not
	// write it can check against the tree.
	Reason string
}

// declaredOrphanActionSpecs answers assertActionSpecsAreAggregated: a package
// listed here may declare an exported ActionSpecs that no production file
// calls.
//
// It is empty, and that is the healthy state for it rather than an unfinished
// one. Every package that was in this state had its specs aggregated by
// nothing while internal/tools/adminspecs served a copy that had drifted from
// it, so the twenty-three of them were deleted rather than declared: a
// declaration would have preserved exactly the trap the rule exists to catch,
// which is a maintainer editing the obvious file and changing nothing a model
// reads.
//
// A declaration that stops describing the tree is itself a finding, on the
// same terms as the state it excuses. See staleOrphanDeclarations.
var declaredOrphanActionSpecs = map[string]orphanActionSpecsDeclaration{}

// metaOnlyProjectionActions answers the projection rule: an action listed here
// may carry no individual tool name, because the individual surface reaches
// what it does under another tool, which the value names.
//
// A declaration the projection walk does not consume is itself a finding, on
// the same terms as the state it excuses. An ID no catalog action carries, or
// one whose action now projects a tool of its own, excuses nothing today and
// would silently excuse the next action given that ID. See
// staleMetaOnlyProjections.
var metaOnlyProjectionActions = map[string]string{
	"server.health_check": "meta-only alias for gitlab_server status; the individual surface uses gitlab_server_status",
}
