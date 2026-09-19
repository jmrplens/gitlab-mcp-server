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
//
// The table is empty, and an empty one is the healthy state rather than an
// unfinished one. It held a single entry until then: internal/tools/adminspecs
// declared the 92 instance-administration actions while their handlers, and so
// their requests, live in the twenty-three domain packages the routes name.
// Each of those actions now names the package it routes to, which is the answer
// a declaration can only approximate, since a declaration excuses a join that
// cannot be made and a true owner makes it.
var declaredSilentOwners = map[string]silentOwnerDeclaration{}
