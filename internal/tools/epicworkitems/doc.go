// Package epicworkitems is the home for the shared GraphQL helpers used by
// the epic-family sub-packages (epics, epicnotes, epicissues). The exported
// functions in workitems.go (ResolveEpicGID, ResolveWorkItemGID) translate
// a (group_path, iid) pair into the work item's global ID for use in
// subsequent GraphQL mutations.
//
// See ADR-0004 (catalog-first registration) for the broader context.
package epicworkitems
