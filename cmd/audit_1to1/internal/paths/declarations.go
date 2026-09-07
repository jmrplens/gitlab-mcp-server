package paths

// silentOwnerDeclaration records why a package the catalog names as owning
// actions was seen issuing no request at all.
type silentOwnerDeclaration struct {
	// Category says what kind of silence this is, so a reader can tell the
	// kinds apart without reading every reason.
	Category string
	// Reason says why, in the words a reviewer needs to judge whether it still
	// holds.
	Reason string
}

// Declaration categories.
const (
	// categoryRecordedElsewhere is a package that declares specs whose handlers
	// live in other packages. Its requests are made, and recorded, under the
	// package that makes them.
	categoryRecordedElsewhere = "recorded-elsewhere"
)

// declaredSilentOwners holds every package the catalog owns actions in that
// issues no request of its own, each with a reason.
//
// This is the same shape -scope=sdk holds a client-go service to: covered by a
// recording, or declared with a category and a reason, and anything that is
// neither is a finding. It is a small table on purpose. A package that owns
// actions and never appears in the inventory is normally a package no test
// drives, which is the defect this whole dimension exists to find, so adding an
// entry here has to be an argument rather than a way to make the gate quiet.
//
// A declaration that no longer describes the tree is a finding too: a package
// that has since recorded a request, or that no longer owns any action, leaves
// a claim behind that a later reader would trust.
var declaredSilentOwners = map[string]silentOwnerDeclaration{
	"adminspecs": {
		Category: categoryRecordedElsewhere,
		Reason: "declares the instance-administration specs whose handlers live in the domain packages " +
			"(topics, settings, system hooks and twenty more), so every request it owns an action for is " +
			"recorded under the package that issues it. Its own silence says nothing about whether those " +
			"requests were seen, which is why the count of silent actions is read package by package.",
	},
}
