// Package epicworkitems is the home for the shared GraphQL helpers used by
// the epic-family sub-packages (epics, epicnotes, epicdiscussions,
// epicissues). The exported functions in work_items.go (ResolveEpicGID,
// ResolveWorkItemGID) translate a (group_path, iid) pair into the work item's
// global ID for use in subsequent GraphQL mutations, and CheckNoteOnEpic
// holds a note write to a note of the epic the caller named, since GitLab
// addresses a note by its global ID alone. RequireEpic, ResolveEpicForNote,
// RequireNoteOnEpic and EpicNotAnswered are the checks and the refusals the
// epicnotes and epicdiscussions handlers share, so each is written once.
//
// See ADR-0004 (catalog-first registration) for the broader context.
package epicworkitems
